package rpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iamxvbaba/td/tgerr"
	"go.uber.org/zap/zaptest"
	"telesrv/internal/domain"
)

type registrationPasswordAuth struct {
	*futureTokenAuthCapture
	pending bool
	err     error
}

func (s *registrationPasswordAuth) RegistrationPasswordPending(context.Context, int64) (bool, error) {
	return s.pending, s.err
}

func TestRegistrationPasswordGate(t *testing.T) {
	u := domain.User{ID: 1000000247, FirstName: "Registration"}
	svc := &registrationPasswordAuth{futureTokenAuthCapture: &futureTokenAuthCapture{captureAuthService: &captureAuthService{signInUser: u}}, pending: true}
	r := New(Config{}, Deps{Auth: svc, Users: &captureUsersService{user: u}}, zaptest.NewLogger(t), fixedClock{now: time.Now()})
	ctx := WithUserID(context.Background(), u.ID)
	for _, method := range []string{"messages.sendMessage", "messages.sendMedia", "channels.createChannel", "auth.exportAuthorization", "auth.exportLoginToken", "account.deleteAccount", "payments.sendStarsForm"} {
		if err := r.checkRegistrationPasswordRPC(ctx, method); !tgerr.Is(err, "REGISTRATION_PASSWORD_REQUIRED") {
			t.Fatalf("%s bypassed setup: %v", method, err)
		}
	}
	for _, method := range []string{"account.getPassword", "account.updatePasswordSettings", "account.confirmPasswordEmail", "account.resendPasswordEmail", "help.getAppConfig", "updates.getState", "auth.logOut"} {
		if err := r.checkRegistrationPasswordRPC(ctx, method); err != nil {
			t.Fatalf("%s blocked: %v", method, err)
		}
	}
	auth := r.authorizationWithFutureToken(ctx, u)
	if !auth.SetupPasswordRequired || auth.OtherwiseReloginDays != 0 || len(auth.FutureAuthToken) != 0 {
		t.Fatal("pending registration must request setup without a reusable login token")
	}
	svc.pending = false
	if err := r.checkRegistrationPasswordRPC(ctx, "messages.sendMessage"); err != nil {
		t.Fatal(err)
	}
	if r.authorizationWithFutureToken(ctx, u).SetupPasswordRequired {
		t.Fatal("existing account required setup")
	}
	svc.err = errors.New("database unavailable")
	if err := r.checkRegistrationPasswordRPC(ctx, "messages.sendMessage"); !tgerr.Is(err, "INTERNAL_SERVER_ERROR") {
		t.Fatal("gate failed open", err)
	}
}

func TestRegistrationPasswordAppConfig(t *testing.T) {
	base := domain.AppConfig{Hash: 123, JSON: []byte(`{"safelink_registration_password_required":true,"other_setting":7}`)}
	active, err := registrationPasswordAppConfig(base, true, false)
	if err != nil {
		t.Fatal(err)
	}
	done, err := registrationPasswordAppConfig(base, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if active.Hash == done.Hash || active.Hash == 0 || done.Hash == 0 {
		t.Fatal("setup state not reflected in cache hash")
	}
	again, _ := registrationPasswordAppConfig(base, true, false)
	if again.Hash != active.Hash {
		t.Fatal("unstable config hash")
	}
}
