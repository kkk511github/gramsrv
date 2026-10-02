package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"telesrv/internal/linksettings"
	"testing"
)

func TestDomainSettingsDryRunAndSave(t *testing.T) {
	p := filepath.Join(t.TempDir(), "links.json")
	s := &server{cfg: uiConfig{PublicLinkSettingsFile: p}}
	for _, confirm := range []string{"false", "true"} {
		r := httptest.NewRequest("POST", "/api/actions/set-server-domains", strings.NewReader(`{"reason":"domain test","confirm":`+confirm+`,"public_url":"https://new.example.test","web_url":"https://web.new.example.test"}`))
		w := httptest.NewRecorder()
		s.handleSetServerDomainsAPI(w, r)
		if w.Code != http.StatusOK {
			t.Fatal(w.Body.String())
		}
		got, err := linksettings.Read(p)
		if err != nil || (got != nil) != (confirm == "true") {
			t.Fatalf("confirm=%s got=%v err=%v", confirm, got, err)
		}
	}
	w := httptest.NewRecorder()
	s.handleSetServerDomainsAPI(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"reason":"invalid","confirm":true,"public_url":"https://bad.test/?x=1","web_url":"https://web.example.test"}`)))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatal(w.Body.String())
	}
	got, _ := linksettings.Read(p)
	if got.PublicURL != "https://new.example.test" {
		t.Fatal("invalid save changed settings")
	}
}
