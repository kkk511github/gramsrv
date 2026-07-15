package rpc

import (
	"context"
	"strings"

	"github.com/iamxvbaba/td/tg"

	"telesrv/internal/domain"
)

const (
	maxPushTokenBytes  = 4096
	maxPushSecretBytes = 512
)

func (r *Router) onAccountRegisterDevice(ctx context.Context, req *tg.AccountRegisterDeviceRequest) (bool, error) {
	if req == nil {
		return false, inputRequestInvalidErr()
	}
	userID, found, err := r.currentUserID(ctx)
	if err != nil {
		return false, internalErr()
	}
	if !found {
		return false, authKeyUnregisteredErr()
	}
	token := strings.TrimSpace(req.Token)
	if req.TokenType <= 0 || token == "" || len(token) > maxPushTokenBytes || len(req.Secret) > maxPushSecretBytes {
		return false, tgerr400("TOKEN_INVALID")
	}
	// Type 9 is APNs VoIP. SafeLink currently enables ordinary message push
	// only; accept the registration for protocol compatibility without using it.
	if req.TokenType == 9 {
		return true, nil
	}
	if req.TokenType != domain.PushTokenAPNS && req.TokenType != domain.PushTokenFCM {
		return true, nil
	}
	if r.deps.PushDevices == nil {
		// Keep protocol compatibility for embedded/test routers that do not
		// configure the optional persistence dependency.
		return true, nil
	}
	authKeyID, _ := AuthKeyIDFrom(ctx)
	if err := r.deps.PushDevices.UpsertPushDevice(ctx, domain.PushDevice{
		UserID: userID, AuthKeyID: authKeyID, TokenType: req.TokenType,
		Token: token, AppSandbox: req.AppSandbox, Secret: append([]byte(nil), req.Secret...), NoMuted: req.NoMuted,
	}); err != nil {
		return false, internalErr()
	}
	return true, nil
}

func (r *Router) onAccountUnregisterDevice(ctx context.Context, req *tg.AccountUnregisterDeviceRequest) (bool, error) {
	if req == nil {
		return false, inputRequestInvalidErr()
	}
	userID, found, err := r.currentUserID(ctx)
	if err != nil {
		return false, internalErr()
	}
	if !found {
		return false, authKeyUnregisteredErr()
	}
	if r.deps.PushDevices == nil {
		return true, nil
	}
	if err := r.deps.PushDevices.DeletePushDevice(ctx, userID, req.TokenType, strings.TrimSpace(req.Token)); err != nil {
		return false, internalErr()
	}
	return true, nil
}
