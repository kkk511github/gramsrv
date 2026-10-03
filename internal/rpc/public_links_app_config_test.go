package rpc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/iamxvbaba/td/bin"
	"github.com/iamxvbaba/td/tg"
	"go.uber.org/zap/zaptest"
	"telesrv/internal/domain"
)

func TestPublicLinksAppConfig(t *testing.T) {
	base := domain.AppConfig{JSON: []byte(`{"other_setting":7,"safelink_registration_invite_required":true}`)}
	first, err := publicLinksAppConfig(base, "https://safelink.chat/")
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	if err := json.Unmarshal(first.JSON, &values); err != nil {
		t.Fatal(err)
	}
	if values["safelink_public_link_prefix"] != "https://safelink.chat/" || values["other_setting"] != float64(7) || values["safelink_registration_invite_required"] != true {
		t.Fatalf("config changed unexpectedly: %s", first.JSON)
	}
	if value, exists := values["safelink_web_auth_tokens_enabled"]; !exists || value != false {
		t.Fatal("unsupported bearer Web-token login must be explicitly disabled")
	}
	second, err := publicLinksAppConfig(first, "https://chat.example.com/")
	if err != nil || first.Hash == 0 || second.Hash == 0 || first.Hash == second.Hash {
		t.Fatalf("domain did not invalidate cache: %d, %d, %v", first.Hash, second.Hash, err)
	}
	unchanged, err := publicLinksAppConfig(second, "https://chat.example.com/")
	if err != nil || unchanged.Hash != second.Hash {
		t.Fatal("unchanged domain changed config hash")
	}
	for _, invalid := range []string{"null", "[]", "invalid"} {
		if _, err := publicLinksAppConfig(domain.AppConfig{JSON: []byte(invalid)}, "https://safelink.chat/"); err == nil {
			t.Fatalf("accepted invalid config %q", invalid)
		}
	}
}

func TestPublicLinksAppConfigRPC(t *testing.T) {
	r := New(Config{PublicBaseURL: "https://safelink.chat"}, Deps{Help: registrationInviteConfigHelp{}}, zaptest.NewLogger(t), fixedClock{now: time.Now()})
	read := func(hash int) bin.Encoder {
		t.Helper()
		var b bin.Buffer
		if err := (&tg.HelpGetAppConfigRequest{Hash: hash}).Encode(&b); err != nil {
			t.Fatal(err)
		}
		result, err := r.Dispatch(context.Background(), [8]byte{}, 0, &b)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	lastHash := 0
	for _, baseURL := range []string{"https://safelink.chat", "https://chat.example.com/", "http://212.189.31.87:8080"} {
		r.cfg.PublicBaseURL = baseURL
		cfg, ok := read(lastHash).(*tg.HelpAppConfig)
		if !ok || cfg.Hash == lastHash {
			t.Fatal("domain change did not return fresh config")
		}
		found := false
		for _, item := range cfg.Config.(*tg.JSONObject).Value {
			if item.Key == "safelink_public_link_prefix" {
				value, ok := item.Value.(*tg.JSONString)
				if !ok || value.Value != r.publicLink("") {
					t.Fatalf("unexpected prefix: %#v", item.Value)
				}
				found = true
			}
		}
		if !found {
			t.Fatal("link prefix missing")
		}
		lastHash = cfg.Hash
		if _, ok := read(lastHash).(*tg.HelpAppConfigNotModified); !ok {
			t.Fatal("unchanged domain bypassed cache")
		}
	}
}
