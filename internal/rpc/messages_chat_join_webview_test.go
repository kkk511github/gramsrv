package rpc

import (
	"context"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tgerr"
	"telesrv/internal/domain"
	"telesrv/internal/store"
)

func storeJoinSession(userID, botID int64) store.WebViewSession {
	return store.WebViewSession{UserID: userID, BotUserID: botID, Peer: domain.Peer{Type: domain.PeerTypeChannel, ID: 123}, Source: chatJoinWebViewSource, StartParam: "verify_join"}
}

func TestChatJoinWebViewOwnerExpirySourceAndConsumption(t *testing.T) {
	f := newInlineBotRPCTestFixture(t)
	ctx := WithUserID(context.Background(), f.owner.ID)
	now := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	f.router.clock = fixedClock{now: now}
	if _, err := f.bots.SetBotMenuButton(ctx, f.bot.ID, domain.BotMenuButton{Type: domain.BotMenuButtonWebView, Text: "Verify", URL: "https://local.example/verify?existing=1"}); err != nil {
		t.Fatal(err)
	}
	session := f.router.webviews.registerContext(ctx, now, storeJoinSession(f.owner.ID, f.bot.ID))
	req := &tg.MessagesRequestChatJoinWebViewRequest{QueryID: session.QueryID, Platform: "ios"}
	req.SetThemeParams(tg.DataJSON{Data: `{"bg_color":"#ffffff"}`})
	out, err := f.router.onMessagesRequestChatJoinWebView(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(out.URL)
	if parsed.Host != "local.example" || parsed.Query().Get("existing") != "1" || parsed.Query().Get("tgWebAppThemeParams") != req.ThemeParams.Data {
		t.Fatalf("URL = %s", out.URL)
	}
	init := webViewInitDataFromURL(t, out.URL)
	if init.Get("query_id") != strconv.FormatInt(session.QueryID, 10) {
		t.Fatal("query binding lost")
	}
	profile, found, err := f.bots.BotInfo(ctx, f.bot.ID)
	if err != nil || !found {
		t.Fatal(err)
	}
	if init.Get("hash") != webViewInitDataHash(init, domain.FormatBotToken(profile.BotUserID, profile.TokenSecret)) {
		t.Fatal("bad signature")
	}
	if _, err := f.router.onMessagesRequestChatJoinWebView(WithUserID(context.Background(), f.peer.ID), req); !tgerr.Is(err, "QUERY_ID_INVALID") {
		t.Fatalf("cross-user = %v", err)
	}
	if _, err := f.router.onMessagesRequestChatJoinWebView(WithUserID(context.Background(), f.bot.ID), req); !tgerr.Is(err, "BOT_METHOD_INVALID") {
		t.Fatalf("bot caller = %v", err)
	}
	if err := f.router.sendWebViewDomainResultMessage(ctx, f.bot.ID, session.BotQueryID, domain.BotInlineResult{Message: "bypass"}); !tgerr.Is(err, "QUERY_ID_INVALID") {
		t.Fatalf("join token used to send = %v", err)
	}
	if _, err := f.router.onMessagesProlongWebView(ctx, &tg.MessagesProlongWebViewRequest{QueryID: session.QueryID, Peer: &tg.InputPeerSelf{}, Bot: inputUser(f.bot)}); !tgerr.Is(err, "QUERY_ID_INVALID") {
		t.Fatalf("join token prolonged = %v", err)
	}
	if allowed, err := f.bots.CanSendMessage(ctx, f.owner.ID, f.bot.ID); err != nil || allowed {
		t.Fatalf("write access granted = %v/%v", allowed, err)
	}
	ordinary := storeJoinSession(f.owner.ID, f.bot.ID)
	ordinary.Source = "webview"
	ordinary = f.router.webviews.registerContext(ctx, now, ordinary)
	if _, err := f.router.onMessagesRequestChatJoinWebView(ctx, &tg.MessagesRequestChatJoinWebViewRequest{QueryID: ordinary.QueryID, Platform: "ios"}); !tgerr.Is(err, "QUERY_ID_INVALID") {
		t.Fatalf("ordinary token = %v", err)
	}
	f.router.clock = fixedClock{now: now.Add(webViewSessionTTL)}
	if _, err := f.router.onMessagesRequestChatJoinWebView(ctx, req); !tgerr.Is(err, "QUERY_ID_INVALID") {
		t.Fatalf("expired = %v", err)
	}
	f.router.clock = fixedClock{now: now}
	session = f.router.webviews.registerContext(ctx, now, storeJoinSession(f.owner.ID, f.bot.ID))
	req.QueryID = session.QueryID
	f.router.webviews.consumeContext(ctx, session.QueryID, session.BotQueryID)
	if _, err := f.router.onMessagesRequestChatJoinWebView(ctx, req); !tgerr.Is(err, "QUERY_ID_INVALID") {
		t.Fatalf("consumed = %v", err)
	}
}

func TestChatJoinWebViewSharedRegistryRejectsExpiredAndCrossUser(t *testing.T) {
	f := newInlineBotRPCTestFixture(t)
	ctx := WithUserID(context.Background(), f.owner.ID)
	now := time.Now()
	f.router.clock = fixedClock{now: now}
	shared := newTestInlineRegistryStore()
	writer := newWebViewRegistry(webViewSessionTTL, shared)
	f.router.webviews = newWebViewRegistry(webViewSessionTTL, shared)
	session := writer.registerContext(ctx, now, storeJoinSession(f.owner.ID, f.bot.ID))
	if recovered, found := f.router.chatJoinWebViewSession(ctx, f.owner.ID, session.QueryID); !found || recovered.BotUserID != f.bot.ID {
		t.Fatal("shared session not recovered")
	}
	if _, found := f.router.chatJoinWebViewSession(ctx, f.peer.ID, session.QueryID); found {
		t.Fatal("shared cross-user session admitted")
	}
	f.router.clock = fixedClock{now: now.Add(webViewSessionTTL)}
	if _, found := f.router.chatJoinWebViewSession(ctx, f.owner.ID, session.QueryID); found {
		t.Fatal("shared expired session admitted")
	}
	f.router.clock = fixedClock{now: now}
	f.router.webviews.consumeContext(ctx, session.QueryID, session.BotQueryID)
	if _, found := f.router.chatJoinWebViewSession(ctx, f.owner.ID, session.QueryID); found {
		t.Fatal("shared consumed session admitted")
	}
}

func TestChatJoinWebViewRejectsMissingConfigAndMalformedTheme(t *testing.T) {
	f := newInlineBotRPCTestFixture(t)
	ctx := WithUserID(context.Background(), f.owner.ID)
	session := f.router.webviews.registerContext(ctx, f.router.clock.Now(), storeJoinSession(f.owner.ID, f.bot.ID))
	req := &tg.MessagesRequestChatJoinWebViewRequest{QueryID: session.QueryID, Platform: "ios"}
	if _, err := f.router.onMessagesRequestChatJoinWebView(ctx, req); !tgerr.Is(err, "BOT_WEBVIEW_DISABLED") {
		t.Fatalf("missing local config = %v", err)
	}
	if _, err := f.bots.SetBotMenuButton(ctx, f.bot.ID, domain.BotMenuButton{Type: domain.BotMenuButtonWebView, Text: "Verify", URL: "https://local.example/verify"}); err != nil {
		t.Fatal(err)
	}
	req.SetThemeParams(tg.DataJSON{Data: `[]`})
	if _, err := f.router.onMessagesRequestChatJoinWebView(ctx, req); !tgerr.Is(err, "DATA_INVALID") {
		t.Fatalf("theme = %v", err)
	}
	req.Platform = " "
	if _, err := f.router.onMessagesRequestChatJoinWebView(ctx, req); !tgerr.Is(err, "PLATFORM_INVALID") {
		t.Fatalf("platform = %v", err)
	}
}
