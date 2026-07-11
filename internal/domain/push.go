package domain

import "time"

const (
	PushTokenAPNS = 1
	PushTokenFCM  = 2
)

// PushDevice is one account-scoped mobile push registration.
type PushDevice struct {
	ID         int64
	UserID     int64
	AuthKeyID  [8]byte
	TokenType  int
	Token      string
	AppSandbox bool
	Secret     []byte
	NoMuted    bool
	UpdatedAt  time.Time
}

// PushNotificationJob is a durable request to notify an offline account.
type PushNotificationJob struct {
	ID                 int64
	TargetUserID       int64
	Pts                int
	Title              string
	Body               string
	Attempts           int
	DeliveredDeviceIDs []int64
}
