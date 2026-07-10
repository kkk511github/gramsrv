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
		"tg://addstickers?set=fresh_pack",
		"Add Stickers",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"telesrv://"} {
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
		"tg://addemoji?set=emoji_pack",
		"Add Emoji",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"telesrv://"} {
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
		"tg://addlist?slug=zNhytIbwRwjaC2GH",
		"preview and add it",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"telesrv://"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("landing page must not contain %q:\n%s", forbidden, body)
		}
	}
}

func TestHandlerServesBotUsernameLandingPage(t *testing.T) {
	users := fakeUsers{
		"tetrisbot": {
			ID:        1001,
			Username:  "TetrisBot",
			FirstName: "Tetris Bot",
			Bot:       true,
		},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tetrisbot", nil)

	NewHandlerWithPublicPeers(fakeResolver{}, users, nil, nil, nil, "http://127.0.0.1:2401").ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"Tetris Bot",
		"SafeLink Bot",
		"@TetrisBot",
		"http://127.0.0.1:2401/TetrisBot",
		"safelink://resolve?domain=TetrisBot",
		"tg://resolve?domain=TetrisBot",
		"Start Bot",
		"Open SafeLink to start a chat with this bot.",
		`property="og:title" content="Tetris Bot"`,
		`property="al:android:url" content="safelink://resolve?domain=TetrisBot"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, `window.location.href = "tg://`) {
		t.Fatalf("landing page must not auto-open tg:// and steal official Telegram:\n%s", body)
	}
	if strings.Contains(body, "telesrv://") {
		t.Fatalf("landing page must not contain old scheme:\n%s", body)
	}
}

func TestHandlerServesUserChannelAndSupergroupLandingPages(t *testing.T) {
	users := fakeUsers{
		"alice": {
			ID:         2001,
			AccessHash: 987654321,
			Phone:      "+15551234567",
			Username:   "Alice",
			FirstName:  "Alice",
			LastName:   "Example",
			About:      "Public bio",
			Verified:   true,
			LastSeenAt: 1700000000,
		},
	}
	channels := fakeChannels{
		"newsroom": {
			ID:                3001,
			Username:          "NewsRoom",
			Title:             "News Room",
			About:             "Public channel description",
			Broadcast:         true,
			ParticipantsCount: 12001,
			Verified:          true,
			PhotoID:           301,
		},
		"studygroup": {
			ID:                3002,
			Username:          "StudyGroup",
			Title:             "Study Group",
			About:             "A public supergroup",
			Megagroup:         true,
			ParticipantsCount: 1,
		},
	}
	photos := &fakePhotos{byID: map[int64]domain.Photo{
		301: {ID: 301, Sizes: []domain.PhotoSize{{Kind: domain.PhotoSizeKindDefault, Type: "c", W: 640, H: 640, Size: 12}}},
	}}
	handler := NewHandlerWithPublicPeers(fakeResolver{}, users, channels, nil, photos, "https://safelink.chat")

	for _, tc := range []struct {
		path  string
		wants []string
	}{
		{
			path: "/aLiCe/",
			wants: []string{
				"Alice Example", "Public bio", "@Alice", "Send Message", "Verified",
				"https://safelink.chat/Alice", "safelink://resolve?domain=Alice",
			},
		},
		{
			path: "/NewsRoom",
			wants: []string{
				"News Room", "Public channel description", "12 001 subscribers", "View Channel",
				"https://safelink.chat/NewsRoom", "safelink://resolve?domain=NewsRoom",
				"/_public/avatar/NewsRoom/301",
			},
		},
		{
			path: "/StudyGroup",
			wants: []string{
				"Study Group", "A public supergroup", "1 member", "View Group",
			},
		},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
			}
			for _, want := range tc.wants {
				if !strings.Contains(rr.Body.String(), want) {
					t.Fatalf("body missing %q:\n%s", want, rr.Body.String())
				}
			}
			if tc.path == "/aLiCe/" && strings.Count(rr.Body.String(), `<p class="username">@Alice</p>`) != 1 {
				t.Fatalf("ordinary user username rendered more than once:\n%s", rr.Body.String())
			}
			if strings.Contains(rr.Body.String(), "+15551234567") || strings.Contains(rr.Body.String(), "987654321") || strings.Contains(rr.Body.String(), "1700000000") {
				t.Fatalf("private protocol fields leaked into page:\n%s", rr.Body.String())
			}
		})
	}
}

func TestHandlerPreservesBoundedResolveQueryAndOverridesDomain(t *testing.T) {
	handler := NewHandlerWithPublicPeers(fakeResolver{}, fakeUsers{
		"tetrisbot": {ID: 2001, Username: "TetrisBot", FirstName: "Tetris", Bot: true},
	}, nil, nil, nil, "https://safelink.chat")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/TetrisBot?start=hello&ref=campaign&domain=EvilBot", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"safelink://resolve?domain=TetrisBot&amp;ref=campaign&amp;start=hello",
		"tg://resolve?domain=TetrisBot&amp;ref=campaign&amp;start=hello",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing sanitized query %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "EvilBot") {
		t.Fatalf("caller-controlled domain leaked into page:\n%s", body)
	}

	for _, target := range []string{
		"/TetrisBot?bad-key=value",
		"/TetrisBot?start=" + strings.Repeat("a", maxPublicLinkValueLen+1),
		"/TetrisBot?" + strings.Repeat("a", maxPublicLinkRawQuery+1),
	} {
		rr = httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, target, nil))
		if rr.Code != http.StatusRequestURITooLong {
			t.Fatalf("%s status = %d, want 414", target, rr.Code)
		}
	}
}

func TestHandlerHonorsAnonymousAboutAndPhotoPrivacy(t *testing.T) {
	const userID int64 = 2001
	photos := &fakePhotos{
		photos: map[photoLookupKey]domain.Photo{
			{ownerType: domain.PeerTypeUser, ownerID: userID, kind: domain.ProfilePhotoKindProfile}: {
				ID: 10, Sizes: []domain.PhotoSize{{Kind: domain.PhotoSizeKindDefault, Type: "c", W: 640, H: 640, Size: 12}},
			},
			{ownerType: domain.PeerTypeUser, ownerID: userID, kind: domain.ProfilePhotoKindFallback}: {
				ID: 11, Sizes: []domain.PhotoSize{{Kind: domain.PhotoSizeKindDefault, Type: "c", W: 640, H: 640, Size: 12}},
			},
		},
	}
	privacy := fakeAnonymousPrivacy{
		domain.PrivacyKeyAbout:        false,
		domain.PrivacyKeyProfilePhoto: false,
	}
	handler := NewHandlerWithPublicPeers(fakeResolver{}, fakeUsers{
		"alice": {ID: userID, Username: "Alice", FirstName: "Alice", About: "private biography"},
	}, nil, privacy, photos, "https://safelink.chat")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/Alice", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, "private biography") || strings.Contains(body, "/10") {
		t.Fatalf("private about or main photo leaked:\n%s", body)
	}
	if !strings.Contains(body, "/_public/avatar/Alice/11") {
		t.Fatalf("fallback public photo missing:\n%s", body)
	}
}

func TestHandlerServesBoundedCurrentAvatarWithETag(t *testing.T) {
	const userID int64 = 2001
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01}
	photos := &fakePhotos{
		photos: map[photoLookupKey]domain.Photo{
			{ownerType: domain.PeerTypeUser, ownerID: userID, kind: domain.ProfilePhotoKindProfile}: {
				ID: 99, Date: 1700000000,
				Sizes: []domain.PhotoSize{{Kind: domain.PhotoSizeKindDefault, Type: "c", W: 640, H: 640, Size: len(jpeg)}},
			},
		},
		files: map[string]domain.FileChunk{
			"photo:99:c": {Bytes: jpeg, MimeType: "image/jpeg", Total: int64(len(jpeg))},
		},
	}
	handler := NewHandlerWithPublicPeers(fakeResolver{}, fakeUsers{
		"alice": {ID: userID, Username: "Alice", FirstName: "Alice"},
	}, nil, nil, photos, "https://safelink.chat")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/_public/avatar/Alice/99", nil))
	if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "image/jpeg" || rr.Body.String() != string(jpeg) {
		t.Fatalf("avatar response status=%d type=%q body=%x", rr.Code, rr.Header().Get("Content-Type"), rr.Body.Bytes())
	}
	if rr.Header().Get("ETag") == "" || rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("avatar headers = %+v", rr.Header())
	}
	etag := rr.Header().Get("ETag")
	req := httptest.NewRequest(http.MethodGet, "/_public/avatar/Alice/99", nil)
	req.Header.Set("If-None-Match", etag)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotModified || rr.Body.Len() != 0 {
		t.Fatalf("conditional avatar status=%d body=%x", rr.Code, rr.Body.Bytes())
	}

	for _, path := range []string{"/_public/avatar/Alice/100", "/_public/avatar/Missing/99"} {
		rr = httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", path, rr.Code)
		}
	}
}

func TestHandlerFailsFastForAmbiguousUsernameOwner(t *testing.T) {
	handler := NewHandlerWithPublicPeers(fakeResolver{}, fakeUsers{
		"sharedname": {ID: 2001, Username: "SharedName", FirstName: "User"},
	}, fakeChannels{
		"sharedname": {ID: 3001, Username: "SharedName", Title: "Channel", Broadcast: true},
	}, nil, nil, "https://safelink.chat")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/SharedName", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("ambiguous owner status = %d, want 500", rr.Code)
	}
}

func TestHandlerReturnsTrustedUsernameNotFoundPage(t *testing.T) {
	handler := NewHandlerWithPublicPeers(fakeResolver{}, fakeUsers{}, fakeChannels{}, nil, nil, "https://safelink.chat")
	for _, path := range []string{"/MissingName", "/bad-name", "/Nope"} {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusNotFound || !strings.Contains(rr.Body.String(), "Username not found") || !strings.Contains(rr.Body.String(), "noindex,nofollow") {
			t.Fatalf("%s status=%d body=%s", path, rr.Code, rr.Body.String())
		}
		if strings.Contains(rr.Body.String(), "telesrv://resolve") {
			t.Fatalf("not-found page contains a fabricated app link: %s", rr.Body.String())
		}
		if rr.Header().Get("Content-Security-Policy") == "" || rr.Header().Get("X-Frame-Options") != "DENY" {
			t.Fatalf("not-found security headers = %+v", rr.Header())
		}
	}
}

func TestPublicAvatarRejectsOversizedOrUnsafeBlob(t *testing.T) {
	const userID int64 = 2001
	photo := domain.Photo{
		ID:    99,
		Sizes: []domain.PhotoSize{{Kind: domain.PhotoSizeKindDefault, Type: "c", W: 640, H: 640, Size: 12}},
	}
	for _, tc := range []struct {
		name  string
		chunk domain.FileChunk
	}{
		{name: "oversized", chunk: domain.FileChunk{Bytes: []byte("x"), MimeType: "image/jpeg", Total: maxPublicAvatarBytes + 1}},
		{name: "unsafe mime", chunk: domain.FileChunk{Bytes: []byte("<svg></svg>"), MimeType: "image/svg+xml", Total: int64(len("<svg></svg>"))}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			photos := &fakePhotos{
				photos: map[photoLookupKey]domain.Photo{
					{ownerType: domain.PeerTypeUser, ownerID: userID, kind: domain.ProfilePhotoKindProfile}: photo,
				},
				files: map[string]domain.FileChunk{"photo:99:c": tc.chunk},
			}
			handler := NewHandlerWithPublicPeers(fakeResolver{}, fakeUsers{
				"alice": {ID: userID, Username: "Alice", FirstName: "Alice"},
			}, nil, nil, photos, "https://safelink.chat")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/_public/avatar/Alice/99", nil))
			if rr.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404; body=%s", rr.Code, rr.Body.String())
			}
		})
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
	handler := NewHandlerWithPublicPeers(fakeResolver{}, fakeUsers{
		"alice": {
			ID:        2001,
			Username:  "Alice",
			FirstName: "Alice",
		},
	}, nil, nil, nil, "https://safelink.chat")
	for _, path := range []string{
		"/addstickers/missing_pack",
		"/addstickers/bad-name",
		"/addemoji/%E4%B8%AD%E6%96%87",
		"/addlist/bad!slug",
		"/addlist/%E4%B8%AD%E6%96%87",
		"/MissingBot",
		"/bad-name-bot",
		"/1stBot",
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
		path       string
		want       []string
		compatWant string
	}{
		{
			path: "/",
			want: []string{
				"安全连接每一段对话",
				"下载 SafeLink",
				"打开网页版",
				"网页版地址：",
				`href="/faq"`,
				`href="/apps"`,
				`href="/api"`,
				`href="/safety"`,
				`href="/updates"`,
				`href="/support"`,
				`href="/terms"`,
				`href="/translations"`,
				`href="/instantview"`,
				"/assets/safelink-home-devices.png",
				"/assets/safelink-mark.png",
				"https://web.safelink.chat/",
				"safelink://",
			},
		},
		{
			path: "/+qgkY-PBX5nn3M_EzEn0F1tVj",
			want: []string{
				"https://safelink.chat/+qgkY-PBX5nn3M_EzEn0F1tVj",
				"safelink://join?invite=qgkY-PBX5nn3M_EzEn0F1tVj",
				"Join chat",
				"data-open-app",
				"openLaunchWithFallback(700)",
				"desktopBrowser",
			},
			compatWant: "tg://join?invite=qgkY-PBX5nn3M_EzEn0F1tVj",
		},
		{
			path: "/kkk03",
			want: []string{
				"https://safelink.chat/kkk03",
				"safelink://resolve?domain=kkk03",
				"@kkk03",
			},
			compatWant: "tg://resolve?domain=kkk03",
		},
		{
			path: "/kkk03/12",
			want: []string{
				"https://safelink.chat/kkk03/12",
				"safelink://resolve?domain=kkk03&post=12",
				"Message #12",
			},
			compatWant: "tg://resolve?domain=kkk03&post=12",
		},
		{
			path: "/c/123/45",
			want: []string{
				"https://safelink.chat/c/123/45",
				"safelink://privatepost?channel=123&post=45",
				"Private channel message",
			},
			compatWant: "tg://privatepost?channel=123&post=45",
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
			if tt.compatWant != "" && !strings.Contains(body, tt.compatWant) {
				t.Fatalf("body missing compatibility app URL %q:\n%s", tt.compatWant, body)
			}
			for _, forbidden := range []string{"telesrv://", "telesrv.net"} {
				if strings.Contains(body, forbidden) {
					t.Fatalf("landing page must not contain %q:\n%s", forbidden, body)
				}
			}
			if tt.path == "/" {
				for _, forbidden := range []string{"Telegram", "Tiding", "t.me", "tg://"} {
					if strings.Contains(body, forbidden) {
						t.Fatalf("home page must not contain %q:\n%s", forbidden, body)
					}
				}
			}
		})
	}
}

func TestHandlerServesSafeLinkSitePages(t *testing.T) {
	handler := NewHandler(fakeResolver{}, "https://safelink.chat")
	tests := []struct {
		path string
		want []string
	}{
		{path: "/faq", want: []string{"常见问题", "SafeLink 是什么？", "safelink://"}},
		{path: "/apps", want: []string{"SafeLink 客户端", "当前先使用网页版地址：https://web.safelink.chat/", "iOS", "Android", "打开 Web"}},
		{path: "/api", want: []string{"开发者与服务端接口", "MTProto 连接", "Admin API"}},
		{path: "/safety", want: []string{"安全与隐私", "邮箱验证码", "设备管理"}},
		{path: "/updates", want: []string{"更新日志", "官网首页与中文内容", "媒体收发"}},
		{path: "/blog", want: []string{"更新日志", "官网首页与中文内容", "媒体收发"}},
		{path: "/links", want: []string{"SafeLink 链接规则", "https://safelink.chat/+invite_hash", "safelink://join?invite=invite_hash"}},
		{path: "/privacy", want: []string{"隐私说明", "数据范围", "会话管理"}},
		{path: "/press", want: []string{"品牌与媒体资料", "产品名统一写作 SafeLink", "www.safelink.chat"}},
		{path: "/support", want: []string{"帮助与支持", "登录与验证码", "媒体下载"}},
		{path: "/terms", want: []string{"服务条款", "自有实例", "测试阶段"}},
		{path: "/tos", want: []string{"服务条款", "自有实例", "测试阶段"}},
		{path: "/translations", want: []string{"本地化与翻译", "中文优先", "SafeLink"}},
		{path: "/instantview", want: []string{"链接预览与即时视图", "邀请预览", "Open Graph"}},
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
			for _, forbidden := range []string{"Telegram", "Tiding", "t.me", "tg://", "telesrv.net"} {
				if strings.Contains(body, forbidden) {
					t.Fatalf("site page must not contain %q:\n%s", forbidden, body)
				}
			}
		})
	}
}

func TestHandlerServesHomeDeviceAsset(t *testing.T) {
	handler := NewHandler(fakeResolver{}, "https://safelink.chat")
	for _, path := range []string{
		"/assets/safelink-home-devices.png",
		"/assets/safelink-mark.png",
	} {
		t.Run(path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rr.Code)
			}
			if got := rr.Header().Get("Content-Type"); !strings.HasPrefix(got, "image/png") {
				t.Fatalf("Content-Type = %q, want image/png", got)
			}
			if rr.Body.Len() == 0 {
				t.Fatal("asset body is empty")
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

type fakeUsers map[string]domain.User

func (f fakeUsers) ByUsername(_ context.Context, username string) (domain.User, bool, error) {
	u, ok := f[strings.ToLower(strings.TrimPrefix(username, "@"))]
	return u, ok, nil
}

type fakeChannels map[string]domain.Channel

func (f fakeChannels) ResolvePublicChannelUsername(_ context.Context, _ int64, username string) (domain.Channel, bool, error) {
	ch, ok := f[strings.ToLower(strings.TrimPrefix(username, "@"))]
	return ch, ok, nil
}

type fakeAnonymousPrivacy map[domain.PrivacyKey]bool

func (f fakeAnonymousPrivacy) CanSeeAnonymous(_ context.Context, _ int64, key domain.PrivacyKey) (bool, error) {
	visible, ok := f[key]
	if !ok {
		return true, nil
	}
	return visible, nil
}

type photoLookupKey struct {
	ownerType domain.PeerType
	ownerID   int64
	kind      domain.ProfilePhotoKind
}

type fakePhotos struct {
	photos map[photoLookupKey]domain.Photo
	byID   map[int64]domain.Photo
	files  map[string]domain.FileChunk
}

func (f *fakePhotos) CurrentProfilePhotoKind(_ context.Context, ownerType domain.PeerType, ownerID int64, kind domain.ProfilePhotoKind) (domain.Photo, bool, error) {
	photo, ok := f.photos[photoLookupKey{ownerType: ownerType, ownerID: ownerID, kind: kind}]
	return photo, ok, nil
}

func (f *fakePhotos) GetPhoto(_ context.Context, id int64) (domain.Photo, bool, error) {
	photo, ok := f.byID[id]
	return photo, ok, nil
}

func (f *fakePhotos) GetFile(_ context.Context, req domain.FileDownloadRequest) (domain.FileChunk, bool, error) {
	chunk, ok := f.files[req.LocationKey]
	return chunk, ok, nil
}
