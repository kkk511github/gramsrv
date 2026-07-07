package stickerlinks

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"telesrv/internal/brand"
	"telesrv/internal/domain"
	"telesrv/internal/links"
)

type Config struct {
	Addr          string
	PublicBaseURL string
	AppScheme     string
}

type Resolver interface {
	ResolveStickerSet(ctx context.Context, ref domain.StickerSetRef) (domain.StickerSet, []domain.Document, bool, error)
}

func Start(ctx context.Context, cfg Config, resolver Resolver, logger *zap.Logger) (*http.Server, error) {
	addr := strings.TrimSpace(cfg.Addr)
	if addr == "" {
		return nil, nil
	}
	if resolver == nil {
		return nil, fmt.Errorf("sticker links resolver is nil")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	handler := NewHandlerWithConfig(resolver, HandlerConfig{
		PublicBaseURL: cfg.PublicBaseURL,
		AppScheme:     cfg.AppScheme,
	})
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	go func() {
		logger.Info("Public link Web endpoint enabled", zap.String("addr", addr), zap.String("public_base_url", normalizePublicBaseURL(cfg.PublicBaseURL)))
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Warn("Public link Web endpoint exited", zap.Error(err))
		}
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	return srv, nil
}

func NewHandler(resolver Resolver, publicBaseURL string) http.Handler {
	return NewHandlerWithConfig(resolver, HandlerConfig{PublicBaseURL: publicBaseURL})
}

type HandlerConfig struct {
	PublicBaseURL string
	AppScheme     string
}

func NewHandlerWithConfig(resolver Resolver, cfg HandlerConfig) http.Handler {
	h := &handler{
		resolver:      resolver,
		publicBaseURL: normalizePublicBaseURL(cfg.PublicBaseURL),
		appScheme:     normalizeAppScheme(cfg.AppScheme),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("GET /", h.root)
	mux.HandleFunc("GET /addstickers/{shortName}", h.addStickers)
	mux.HandleFunc("GET /addemoji/{shortName}", h.addEmoji)
	mux.HandleFunc("GET /addlist/{slug}", h.addList)
	mux.HandleFunc("GET /c/{channelID}/{messageID}", h.privateMessage)
	mux.HandleFunc("GET /call/{slug}", h.call)
	mux.HandleFunc("GET /m/{slug}", h.businessChat)
	mux.HandleFunc("GET /addstyle/{slug}", h.aiStyle)
	mux.HandleFunc("GET /{username}/{messageID}", h.publicMessage)
	mux.HandleFunc("GET /{username}", h.username)
	return mux
}

type handler struct {
	resolver      Resolver
	publicBaseURL string
	appScheme     string
}

func (h *handler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

func (h *handler) root(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	appName := brand.DefaultAppName
	h.serveLanding(w, landingPage{
		Title:        appName,
		CanonicalURL: h.publicBaseURL + "/",
		AppURL:       h.appURL(""),
		KindLabel:    "Instant messaging",
		PageTitle:    appName,
		Description:  "Open " + appName + " to continue.",
		ActionText:   "Open " + appName,
		Icon:         "paper",
		PathFull:     "/",
	})
}

func (h *handler) addStickers(w http.ResponseWriter, r *http.Request) {
	h.serveSet(w, r, "addstickers")
}

func (h *handler) addEmoji(w http.ResponseWriter, r *http.Request) {
	h.serveSet(w, r, "addemoji")
}

func (h *handler) joinInvite(w http.ResponseWriter, r *http.Request) {
	hash := strings.TrimPrefix(strings.TrimSpace(r.PathValue("username")), "+")
	if !validTokenPath(hash) {
		http.NotFound(w, r)
		return
	}
	appName := brand.DefaultAppName
	h.serveLanding(w, landingPage{
		Title:        appName + ": Join Chat",
		CanonicalURL: h.publicURL("/+" + hash),
		AppURL:       h.appURL("join", "invite", hash),
		KindLabel:    "Invite Link",
		PageTitle:    "Join chat",
		Description:  "Open this invite link in " + appName + " to view or join the chat.",
		ActionText:   "Open in " + appName,
		Icon:         "invite",
		PathFull:     "/+" + hash,
	})
}

func (h *handler) username(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.PathValue("username"))
	if strings.HasPrefix(username, "+") {
		h.joinInvite(w, r)
		return
	}
	if !validUsernamePath(username) {
		http.NotFound(w, r)
		return
	}
	appName := brand.DefaultAppName
	h.serveLanding(w, landingPage{
		Title:        appName + ": View @" + username,
		CanonicalURL: h.publicURL("/" + username),
		AppURL:       h.appURL("resolve", "domain", username),
		KindLabel:    appName + " Profile",
		PageTitle:    "@" + username,
		Extra:        "Open profile or channel",
		Description:  "If you have " + appName + ", you can open this link right away.",
		ActionText:   "View in " + appName,
		Icon:         "profile",
		PathFull:     "/" + username,
	})
}

func (h *handler) publicMessage(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.PathValue("username"))
	messageID, ok := positivePathInt(r.PathValue("messageID"))
	if !validUsernamePath(username) || !ok {
		http.NotFound(w, r)
		return
	}
	appName := brand.DefaultAppName
	h.serveLanding(w, landingPage{
		Title:        appName + ": View Message",
		CanonicalURL: h.publicURL("/" + username + "/" + strconv.Itoa(messageID)),
		AppURL:       h.appURL("resolve", "domain", username, "post", strconv.Itoa(messageID)),
		KindLabel:    appName + " Message",
		PageTitle:    "@" + username,
		Extra:        "Message #" + strconv.Itoa(messageID),
		Description:  "Open this message in " + appName + ".",
		ActionText:   "View in " + appName,
		Icon:         "message",
		PathFull:     "/" + username + "/" + strconv.Itoa(messageID),
	})
}

func (h *handler) privateMessage(w http.ResponseWriter, r *http.Request) {
	channelID, channelOK := positivePathInt(r.PathValue("channelID"))
	messageID, messageOK := positivePathInt(r.PathValue("messageID"))
	if !channelOK || !messageOK {
		http.NotFound(w, r)
		return
	}
	channel := strconv.Itoa(channelID)
	post := strconv.Itoa(messageID)
	appName := brand.DefaultAppName
	h.serveLanding(w, landingPage{
		Title:        appName + ": View Message",
		CanonicalURL: h.publicURL("/c/" + channel + "/" + post),
		AppURL:       h.appURL("privatepost", "channel", channel, "post", post),
		KindLabel:    appName + " Message",
		PageTitle:    "Private channel message",
		Extra:        "Message #" + post,
		Description:  "Open this message in " + appName + ".",
		ActionText:   "View in " + appName,
		Icon:         "message",
		PathFull:     "/c/" + channel + "/" + post,
	})
}

func (h *handler) call(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.PathValue("slug"))
	if !validTokenPath(slug) {
		http.NotFound(w, r)
		return
	}
	appName := brand.DefaultAppName
	h.serveLanding(w, landingPage{
		Title:        appName + ": Voice Chat",
		CanonicalURL: h.publicURL("/call/" + slug),
		AppURL:       h.appURL("call", "slug", slug),
		KindLabel:    appName + " Call",
		PageTitle:    "Join voice chat",
		Description:  "Open this call link in " + appName + ".",
		ActionText:   "Open in " + appName,
		Icon:         "call",
		PathFull:     "/call/" + slug,
	})
}

func (h *handler) businessChat(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.PathValue("slug"))
	if !validTokenPath(slug) {
		http.NotFound(w, r)
		return
	}
	appName := brand.DefaultAppName
	h.serveLanding(w, landingPage{
		Title:        appName + ": Chat Link",
		CanonicalURL: h.publicURL("/m/" + slug),
		AppURL:       h.appURL("message", "slug", slug),
		KindLabel:    appName + " Chat Link",
		PageTitle:    "Start chat",
		Description:  "Open this business chat link in " + appName + ".",
		ActionText:   "Open in " + appName,
		Icon:         "message",
		PathFull:     "/m/" + slug,
	})
}

func (h *handler) aiStyle(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.PathValue("slug"))
	if !validTokenPath(slug) {
		http.NotFound(w, r)
		return
	}
	appName := brand.DefaultAppName
	h.serveLanding(w, landingPage{
		Title:        appName + ": Add Style",
		CanonicalURL: h.publicURL("/addstyle/" + slug),
		AppURL:       h.appURL("addstyle", "slug", slug),
		KindLabel:    appName + " AI Style",
		PageTitle:    "Add writing style",
		Description:  "Open this AI writing style in " + appName + ".",
		ActionText:   "Open in " + appName,
		Icon:         "style",
		PathFull:     "/addstyle/" + slug,
	})
}

func (h *handler) addList(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.PathValue("slug"))
	if !validSlugPath(slug) {
		http.NotFound(w, r)
		return
	}
	appName := brand.DefaultAppName
	h.serveLanding(w, landingPage{
		Title:        appName + ": Shared Folder",
		CanonicalURL: h.publicURL("/addlist/" + url.PathEscape(slug)),
		AppURL:       h.appURL("addlist", "slug", slug),
		KindLabel:    appName + " Shared Folder",
		PageTitle:    "Shared Folder",
		Extra:        slug,
		Description:  "Open this shared folder in " + appName + " to preview and add it.",
		ActionText:   "Open in " + appName,
		Icon:         "folder",
		PathFull:     "/addlist/" + slug,
	})
}

func (h *handler) serveSet(w http.ResponseWriter, r *http.Request, pathKind string) {
	shortName := strings.TrimSpace(r.PathValue("shortName"))
	if !validShortNamePath(shortName) {
		http.NotFound(w, r)
		return
	}
	set, docs, found, err := h.resolver.ResolveStickerSet(r.Context(), domain.StickerSetRef{
		Kind:      domain.StickerSetRefByShortName,
		ShortName: shortName,
	})
	if err != nil {
		http.Error(w, "sticker set lookup failed", http.StatusInternalServerError)
		return
	}
	if !found || set.Deleted {
		http.NotFound(w, r)
		return
	}
	canonicalKind := linkKind(set)
	if canonicalKind != pathKind {
		http.Redirect(w, r, h.setURL(canonicalKind, set.ShortName), http.StatusPermanentRedirect)
		return
	}
	count := set.Count
	if count == 0 {
		count = len(docs)
	}
	data := pageData{
		Title:        fallbackTitle(set),
		ShortName:    set.ShortName,
		Count:        count,
		KindLabel:    kindLabel(set),
		ItemNoun:     itemNoun(set, count),
		CanonicalURL: h.setURL(canonicalKind, set.ShortName),
		AppURL:       h.appURL(canonicalKind, "set", set.ShortName),
		ActionText:   stickerActionText(set),
		PathFull:     "/" + canonicalKind + "/" + set.ShortName,
	}
	h.serveStickerLanding(w, data)
}

func (h *handler) setURL(kind, shortName string) string {
	return h.publicBaseURL + "/" + kind + "/" + url.PathEscape(shortName)
}

func (h *handler) publicURL(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return h.publicBaseURL + path
}

func (h *handler) appURL(command string, params ...string) string {
	if command == "" {
		return h.appScheme + "://"
	}
	u := url.URL{Scheme: h.appScheme, Host: command}
	if len(params) > 0 {
		q := u.Query()
		for i := 0; i+1 < len(params); i += 2 {
			q.Set(params[i], params[i+1])
		}
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func (h *handler) serveStickerLanding(w http.ResponseWriter, data pageData) {
	appName := brand.DefaultAppName
	h.serveLanding(w, landingPage{
		Title:        data.Title + " - " + appName,
		CanonicalURL: data.CanonicalURL,
		AppURL:       data.AppURL,
		KindLabel:    data.KindLabel,
		PageTitle:    data.Title,
		Extra:        "@" + data.ShortName + " - " + strconv.Itoa(data.Count) + " " + data.ItemNoun,
		Description:  "A " + appName + " user has created the " + data.Title + " " + data.KindLabel + ".",
		ActionText:   data.ActionText,
		Icon:         "stickers",
		PathFull:     data.PathFull,
	})
}

func (h *handler) serveLanding(w http.ResponseWriter, data landingPage) {
	if data.Title == "" {
		data.Title = brand.DefaultAppName
	}
	if data.CanonicalURL == "" {
		data.CanonicalURL = h.publicBaseURL + "/"
	}
	if data.AppURL == "" {
		data.AppURL = h.appURL("")
	}
	if data.ActionText == "" {
		data.ActionText = "Open in " + brand.DefaultAppName
	}
	if data.SiteName == "" {
		data.SiteName = brand.DefaultAppName
	}
	if data.SiteInitial == "" {
		data.SiteInitial = brandInitial(data.SiteName)
	}
	if data.Icon == "" {
		data.Icon = "paper"
	}
	data.AppURLJS = template.JS(strconv.Quote(data.AppURL))
	data.AppURLAttr = template.URL(data.AppURL)
	data.PathFullJS = template.JS(strconv.Quote(data.PathFull))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	if err := landingTemplate.Execute(w, data); err != nil {
		http.Error(w, "render public link page failed", http.StatusInternalServerError)
	}
}

func normalizePublicBaseURL(raw string) string {
	u, err := url.Parse(links.NormalizeBaseURL(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return brand.DefaultPublicBaseURL
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/")
}

func normalizeAppScheme(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return brand.DefaultAppScheme
	}
	raw = strings.TrimSuffix(raw, "://")
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '+' || r == '-' || r == '.':
		default:
			return brand.DefaultAppScheme
		}
	}
	return strings.ToLower(raw)
}

func brandInitial(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "S"
	}
	for _, r := range name {
		return strings.ToUpper(string(r))
	}
	return "S"
}

func validShortNamePath(shortName string) bool {
	if shortName == "" || len(shortName) > 64 {
		return false
	}
	for _, r := range shortName {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '.':
		default:
			return false
		}
	}
	return true
}

func validUsernamePath(username string) bool {
	if username == "" || len(username) > 64 {
		return false
	}
	for _, r := range username {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return false
		}
	}
	return true
}

func validTokenPath(token string) bool {
	if token == "" || len(token) > 256 {
		return false
	}
	for _, r := range token {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.':
		default:
			return false
		}
	}
	return true
}

func positivePathInt(raw string) (int, bool) {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

func validSlugPath(slug string) bool {
	return links.ValidChatlistSlug(slug)
}

func linkKind(set domain.StickerSet) string {
	if set.Kind == domain.StickerSetKindEmoji || set.Emojis {
		return "addemoji"
	}
	return "addstickers"
}

func fallbackTitle(set domain.StickerSet) string {
	if title := strings.TrimSpace(set.Title); title != "" {
		return title
	}
	return set.ShortName
}

func kindLabel(set domain.StickerSet) string {
	switch {
	case set.Kind == domain.StickerSetKindEmoji || set.Emojis:
		return "custom emoji set"
	case set.Kind == domain.StickerSetKindMasks || set.Masks:
		return "mask set"
	default:
		return "sticker set"
	}
}

func itemNoun(set domain.StickerSet, count int) string {
	if set.Kind == domain.StickerSetKindEmoji || set.Emojis {
		if count == 1 {
			return "custom emoji"
		}
		return "custom emoji"
	}
	if count == 1 {
		return "sticker"
	}
	return "stickers"
}

func stickerActionText(set domain.StickerSet) string {
	if set.Kind == domain.StickerSetKindEmoji || set.Emojis {
		return "Add Emoji"
	}
	return "Add Stickers"
}

type pageData struct {
	Title        string
	ShortName    string
	Count        int
	KindLabel    string
	ItemNoun     string
	CanonicalURL string
	AppURL       string
	ActionText   string
	PathFull     string
}

type landingPage struct {
	SiteName     string
	SiteInitial  string
	Title        string
	CanonicalURL string
	AppURL       string
	KindLabel    string
	PageTitle    string
	Extra        string
	Description  string
	ActionText   string
	Icon         string
	PathFull     string
	AppURLJS     template.JS
	AppURLAttr   template.URL
	PathFullJS   template.JS
}

var landingTemplate = template.Must(template.New("landing").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <link rel="canonical" href="{{.CanonicalURL}}">
  <meta property="og:title" content="{{.PageTitle}}">
  <meta property="og:site_name" content="{{.SiteName}}">
  <meta property="og:description" content="{{.Description}}">
  <meta property="og:url" content="{{.CanonicalURL}}">
  <meta name="twitter:card" content="summary">
  <meta name="twitter:title" content="{{.PageTitle}}">
  <meta name="twitter:description" content="{{.Description}}">
  <meta property="al:ios:app_name" content="{{.SiteName}}">
  <meta property="al:ios:url" content="{{.AppURL}}">
  <meta property="al:android:app_name" content="{{.SiteName}}">
  <meta property="al:android:url" content="{{.AppURL}}">
  <meta name="apple-itunes-app" content="app-argument: {{.AppURL}}">
  <meta name="robots" content="noindex">
  <style>
    :root { color-scheme: light dark; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; --accent: #3390ec; --text: #17212b; --muted: #6f7f8f; --card: #fff; --line: #dfe7ef; --bg1: #d9dfb8; --bg2: #6da98a; }
    * { box-sizing: border-box; }
    body { margin: 0; min-height: 100vh; color: var(--text); background: linear-gradient(135deg, var(--bg1), var(--bg2)); overflow-x: hidden; }
    body:before { content: ""; position: fixed; inset: 0; opacity: .22; background-image: radial-gradient(circle at 12px 12px, rgba(255,255,255,.55) 1px, transparent 1.5px); background-size: 28px 28px; }
    .wrap { position: relative; min-height: 100vh; display: flex; flex-direction: column; }
    header { height: 54px; display: flex; align-items: center; justify-content: space-between; width: min(100%, 930px); margin: 0 auto; padding: 0 18px; }
    .brand { display: inline-flex; align-items: center; gap: 9px; color: #fff; text-decoration: none; font-size: 20px; font-weight: 700; text-shadow: 0 1px 1px rgba(0,0,0,.12); }
    .brand-mark, .page-icon { display: inline-grid; place-items: center; border-radius: 50%; background: var(--accent); color: #fff; box-shadow: 0 8px 24px rgba(0,0,0,.15); }
    .brand-mark { width: 34px; height: 34px; font-size: 18px; }
    .download { color: #fff; text-decoration: none; font-weight: 600; opacity: .95; }
    main { flex: 1; display: grid; place-items: center; padding: 26px 14px 54px; }
    .page { width: min(100%, 420px); text-align: center; padding: 34px 28px 30px; border: 1px solid rgba(255,255,255,.56); border-radius: 4px; background: rgba(255,255,255,.94); box-shadow: 0 16px 42px rgba(31, 55, 75, .18); backdrop-filter: blur(8px); }
    .page-icon { width: 86px; height: 86px; margin: 0 auto 21px; font-size: 42px; }
    .kind { margin: 0 0 8px; color: var(--muted); font-size: 14px; }
    h1 { margin: 0; font-size: 26px; line-height: 1.22; font-weight: 700; overflow-wrap: anywhere; }
    .extra { margin: 8px 0 0; color: var(--muted); font-size: 15px; overflow-wrap: anywhere; }
    .desc { margin: 20px 0 24px; color: #445569; font-size: 16px; line-height: 1.45; }
    .button { display: inline-flex; align-items: center; justify-content: center; min-height: 44px; min-width: 164px; padding: 0 20px; border-radius: 22px; background: var(--accent); color: #fff; text-decoration: none; font-weight: 700; box-shadow: 0 4px 12px rgba(51,144,236,.32); }
    .raw { display: block; margin-top: 22px; color: var(--accent); font-size: 13px; overflow-wrap: anywhere; text-decoration: none; }
    @media (prefers-color-scheme: dark) {
      :root { --text: #eef4fb; --muted: #9dafbf; --card: #17212b; --line: #2b3a48; --bg1: #1e2f35; --bg2: #274b45; }
      .page { background: rgba(23,33,43,.94); border-color: rgba(255,255,255,.09); box-shadow: 0 16px 42px rgba(0,0,0,.26); }
      .desc { color: #c2ced9; }
    }
  </style>
</head>
<body>
  <div class="wrap">
    <header>
      <a class="brand" href="/"><span class="brand-mark">{{.SiteInitial}}</span><span>{{.SiteName}}</span></a>
      <a class="download" href="{{.AppURLAttr}}">Open</a>
    </header>
    <main>
      <section class="page">
        <div class="page-icon">{{if eq .Icon "stickers"}}S{{else if eq .Icon "invite"}}+{{else if eq .Icon "message"}}M{{else if eq .Icon "call"}}C{{else if eq .Icon "style"}}A{{else if eq .Icon "profile"}}@{{else if eq .Icon "folder"}}F{{else}}{{.SiteInitial}}{{end}}</div>
        <p class="kind">{{.KindLabel}}</p>
        <h1>{{.PageTitle}}</h1>
        {{if .Extra}}<p class="extra">{{.Extra}}</p>{{end}}
        <p class="desc">{{.Description}}</p>
        <a class="button" href="{{.AppURLAttr}}">{{.ActionText}}</a>
        <a class="raw" href="{{.CanonicalURL}}">{{.CanonicalURL}}</a>
      </section>
    </main>
  </div>
  <script>
    try {
      if (window.parent && window.parent !== window) {
        window.parent.postMessage(JSON.stringify({eventType: "web_app_open_safelink", eventData: {path_full: {{.PathFullJS}}}}), "*");
      }
    } catch (e) {}
    setTimeout(function () {
      window.location.href = {{.AppURLJS}};
    }, 100);
  </script>
</body>
</html>
`))
