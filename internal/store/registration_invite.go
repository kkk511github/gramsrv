package store

import (
	"context"
	"errors"
	"strings"

	"telesrv/internal/domain"
)

var ErrRegistrationInviteRequired = errors.New("registration invite required")
var ErrRegistrationInviteInvalid = errors.New("registration invite invalid")

const RegistrationInviteSeparator = ":safelink-invite:"

// The code hash stays opaque on the wire; only signUp accepts this extension.
func SplitRegistrationInvite(hash string) (string, string) {
	base, code, _ := strings.Cut(hash, RegistrationInviteSeparator)
	return base, strings.ToUpper(strings.TrimSpace(code))
}

type RegistrationStore interface {
	ValidateRegistrationInvite(context.Context, string) error
	CreateRegistration(context.Context, domain.User, string) (domain.User, error)
}

type RegistrationInvitePolicyReader interface {
	RegistrationInviteRequired(context.Context) (bool, error)
}
