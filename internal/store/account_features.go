package store

import (
	"context"
	"telesrv/internal/domain"
)

// AccountFeatureStore is optional so unsupported stores cannot acknowledge a
// mutation without persistence. Confirmation must atomically match owner+bot.
type AccountFeatureStore interface {
	GetMainProfileTab(context.Context, int64) (domain.ProfileTab, bool, error)
	SetMainProfileTab(context.Context, int64, domain.ProfileTab) error
	ConfirmBotConnection(context.Context, int64, int64, int64) error
}
