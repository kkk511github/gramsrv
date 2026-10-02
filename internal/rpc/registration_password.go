package rpc

import (
	"context"
	"encoding/json"
	"hash/crc32"
	"strings"

	"github.com/iamxvbaba/td/tgerr"
	"telesrv/internal/domain"
	"telesrv/internal/store"
)

func (r *Router) registrationPasswordPending(ctx context.Context, userID int64) (bool, error) {
	if userID == 0 {
		return false, nil
	}
	if reader, ok := r.deps.Auth.(store.RegistrationPasswordReader); ok {
		return reader.RegistrationPasswordPending(ctx, userID)
	}
	return false, nil
}

// These are the bootstrap, password setup and exit paths, not business writes.
func registrationPasswordRPCAllowed(method string) bool {
	if strings.HasPrefix(method, "help.") || strings.HasPrefix(method, "langpack.") || strings.HasPrefix(method, "updates.") {
		return true
	}
	switch method {
	case "account.getPassword", "account.getPasswordSettings", "account.updatePasswordSettings",
		"account.confirmPasswordEmail", "account.resendPasswordEmail", "account.cancelPasswordEmail",
		"account.getAuthorizations", "account.resetAuthorization", "account.registerDevice", "account.unregisterDevice",
		"account.updateStatus", "account.getNotifySettings", "account.getPrivacy", "account.getAccountTTL",
		"users.getUsers", "users.getFullUser", "messages.getDialogFilters", "messages.getDialogs",
		"messages.getPeerDialogs", "messages.getHistory", "auth.logOut", "auth.resetAuthorizations",
		"auth.checkPassword", "auth.sendCode", "auth.resendCode", "auth.signIn", "auth.signUp",
		"auth.cancelCode", "ping", "ping_delay_disconnect":
		return true
	}
	return false
}

func (r *Router) checkRegistrationPasswordRPC(ctx context.Context, method string) error {
	if registrationPasswordRPCAllowed(method) {
		return nil
	}
	userID, _ := UserIDFrom(ctx)
	pending, err := r.registrationPasswordPending(ctx, userID)
	if err != nil {
		return internalErr()
	}
	if pending {
		return tgerr.New(403, "REGISTRATION_PASSWORD_REQUIRED")
	}
	return nil
}

func registrationPasswordAppConfig(cfg domain.AppConfig, pending bool, inviteRequired bool) (domain.AppConfig, error) {
	values := make(map[string]json.RawMessage)
	if err := json.Unmarshal(cfg.JSON, &values); err != nil {
		return cfg, err
	}
	values["safelink_registration_password_required"], _ = json.Marshal(pending)
	values["safelink_registration_invite_required"], _ = json.Marshal(inviteRequired)
	body, err := json.Marshal(values)
	if err != nil {
		return cfg, err
	}
	hash := int(crc32.ChecksumIEEE(body) & 0x7fffffff)
	if hash == 0 {
		hash = 1
	}
	cfg.JSON, cfg.Hash = body, hash
	return cfg, nil
}
