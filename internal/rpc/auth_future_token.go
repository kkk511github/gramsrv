package rpc

import (
	"context"

	"github.com/iamxvbaba/td/tg"
	"go.uber.org/zap"

	"telesrv/internal/domain"
)

type futureAuthService interface {
	TryFutureAuthToken(context.Context, domain.Authorization, string, [][]byte) (domain.User, bool, error)
	IssueFutureAuthToken(context.Context, [8]byte) ([]byte, error)
	LogOutWithFutureToken(context.Context, [8]byte) ([]byte, error)
}

func (r *Router) authorizationWithFutureToken(ctx context.Context, u domain.User) *tg.AuthAuthorization {
	result := &tg.AuthAuthorization{User: r.tgSelfUserWithUsernames(ctx, u)}
	if pending, err := r.registrationPasswordPending(ctx, u.ID); err != nil || pending {
		// Zero is SafeLink's mandatory-registration mode, not an SMS cooldown.
		result.SetSetupPasswordRequired(true)
		result.SetOtherwiseReloginDays(0)
		return result
	}
	if svc, ok := r.deps.Auth.(futureAuthService); ok {
		key, _ := AuthKeyIDFrom(ctx)
		token, err := svc.IssueFutureAuthToken(ctx, key)
		if err != nil {
			r.log.Warn("future auth token issuance failed", zap.Error(err))
		} else if len(token) != 0 {
			result.SetFutureAuthToken(token)
		}
	}
	return result
}
