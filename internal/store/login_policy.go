package store

import (
	"context"
	"fmt"
)

type LoginPolicy struct {
	RegistrationPasswordRequired bool `json:"registration_password_required"`
	FutureAuthEnabled            bool `json:"future_auth_enabled"`
	FutureAuthDays               int  `json:"future_auth_days"`
}

func DefaultLoginPolicy() LoginPolicy {
	return LoginPolicy{FutureAuthEnabled: true, FutureAuthDays: 30}
}

func (p LoginPolicy) Validate() error {
	if p.FutureAuthDays < 1 || p.FutureAuthDays > 365 {
		return fmt.Errorf("登录凭证有效期须为 1 至 365 天")
	}
	return nil
}

type LoginPolicyReader interface {
	LoginPolicy(context.Context) (LoginPolicy, error)
}

type RegistrationPasswordReader interface {
	RegistrationPasswordPending(context.Context, int64) (bool, error)
}
