package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"telesrv/internal/domain"
	"telesrv/internal/store/postgres/sqlcgen"
)

const pushClaimLease = 30 * time.Second

type PushStore struct {
	db sqlcgen.DBTX
}

func NewPushStore(db sqlcgen.DBTX) *PushStore {
	return &PushStore{db: db}
}

func (s *PushStore) UpsertPushDevice(ctx context.Context, device domain.PushDevice) error {
	if device.UserID == 0 || device.TokenType <= 0 || strings.TrimSpace(device.Token) == "" {
		return fmt.Errorf("invalid push device")
	}
	_, err := s.db.Exec(ctx, `
INSERT INTO push_devices (
  user_id, auth_key_id, token_type, token, app_sandbox, secret, no_muted, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, now())
ON CONFLICT (user_id, token_type, token) DO UPDATE SET
  auth_key_id = EXCLUDED.auth_key_id,
  app_sandbox = EXCLUDED.app_sandbox,
  secret = EXCLUDED.secret,
  no_muted = EXCLUDED.no_muted,
  updated_at = now()`,
		device.UserID, device.AuthKeyID[:], device.TokenType, strings.TrimSpace(device.Token),
		device.AppSandbox, device.Secret, device.NoMuted)
	if err != nil {
		return fmt.Errorf("upsert push device: %w", err)
	}
	return nil
}

func (s *PushStore) DeletePushDevice(ctx context.Context, userID int64, tokenType int, token string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM push_devices WHERE user_id = $1 AND token_type = $2 AND token = $3`, userID, tokenType, strings.TrimSpace(token))
	if err != nil {
		return fmt.Errorf("delete push device: %w", err)
	}
	return nil
}

func (s *PushStore) DeletePushDeviceByID(ctx context.Context, id int64) error {
	_, err := s.db.Exec(ctx, `DELETE FROM push_devices WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete push device by id: %w", err)
	}
	return nil
}

func (s *PushStore) ListPushDevices(ctx context.Context, userID int64) ([]domain.PushDevice, error) {
	rows, err := s.db.Query(ctx, `
SELECT id, user_id, auth_key_id, token_type, token, app_sandbox, secret, no_muted, updated_at
FROM push_devices WHERE user_id = $1 ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list push devices: %w", err)
	}
	defer rows.Close()
	var out []domain.PushDevice
	for rows.Next() {
		var device domain.PushDevice
		var authKeyID []byte
		if err := rows.Scan(&device.ID, &device.UserID, &authKeyID, &device.TokenType, &device.Token,
			&device.AppSandbox, &device.Secret, &device.NoMuted, &device.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan push device: %w", err)
		}
		copy(device.AuthKeyID[:], authKeyID)
		out = append(out, device)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate push devices: %w", err)
	}
	return out, nil
}

func (s *PushStore) EnqueuePushNotification(ctx context.Context, notification domain.PushNotificationJob) error {
	if notification.TargetUserID == 0 || notification.Pts <= 0 {
		return nil
	}
	_, err := s.db.Exec(ctx, `
INSERT INTO push_notification_outbox (target_user_id, pts, title, body)
VALUES ($1, $2, $3, $4)
ON CONFLICT (target_user_id, pts) DO NOTHING`, notification.TargetUserID, notification.Pts, notification.Title, notification.Body)
	if err != nil {
		return fmt.Errorf("enqueue push notification: %w", err)
	}
	return nil
}

func (s *PushStore) ClaimPushNotifications(ctx context.Context, limit int) ([]domain.PushNotificationJob, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.Query(ctx, `
WITH picked AS (
  SELECT id FROM push_notification_outbox
  WHERE next_attempt_at <= now()
    AND (locked_until IS NULL OR locked_until < now())
  ORDER BY id
  FOR UPDATE SKIP LOCKED
  LIMIT $1
)
UPDATE push_notification_outbox AS p
SET locked_until = now() + ($2 * interval '1 second')
FROM picked
WHERE p.id = picked.id
RETURNING p.id, p.target_user_id, p.pts, p.title, p.body, p.attempts, p.delivered_device_ids`, limit, int(pushClaimLease/time.Second))
	if err != nil {
		return nil, fmt.Errorf("claim push notifications: %w", err)
	}
	defer rows.Close()
	var out []domain.PushNotificationJob
	for rows.Next() {
		var job domain.PushNotificationJob
		if err := rows.Scan(&job.ID, &job.TargetUserID, &job.Pts, &job.Title, &job.Body, &job.Attempts, &job.DeliveredDeviceIDs); err != nil {
			return nil, fmt.Errorf("scan push notification: %w", err)
		}
		out = append(out, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate push notifications: %w", err)
	}
	return out, nil
}

func (s *PushStore) MarkPushDeviceDelivered(ctx context.Context, notificationID, deviceID int64) error {
	_, err := s.db.Exec(ctx, `
UPDATE push_notification_outbox
SET delivered_device_ids = array_append(delivered_device_ids, $2)
WHERE id = $1 AND NOT ($2 = ANY(delivered_device_ids))`, notificationID, deviceID)
	if err != nil {
		return fmt.Errorf("mark push device delivered: %w", err)
	}
	return nil
}

func (s *PushStore) MarkPushNotificationDelivered(ctx context.Context, id int64) error {
	_, err := s.db.Exec(ctx, `DELETE FROM push_notification_outbox WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("mark push notification delivered: %w", err)
	}
	return nil
}

func (s *PushStore) MarkPushNotificationFailed(ctx context.Context, id int64, reason string) error {
	if len(reason) > 512 {
		reason = reason[:512]
	}
	_, err := s.db.Exec(ctx, `
UPDATE push_notification_outbox
SET attempts = attempts + 1,
    next_attempt_at = now() + (LEAST(300, (1 << LEAST(attempts, 8))) * interval '1 second'),
    locked_until = NULL,
    last_error = $2
WHERE id = $1`, id, reason)
	if err != nil {
		return fmt.Errorf("mark push notification failed: %w", err)
	}
	return nil
}
