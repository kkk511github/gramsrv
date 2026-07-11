package push

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"telesrv/internal/domain"
	"telesrv/internal/store"
)

type Config struct {
	Enabled               bool
	APNSTopic             string
	APNSTeamID            string
	APNSKeyID             string
	APNSPrivateKeyPath    string
	FCMProjectID          string
	FCMServiceAccountJSON string
	Workers               int
	Batch                 int
	Interval              time.Duration
	SendTimeout           time.Duration
}

type deviceSender interface {
	Send(ctx context.Context, device domain.PushDevice, job domain.PushNotificationJob) (invalid bool, err error)
}

type Service struct {
	store       store.PushStore
	apns        deviceSender
	fcm         deviceSender
	log         *zap.Logger
	workers     int
	batch       int
	interval    time.Duration
	sendTimeout time.Duration
}

func New(cfg Config, pushStore store.PushStore, logger *zap.Logger) (*Service, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if pushStore == nil {
		return nil, fmt.Errorf("push store is nil")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	service := &Service{
		store:       pushStore,
		log:         logger,
		workers:     cfg.Workers,
		batch:       cfg.Batch,
		interval:    cfg.Interval,
		sendTimeout: cfg.SendTimeout,
	}
	if service.workers <= 0 {
		service.workers = 4
	}
	if service.batch <= 0 {
		service.batch = 50
	}
	if service.interval <= 0 {
		service.interval = 500 * time.Millisecond
	}
	if service.sendTimeout <= 0 {
		service.sendTimeout = 10 * time.Second
	}

	if cfg.APNSTopic != "" || cfg.APNSTeamID != "" || cfg.APNSKeyID != "" || cfg.APNSPrivateKeyPath != "" {
		apns, err := newAPNSSender(cfg)
		if err != nil {
			return nil, err
		}
		service.apns = apns
	}
	if cfg.FCMProjectID != "" || cfg.FCMServiceAccountJSON != "" {
		fcm, err := newFCMSender(cfg)
		if err != nil {
			return nil, err
		}
		service.fcm = fcm
	}
	if service.apns == nil && service.fcm == nil {
		return nil, fmt.Errorf("push is enabled but no APNs or FCM provider is configured")
	}
	logger.Info("mobile push notifications enabled",
		zap.Bool("apns", service.apns != nil),
		zap.Bool("fcm", service.fcm != nil),
		zap.Int("workers", service.workers),
	)
	return service, nil
}

func (s *Service) Run(ctx context.Context) {
	if s == nil {
		return
	}
	var wg sync.WaitGroup
	for range s.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runWorker(ctx)
		}()
	}
	wg.Wait()
}

func (s *Service) runWorker(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		if !s.dispatchOnce(ctx) {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func (s *Service) dispatchOnce(ctx context.Context) bool {
	jobs, err := s.store.ClaimPushNotifications(ctx, s.batch)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			s.log.Warn("claim push notifications", zap.Error(err))
		}
		return false
	}
	for _, job := range jobs {
		s.dispatchJob(ctx, job)
	}
	return len(jobs) > 0
}

func (s *Service) dispatchJob(ctx context.Context, job domain.PushNotificationJob) {
	devices, err := s.store.ListPushDevices(ctx, job.TargetUserID)
	if err != nil {
		s.failJob(ctx, job, err)
		return
	}
	var transientErr error
	for _, device := range devices {
		if pushDeviceDelivered(job.DeliveredDeviceIDs, device.ID) {
			continue
		}
		sender := s.senderFor(device.TokenType)
		if sender == nil {
			continue
		}
		sendCtx, cancel := context.WithTimeout(ctx, s.sendTimeout)
		invalid, err := sender.Send(sendCtx, device, job)
		cancel()
		if invalid {
			if deleteErr := s.store.DeletePushDeviceByID(ctx, device.ID); deleteErr != nil {
				s.log.Warn("delete invalid push device", zap.Int64("device_id", device.ID), zap.Error(deleteErr))
			}
			continue
		}
		if err != nil {
			transientErr = errors.Join(transientErr, err)
			continue
		}
		if err := s.store.MarkPushDeviceDelivered(ctx, job.ID, device.ID); err != nil {
			transientErr = errors.Join(transientErr, err)
		}
	}
	if transientErr != nil {
		s.failJob(ctx, job, transientErr)
		return
	}
	if err := s.store.MarkPushNotificationDelivered(ctx, job.ID); err != nil {
		s.log.Warn("mark push notification delivered", zap.Int64("push_id", job.ID), zap.Error(err))
		return
	}
	s.log.Debug("push notification delivered", zap.Int64("push_id", job.ID), zap.Int64("user_id", job.TargetUserID), zap.Int("devices", len(devices)))
}

func pushDeviceDelivered(delivered []int64, deviceID int64) bool {
	for _, id := range delivered {
		if id == deviceID {
			return true
		}
	}
	return false
}

func (s *Service) failJob(ctx context.Context, job domain.PushNotificationJob, cause error) {
	if err := s.store.MarkPushNotificationFailed(ctx, job.ID, cause.Error()); err != nil {
		s.log.Warn("mark push notification failed", zap.Int64("push_id", job.ID), zap.Error(err))
		return
	}
	s.log.Warn("push notification delivery failed", zap.Int64("push_id", job.ID), zap.Int64("user_id", job.TargetUserID), zap.Int("attempt", job.Attempts+1), zap.Error(cause))
}

func (s *Service) senderFor(tokenType int) deviceSender {
	switch tokenType {
	case domain.PushTokenAPNS:
		return s.apns
	case domain.PushTokenFCM:
		return s.fcm
	default:
		return nil
	}
}
