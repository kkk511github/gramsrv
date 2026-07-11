package store

import (
	"context"

	"telesrv/internal/domain"
)

type PushStore interface {
	UpsertPushDevice(ctx context.Context, device domain.PushDevice) error
	DeletePushDevice(ctx context.Context, userID int64, tokenType int, token string) error
	DeletePushDeviceByID(ctx context.Context, id int64) error
	ListPushDevices(ctx context.Context, userID int64) ([]domain.PushDevice, error)

	EnqueuePushNotification(ctx context.Context, notification domain.PushNotificationJob) error
	ClaimPushNotifications(ctx context.Context, limit int) ([]domain.PushNotificationJob, error)
	MarkPushDeviceDelivered(ctx context.Context, notificationID, deviceID int64) error
	MarkPushNotificationDelivered(ctx context.Context, id int64) error
	MarkPushNotificationFailed(ctx context.Context, id int64, reason string) error
}
