package rpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iamxvbaba/td/bin"
	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tgerr"
	"go.uber.org/zap/zaptest"
	"telesrv/internal/domain"
)

type registrationInviteConfigAuth struct {
	*captureAuthService
	required bool
	err      error
}

func (a *registrationInviteConfigAuth) RegistrationInviteRequired(context.Context) (bool, error) {
	return a.required, a.err
}

type registrationInviteConfigHelp struct{ HelpService }

func (registrationInviteConfigHelp) GetAppConfig(context.Context, int64, int) (domain.AppConfig, bool, error) {
	return domain.AppConfig{JSON: []byte(`{"other_setting":7}`)}, false, nil
}

func TestRegistrationInviteConfigBeforeLogin(t *testing.T) {
	auth := &registrationInviteConfigAuth{captureAuthService: &captureAuthService{}}
	r := New(Config{}, Deps{Auth: auth, Help: registrationInviteConfigHelp{}}, zaptest.NewLogger(t), fixedClock{now: time.Now()})
	read := func(hash int) (bin.Encoder, error) {
		t.Helper()
		var b bin.Buffer
		if err := (&tg.HelpGetAppConfigRequest{Hash: hash}).Encode(&b); err != nil {
			t.Fatal(err)
		}
		return r.Dispatch(context.Background(), [8]byte{}, 0, &b)
	}
	lastHash := 0
	for _, required := range []bool{false, true, false} {
		auth.required = required
		result, err := read(lastHash)
		if err != nil {
			t.Fatal(err)
		}
		cfg, ok := result.(*tg.HelpAppConfig)
		if !ok {
			t.Fatalf("changed policy returned %T", result)
		}
		if cfg.Hash == 0 || cfg.Hash == lastHash {
			t.Fatal("policy change did not invalidate config hash")
		}
		lastHash = cfg.Hash
		object, ok := cfg.Config.(*tg.JSONObject)
		if !ok {
			t.Fatalf("config type %T", cfg.Config)
		}
		found := false
		for _, item := range object.Value {
			if item.Key == "safelink_registration_invite_required" {
				value, ok := item.Value.(*tg.JSONBool)
				if !ok || value.Value != required {
					t.Fatalf("wrong invite requirement: %#v", item.Value)
				}
				found = true
			}
		}
		if !found {
			t.Fatal("registration policy missing before login")
		}
		cached, err := read(lastHash)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := cached.(*tg.HelpAppConfigNotModified); !ok {
			t.Fatalf("unchanged config returned %T", cached)
		}
	}
	auth.err = errors.New("database unavailable")
	if _, err := read(0); !tgerr.Is(err, "INTERNAL_SERVER_ERROR") {
		t.Fatalf("failed policy lookup silently treated as optional: %v", err)
	}
}
