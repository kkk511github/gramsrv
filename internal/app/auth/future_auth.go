package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"time"

	"telesrv/internal/domain"
	"telesrv/internal/store"
)

func (s *Service) LoginPolicy(ctx context.Context) (store.LoginPolicy, error) {
	if reader, ok := s.futureTokens.(store.LoginPolicyReader); ok {
		return reader.LoginPolicy(ctx)
	}
	return store.DefaultLoginPolicy(), nil
}

func WithFutureAuthTokens(tokens store.FutureAuthTokenStore) Option {
	return func(s *Service) { s.futureTokens = tokens }
}

func (s *Service) RegistrationPasswordPending(ctx context.Context, userID int64) (bool, error) {
	if reader, ok := s.users.(store.RegistrationPasswordReader); ok {
		return reader.RegistrationPasswordPending(ctx, userID)
	}
	return false, nil
}

func (s *Service) RegistrationInviteRequired(ctx context.Context) (bool, error) {
	if reader, ok := s.users.(store.RegistrationInvitePolicyReader); ok {
		return reader.RegistrationInviteRequired(ctx)
	}
	return false, nil
}

func (s *Service) IssueFutureAuthToken(ctx context.Context, key [8]byte) ([]byte, error) {
	return s.issueFutureAuthToken(ctx, key, false)
}

func (s *Service) LogOutWithFutureToken(ctx context.Context, key [8]byte) ([]byte, error) {
	if s.futureTokens == nil {
		return nil, s.LogOut(ctx, key)
	}
	return s.issueFutureAuthToken(ctx, key, true)
}

func (s *Service) issueFutureAuthToken(ctx context.Context, key [8]byte, logout bool) ([]byte, error) {
	if s.futureTokens == nil || key == ([8]byte{}) {
		return nil, nil
	}
	policy, err := s.LoginPolicy(ctx)
	if err != nil {
		return nil, err
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	ok, err := s.futureTokens.IssueFutureAuthToken(ctx, key, sha256.Sum256(token), time.Now().Add(time.Duration(policy.FutureAuthDays)*24*time.Hour), logout)
	if err != nil || !ok {
		return nil, err
	}
	return token, nil
}

// TryFutureAuthToken never treats an invalid token as proof of phone ownership.
// It falls back to normal code delivery; a valid token still requires current 2FA.
func (s *Service) TryFutureAuthToken(ctx context.Context, a domain.Authorization, phone string, tokens [][]byte) (domain.User, bool, error) {
	phone = normalizePhone(phone)
	if !validPhone(phone) || systemLoginPhoneForbidden(phone) {
		return domain.User{}, false, ErrPhoneNumberInvalid
	}
	if s.futureTokens == nil || len(tokens) == 0 || a.AuthKeyID == ([8]byte{}) {
		return domain.User{}, false, nil
	}
	digests := make([][32]byte, 0, 20)
	for i, token := range tokens {
		if i >= 20 {
			break
		}
		if len(token) == 32 {
			digests = append(digests, sha256.Sum256(token))
		}
	}
	bound, found, err := s.futureTokens.RedeemFutureAuthToken(ctx, phone, digests, a)
	if err != nil || !found {
		return domain.User{}, found, err
	}
	u, found, err := s.users.ByID(ctx, bound.UserID)
	if err != nil {
		return domain.User{}, true, err
	}
	if !found || systemUserLoginForbidden(u) {
		return domain.User{}, true, ErrSystemUserLoginForbidden
	}
	if bound.PasswordPending {
		return u, true, domain.ErrSessionPasswordNeeded
	}
	s.recordWelcomeMessage(ctx, u)
	return u, true, nil
}
