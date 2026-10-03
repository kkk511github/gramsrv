package rpc

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/iamxvbaba/td/tg"
	"telesrv/internal/domain"
	"telesrv/internal/store"
)

const chatJoinWebViewSource = "chat_join"

// A join challenge must be issued by a trusted server-side join workflow. An
// ordinary app/webview token must never become a membership-verification token.
func (r *Router) chatJoinWebViewSession(ctx context.Context, userID, queryID int64) (store.WebViewSession, bool) {
	if r.webviews == nil || queryID == 0 {
		return store.WebViewSession{}, false
	}
	now := r.clock.Now()
	r.webviews.mu.Lock()
	r.webviews.pruneLocked(now)
	registered, found := r.webviews.byQueryID[queryID]
	r.webviews.mu.Unlock()
	var session store.WebViewSession
	if found {
		session = cloneWebViewSession(registered.session)
	} else if r.webviews.shared != nil {
		var err error
		session, found, err = r.webviews.shared.GetWebViewSession(ctx, queryID)
		if err != nil {
			return store.WebViewSession{}, false
		}
	}
	if !found || session.QueryID != queryID || session.UserID != userID || session.Source != chatJoinWebViewSource ||
		session.BotUserID <= 0 || session.BotQueryID == "" || session.Peer.Type != domain.PeerTypeChannel || session.Peer.ID <= 0 ||
		!session.ExpiresAt.After(now) || session.CreatedAt.After(now) || session.WriteAllowed || session.SendAs != nil || session.ReplyTo != nil {
		return store.WebViewSession{}, false
	}
	return session, true
}

func (r *Router) onMessagesRequestChatJoinWebView(ctx context.Context, req *tg.MessagesRequestChatJoinWebViewRequest) (*tg.WebViewResultURL, error) {
	userID, err := r.currentAIComposeUserID(ctx)
	if err != nil {
		return nil, err
	}
	if req == nil || req.QueryID == 0 {
		return nil, queryIDInvalidErr()
	}
	if err := validateWebViewPlatform(req.Platform); err != nil {
		return nil, err
	}
	if r.deps.Users == nil || r.deps.Bots == nil {
		return nil, botWebviewDisabledErr()
	}
	user, err := r.deps.Users.Self(ctx, userID)
	if err != nil {
		return nil, internalErr()
	}
	if user.Bot {
		return nil, botMethodInvalidErr()
	}
	session, found := r.chatJoinWebViewSession(ctx, userID, req.QueryID)
	if !found {
		return nil, queryIDInvalidErr()
	}
	bot, found, err := r.deps.Users.ByID(ctx, userID, session.BotUserID)
	if err != nil {
		return nil, internalErr()
	}
	if !found || !bot.Bot {
		return nil, botInvalidErr()
	}
	profile, found, err := r.deps.Bots.BotInfo(ctx, bot.ID)
	if err != nil {
		return nil, internalErr()
	}
	if !found || profile.BotUserID != bot.ID || profile.TokenSecret == "" {
		return nil, botInvalidErr()
	}
	var rawURL string
	if session.AppID != 0 {
		apps, err := r.deps.Bots.ListBotApps(ctx, bot.ID)
		if err != nil {
			return nil, internalErr()
		}
		for _, app := range apps {
			if app.ID == session.AppID && app.BotUserID == bot.ID && !app.Inactive {
				rawURL = app.URL
				break
			}
		}
		if rawURL == "" {
			return nil, botAppInvalidErr()
		}
	} else {
		rawURL, err = webViewMenuAppURL(profile, botWebviewDisabledErr())
		if err != nil {
			return nil, err
		}
	}
	if validateInlineWebURL(rawURL, true) != nil {
		return nil, urlInvalidErr()
	}
	if session.StartParam != "" && !validInlineStartParam(session.StartParam) {
		return nil, startParamInvalidErr()
	}
	theme, themeSet := req.GetThemeParams()
	if themeSet {
		var object map[string]json.RawMessage
		if len(theme.Data) > 16*1024 || json.Unmarshal([]byte(theme.Data), &object) != nil || object == nil {
			return nil, dataInvalidErr()
		}
	}
	signedURL, err := webViewURLWithInitData(rawURL, session.BotQueryID, profile, user, session.StartParam, req.Platform, r.clock.Now())
	if err != nil {
		return nil, internalErr()
	}
	if themeSet {
		parsed, err := url.Parse(signedURL)
		if err != nil {
			return nil, internalErr()
		}
		q := parsed.Query()
		q.Set("tgWebAppThemeParams", theme.Data)
		parsed.RawQuery = q.Encode()
		signedURL = parsed.String()
	}
	out := &tg.WebViewResultURL{URL: signedURL}
	out.SetQueryID(session.QueryID)
	return out, nil
}
