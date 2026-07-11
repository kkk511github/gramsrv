package push

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"telesrv/internal/domain"
)

func TestServiceDispatchesAndDeletesInvalidDevice(t *testing.T) {
	store := &pushStoreStub{
		jobs: []domain.PushNotificationJob{{ID: 9, TargetUserID: 42, Pts: 7}},
		devices: []domain.PushDevice{
			{ID: 1, UserID: 42, TokenType: domain.PushTokenAPNS, Token: "valid"},
			{ID: 2, UserID: 42, TokenType: domain.PushTokenAPNS, Token: "invalid"},
		},
	}
	sender := &senderStub{invalidToken: "invalid"}
	service := &Service{store: store, apns: sender, batch: 10, log: zap.NewNop()}

	if !service.dispatchOnce(context.Background()) {
		t.Fatal("dispatchOnce = false, want claimed work")
	}
	if store.deliveredID != 9 {
		t.Fatalf("delivered id = %d, want 9", store.deliveredID)
	}
	if len(store.deletedIDs) != 1 || store.deletedIDs[0] != 2 {
		t.Fatalf("deleted ids = %v, want [2]", store.deletedIDs)
	}
	if sender.calls != 2 {
		t.Fatalf("sender calls = %d, want 2", sender.calls)
	}
	if len(store.deliveredDeviceIDs) != 1 || store.deliveredDeviceIDs[0] != 1 {
		t.Fatalf("delivered device ids = %v, want [1]", store.deliveredDeviceIDs)
	}
}

func TestServiceRetriesTransientProviderFailure(t *testing.T) {
	store := &pushStoreStub{
		jobs:    []domain.PushNotificationJob{{ID: 10, TargetUserID: 42, Pts: 8}},
		devices: []domain.PushDevice{{ID: 1, UserID: 42, TokenType: domain.PushTokenFCM, Token: "temporary"}},
	}
	service := &Service{store: store, fcm: &senderStub{sendErr: errors.New("provider unavailable")}, batch: 10, log: zap.NewNop()}

	service.dispatchOnce(context.Background())
	if store.deliveredID != 0 || store.failedID != 10 {
		t.Fatalf("delivered=%d failed=%d, want 0/10", store.deliveredID, store.failedID)
	}
}

type senderStub struct {
	invalidToken string
	sendErr      error
	calls        int
}

func (s *senderStub) Send(_ context.Context, device domain.PushDevice, _ domain.PushNotificationJob) (bool, error) {
	s.calls++
	return device.Token == s.invalidToken, s.sendErr
}

type pushStoreStub struct {
	jobs               []domain.PushNotificationJob
	devices            []domain.PushDevice
	deliveredID        int64
	failedID           int64
	deletedIDs         []int64
	deliveredDeviceIDs []int64
}

func (s *pushStoreStub) UpsertPushDevice(context.Context, domain.PushDevice) error  { return nil }
func (s *pushStoreStub) DeletePushDevice(context.Context, int64, int, string) error { return nil }
func (s *pushStoreStub) DeletePushDeviceByID(_ context.Context, id int64) error {
	s.deletedIDs = append(s.deletedIDs, id)
	return nil
}
func (s *pushStoreStub) ListPushDevices(context.Context, int64) ([]domain.PushDevice, error) {
	return append([]domain.PushDevice(nil), s.devices...), nil
}
func (s *pushStoreStub) EnqueuePushNotification(context.Context, domain.PushNotificationJob) error {
	return nil
}
func (s *pushStoreStub) ClaimPushNotifications(context.Context, int) ([]domain.PushNotificationJob, error) {
	jobs := append([]domain.PushNotificationJob(nil), s.jobs...)
	s.jobs = nil
	return jobs, nil
}
func (s *pushStoreStub) MarkPushDeviceDelivered(_ context.Context, _, deviceID int64) error {
	s.deliveredDeviceIDs = append(s.deliveredDeviceIDs, deviceID)
	return nil
}
func (s *pushStoreStub) MarkPushNotificationDelivered(_ context.Context, id int64) error {
	s.deliveredID = id
	return nil
}
func (s *pushStoreStub) MarkPushNotificationFailed(_ context.Context, id int64, _ string) error {
	s.failedID = id
	return nil
}
