package store

import (
	"context"
	"time"

	"telesrv/internal/domain"
)

// FutureAuthTokenStore is an authorization boundary, not a generic token cache.
// Issue checks a fully authorized human session. Logout and token creation, and
// redemption and authorization binding, must each be atomic.
type FutureAuthTokenStore interface {
	IssueFutureAuthToken(context.Context, [8]byte, [32]byte, time.Time, bool) (bool, error)
	RedeemFutureAuthToken(context.Context, string, [][32]byte, domain.Authorization) (domain.Authorization, bool, error)
}
