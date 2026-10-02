package config

import (
	"path/filepath"
	"telesrv/internal/linksettings"
	"testing"
)

func TestPublicDomainOverrideUpdatesClientAndWebOrigins(t *testing.T) {
	p := filepath.Join(t.TempDir(), "links.json")
	if err := linksettings.Save(p, linksettings.Settings{PublicURL: "https://new.example.test", WebURL: "https://web.new.example.test"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TELESRV_PUBLIC_LINK_SETTINGS_FILE", p)
	t.Setenv("TELESRV_PUBLIC_BASE_URL", "https://old.example.test")
	t.Setenv("TELESRV_PUBLIC_WEB_BASE_URL", "https://web.old.example.test")
	t.Setenv("TELESRV_PUBLIC_APP_LINK_BASE", "safelink://old.example.test")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicBaseURL != "https://new.example.test" || cfg.PublicWebBaseURL != "https://web.new.example.test" || cfg.Branding.PublicBaseURL != cfg.PublicBaseURL || cfg.PublicAppScheme != "safelink" || cfg.PublicAppLinkBase != "" {
		t.Fatal("public domain override was not applied consistently")
	}
	if len(cfg.WebSocketAllowedOrigins) != 1 || cfg.WebSocketAllowedOrigins[0] != cfg.PublicWebBaseURL {
		t.Fatal("websocket origin did not follow web domain")
	}
}
