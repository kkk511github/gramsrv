package rpc

import (
	"context"
	"testing"
	"time"

	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tgerr"
	"go.uber.org/zap/zaptest"
	"telesrv/internal/domain"
	"telesrv/internal/store"
)

type futureTokenAuthCapture struct {
	*captureAuthService
	matched   bool
	proofErr  error
	sendCount int
	tokens    [][]byte
}

func (s *futureTokenAuthCapture) TryFutureAuthToken(_ context.Context, _ domain.Authorization, _ string, tokens [][]byte) (domain.User, bool, error) {
	s.tokens = tokens
	return s.signInUser, s.matched, s.proofErr
}
func (s *futureTokenAuthCapture) IssueFutureAuthToken(context.Context, [8]byte) ([]byte, error) {
	return []byte("new-future-token"), nil
}
func (s *futureTokenAuthCapture) LogOutWithFutureToken(context.Context, [8]byte) ([]byte, error) {
	return []byte("logout-future-token"), nil
}
func (s *futureTokenAuthCapture) SendCode(ctx context.Context, phone string) (string, error) {
	s.sendCount++
	return s.captureAuthService.SendCode(ctx, phone)
}

func TestAuthSendCodeFutureTokenRPC(t *testing.T) {
	for _, tc := range []struct {
		name    string
		matched bool
		err     error
	}{
		{"password", true, domain.ErrSessionPasswordNeeded},
		{"no password", true, nil},
		{"invalid token fallback", false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := domain.User{ID: 1000000247, Phone: "15550000247", FirstName: "Token"}
			svc := &futureTokenAuthCapture{captureAuthService: &captureAuthService{signInUser: u}, matched: tc.matched, proofErr: tc.err}
			r := New(Config{}, Deps{Auth: svc, Users: &captureUsersService{user: u}}, zaptest.NewLogger(t), fixedClock{now: time.Now()})
			ctx := WithAuthKeyID(context.Background(), [8]byte{1})
			settings := tg.CodeSettings{}
			settings.SetLogoutTokens([][]byte{[]byte("proof")})
			got, err := r.onAuthSendCode(ctx, &tg.AuthSendCodeRequest{PhoneNumber: u.Phone, Settings: settings})
			if tc.err != nil {
				if !tgerr.Is(err, "SESSION_PASSWORD_NEEDED") {
					t.Fatalf("error %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if tc.matched && svc.sendCount != 0 {
				t.Fatal("valid future token sent OTP")
			}
			if !tc.matched && svc.sendCount != 1 {
				t.Fatal("invalid future token skipped OTP")
			}
			if tc.matched && tc.err == nil {
				success, ok := got.(*tg.AuthSentCodeSuccess)
				if !ok {
					t.Fatalf("result %T", got)
				}
				auth, ok := success.Authorization.(*tg.AuthAuthorization)
				if !ok {
					t.Fatal("missing authorization")
				}
				if token, ok := auth.GetFutureAuthToken(); !ok || string(token) != "new-future-token" {
					t.Fatal("missing replacement token")
				}
			}
		})
	}
}

func TestRegistrationInviteRPCError(t *testing.T) {
	if !tgerr.Is(signInErr(store.ErrRegistrationInviteRequired), "INVITE_CODE_REQUIRED") {
		t.Fatal("missing required invite error")
	}
	if !tgerr.Is(signInErr(store.ErrRegistrationInviteInvalid), "INVITE_CODE_INVALID") {
		t.Fatal("missing invalid invite error")
	}
}
