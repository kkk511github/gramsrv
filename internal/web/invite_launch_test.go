package web

import (
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestInviteLandingUsesSafeLinkAndExplicitWebFallback(t *testing.T) {
	const hash = "CNxZg8gdDwV1_yFWuCibDRLz"
	h, err := NewHandler(Config{StickerSets: fakeResolver{}, PublicBaseURL: "https://safelink.chat", WebBaseURL: "https://web.safelink.chat"})
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"iPhone", "Android", "Windows NT 10.0", "Macintosh"} {
		t.Run(agent, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/+"+hash, nil)
			req.Header.Set("User-Agent", agent)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d", rr.Code)
			}
			body := html.UnescapeString(rr.Body.String())
			app := "safelink://join?invite=" + hash
			for _, want := range []string{`href="` + app + `" data-open-app`, `href="https://web.safelink.chat/#?tgaddr=` + url.QueryEscape(app) + `" data-open-web`} {
				if !strings.Contains(body, want) {
					t.Fatalf("missing launch target %q", want)
				}
			}
			for _, forbidden := range []string{"tg://", "window.location", "setTimeout(", "desktopBrowser"} {
				if strings.Contains(body, forbidden) {
					t.Fatalf("unexpected automatic or Telegram launch: %q", forbidden)
				}
			}
		})
	}
}

func TestPublicDomainChangeDoesNotRequireNewAppScheme(t *testing.T) {
	h, err := NewHandler(Config{StickerSets: fakeResolver{}, PublicBaseURL: "https://new.example.test", WebBaseURL: "https://web.new.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/faq", "/+CNxZg8gdDwV1_yFWuCibDRLz"} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, path, nil))
		body := html.UnescapeString(r.Body.String())
		if r.Code != http.StatusOK || !strings.Contains(body, "https://web.new.example.test") || !strings.Contains(body, "safelink://") {
			t.Fatalf("new domain missing from %s", path)
		}
		if strings.Contains(body, `href="https://web.safelink.chat`) {
			t.Fatalf("hard-coded web destination in %s", path)
		}
	}
}
