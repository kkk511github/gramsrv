package stickerlinks

import (
	"context"
	"errors"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"telesrv/internal/domain"
)

func TestHandlerServesStickerSetLandingPage(t *testing.T) {
	resolver := fakeResolver{
		"fresh_pack": {
			ID:        10,
			ShortName: "fresh_pack",
			Title:     "Fresh Pack",
			Count:     2,
			Kind:      domain.StickerSetKindStickers,
		},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/addstickers/fresh_pack", nil)

	NewHandler(resolver, "https://safelink.chat/").ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := html.UnescapeString(rr.Body.String())
	for _, want := range []string{
		"Fresh Pack",
		"https://safelink.chat/addstickers/fresh_pack",
		"safelink://addstickers?set=fresh_pack",
		"Add Stickers",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"tg://", "telesrv://"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("landing page must not contain %q:\n%s", forbidden, body)
		}
	}
	if strings.Contains(body, "/upload/getFile") {
		t.Fatalf("landing page should not expose media download paths:\n%s", body)
	}
}

func TestHandlerServesEmojiLandingPage(t *testing.T) {
	resolver := fakeResolver{
		"emoji_pack": {
			ID:        11,
			ShortName: "emoji_pack",
			Title:     "Emoji Pack",
			Count:     1,
			Kind:      domain.StickerSetKindEmoji,
			Emojis:    true,
		},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/addemoji/emoji_pack", nil)

	NewHandler(resolver, "https://example.test/base").ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := html.UnescapeString(rr.Body.String())
	for _, want := range []string{
		"custom emoji set",
		"https://example.test/base/addemoji/emoji_pack",
		"safelink://addemoji?set=emoji_pack",
		"Add Emoji",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"tg://", "telesrv://"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("landing page must not contain %q:\n%s", forbidden, body)
		}
	}
}

func TestHandlerServesChatlistLandingPage(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/addlist/zNhytIbwRwjaC2GH", nil)

	NewHandler(fakeResolver{}, "http://127.0.0.1:2401").ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"Shared Folder",
		"http://127.0.0.1:2401/addlist/zNhytIbwRwjaC2GH",
		"safelink://addlist?slug=zNhytIbwRwjaC2GH",
		"preview and add it",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"tg://", "telesrv://"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("landing page must not contain %q:\n%s", forbidden, body)
		}
	}
}

func TestHandlerRedirectsMismatchedKindToCanonicalURL(t *testing.T) {
	resolver := fakeResolver{
		"emoji_pack": {
			ID:        11,
			ShortName: "emoji_pack",
			Title:     "Emoji Pack",
			Kind:      domain.StickerSetKindEmoji,
			Emojis:    true,
		},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/addstickers/emoji_pack", nil)

	NewHandler(resolver, "https://safelink.chat").ServeHTTP(rr, req)

	if rr.Code != http.StatusPermanentRedirect {
		t.Fatalf("status = %d, want 308; body=%s", rr.Code, rr.Body.String())
	}
	if got, want := rr.Header().Get("Location"), "https://safelink.chat/addemoji/emoji_pack"; got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

func TestHandlerNotFoundForMissingOrInvalidShortName(t *testing.T) {
	handler := NewHandler(fakeResolver{}, "https://safelink.chat")
	for _, path := range []string{
		"/addstickers/missing_pack",
		"/addstickers/bad-name",
		"/addemoji/%E4%B8%AD%E6%96%87",
		"/addlist/bad!slug",
		"/addlist/%E4%B8%AD%E6%96%87",
	} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", path, rr.Code)
		}
	}
}

func TestHandlerLookupErrorIsInternalServerError(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/addstickers/fresh_pack", nil)

	NewHandler(errorResolver{}, "https://safelink.chat").ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}

func TestHandlerServesPublicLinkLandingPages(t *testing.T) {
	handler := NewHandler(fakeResolver{}, "https://safelink.chat")
	tests := []struct {
		path string
		want []string
	}{
		{
			path: "/",
			want: []string{"Open SafeLink", "safelink://"},
		},
		{
			path: "/+qgkY-PBX5nn3M_EzEn0F1tVj",
			want: []string{
				"https://safelink.chat/+qgkY-PBX5nn3M_EzEn0F1tVj",
				"safelink://join?invite=qgkY-PBX5nn3M_EzEn0F1tVj",
				"Join chat",
			},
		},
		{
			path: "/kkk03",
			want: []string{
				"https://safelink.chat/kkk03",
				"safelink://resolve?domain=kkk03",
				"@kkk03",
			},
		},
		{
			path: "/kkk03/12",
			want: []string{
				"https://safelink.chat/kkk03/12",
				"safelink://resolve?domain=kkk03&post=12",
				"Message #12",
			},
		},
		{
			path: "/c/123/45",
			want: []string{
				"https://safelink.chat/c/123/45",
				"safelink://privatepost?channel=123&post=45",
				"Private channel message",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
			}
			body := html.UnescapeString(rr.Body.String())
			for _, want := range tt.want {
				if !strings.Contains(body, want) {
					t.Fatalf("body missing %q:\n%s", want, body)
				}
			}
			for _, forbidden := range []string{"tg://", "telesrv://", "telesrv.net"} {
				if strings.Contains(body, forbidden) {
					t.Fatalf("landing page must not contain %q:\n%s", forbidden, body)
				}
			}
		})
	}
}

func TestHandlerCanUseRegisteredClientScheme(t *testing.T) {
	handler := NewHandlerWithConfig(fakeResolver{}, HandlerConfig{
		PublicBaseURL: "https://safelink.chat",
		AppScheme:     "tg",
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/+abc123", nil)

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := html.UnescapeString(rr.Body.String())
	for _, want := range []string{
		"https://safelink.chat/+abc123",
		"tg://join?invite=abc123",
		"Join chat",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
}

func TestHandlerRejectsInvalidPublicLinkPaths(t *testing.T) {
	handler := NewHandler(fakeResolver{}, "https://safelink.chat")
	for _, path := range []string{
		"/bad-name",
		"/kkk03/0",
		"/c/0/45",
		"/call/%E4%B8%AD%E6%96%87",
	} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404; body=%s", path, rr.Code, rr.Body.String())
		}
	}
}

type fakeResolver map[string]domain.StickerSet

func (f fakeResolver) ResolveStickerSet(_ context.Context, ref domain.StickerSetRef) (domain.StickerSet, []domain.Document, bool, error) {
	set, ok := f[ref.ShortName]
	if !ok {
		return domain.StickerSet{}, nil, false, nil
	}
	docs := make([]domain.Document, set.Count)
	return set, docs, true, nil
}

type errorResolver struct{}

func (errorResolver) ResolveStickerSet(context.Context, domain.StickerSetRef) (domain.StickerSet, []domain.Document, bool, error) {
	return domain.StickerSet{}, nil, false, errors.New("boom")
}
