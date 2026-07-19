// Package web serves SafeLink's read-only public link landing pages.
package web

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"

	"telesrv/internal/brand"
	"telesrv/internal/domain"
	"telesrv/internal/links"
)

//go:embed assets/safelink-home-devices.png
var homeDevicesImage []byte

//go:embed assets/safelink-mark.png
var homeMarkImage []byte

type Config struct {
	Addr            string
	PublicBaseURL   string
	AppScheme       string
	WebBaseURL      string
	AppName         string
	StickerSets     StickerSetResolver
	Users           UsernameResolver
	Channels        PublicChannelResolver
	Privacy         AnonymousPrivacyResolver
	Photos          ProfilePhotoResolver
	UniqueGifts     UniqueStarGiftResolver
	GiftWithdrawals StarGiftWithdrawalResolver
}

type StickerSetResolver interface {
	ResolveStickerSet(ctx context.Context, ref domain.StickerSetRef) (domain.StickerSet, []domain.Document, bool, error)
}

type UsernameResolver interface {
	ByUsername(ctx context.Context, username string) (domain.User, bool, error)
}

// PublicChannelResolver exposes only the viewer-independent public username
// projection. viewerUserID is always zero for this anonymous Web endpoint.
type PublicChannelResolver interface {
	ResolvePublicChannelUsername(ctx context.Context, viewerUserID int64, username string) (domain.Channel, bool, error)
}

type AnonymousPrivacyResolver interface {
	CanSeeAnonymous(ctx context.Context, ownerUserID int64, key domain.PrivacyKey) (bool, error)
}

type ProfilePhotoResolver interface {
	CurrentProfilePhotoKind(ctx context.Context, ownerType domain.PeerType, ownerID int64, kind domain.ProfilePhotoKind) (domain.Photo, bool, error)
	GetPhoto(ctx context.Context, id int64) (domain.Photo, bool, error)
	GetFile(ctx context.Context, req domain.FileDownloadRequest) (domain.FileChunk, bool, error)
}

type UniqueStarGiftResolver interface {
	UniqueBySlug(ctx context.Context, slug string) (domain.UniqueStarGift, bool, error)
}

type StarGiftWithdrawalResolver interface {
	ResolveWithdrawal(ctx context.Context, providerRequestID string) (domain.StarGiftWithdrawal, bool, error)
	CompleteWithdrawal(ctx context.Context, providerRequestID string, date int) (domain.StarGiftWithdrawal, error)
}

func Start(ctx context.Context, cfg Config, logger *zap.Logger) (*http.Server, error) {
	addr := strings.TrimSpace(cfg.Addr)
	if addr == "" {
		return nil, nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	handler, err := newHandler(cfg, logger)
	if err != nil {
		return nil, err
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	go func() {
		logger.Info("Public link Web endpoint enabled",
			zap.String("addr", addr),
			zap.String("public_base_url", cfg.PublicBaseURL),
			zap.String("app_scheme", cfg.AppScheme),
			zap.String("web_base_url", cfg.WebBaseURL))
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

func NewHandler(cfg Config) (http.Handler, error) {
	return newHandler(cfg, zap.NewNop())
}

func newHandler(cfg Config, logger *zap.Logger) (http.Handler, error) {
	var err error
	if cfg.StickerSets == nil {
		return nil, fmt.Errorf("public Web sticker set resolver is nil")
	}
	if strings.TrimSpace(cfg.WebBaseURL) == "" {
		cfg.WebBaseURL = links.DefaultWebBaseURL
	}
	if strings.TrimSpace(cfg.AppName) == "" {
		cfg.AppName = links.DefaultAppName
	}
	if cfg.PublicBaseURL, err = links.ValidateBaseURL(cfg.PublicBaseURL); err != nil {
		return nil, fmt.Errorf("public base URL: %w", err)
	}
	if cfg.AppScheme, err = links.ValidateAppScheme(cfg.AppScheme); err != nil {
		return nil, fmt.Errorf("app scheme: %w", err)
	}
	if cfg.WebBaseURL, err = links.ValidateBaseURL(cfg.WebBaseURL); err != nil {
		return nil, fmt.Errorf("Web base URL: %w", err)
	}
	if cfg.AppName, err = links.ValidateAppName(cfg.AppName); err != nil {
		return nil, fmt.Errorf("app name: %w", err)
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	h := &handler{
		stickerSets:      cfg.StickerSets,
		users:            cfg.Users,
		channels:         cfg.Channels,
		anonymousPrivacy: cfg.Privacy,
		photos:           cfg.Photos,
		uniqueGifts:      cfg.UniqueGifts,
		giftWithdrawals:  cfg.GiftWithdrawals,
		publicBaseURL:    cfg.PublicBaseURL,
		appScheme:        cfg.AppScheme,
		webBaseURL:       cfg.WebBaseURL,
		appName:          cfg.AppName,
		logger:           logger,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("GET /assets/safelink-home-devices.png", h.homeDevicesAsset)
	mux.HandleFunc("GET /assets/safelink-mark.png", h.homeMarkAsset)
	mux.HandleFunc("GET /_public/avatar/{username}/{photoID}", h.publicAvatar)
	mux.HandleFunc("GET /", h.root)
	mux.HandleFunc("GET /faq", h.faq)
	mux.HandleFunc("GET /apps", h.apps)
	mux.HandleFunc("GET /api", h.api)
	mux.HandleFunc("GET /safety", h.safety)
	mux.HandleFunc("GET /updates", h.updates)
	mux.HandleFunc("GET /blog", h.updates)
	mux.HandleFunc("GET /links", h.linksPage)
	mux.HandleFunc("GET /privacy", h.privacy)
	mux.HandleFunc("GET /press", h.press)
	mux.HandleFunc("GET /support", h.support)
	mux.HandleFunc("GET /terms", h.terms)
	mux.HandleFunc("GET /tos", h.terms)
	mux.HandleFunc("GET /translations", h.translations)
	mux.HandleFunc("GET /instantview", h.instantView)
	mux.HandleFunc("GET /addstickers/{shortName}", h.addStickers)
	mux.HandleFunc("GET /addemoji/{shortName}", h.addEmoji)
	mux.HandleFunc("GET /addlist/{slug}", h.addList)
	mux.HandleFunc("GET /nft/{slug}", h.uniqueGift)
	mux.HandleFunc("GET /nft/{slug}/{$}", h.uniqueGift)
	mux.HandleFunc("GET /gift-withdrawal/{requestID}", h.starGiftWithdrawal)
	mux.HandleFunc("POST /gift-withdrawal/{requestID}", h.completeStarGiftWithdrawal)
	mux.HandleFunc("GET /c/{channelID}/{messageID}", h.privateMessage)
	mux.HandleFunc("GET /call/{slug}", h.call)
	mux.HandleFunc("GET /m/{slug}", h.businessChat)
	mux.HandleFunc("GET /addstyle/{slug}", h.aiStyle)
	mux.HandleFunc("GET /{username}/{messageID}", h.publicMessage)
	mux.HandleFunc("GET /{username}/{$}", h.username)
	mux.HandleFunc("GET /{username}", h.username)
	return publicSecurityHeaders(mux), nil
}

type handler struct {
	stickerSets      StickerSetResolver
	users            UsernameResolver
	channels         PublicChannelResolver
	anonymousPrivacy AnonymousPrivacyResolver
	photos           ProfilePhotoResolver
	uniqueGifts      UniqueStarGiftResolver
	giftWithdrawals  StarGiftWithdrawalResolver
	publicBaseURL    string
	appScheme        string
	webBaseURL       string
	appName          string
	logger           *zap.Logger
}

type starGiftWithdrawalPage struct {
	AppName      string
	Title        string
	Slug         string
	Status       string
	OwnerAddress string
	GiftAddress  string
	ExpiresAt    string
	CanComplete  bool
}

var starGiftWithdrawalTemplate = template.Must(template.New("star-gift-withdrawal").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}} · {{.AppName}}</title><style>
body{font:16px/1.5 system-ui,sans-serif;background:#f4f6f8;color:#17212b;margin:0;padding:32px}.card{max-width:560px;margin:8vh auto;background:#fff;border-radius:16px;padding:28px;box-shadow:0 8px 32px #0002}h1{margin-top:0}.meta{overflow-wrap:anywhere;color:#53606d}button{border:0;border-radius:10px;padding:12px 18px;background:#2481cc;color:#fff;font-weight:600;cursor:pointer}.done{color:#18864b;font-weight:600}
</style></head><body><main class="card"><h1>{{.Title}}</h1><p class="meta">Collectible: {{.Slug}}</p>
{{if .CanComplete}}<p>This export is handled only by {{.AppName}}'s internal ledger. No external blockchain or wallet is contacted.</p><form method="post"><button type="submit">Complete local export</button></form><p class="meta">Expires: {{.ExpiresAt}}</p>{{else}}<p class="done">Status: {{.Status}}</p>{{if .OwnerAddress}}<p class="meta">Owner address: {{.OwnerAddress}}</p><p class="meta">Gift address: {{.GiftAddress}}</p>{{end}}{{end}}
</main></body></html>`))

func (h *handler) starGiftWithdrawal(w http.ResponseWriter, r *http.Request) {
	h.renderStarGiftWithdrawal(w, r, false)
}

func (h *handler) completeStarGiftWithdrawal(w http.ResponseWriter, r *http.Request) {
	h.renderStarGiftWithdrawal(w, r, true)
}

func (h *handler) renderStarGiftWithdrawal(w http.ResponseWriter, r *http.Request, complete bool) {
	requestID := strings.TrimSpace(r.PathValue("requestID"))
	if h.giftWithdrawals == nil || requestID == "" || len(requestID) > 256 {
		http.NotFound(w, r)
		return
	}
	var withdrawal domain.StarGiftWithdrawal
	var found bool
	var err error
	if complete {
		withdrawal, err = h.giftWithdrawals.CompleteWithdrawal(r.Context(), requestID, int(time.Now().Unix()))
		found = err == nil
	} else {
		withdrawal, found, err = h.giftWithdrawals.ResolveWithdrawal(r.Context(), requestID)
	}
	if err != nil || !found {
		http.NotFound(w, r)
		return
	}
	page := starGiftWithdrawalPage{AppName: h.appName, Title: withdrawal.Gift.Title, Slug: withdrawal.Gift.Slug,
		Status: withdrawal.Status, OwnerAddress: withdrawal.Gift.OwnerAddress, GiftAddress: withdrawal.Gift.GiftAddress,
		ExpiresAt:   time.Unix(int64(withdrawal.ExpiresAt), 0).UTC().Format(time.RFC3339),
		CanComplete: withdrawal.Status == "pending" && withdrawal.ExpiresAt > int(time.Now().Unix())}
	if page.Title == "" {
		page.Title = "Collectible gift export"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := starGiftWithdrawalTemplate.Execute(w, page); err != nil {
		h.logger.Warn("render star gift withdrawal", zap.Error(err))
	}
}

func (h *handler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

func (h *handler) homeDevicesAsset(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	http.ServeContent(w, r, "safelink-home-devices.png", time.Time{}, bytes.NewReader(homeDevicesImage))
}

func (h *handler) homeMarkAsset(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	http.ServeContent(w, r, "safelink-mark.png", time.Time{}, bytes.NewReader(homeMarkImage))
}

func (h *handler) root(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	h.serveHome(w, r, homePage{
		AppName:      brand.DefaultAppName,
		CanonicalURL: h.canonicalURL(r, "/"),
		AppURL:       h.appURL(""),
		WebURL:       "https://web.safelink.chat/",
		DevicesImage: "/assets/safelink-home-devices.png",
		MarkImage:    "/assets/safelink-mark.png",
	})
}

func (h *handler) faq(w http.ResponseWriter, r *http.Request) {
	h.serveSitePage(w, r, sitePage{
		Active:      "faq",
		Kicker:      "FAQ",
		Heading:     "常见问题",
		Intro:       "这里整理 SafeLink 测试阶段最常被问到的问题：账号、客户端、链接、服务端和数据边界。",
		PrimaryText: "打开网页版",
		PrimaryURL:  template.URL("https://web.safelink.chat/"),
		Sections: []siteSection{
			{Title: "基础", Items: []siteItem{
				{Title: "SafeLink 是什么？", Body: "SafeLink 是一套自有服务端和多端客户端体验，目标是在自己的域名与实例下提供熟悉、快速、同步的即时通讯。"},
				{Title: "为什么页面主中文？", Body: "当前实例主要面向中文测试用户，所以官网、说明和链接页默认使用中文，必要位置保留英文短标签方便客户端和开发者识别。"},
				{Title: "测试阶段可以直接注册吗？", Body: "可以按当前客户端支持的登录方式测试。邮箱验证码、设备信息、二次验证和多端登录仍会持续优化。"},
			}},
			{Title: "链接与客户端", Items: []siteItem{
				{Title: "为什么要用 safelink.chat？", Body: "公开邀请、用户名、贴纸、表情和共享文件夹都应该使用 SafeLink 域名，避免把用户带到外部默认入口。"},
				{Title: "safelink:// 做什么？", Body: "网页落地页会生成 safelink:// 深链，用来直接唤起 SafeLink 客户端并进入对应聊天、资料、贴纸或共享文件夹。"},
				{Title: "没有安装客户端怎么办？", Body: "用户可以先打开网页版，或从客户端页面下载对应平台的安装包。正式包地址接入后会在 Apps 页面统一展示。"},
			}},
			{Title: "部署与数据", Items: []siteItem{
				{Title: "服务端是否支持私有实例？", Body: "目标就是让实例使用自己的域名、密钥、服务端和客户端配置。生产部署时需要确保公开链接服务、WebSocket、MTProto 与 Nginx 路由一致。"},
				{Title: "媒体和大文件会走哪里？", Body: "媒体、图片、视频和文件由 SafeLink 服务端处理。客户端异常时优先检查服务端媒体路由、存储路径和客户端连接配置。"},
				{Title: "以后部署不能忘什么？", Body: "不能忘记公开链接路由、safelink:// scheme、公钥、客户端 DC 配置、Web 入口和邮件验证码配置。部署说明会继续写入 README 或部署文档。"},
			}},
		},
	})
}

func (h *handler) apps(w http.ResponseWriter, r *http.Request) {
	h.serveSitePage(w, r, sitePage{
		Active:        "apps",
		Kicker:        "Apps",
		Heading:       "SafeLink 客户端",
		Intro:         "移动端、桌面端和网页版都要指向同一个 SafeLink 实例。当前先使用网页版地址：https://web.safelink.chat/，正式下载地址接入后，这里会成为统一入口。",
		PrimaryText:   "打开网页版",
		PrimaryURL:    template.URL("https://web.safelink.chat/"),
		SecondaryText: "查看链接规则",
		SecondaryURL:  template.URL("/links"),
		Sections: []siteSection{
			{Title: "移动端", Items: []siteItem{
				{Title: "iOS", Body: "面向 iPhone 与 iPad。需要配置 SafeLink 服务端地址、公钥和 safelink:// 链接唤起。", LinkText: "准备接入安装包", LinkURL: template.URL("/apps#ios")},
				{Title: "Android", Body: "面向手机和平板。需要确保 DC、RSA 公钥、域名前缀和 deep link 都是 SafeLink 配置。", LinkText: "准备接入安装包", LinkURL: template.URL("/apps#android")},
			}},
			{Title: "桌面端", Items: []siteItem{
				{Title: "macOS", Body: "面向 Apple Silicon 与 Intel。登录、收发消息、媒体下载和二次验证都要跟服务端兼容。", LinkText: "准备接入安装包", LinkURL: template.URL("/apps#macos")},
				{Title: "Windows", Body: "桌面端需要使用 SafeLink DC、公钥、域名和链接前缀，避免显示历史品牌。", LinkText: "准备接入安装包", LinkURL: template.URL("/apps#windows")},
				{Title: "Linux", Body: "Linux 桌面包跟随桌面端同一配置方向，正式产物接入后统一展示。", LinkText: "准备接入安装包", LinkURL: template.URL("/apps#linux")},
			}},
			{Title: "网页版", Items: []siteItem{
				{Title: "Web", Body: "浏览器入口用于测试和临时访问，当前网页版地址是 https://web.safelink.chat/。生产环境需要允许 safelink.chat 相关 WebSocket Origin。", LinkText: "打开 Web", LinkURL: template.URL("https://web.safelink.chat/")},
			}},
		},
	})
}

func (h *handler) api(w http.ResponseWriter, r *http.Request) {
	h.serveSitePage(w, r, sitePage{
		Active:        "api",
		Kicker:        "API",
		Heading:       "开发者与服务端接口",
		Intro:         "SafeLink 需要兼容真实客户端，也需要给机器人、后台和公开链接提供稳定边界。",
		PrimaryText:   "查看链接规则",
		PrimaryURL:    template.URL("/links"),
		SecondaryText: "打开后台域名",
		SecondaryURL:  template.URL("https://admin.hsgram.cloud/"),
		Sections: []siteSection{
			{Title: "客户端协议", Items: []siteItem{
				{Title: "MTProto 连接", Body: "客户端连接服务端时必须使用 SafeLink DC 地址、生产 RSA 公钥和正确端口。stock 客户端不会自动信任你的实例。"},
				{Title: "WebSocket", Body: "网页版需要服务端开启 WebSocket，并在 Origin 白名单里加入 safelink.chat 与 web.safelink.chat。"},
				{Title: "Deep Link", Body: "公开网页负责把 safelink.chat 链接转换为 safelink:// 深链，让安装了客户端的用户直接进入对应对象。"},
			}},
			{Title: "服务端能力", Items: []siteItem{
				{Title: "Bot API", Body: "服务端保留机器人网关方向，用于后续接入通知、自动化和业务机器人。"},
				{Title: "Admin API", Body: "后台管理接口用于账号、会话、消息和系统状态排查，生产域名为 admin.hsgram.cloud。"},
				{Title: "Public Link API", Body: "公开链接服务负责邀请、用户名、消息、贴纸、表情、共享文件夹和通话链接的网页落地页。"},
			}},
		},
	})
}

func (h *handler) safety(w http.ResponseWriter, r *http.Request) {
	h.serveSitePage(w, r, sitePage{
		Active:        "safety",
		Kicker:        "Safety",
		Heading:       "安全与隐私",
		Intro:         "SafeLink 的安全重点是自有实例、自有链接、自有客户端配置，以及可检查的登录和设备状态。",
		PrimaryText:   "查看 FAQ",
		PrimaryURL:    template.URL("/faq"),
		SecondaryText: "隐私说明",
		SecondaryURL:  template.URL("/privacy"),
		Sections: []siteSection{
			{Title: "账号安全", Items: []siteItem{
				{Title: "邮箱验证码", Body: "验证码邮件以 SafeLink 名称发送，测试阶段优先保证验证码长度、过期时间和错误提示清晰。"},
				{Title: "二次验证", Body: "二次密码流程需要客户端和服务端同时兼容。遇到卡住时优先看客户端日志和服务端 auth 过程。"},
				{Title: "设备管理", Body: "设备页应展示真实设备信息、真实 IP 和可解析地理位置；解析不到时再显示未知状态。"},
			}},
			{Title: "链接安全", Items: []siteItem{
				{Title: "域名一致", Body: "邀请、用户名、贴纸和共享文件夹都使用 safelink.chat，避免用户跳到外部默认入口。"},
				{Title: "客户端唤起", Body: "safelink:// scheme 必须由 SafeLink 客户端注册，网页只负责生成正确深链和兜底入口。"},
				{Title: "公开页面", Body: "公开落地页只展示必要说明，不暴露内部媒体下载路径或敏感服务端细节。"},
			}},
			{Title: "部署安全", Items: []siteItem{
				{Title: "公钥与 DC", Body: "每个客户端包都需要对应生产公钥和 DC 配置，否则登录、消息和媒体可能异常。"},
				{Title: "Nginx 路由", Body: "公开链接路由要排在 Web SPA fallback 前面，否则邀请链接会被网页版吞掉。"},
				{Title: "邮件与后台", Body: "SMTP、后台域名、后台口令和日志访问需要按生产环境独立管理。"},
			}},
		},
	})
}

func (h *handler) updates(w http.ResponseWriter, r *http.Request) {
	h.serveSitePage(w, r, sitePage{
		Active:        "updates",
		Kicker:        "Updates",
		Heading:       "更新日志",
		Intro:         "这里记录当前 SafeLink 实例最近接入的服务端、客户端和公开链接能力。",
		PrimaryText:   "查看客户端",
		PrimaryURL:    template.URL("/apps"),
		SecondaryText: "查看 FAQ",
		SecondaryURL:  template.URL("/faq"),
		Sections: []siteSection{
			{Title: "2026 年 7 月", Items: []siteItem{
				{Title: "官网首页与中文内容", Body: "www.safelink.chat 接入 SafeLink 蓝白官网首页，主中文，兼容桌面与移动端。"},
				{Title: "公开链接 SafeLink 化", Body: "邀请、用户主页、贴纸、表情和共享文件夹页面继续生成 safelink:// 深链。"},
				{Title: "设备与地理位置排查", Body: "设备页围绕真实设备 IP、地理位置显示和未知状态做了服务端定位。"},
			}},
			{Title: "测试重点", Items: []siteItem{
				{Title: "邮箱验证码", Body: "验证码发送、文案品牌、五位/六位验证逻辑和二次密码流程继续测试。"},
				{Title: "媒体收发", Body: "图片、视频和文件下载持续验证，重点看服务端媒体路由和客户端缓存。"},
				{Title: "多端客户端", Body: "iOS、Android、macOS、Windows、Linux 和 Web 都要使用同一套 SafeLink 域名与公钥。"},
			}},
		},
	})
}

func (h *handler) linksPage(w http.ResponseWriter, r *http.Request) {
	h.serveSitePage(w, r, sitePage{
		Active:        "links",
		Kicker:        "Links",
		Heading:       "SafeLink 链接规则",
		Intro:         "所有公开入口都使用 safelink.chat，所有客户端唤起都使用 safelink://。",
		PrimaryText:   "打开 SafeLink",
		PrimaryURL:    template.URL(h.appURL("")),
		SecondaryText: "打开网页版",
		SecondaryURL:  template.URL("https://web.safelink.chat/"),
		Sections: []siteSection{
			{Title: "公开网页链接", Items: []siteItem{
				{Title: "邀请链接", Body: "https://safelink.chat/+invite_hash 用于加入群组或频道。"},
				{Title: "用户名", Body: "https://safelink.chat/username 用于打开用户、群组或频道主页。"},
				{Title: "消息链接", Body: "https://safelink.chat/username/message_id 与 /c/channel_id/message_id 用于打开公开或私有频道消息。"},
				{Title: "贴纸与表情", Body: "https://safelink.chat/addstickers/name 与 /addemoji/name 用于安装资源包。"},
				{Title: "共享文件夹", Body: "https://safelink.chat/addlist/slug 用于预览并导入共享文件夹。"},
				{Title: "通话与业务链接", Body: "https://safelink.chat/call/slug 与 /m/slug 用于打开通话或会话入口。"},
			}},
			{Title: "客户端深链", Items: []siteItem{
				{Title: "加入聊天", Body: "safelink://join?invite=invite_hash"},
				{Title: "打开用户名", Body: "safelink://resolve?domain=username"},
				{Title: "打开消息", Body: "safelink://resolve?domain=username&post=message_id"},
				{Title: "安装贴纸", Body: "safelink://addstickers?set=name"},
				{Title: "安装表情", Body: "safelink://addemoji?set=name"},
				{Title: "导入文件夹", Body: "safelink://addlist?slug=slug"},
			}},
		},
	})
}

func (h *handler) privacy(w http.ResponseWriter, r *http.Request) {
	h.serveSitePage(w, r, sitePage{
		Active:        "privacy",
		Kicker:        "Privacy",
		Heading:       "隐私说明",
		Intro:         "测试阶段的隐私原则很简单：只收集运行实例、登录、消息和排障所必需的数据。",
		PrimaryText:   "查看安全说明",
		PrimaryURL:    template.URL("/safety"),
		SecondaryText: "查看 FAQ",
		SecondaryURL:  template.URL("/faq"),
		Sections: []siteSection{
			{Title: "数据范围", Items: []siteItem{
				{Title: "账号与登录", Body: "登录需要手机号、邮箱验证码或客户端支持的验证信息。设备、IP 和会话状态用于安全提示和排障。"},
				{Title: "消息与媒体", Body: "消息、图片、视频和文件由当前 SafeLink 实例处理，存储策略跟随服务端部署配置。"},
				{Title: "日志", Body: "服务端日志用于定位登录、媒体、设备和链接问题。生产环境应避免写入不必要的敏感内容。"},
			}},
			{Title: "用户控制", Items: []siteItem{
				{Title: "会话管理", Body: "用户可以通过设备页查看并结束异常会话。"},
				{Title: "链接控制", Body: "群组、频道、贴纸、表情和共享文件夹链接由实例生成并使用 SafeLink 前缀。"},
				{Title: "部署控制", Body: "私有部署可以管理自己的域名、证书、服务端、公钥和客户端包。"},
			}},
		},
	})
}

func (h *handler) press(w http.ResponseWriter, r *http.Request) {
	h.serveSitePage(w, r, sitePage{
		Active:        "press",
		Kicker:        "Press",
		Heading:       "品牌与媒体资料",
		Intro:         "SafeLink 官网、客户端和邮件文案都应统一使用 SafeLink 名称与 safelink.chat 域名。",
		PrimaryText:   "查看更新",
		PrimaryURL:    template.URL("/updates"),
		SecondaryText: "打开首页",
		SecondaryURL:  template.URL("/"),
		Sections: []siteSection{
			{Title: "品牌用法", Items: []siteItem{
				{Title: "名称", Body: "产品名统一写作 SafeLink。历史品牌词、默认外部前缀和旧协议名不应出现在用户可见页面里。"},
				{Title: "域名", Body: "官网与公开链接使用 safelink.chat，后台测试域名按部署环境单独管理。"},
				{Title: "视觉", Body: "当前官网使用蓝白清爽色调，保留熟悉的信息结构，但不复制外部品牌资产。"},
			}},
			{Title: "对外说明", Items: []siteItem{
				{Title: "一句话", Body: "SafeLink 是面向自有实例的安全即时通讯体验，支持移动端、桌面端、网页版和自定义链接。"},
				{Title: "测试阶段", Body: "当前重点是服务端兼容、客户端登录、媒体收发、公开链接和品牌文案统一。"},
			}},
		},
	})
}

func (h *handler) support(w http.ResponseWriter, r *http.Request) {
	h.serveSitePage(w, r, sitePage{
		Active:        "support",
		Kicker:        "Support",
		Heading:       "帮助与支持",
		Intro:         "遇到登录、消息、媒体或链接问题时，优先按客户端版本、真实设备 IP、服务端日志和当前域名配置一起排查。",
		PrimaryText:   "查看 FAQ",
		PrimaryURL:    template.URL("/faq"),
		SecondaryText: "查看安全说明",
		SecondaryURL:  template.URL("/safety"),
		Sections: []siteSection{
			{Title: "常见排查", Items: []siteItem{
				{Title: "登录与验证码", Body: "确认手机号、邮箱验证码、二次密码和客户端版本是否匹配当前 SafeLink 服务端。验证码错误时需要同时看邮件发送记录和服务端验证记录。"},
				{Title: "消息收发", Body: "如果消息一直显示正在更新或正在刷新，先确认客户端 DC、公钥、WebSocket 与服务端地址都指向同一个实例。"},
				{Title: "媒体下载", Body: "图片、视频和文件下载失败时，重点看媒体存储、Nginx 转发、客户端缓存和服务端文件路由。"},
			}},
			{Title: "反馈需要带什么", Items: []siteItem{
				{Title: "设备信息", Body: "请记录系统版本、客户端平台、客户端版本、登录时间和真实外网 IP，避免用手机号代替设备定位。"},
				{Title: "链接样例", Body: "公开邀请、用户名、消息、贴纸、表情和共享文件夹问题都应附上 safelink.chat 链接，便于复现。"},
				{Title: "截图与日志", Body: "截图用于确认界面状态，日志用于定位服务端和客户端流程。敏感信息应在生产排障前做脱敏处理。"},
			}},
			{Title: "测试阶段说明", Items: []siteItem{
				{Title: "版本统一", Body: "iOS、Android、macOS、Windows、Linux 和 Web 的测试包需要同步服务端域名、公钥和链接前缀。"},
				{Title: "邮件通知", Body: "验证码、通知和后台邮件文案都应该使用 SafeLink 名称，避免出现历史服务名。"},
				{Title: "后台", Body: "管理后台仅用于测试和排障，入口、口令和日志访问需要按生产环境单独保护。"},
			}},
		},
	})
}

func (h *handler) terms(w http.ResponseWriter, r *http.Request) {
	h.serveSitePage(w, r, sitePage{
		Active:        "terms",
		Kicker:        "Terms",
		Heading:       "服务条款",
		Intro:         "这份测试阶段条款用于说明 SafeLink 实例、客户端和公开链接的使用边界，正式上线前可继续补充法律版本。",
		PrimaryText:   "查看隐私说明",
		PrimaryURL:    template.URL("/privacy"),
		SecondaryText: "查看支持",
		SecondaryURL:  template.URL("/support"),
		Sections: []siteSection{
			{Title: "使用边界", Items: []siteItem{
				{Title: "自有实例", Body: "SafeLink 面向自有服务端和自有客户端配置。使用者应确认域名、证书、公钥、客户端包和后台权限属于受控环境。"},
				{Title: "账号安全", Body: "用户需要保护自己的验证码、二次密码和登录设备。发现异常会话时应及时在设备页终止。"},
				{Title: "内容责任", Body: "群组、频道、消息、文件和公开链接由实例使用者管理，应遵守适用法律和测试环境规则。"},
			}},
			{Title: "服务状态", Items: []siteItem{
				{Title: "测试阶段", Body: "当前服务仍在验证登录、媒体、多端同步、链接唤起和后台能力，可能出现短时调整或重启。"},
				{Title: "变更", Body: "服务端、客户端、邮件配置、链接规则和公开页面可能随测试反馈更新，更新记录会写入官网或部署文档。"},
				{Title: "限制", Body: "如果账号、链接或客户端行为影响实例安全，管理员可以限制访问、撤销链接或要求重新验证。"},
			}},
		},
	})
}

func (h *handler) translations(w http.ResponseWriter, r *http.Request) {
	h.serveSitePage(w, r, sitePage{
		Active:        "translations",
		Kicker:        "Translations",
		Heading:       "本地化与翻译",
		Intro:         "SafeLink 官网主中文，客户端、邮件、后台和公开链接页也要统一文案，避免用户看到历史服务名或外部默认入口。",
		PrimaryText:   "查看品牌资料",
		PrimaryURL:    template.URL("/press"),
		SecondaryText: "查看支持",
		SecondaryURL:  template.URL("/support"),
		Sections: []siteSection{
			{Title: "中文优先", Items: []siteItem{
				{Title: "官网", Body: "首页、FAQ、Apps、API、Safety、更新、隐私和媒体资料默认中文表达，英文只作为短标签辅助识别。"},
				{Title: "客户端", Body: "登录、验证码、设备、二次密码、媒体下载和链接打开提示都应使用 SafeLink 统一文案。"},
				{Title: "邮件", Body: "验证码邮件标题、发件人显示名和正文都使用 SafeLink，测试 SMTP 配置也要同步检查。"},
			}},
			{Title: "术语表", Items: []siteItem{
				{Title: "SafeLink", Body: "产品名固定写作 SafeLink，不拆写、不替换为历史服务名。"},
				{Title: "公开链接", Body: "网页入口统一写作 safelink.chat，客户端唤起统一写作 safelink://。"},
				{Title: "网页版", Body: "浏览器入口写作 SafeLink 网页版，生产建议使用 web.safelink.chat。"},
			}},
			{Title: "后续接入", Items: []siteItem{
				{Title: "多语言", Body: "如果后续支持英文或其他语言，应保留中文为主站默认语言，并保证链接前缀和品牌名一致。"},
				{Title: "客户端资源", Body: "移动端和桌面端的本地化文件需要跟服务端邮件、官网和后台文案一起检查。"},
			}},
		},
	})
}

func (h *handler) instantView(w http.ResponseWriter, r *http.Request) {
	h.serveSitePage(w, r, sitePage{
		Active:        "instantview",
		Kicker:        "Instant View",
		Heading:       "链接预览与即时视图",
		Intro:         "SafeLink 的公开页面需要让用户先看清目标，再一键打开客户端；文章、资源和邀请页也要保留网页兜底。",
		PrimaryText:   "查看链接规则",
		PrimaryURL:    template.URL("/links"),
		SecondaryText: "打开网页版",
		SecondaryURL:  template.URL("https://web.safelink.chat/"),
		Sections: []siteSection{
			{Title: "公开预览", Items: []siteItem{
				{Title: "邀请预览", Body: "邀请链接打开后应展示 SafeLink 群组或频道的基础信息，并提供 safelink:// 加入深链和网页版兜底。"},
				{Title: "资源预览", Body: "贴纸、表情、共享文件夹和样式链接应在网页上说明内容类型，再跳转到 SafeLink 客户端。"},
				{Title: "消息预览", Body: "公开消息链接保留规范路径，页面应展示目标消息编号、归属对象和客户端打开入口。"},
			}},
			{Title: "分享元信息", Items: []siteItem{
				{Title: "标题", Body: "Open Graph 与浏览器标题使用 SafeLink 名称和目标对象，不暴露内部服务名。"},
				{Title: "描述", Body: "描述文案保持中文清晰，告诉用户这是 SafeLink 公开链接、邀请、资源或消息。"},
				{Title: "兜底", Body: "未安装客户端时，页面保留网页版和客户端下载入口，避免用户被导向外部默认应用。"},
			}},
		},
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
		if h.users != nil || h.channels != nil {
			h.serveUsernameNotFound(w, username)
			return
		}
		http.NotFound(w, r)
		return
	}
	if h.users != nil || h.channels != nil {
		h.usernameLink(w, r)
		return
	}
	if h.serveBotUsername(w, r, username) {
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

func (h *handler) serveBotUsername(w http.ResponseWriter, r *http.Request, username string) bool {
	if h.users == nil {
		return false
	}
	u, found, err := h.users.ByUsername(r.Context(), username)
	if err != nil {
		http.Error(w, "username lookup failed", http.StatusInternalServerError)
		return true
	}
	if !found || !u.Bot || strings.TrimSpace(u.Username) == "" {
		return false
	}
	appName := brand.DefaultAppName
	title := strings.TrimSpace(u.FirstName)
	if title == "" {
		title = u.Username
	}
	h.serveLanding(w, landingPage{
		Title:        appName + ": " + title,
		CanonicalURL: h.publicUsernameURL(u.Username),
		AppURL:       h.appURL("resolve", "domain", u.Username),
		KindLabel:    appName + " Bot",
		PageTitle:    title,
		Extra:        "@" + u.Username,
		Description:  "Open this bot in " + appName + " to start a chat.",
		ActionText:   "Open in " + appName,
		Icon:         "profile",
		PathFull:     "/" + u.Username,
	})
	return true
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
	appName := h.appName
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

func (h *handler) uniqueGift(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if h.uniqueGifts == nil || !validStarGiftSlugPath(slug) {
		http.NotFound(w, r)
		return
	}
	unique, found, err := h.uniqueGifts.UniqueBySlug(r.Context(), slug)
	if err != nil {
		h.logger.Error("Public unique star gift lookup failed", zap.String("slug", slug), zap.Error(err))
		http.Error(w, "collectible gift lookup failed", http.StatusInternalServerError)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	canonicalSlug := unique.Slug
	if unique.ID <= 0 || unique.GiftID <= 0 || unique.Num <= 0 ||
		!validStarGiftSlugPath(canonicalSlug) || !strings.EqualFold(slug, canonicalSlug) ||
		!utf8.ValidString(unique.Title) || utf8.RuneCountInString(unique.Title) > domain.MaxStarGiftTitleRunes {
		h.logger.Error("Public unique star gift resolver returned invalid aggregate",
			zap.String("requested_slug", slug), zap.String("resolved_slug", canonicalSlug),
			zap.Int64("unique_id", unique.ID), zap.Int64("gift_id", unique.GiftID), zap.Int("num", unique.Num))
		http.Error(w, "collectible gift lookup failed", http.StatusInternalServerError)
		return
	}
	if slug != canonicalSlug || strings.HasSuffix(r.URL.Path, "/") {
		http.Redirect(w, r, h.publicURL("/nft/"+url.PathEscape(canonicalSlug)), http.StatusPermanentRedirect)
		return
	}
	title := strings.TrimSpace(unique.Title)
	if title == "" {
		title = "Collectible gift"
	}
	subtitle := fmt.Sprintf("Collectible #%d", unique.Num)
	if unique.AvailabilityIssued > 0 && unique.AvailabilityTotal >= unique.AvailabilityIssued {
		subtitle += fmt.Sprintf(" · %s/%s issued", groupedDecimal(unique.AvailabilityIssued), groupedDecimal(unique.AvailabilityTotal))
	}
	h.serveLanding(w, landingPage{
		Title:        title + " - " + h.appName,
		CanonicalURL: h.publicURL("/nft/" + url.PathEscape(canonicalSlug)),
		AppURL:       h.appURL("nft", "slug", canonicalSlug),
		KindLabel:    h.appName + " Collectible",
		PageTitle:    title,
		Extra:        subtitle,
		Description:  "This collectible was created from a gift on " + h.appName + ". Open it in the app to view its current details.",
		ActionText:   "Open in " + h.appName,
		Icon:         "gift",
		PathFull:     "/nft/" + canonicalSlug,
	})
}

func (h *handler) usernameLink(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.PathValue("username"))
	if !validUsernamePath(username) {
		h.serveUsernameNotFound(w, username)
		return
	}
	params, ok := publicResolveQuery(r.URL.RawQuery)
	if !ok {
		http.Error(w, "public link query is too large or invalid", http.StatusRequestURITooLong)
		return
	}
	peer, found, err := h.resolvePublicPeer(r.Context(), username)
	if err != nil {
		h.logger.Error("Public username lookup failed", zap.String("username", username), zap.Error(err))
		http.Error(w, "username lookup failed", http.StatusInternalServerError)
		return
	}
	if !found {
		h.serveUsernameNotFound(w, username)
		return
	}
	params.Set("domain", peer.username)
	app := schemeURLValues(h.appScheme, "resolve", params)
	legacy := schemeURLValues("tg", "resolve", params)
	description := peer.about
	if description == "" {
		description = peer.fallbackDescription(h.appName)
	}
	data := usernamePageData{
		AppName:      h.appName,
		AppInitial:   appInitial(h.appName),
		Title:        peer.title,
		Username:     peer.username,
		Verified:     peer.verified,
		Extra:        peer.extra(),
		Description:  description,
		CanonicalURL: h.publicUsernameURL(peer.username),
		HomeURL:      h.publicBaseURL + "/",
		AppURL:       template.URL(app),
		LegacyTgURL:  template.URL(legacy),
		WebURL:       template.URL(publicWebAppURL(h.webBaseURL, legacy)),
		ButtonLabel:  peer.buttonLabel(),
		Initials:     peer.initials(),
	}
	if peer.hasPhoto {
		data.PhotoURL = h.publicAvatarURL(peer.username, peer.photo.ID)
	}
	data.AppURLJS = template.JS(strconv.Quote(app))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60, must-revalidate")
	if err := usernameLandingTemplate.Execute(w, data); err != nil {
		h.logger.Error("Render public username page failed", zap.String("username", peer.username), zap.Error(err))
	}
}

const maxPublicAvatarBytes = 4 << 20

func (h *handler) publicAvatar(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.PathValue("username"))
	photoID, err := strconv.ParseInt(r.PathValue("photoID"), 10, 64)
	if err != nil || photoID <= 0 || !validUsernamePath(username) || h.photos == nil {
		http.NotFound(w, r)
		return
	}
	peer, found, err := h.resolvePublicPeer(r.Context(), username)
	if err != nil {
		h.logger.Error("Public avatar peer lookup failed", zap.String("username", username), zap.Error(err))
		http.Error(w, "avatar lookup failed", http.StatusInternalServerError)
		return
	}
	if !found || !peer.hasPhoto || peer.photo.ID != photoID {
		http.NotFound(w, r)
		return
	}
	size, inline, ok := bestPublicPhotoSize(peer.photo.Sizes)
	if !ok {
		http.NotFound(w, r)
		return
	}
	etag := fmt.Sprintf("\"public-avatar-%d-%s-%d\"", peer.photo.ID, size.Type, size.Size)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=300, must-revalidate")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	data := inline
	mimeType := ""
	if len(data) == 0 {
		chunk, found, err := h.photos.GetFile(r.Context(), domain.FileDownloadRequest{
			LocationKey: fmt.Sprintf("photo:%d:%s", peer.photo.ID, size.Type),
			Limit:       maxPublicAvatarBytes + 1,
		})
		if err != nil {
			h.logger.Error("Read public avatar blob failed", zap.String("username", username), zap.Int64("photo_id", photoID), zap.Error(err))
			http.Error(w, "avatar read failed", http.StatusInternalServerError)
			return
		}
		if !found || chunk.Total <= 0 || chunk.Total > maxPublicAvatarBytes || int64(len(chunk.Bytes)) != chunk.Total {
			h.logger.Warn("Public avatar blob is missing or outside bounds", zap.String("username", username), zap.Int64("photo_id", photoID), zap.Int64("total", chunk.Total), zap.Int("bytes", len(chunk.Bytes)))
			http.NotFound(w, r)
			return
		}
		data = chunk.Bytes
		mimeType = chunk.MimeType
	}
	if len(data) == 0 || len(data) > maxPublicAvatarBytes {
		http.NotFound(w, r)
		return
	}
	detected := http.DetectContentType(data)
	if !safePublicImageType(detected) {
		h.logger.Warn("Public avatar blob is not a safe raster image", zap.String("username", username), zap.Int64("photo_id", photoID), zap.String("detected_type", detected), zap.String("stored_type", mimeType))
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", detected)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if peer.photo.Date > 0 {
		w.Header().Set("Last-Modified", time.Unix(int64(peer.photo.Date), 0).UTC().Format(http.TimeFormat))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *handler) serveSet(w http.ResponseWriter, r *http.Request, pathKind string) {
	shortName := strings.TrimSpace(r.PathValue("shortName"))
	if !validShortNamePath(shortName) {
		http.NotFound(w, r)
		return
	}
	set, docs, found, err := h.stickerSets.ResolveStickerSet(r.Context(), domain.StickerSetRef{
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
		AppName:      h.appName,
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
	appName := h.appName
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
		data.Title = h.appName
	}
	if data.CanonicalURL == "" {
		data.CanonicalURL = h.publicBaseURL + "/"
	}
	if data.AppURL == "" {
		data.AppURL = h.appURL("")
	}
	if data.ActionText == "" {
		data.ActionText = "Open in " + h.appName
	}
	if data.SiteName == "" {
		data.SiteName = h.appName
	}
	if data.SiteInitial == "" {
		data.SiteInitial = brandInitial(data.SiteName)
	}
	if data.Icon == "" {
		data.Icon = "paper"
	}
	data.AppURLJS = template.JS(strconv.Quote(data.AppURL))
	data.AppURLAttr = template.URL(data.AppURL)
	data.CompatAppURL = compatAppURL(data.AppURL)
	data.CompatAppURLJS = template.JS(strconv.Quote(data.CompatAppURL))
	data.PathFullJS = template.JS(strconv.Quote(data.PathFull))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	if err := landingTemplate.Execute(w, data); err != nil {
		http.Error(w, "render public link page failed", http.StatusInternalServerError)
	}
}

func (h *handler) serveHome(w http.ResponseWriter, _ *http.Request, data homePage) {
	if data.AppName == "" {
		data.AppName = brand.DefaultAppName
	}
	if data.CanonicalURL == "" {
		data.CanonicalURL = h.publicBaseURL + "/"
	}
	if data.AppURL == "" {
		data.AppURL = h.appURL("")
	}
	if data.WebURL == "" {
		data.WebURL = h.publicBaseURL + "/"
	}
	if data.DevicesImage == "" {
		data.DevicesImage = "/assets/safelink-home-devices.png"
	}
	if data.MarkImage == "" {
		data.MarkImage = "/assets/safelink-mark.png"
	}
	data.Title = data.AppName + " - 安全、快速、同步的即时通讯"
	data.Description = data.AppName + " 是面向团队、社区与私有部署的安全通讯平台，支持移动端、桌面端和网页版。"
	data.AppURLAttr = template.URL(data.AppURL)
	data.WebURLAttr = template.URL(data.WebURL)
	data.DevicesImageAttr = template.URL(data.DevicesImage)
	data.MarkImageAttr = template.URL(data.MarkImage)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	if err := homeTemplate.Execute(w, data); err != nil {
		http.Error(w, "render home page failed", http.StatusInternalServerError)
	}
}

func (h *handler) serveSitePage(w http.ResponseWriter, r *http.Request, data sitePage) {
	if data.AppName == "" {
		data.AppName = brand.DefaultAppName
	}
	if data.Title == "" {
		data.Title = data.Heading + " - " + data.AppName
	}
	if data.Description == "" {
		data.Description = data.Intro
	}
	if data.CanonicalURL == "" {
		data.CanonicalURL = h.canonicalURL(r, r.URL.Path)
	}
	if data.WebURL == "" {
		data.WebURL = "https://web.safelink.chat/"
	}
	if data.AppURL == "" {
		data.AppURL = h.appURL("")
	}
	if data.MarkImage == "" {
		data.MarkImage = "/assets/safelink-mark.png"
	}
	data.AppURLAttr = template.URL(data.AppURL)
	data.WebURLAttr = template.URL(data.WebURL)
	data.MarkImageAttr = template.URL(data.MarkImage)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=60")
	if err := sitePageTemplate.Execute(w, data); err != nil {
		http.Error(w, "render site page failed", http.StatusInternalServerError)
	}
}

func (h *handler) canonicalURL(r *http.Request, path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	host := strings.TrimSpace(r.Host)
	if host == "" || strings.EqualFold(host, "example.com") {
		return h.publicURL(path)
	}
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return scheme + "://" + host + path
}

func compatAppURL(primary string) string {
	u, err := url.Parse(strings.TrimSpace(primary))
	if err != nil || u.Scheme == "" || strings.EqualFold(u.Scheme, "tg") {
		return ""
	}
	u.Scheme = "tg"
	return u.String()
}

func (h *handler) publicUsernameURL(username string) string {
	return h.publicBaseURL + "/" + url.PathEscape(username)
}

func (h *handler) publicAvatarURL(username string, photoID int64) string {
	return h.publicBaseURL + "/_public/avatar/" + url.PathEscape(username) + "/" + strconv.FormatInt(photoID, 10)
}

type publicPeerKind string

const (
	publicPeerUser       publicPeerKind = "user"
	publicPeerBot        publicPeerKind = "bot"
	publicPeerChannel    publicPeerKind = "channel"
	publicPeerSupergroup publicPeerKind = "supergroup"
)

type publicPeer struct {
	kind        publicPeerKind
	username    string
	title       string
	about       string
	verified    bool
	memberCount int
	photo       domain.Photo
	hasPhoto    bool
}

func (h *handler) resolvePublicPeer(ctx context.Context, username string) (publicPeer, bool, error) {
	var (
		u      domain.User
		userOK bool
		ch     domain.Channel
		chOK   bool
		err    error
	)
	if h.users != nil {
		u, userOK, err = h.users.ByUsername(ctx, username)
		if err != nil {
			return publicPeer{}, false, err
		}
	}
	if h.channels != nil {
		ch, chOK, err = h.channels.ResolvePublicChannelUsername(ctx, 0, username)
		if err != nil {
			return publicPeer{}, false, err
		}
	}
	if userOK && chOK {
		return publicPeer{}, false, fmt.Errorf("public username %q has multiple owners", username)
	}
	if userOK {
		return h.publicUserPeer(ctx, username, u)
	}
	if chOK {
		return h.publicChannelPeer(ctx, username, ch)
	}
	return publicPeer{}, false, nil
}

func (h *handler) publicUserPeer(ctx context.Context, requested string, u domain.User) (publicPeer, bool, error) {
	if u.ID == 0 || !strings.EqualFold(strings.TrimSpace(u.Username), requested) || !validUsernamePath(u.Username) {
		return publicPeer{}, false, fmt.Errorf("user username lookup returned invalid owner for %q", requested)
	}
	title := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if title == "" {
		title = u.Username
	}
	if err := validatePublicPeerText(title, u.About); err != nil {
		return publicPeer{}, false, fmt.Errorf("invalid public user %q: %w", u.Username, err)
	}
	about := strings.TrimSpace(u.About)
	photoKind := domain.ProfilePhotoKindProfile
	if !u.Bot && h.anonymousPrivacy != nil {
		visible, err := h.anonymousPrivacy.CanSeeAnonymous(ctx, u.ID, domain.PrivacyKeyAbout)
		if err != nil {
			return publicPeer{}, false, fmt.Errorf("evaluate public about privacy: %w", err)
		}
		if !visible {
			about = ""
		}
		visible, err = h.anonymousPrivacy.CanSeeAnonymous(ctx, u.ID, domain.PrivacyKeyProfilePhoto)
		if err != nil {
			return publicPeer{}, false, fmt.Errorf("evaluate public profile photo privacy: %w", err)
		}
		if !visible {
			photoKind = domain.ProfilePhotoKindFallback
		}
	}
	peer := publicPeer{
		kind:     publicPeerUser,
		username: u.Username,
		title:    title,
		about:    about,
		verified: u.Verified,
	}
	if u.Bot {
		peer.kind = publicPeerBot
	}
	if h.photos != nil {
		photo, found, err := h.photos.CurrentProfilePhotoKind(ctx, domain.PeerTypeUser, u.ID, photoKind)
		if err != nil {
			return publicPeer{}, false, fmt.Errorf("load public user photo: %w", err)
		}
		if found && photo.ID != 0 {
			if _, _, renderable := bestPublicPhotoSize(photo.Sizes); renderable {
				peer.photo, peer.hasPhoto = photo, true
			}
		}
	}
	return peer, true, nil
}

func (h *handler) publicChannelPeer(ctx context.Context, requested string, ch domain.Channel) (publicPeer, bool, error) {
	if ch.ID == 0 || ch.Deleted || ch.ParticipantsCount < 0 || (!ch.Broadcast && !ch.Megagroup) || !strings.EqualFold(strings.TrimSpace(ch.Username), requested) || !validUsernamePath(ch.Username) {
		return publicPeer{}, false, fmt.Errorf("channel username lookup returned invalid owner for %q", requested)
	}
	if err := validatePublicPeerText(ch.Title, ch.About); err != nil {
		return publicPeer{}, false, fmt.Errorf("invalid public channel %q: %w", ch.Username, err)
	}
	peer := publicPeer{
		kind:        publicPeerChannel,
		username:    ch.Username,
		title:       strings.TrimSpace(ch.Title),
		about:       strings.TrimSpace(ch.About),
		verified:    ch.Verified,
		memberCount: ch.ParticipantsCount,
	}
	if ch.Megagroup {
		peer.kind = publicPeerSupergroup
	}
	if h.photos != nil && ch.PhotoID != 0 {
		photo, found, err := h.photos.GetPhoto(ctx, ch.PhotoID)
		if err != nil {
			return publicPeer{}, false, fmt.Errorf("load public channel photo: %w", err)
		}
		if !found {
			return publicPeer{}, false, fmt.Errorf("channel %q current photo %d is missing", ch.Username, ch.PhotoID)
		}
		if photo.ID == ch.PhotoID {
			if _, _, renderable := bestPublicPhotoSize(photo.Sizes); renderable {
				peer.photo, peer.hasPhoto = photo, true
			}
		} else {
			return publicPeer{}, false, fmt.Errorf("channel %q photo lookup returned id %d, want %d", ch.Username, photo.ID, ch.PhotoID)
		}
	}
	return peer, true, nil
}

func validatePublicPeerText(title, about string) error {
	if strings.TrimSpace(title) == "" || utf8.RuneCountInString(title) > 256 {
		return fmt.Errorf("title is empty or too long")
	}
	if utf8.RuneCountInString(about) > 4096 {
		return fmt.Errorf("about is too long")
	}
	return nil
}

func (p publicPeer) buttonLabel() string {
	switch p.kind {
	case publicPeerBot:
		return "Start Bot"
	case publicPeerChannel:
		return "View Channel"
	case publicPeerSupergroup:
		return "View Group"
	default:
		return "Send Message"
	}
}

func (p publicPeer) extra() string {
	switch p.kind {
	case publicPeerBot:
		return brand.DefaultAppName + " Bot"
	case publicPeerChannel:
		return groupedDecimal(p.memberCount) + " " + plural(p.memberCount, "subscriber", "subscribers")
	case publicPeerSupergroup:
		return groupedDecimal(p.memberCount) + " " + plural(p.memberCount, "member", "members")
	default:
		return ""
	}
}

func (p publicPeer) fallbackDescription(appName string) string {
	switch p.kind {
	case publicPeerBot:
		return "Open " + appName + " to start a chat with this bot."
	case publicPeerChannel:
		return "Open " + appName + " to view and join this channel."
	case publicPeerSupergroup:
		return "Open " + appName + " to view and join this group."
	default:
		return "Open " + appName + " to send a message to @" + p.username + "."
	}
}

func (p publicPeer) initials() string {
	words := strings.Fields(p.title)
	if len(words) == 0 {
		words = []string{p.username}
	}
	first := []rune(words[0])
	if len(first) == 0 {
		return "T"
	}
	out := []rune{first[0]}
	if len(words) > 1 {
		last := []rune(words[len(words)-1])
		if len(last) > 0 {
			out = append(out, last[0])
		}
	}
	return strings.ToUpper(string(out))
}

func groupedDecimal(n int) string {
	if n < 0 {
		n = 0
	}
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + " " + s[i:]
	}
	return s
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func appInitial(name string) string {
	for _, r := range name {
		return strings.ToUpper(string(r))
	}
	return "T"
}

const (
	maxPublicLinkRawQuery = 2048
	maxPublicLinkParams   = 16
	maxPublicLinkValues   = 2
	maxPublicLinkValueLen = 512
)

func publicResolveQuery(raw string) (url.Values, bool) {
	if len(raw) > maxPublicLinkRawQuery {
		return nil, false
	}
	values, err := url.ParseQuery(raw)
	if err != nil || len(values) > maxPublicLinkParams {
		return nil, false
	}
	out := make(url.Values, len(values)+1)
	for key, items := range values {
		if strings.EqualFold(key, "domain") {
			continue
		}
		if !validPublicQueryKey(key) || len(items) > maxPublicLinkValues {
			return nil, false
		}
		for _, value := range items {
			if len(value) > maxPublicLinkValueLen || !utf8.ValidString(value) {
				return nil, false
			}
			out.Add(key, value)
		}
	}
	return out, true
}

func validPublicQueryKey(key string) bool {
	if key == "" || len(key) > 32 {
		return false
	}
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func bestPublicPhotoSize(sizes []domain.PhotoSize) (domain.PhotoSize, []byte, bool) {
	var (
		best      domain.PhotoSize
		bestBytes []byte
		bestScore int64 = -1
	)
	for _, size := range sizes {
		if !validPhotoSizeType(size.Type) {
			continue
		}
		var inline []byte
		switch size.Kind {
		case domain.PhotoSizeKindCached:
			if len(size.Bytes) == 0 || len(size.Bytes) > maxPublicAvatarBytes {
				continue
			}
			inline = size.Bytes
		case domain.PhotoSizeKindDefault, domain.PhotoSizeKindProgressive:
			// Downloadable static raster size.
		default:
			continue
		}
		score := int64(size.W) * int64(size.H)
		if score <= 0 {
			score = int64(size.Size)
		}
		if score > bestScore {
			best, bestBytes, bestScore = size, inline, score
		}
	}
	return best, bestBytes, bestScore >= 0
}

func validPhotoSizeType(value string) bool {
	if value == "" || len(value) > 8 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func safePublicImageType(value string) bool {
	switch value {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func schemeURLValues(scheme, kind string, values url.Values) string {
	return (&url.URL{Scheme: scheme, Host: kind, RawQuery: values.Encode()}).String()
}

func publicWebAppURL(webBaseURL, legacyURL string) string {
	return strings.TrimRight(webBaseURL, "/") + "/#?tgaddr=" + url.QueryEscape(legacyURL)
}

func publicSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'; script-src 'unsafe-inline'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
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

func validStarGiftSlugPath(slug string) bool {
	if slug == "" || len(slug) > domain.MaxStarGiftSlugBytes {
		return false
	}
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

func validUsernamePath(username string) bool {
	if username == "" || len(username) < 5 || len(username) > 32 {
		return false
	}
	for i, r := range username {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9', r == '_':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
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
	AppName      string
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
	SiteName       string
	SiteInitial    string
	Title          string
	CanonicalURL   string
	AppURL         string
	KindLabel      string
	PageTitle      string
	Extra          string
	Description    string
	ActionText     string
	Icon           string
	PathFull       string
	CompatAppURL   string
	AppURLJS       template.JS
	CompatAppURLJS template.JS
	AppURLAttr     template.URL
	PathFullJS     template.JS
}

type homePage struct {
	AppName          string
	Title            string
	Description      string
	CanonicalURL     string
	AppURL           string
	WebURL           string
	DevicesImage     string
	MarkImage        string
	AppURLAttr       template.URL
	WebURLAttr       template.URL
	DevicesImageAttr template.URL
	MarkImageAttr    template.URL
}

type sitePage struct {
	AppName       string
	Title         string
	Description   string
	CanonicalURL  string
	AppURL        string
	WebURL        string
	MarkImage     string
	Active        string
	Kicker        string
	Heading       string
	Intro         string
	PrimaryText   string
	PrimaryURL    template.URL
	SecondaryText string
	SecondaryURL  template.URL
	Sections      []siteSection
	AppURLAttr    template.URL
	WebURLAttr    template.URL
	MarkImageAttr template.URL
}

type siteSection struct {
	Title string
	Intro string
	Items []siteItem
}

type siteItem struct {
	Title    string
	Body     string
	LinkText string
	LinkURL  template.URL
}

var homeTemplate = template.Must(template.New("home").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <link rel="canonical" href="{{.CanonicalURL}}">
  <meta name="description" content="{{.Description}}">
  <meta property="og:title" content="{{.Title}}">
  <meta property="og:site_name" content="{{.AppName}}">
  <meta property="og:description" content="{{.Description}}">
  <meta property="og:url" content="{{.CanonicalURL}}">
  <meta name="twitter:card" content="summary_large_image">
  <meta name="twitter:title" content="{{.Title}}">
  <meta name="twitter:description" content="{{.Description}}">
  <meta property="al:ios:app_name" content="{{.AppName}}">
  <meta property="al:ios:url" content="{{.AppURL}}">
  <meta property="al:android:app_name" content="{{.AppName}}">
  <meta property="al:android:url" content="{{.AppURL}}">
  <meta name="apple-itunes-app" content="app-argument: {{.AppURL}}">
  <style>
    :root {
      color-scheme: light;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", Roboto, Helvetica, Arial, sans-serif;
      --blue: #229ed9;
      --blue-strong: #168ad4;
      --ink: #10233f;
      --text: #243b5a;
      --muted: #6d7e95;
      --line: #dce8f4;
      --soft: #f4f9fe;
      --green: #45b36b;
      --amber: #f5a623;
      --white: #fff;
    }
    * { box-sizing: border-box; }
    html { scroll-behavior: smooth; }
    body {
      margin: 0;
      min-width: 320px;
      color: var(--text);
      background: var(--white);
      -webkit-font-smoothing: antialiased;
      text-rendering: optimizeLegibility;
    }
    a { color: inherit; }
    .site-header {
      position: sticky;
      top: 0;
      z-index: 5;
      background: rgba(255,255,255,.94);
      border-bottom: 1px solid var(--line);
      backdrop-filter: blur(14px);
    }
    .nav {
      width: min(100%, 1320px);
      height: 74px;
      margin: 0 auto;
      padding: 0 34px;
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 24px;
    }
    .brand {
      display: inline-flex;
      align-items: center;
      gap: 12px;
      text-decoration: none;
      color: var(--ink);
      font-size: 28px;
      font-weight: 800;
      white-space: nowrap;
    }
    .brand-mark { width: 30px; height: 50px; display: block; object-fit: contain; }
    .nav-links {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 36px;
      color: var(--ink);
      font-size: 15px;
      font-weight: 700;
    }
    .nav-links a {
      min-height: 44px;
      display: inline-flex;
      align-items: center;
      text-decoration: none;
      border-bottom: 3px solid transparent;
    }
    .nav-links a:hover,
    .nav-links a:focus-visible { color: var(--blue); border-bottom-color: var(--blue); outline: 0; }
    .nav-actions { display: inline-flex; align-items: center; gap: 14px; white-space: nowrap; }
    .lang { color: var(--muted); font-size: 14px; font-weight: 700; }
    .button {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      min-height: 46px;
      padding: 0 22px;
      border: 1px solid var(--line);
      border-radius: 8px;
      text-decoration: none;
      font-size: 15px;
      font-weight: 800;
      color: var(--blue-strong);
      background: var(--white);
      box-shadow: 0 1px 2px rgba(16,35,63,.04);
    }
    .button.primary {
      border-color: var(--blue);
      color: #fff;
      background: var(--blue);
      box-shadow: 0 8px 18px rgba(34,158,217,.22);
    }
    .button:hover,
    .button:focus-visible { transform: translateY(-1px); outline: 0; }
    .hero {
      width: min(100%, 1260px);
      margin: 0 auto;
      padding: 46px 34px 48px;
      text-align: center;
    }
    .eyebrow {
      margin: 0 auto 18px;
      width: fit-content;
      padding: 7px 13px;
      border: 1px solid #cfe8fb;
      border-radius: 999px;
      color: var(--blue-strong);
      background: #f3faff;
      font-size: 14px;
      font-weight: 800;
    }
    h1 {
      margin: 0;
      color: var(--ink);
      font-size: clamp(52px, 7vw, 92px);
      line-height: .98;
      font-weight: 800;
      letter-spacing: 0;
    }
    h1 span { color: var(--blue); }
    .hero h2 {
      margin: 18px auto 0;
      max-width: 760px;
      color: var(--ink);
      font-size: clamp(24px, 3vw, 34px);
      line-height: 1.24;
      font-weight: 800;
      letter-spacing: 0;
    }
    .hero-copy {
      margin: 12px auto 0;
      max-width: 680px;
      color: var(--muted);
      font-size: 18px;
      line-height: 1.7;
    }
    .hero-actions {
      margin-top: 24px;
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 16px;
      flex-wrap: wrap;
    }
    .web-address {
      margin: 16px auto 0;
      max-width: 100%;
      color: var(--muted);
      font-size: 15px;
      line-height: 1.6;
      overflow-wrap: anywhere;
    }
    .web-address a {
      color: var(--blue-strong);
      font-weight: 900;
      text-decoration: none;
    }
    .hero-visual {
      margin: 34px auto 0;
      max-width: 900px;
    }
    .hero-visual img {
      display: block;
      width: 100%;
      height: auto;
      border: 0;
    }
    .section {
      border-top: 1px solid var(--line);
    }
    .section-inner {
      width: min(100%, 1260px);
      margin: 0 auto;
      padding: 52px 34px;
    }
    .section-title {
      margin: 0;
      color: var(--ink);
      font-size: clamp(28px, 4vw, 38px);
      line-height: 1.24;
      text-align: center;
      font-weight: 800;
      letter-spacing: 0;
    }
    .section-subtitle {
      margin: 12px auto 0;
      max-width: 660px;
      color: var(--muted);
      text-align: center;
      font-size: 16px;
      line-height: 1.65;
    }
    .platforms {
      margin-top: 36px;
      display: grid;
      grid-template-columns: repeat(6, minmax(0, 1fr));
      gap: 18px;
    }
    .platform {
      min-height: 120px;
      padding: 22px 14px;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      gap: 8px;
      border: 1px solid transparent;
      border-radius: 8px;
      color: var(--ink);
      text-decoration: none;
      background: var(--white);
    }
    .platform:hover,
    .platform:focus-visible { border-color: #bfe1f7; background: #f8fcff; outline: 0; }
    .platform-name { font-size: 18px; font-weight: 800; }
    .platform-note { color: var(--muted); font-size: 13px; text-align: center; line-height: 1.35; }
    .platform-status { color: var(--blue); font-size: 13px; font-weight: 800; }
    .updates-head {
      display: flex;
      align-items: end;
      justify-content: space-between;
      gap: 24px;
      margin-bottom: 28px;
    }
    .updates-head .section-title { text-align: left; }
    .text-link {
      color: var(--blue-strong);
      text-decoration: none;
      font-weight: 800;
      white-space: nowrap;
    }
    .updates {
      display: grid;
      grid-template-columns: repeat(3, minmax(0, 1fr));
      gap: 34px;
    }
    .update {
      display: grid;
      grid-template-columns: 92px 1fr;
      gap: 20px;
      align-items: center;
    }
    .update-thumb {
      width: 92px;
      aspect-ratio: 1;
      border-radius: 8px;
      background:
        linear-gradient(135deg, rgba(34,158,217,.95), rgba(22,138,212,.72)),
        linear-gradient(45deg, rgba(255,255,255,.2), transparent);
      border: 1px solid #c7e7fa;
    }
    .update:nth-child(2) .update-thumb {
      background:
        linear-gradient(135deg, #edf8ff, #d8eefc),
        linear-gradient(45deg, rgba(34,158,217,.22), transparent);
    }
    .update:nth-child(3) .update-thumb {
      background:
        linear-gradient(135deg, #d9f3ff, #edf9ff 46%, #dff5e9),
        linear-gradient(45deg, rgba(69,179,107,.2), transparent);
    }
    .update h3 { margin: 0; color: var(--ink); font-size: 17px; line-height: 1.35; }
    .update time { display: block; margin-top: 8px; color: var(--blue); font-size: 13px; font-weight: 800; }
    .update p { margin: 9px 0 0; color: var(--muted); font-size: 14px; line-height: 1.55; }
    .features {
      margin-top: 38px;
      display: grid;
      grid-template-columns: repeat(3, minmax(0, 1fr));
      border-top: 1px solid var(--line);
      border-left: 1px solid var(--line);
    }
    .feature {
      min-height: 150px;
      padding: 30px;
      border-right: 1px solid var(--line);
      border-bottom: 1px solid var(--line);
      background: var(--white);
    }
    .feature-kicker {
      display: inline-flex;
      margin-bottom: 14px;
      color: var(--blue);
      font-size: 13px;
      font-weight: 900;
      text-transform: uppercase;
    }
    .feature h3 { margin: 0; color: var(--ink); font-size: 19px; line-height: 1.35; }
    .feature p { margin: 9px 0 0; color: var(--muted); font-size: 14px; line-height: 1.6; }
    .link-band {
      background: var(--soft);
    }
    .link-box {
      display: grid;
      grid-template-columns: 1fr auto;
      gap: 24px;
      align-items: center;
      padding: 28px 34px;
      border: 1px solid #bfe1f7;
      border-radius: 8px;
      background: linear-gradient(90deg, #f4fbff, #fff);
    }
    .link-box h2 { margin: 0; color: var(--ink); font-size: 28px; line-height: 1.25; }
    .link-box p { margin: 8px 0 0; max-width: 640px; color: var(--muted); font-size: 15px; line-height: 1.65; }
    .link-code { color: var(--blue-strong); font-weight: 900; }
    .footer {
      border-top: 1px solid var(--line);
      background: #fbfdff;
    }
    .footer-grid {
      width: min(100%, 1260px);
      margin: 0 auto;
      padding: 44px 34px 42px;
      display: grid;
      grid-template-columns: minmax(220px, 2fr) repeat(5, minmax(100px, 1fr));
      gap: 36px;
      color: var(--muted);
      font-size: 14px;
      line-height: 1.8;
    }
    .footer h3 { margin: 0 0 9px; color: var(--ink); font-size: 15px; }
    .footer a { display: block; color: var(--muted); text-decoration: none; }
    .footer a:hover { color: var(--blue); }
    .footer .brand { margin-bottom: 12px; font-size: 24px; }
    @media (max-width: 900px) {
      .nav { height: auto; min-height: 68px; padding: 14px 20px; align-items: flex-start; flex-wrap: wrap; }
      .brand { font-size: 24px; }
      .brand-mark { width: 26px; height: 43px; }
      .nav-links { order: 3; width: 100%; justify-content: flex-start; gap: 20px; overflow-x: auto; padding-top: 4px; font-size: 14px; }
      .nav-actions { margin-left: auto; }
      .hero { padding: 46px 20px 48px; }
      .hero h2 { font-size: 24px; }
      .hero-copy { font-size: 16px; }
      .section-inner { padding: 42px 20px; }
      .platforms { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
      .updates-head { align-items: flex-start; flex-direction: column; }
      .updates { grid-template-columns: 1fr; gap: 26px; }
      .features { grid-template-columns: 1fr; }
      .link-box { grid-template-columns: 1fr; padding: 24px; }
      .link-box .hero-actions { justify-content: flex-start; margin-top: 0; }
      .footer-grid { grid-template-columns: 1fr 1fr; padding: 36px 20px; }
    }
    @media (max-width: 520px) {
      .nav-actions .lang { display: none; }
      .nav-actions .button { min-height: 40px; padding: 0 14px; font-size: 13px; }
      .nav-links { gap: 18px; }
      h1 { font-size: 54px; }
      .button { width: 100%; }
      .hero-actions { width: 100%; }
      .hero-visual { width: calc(100vw - 24px); margin-top: 34px; }
      .platform { min-height: 104px; padding: 18px 10px; }
      .update { grid-template-columns: 76px 1fr; gap: 16px; }
      .update-thumb { width: 76px; }
      .feature { padding: 24px 20px; }
      .footer-grid { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body>
  <header class="site-header">
    <nav class="nav" aria-label="主导航">
      <a class="brand" href="/" aria-label="{{.AppName}} 首页"><img class="brand-mark" src="{{.MarkImageAttr}}" alt="" width="30" height="50"><span>{{.AppName}}</span></a>
      <div class="nav-links">
        <a href="/">首页</a>
        <a href="/faq">FAQ</a>
        <a href="/apps">Apps</a>
        <a href="/api">API</a>
        <a href="/safety">Safety</a>
        <a href="/updates">Updates</a>
      </div>
      <div class="nav-actions">
        <span class="lang">中文 / EN</span>
        <a class="button primary" href="{{.WebURLAttr}}">打开网页版</a>
      </div>
    </nav>
  </header>

  <main>
    <section class="hero" aria-labelledby="home-title">
      <p class="eyebrow">www.safelink.chat</p>
      <h1 id="home-title">Safe<span>Link</span></h1>
      <h2>安全连接每一段对话</h2>
      <p class="hero-copy">SafeLink 为团队、社区和私有实例提供轻快、同步、可控的即时通讯体验。中文优先，多端一致，链接直接打开 SafeLink。</p>
      <div class="hero-actions">
        <a class="button primary" href="/apps">下载 SafeLink</a>
        <a class="button" href="{{.WebURLAttr}}">打开网页版</a>
      </div>
      <p class="web-address">网页版地址：<a href="{{.WebURLAttr}}">{{.WebURL}}</a></p>
      <figure class="hero-visual">
        <img src="{{.DevicesImageAttr}}" alt="SafeLink 手机端与桌面端聊天界面预览" width="830" height="360" loading="eager">
      </figure>
    </section>

    <section id="apps" class="section" aria-labelledby="apps-title">
      <div class="section-inner">
        <h2 id="apps-title" class="section-title">每个平台都有 SafeLink</h2>
        <p class="section-subtitle">手机、桌面和浏览器保持同步。测试阶段下载入口会逐步接入正式安装包。</p>
        <div class="platforms">
          <a class="platform" href="/apps#ios"><span class="platform-name">iOS</span><span class="platform-note">iPhone 与 iPad</span><span class="platform-status">准备接入</span></a>
          <a class="platform" href="/apps#android"><span class="platform-name">Android</span><span class="platform-note">手机与平板</span><span class="platform-status">准备接入</span></a>
          <a class="platform" href="/apps#macos"><span class="platform-name">macOS</span><span class="platform-note">Apple Silicon 与 Intel</span><span class="platform-status">准备接入</span></a>
          <a class="platform" href="/apps#windows"><span class="platform-name">Windows</span><span class="platform-note">桌面客户端</span><span class="platform-status">准备接入</span></a>
          <a class="platform" href="{{.WebURLAttr}}"><span class="platform-name">Web</span><span class="platform-note">浏览器访问</span><span class="platform-status">立即打开</span></a>
          <a class="platform" href="/apps#linux"><span class="platform-name">Linux</span><span class="platform-note">桌面客户端</span><span class="platform-status">准备接入</span></a>
        </div>
      </div>
    </section>

    <section id="updates" class="section" aria-labelledby="updates-title">
      <div class="section-inner">
        <div class="updates-head">
          <h2 id="updates-title" class="section-title">最新进展</h2>
          <a class="text-link" href="/updates">查看全部更新</a>
        </div>
        <div class="updates">
          <article class="update">
            <div class="update-thumb" aria-hidden="true"></div>
            <div>
              <h3>自定义域名与 SafeLink 链接</h3>
              <time datetime="2026-07-09">2026 年 7 月</time>
              <p>公开链接统一使用 safelink.chat 与 safelink://，打开自己的 SafeLink 客户端。</p>
            </div>
          </article>
          <article class="update">
            <div class="update-thumb" aria-hidden="true"></div>
            <div>
              <h3>验证码、设备与登录体验优化</h3>
              <time datetime="2026-07-09">测试阶段</time>
              <p>邮箱验证码、设备信息和多端登录体验持续完善，减少历史品牌残留文案。</p>
            </div>
          </article>
          <article class="update">
            <div class="update-thumb" aria-hidden="true"></div>
            <div>
              <h3>媒体、群组与桌面端兼容</h3>
              <time datetime="2026-07-09">持续迭代</time>
              <p>围绕图片、视频、文件与桌面客户端消息收发做稳定性验证。</p>
            </div>
          </article>
        </div>
      </div>
    </section>

    <section id="security" class="section" aria-labelledby="security-title">
      <div class="section-inner">
        <h2 id="security-title" class="section-title">为什么选择 SafeLink</h2>
        <p class="section-subtitle">保留熟悉的即时通讯体验，同时把服务端、域名和客户端入口交回你自己的实例。</p>
        <div class="features">
          <article class="feature"><span class="feature-kicker">Private</span><h3>默认重视隐私</h3><p>为聊天、通话、文件与登录流程提供清晰可控的安全边界。</p></article>
          <article class="feature"><span class="feature-kicker">Fast</span><h3>轻快可靠</h3><p>面向真实客户端连接优化，弱网下也尽量保持顺滑响应。</p></article>
          <article class="feature"><span class="feature-kicker">Synced</span><h3>多端同步</h3><p>移动端、桌面端与网页版共享消息、媒体和会话状态。</p></article>
          <article class="feature"><span class="feature-kicker">Groups</span><h3>群组与频道</h3><p>支持邀请链接、公开用户名、群组管理、频道与共享文件夹。</p></article>
          <article class="feature"><span class="feature-kicker">Open</span><h3>自有实例</h3><p>使用自己的域名、服务端和客户端配置，避免继续下发外部默认前缀。</p></article>
          <article class="feature"><span class="feature-kicker">Control</span><h3>链接直达</h3><p>SafeLink 链接会生成 safelink:// 深链，直接唤起 SafeLink 客户端。</p></article>
        </div>
      </div>
    </section>

    <section id="links" class="section link-band" aria-labelledby="links-title">
      <div class="section-inner">
        <div class="link-box">
          <div>
            <h2 id="links-title">一个链接，打开 SafeLink</h2>
            <p>公开入口使用 <span class="link-code">https://safelink.chat/your-link</span>，客户端深链使用 <span class="link-code">safelink://</span>。邀请、用户主页、贴纸、表情和共享文件夹都走 SafeLink 逻辑。</p>
          </div>
          <div class="hero-actions">
            <a class="button primary" href="{{.AppURLAttr}}">打开 SafeLink</a>
            <a class="button" href="{{.WebURLAttr}}">打开网页版</a>
          </div>
        </div>
      </div>
    </section>
  </main>

  <footer class="footer">
    <div class="footer-grid">
      <div>
        <a class="brand" href="/"><img class="brand-mark" src="{{.MarkImageAttr}}" alt="" width="30" height="50"><span>{{.AppName}}</span></a>
        <p>安全、快速、同步的即时通讯。SafeLink keeps your own instance connected.</p>
        <p>© 2026 {{.AppName}}</p>
      </div>
      <div><h3>SafeLink</h3><a href="/faq">FAQ</a><a href="/safety">功能与安全</a><a href="/links">链接规则</a><a href="/updates">更新</a></div>
      <div><h3>客户端</h3><a href="/apps#ios">iOS</a><a href="/apps#android">Android</a><a href="/apps#macos">macOS</a><a href="/apps#windows">Windows</a><a href="{{.WebURLAttr}}">Web</a></div>
      <div><h3>平台</h3><a href="/api">API</a><a href="/translations">本地化</a><a href="/instantview">即时视图</a></div>
      <div><h3>支持</h3><a href="/support">帮助中心</a><a href="/privacy">隐私</a><a href="/terms">条款</a><a href="/press">媒体资料</a></div>
      <div><h3>连接</h3><a href="{{.CanonicalURL}}">www.safelink.chat</a><a href="{{.AppURLAttr}}">safelink://</a><a href="{{.WebURLAttr}}">网页版</a></div>
    </div>
  </footer>
</body>
</html>
`))

type usernamePageData struct {
	AppName      string
	AppInitial   string
	Title        string
	Username     string
	Verified     bool
	Extra        string
	Description  string
	CanonicalURL string
	HomeURL      string
	PhotoURL     string
	Initials     string
	ButtonLabel  string
	AppURL       template.URL
	LegacyTgURL  template.URL
	WebURL       template.URL
	AppURLJS     template.JS
}

func (h *handler) serveUsernameNotFound(w http.ResponseWriter, username string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=30, must-revalidate")
	w.WriteHeader(http.StatusNotFound)
	if err := usernameNotFoundTemplate.Execute(w, struct {
		Username string
		HomeURL  string
		AppName  string
	}{Username: username, HomeURL: h.publicBaseURL + "/", AppName: h.appName}); err != nil {
		h.logger.Error("Render public username not-found page failed", zap.String("username", username), zap.Error(err))
	}
}

var usernameLandingTemplate = template.Must(template.New("username-landing").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
  <meta name="theme-color" content="#0e1621">
  <title>{{.Title}} (@{{.Username}}) - {{.AppName}}</title>
  <meta name="description" content="{{.Description}}">
  <meta name="robots" content="index,follow,max-image-preview:large">
  <link rel="canonical" href="{{.CanonicalURL}}">
  <meta property="og:type" content="profile">
  <meta property="og:site_name" content="{{.AppName}}">
  <meta property="og:title" content="{{.Title}}">
  <meta property="og:description" content="{{.Description}}">
  <meta property="og:url" content="{{.CanonicalURL}}">
  {{if .PhotoURL}}<meta property="og:image" content="{{.PhotoURL}}">{{end}}
  <meta property="al:android:url" content="{{.AppURL}}">
  <meta property="al:ios:url" content="{{.AppURL}}">
  <meta name="twitter:card" content="summary">
  <meta name="twitter:title" content="{{.Title}}">
  <meta name="twitter:description" content="{{.Description}}">
  {{if .PhotoURL}}<meta name="twitter:image" content="{{.PhotoURL}}">{{end}}
  <style>
    :root { color-scheme: dark; font-family: Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    * { box-sizing: border-box; }
    body { margin: 0; min-height: 100svh; color: #f5f8fb; background:
      radial-gradient(circle at 50% -20%, rgba(50, 161, 255, .25), transparent 42%), #0e1621; }
    .shell { min-height: 100svh; display: grid; grid-template-rows: auto 1fr auto; }
    .brand { display: flex; align-items: center; gap: 10px; width: fit-content; margin: 28px auto 0; color: #dceeff;
      font-size: 17px; font-weight: 700; letter-spacing: .01em; text-decoration: none; }
    .brand-mark { display: grid; place-items: center; width: 34px; height: 34px; border-radius: 50%; color: white;
      background: linear-gradient(145deg, #52b8ff, #168de2); box-shadow: 0 8px 24px rgba(31, 151, 232, .3); }
    main { display: grid; place-items: center; padding: 32px 18px; }
    .card { width: min(100%, 420px); padding: 34px 30px 28px; text-align: center; border: 1px solid rgba(255,255,255,.08);
      border-radius: 24px; background: rgba(23, 33, 43, .92); box-shadow: 0 28px 90px rgba(0,0,0,.34); backdrop-filter: blur(18px); }
    .avatar { display: grid; place-items: center; width: 112px; height: 112px; margin: 0 auto 22px; overflow: hidden;
      border-radius: 50%; background: linear-gradient(145deg, #47b7ff, #167bc1); box-shadow: 0 16px 44px rgba(10, 112, 183, .3); }
    .avatar img { display: block; width: 100%; height: 100%; object-fit: cover; }
    .initials { font-size: 38px; font-weight: 750; letter-spacing: -.04em; color: white; }
    h1 { display: flex; align-items: center; justify-content: center; gap: 8px; margin: 0; font-size: clamp(25px, 7vw, 32px);
      line-height: 1.18; letter-spacing: -.025em; overflow-wrap: anywhere; }
    .verified { display: inline-grid; flex: 0 0 auto; place-items: center; width: 21px; height: 21px; border-radius: 50%;
      color: #fff; background: #3aa8f7; font-size: 13px; font-weight: 900; }
    .username { margin: 8px 0 0; color: #67bff9; font-size: 16px; overflow-wrap: anywhere; }
    .extra { margin: 7px 0 0; color: #91a3b5; font-size: 14px; }
    .description { margin: 22px auto 0; color: #c5d0da; font-size: 15px; line-height: 1.55; white-space: pre-line;
      overflow-wrap: anywhere; }
    .actions { display: grid; gap: 11px; margin-top: 28px; }
    .button { display: inline-flex; align-items: center; justify-content: center; min-height: 48px; padding: 0 20px; border-radius: 13px;
      font-size: 15px; font-weight: 720; text-decoration: none; transition: transform .16s ease, background .16s ease; }
    .button:hover { transform: translateY(-1px); }
    .primary { color: #fff; background: linear-gradient(135deg, #31a9f5, #168de2); box-shadow: 0 10px 28px rgba(22,141,226,.25); }
    .secondary { color: #a9dafa; background: rgba(72, 164, 226, .12); border: 1px solid rgba(89, 180, 241, .15); }
    footer { padding: 0 18px 26px; color: #657789; font-size: 12px; text-align: center; }
    @media (max-width: 480px) {
      .brand { margin-top: 20px; }
      main { padding: 24px 14px; align-items: start; }
      .card { padding: 28px 22px 24px; border-radius: 20px; }
      .avatar { width: 96px; height: 96px; }
    }
    @media (prefers-reduced-motion: reduce) { .button { transition: none; } }
  </style>
</head>
<body>
  <div class="shell">
    <a class="brand" href="{{.HomeURL}}" aria-label="{{.AppName}} home"><span class="brand-mark">{{.AppInitial}}</span><span>{{.AppName}}</span></a>
    <main>
      <article class="card">
        <div class="avatar">{{if .PhotoURL}}<img src="{{.PhotoURL}}" alt="{{.Title}} profile photo" width="112" height="112">{{else}}<span class="initials" aria-hidden="true">{{.Initials}}</span>{{end}}</div>
        <h1><span>{{.Title}}</span>{{if .Verified}}<span class="verified" title="Verified" aria-label="Verified">✓</span>{{end}}</h1>
        <p class="username">@{{.Username}}</p>
        {{if .Extra}}<p class="extra">{{.Extra}}</p>{{end}}
        <p class="description">{{.Description}}</p>
        <div class="actions">
          <a class="button primary" href="{{.AppURL}}">{{.ButtonLabel}}</a>
          <a class="button secondary" href="{{.WebURL}}">Open in Web</a>
        </div>
      </article>
    </main>
    <footer>If you have {{.AppName}}, this page can open the chat directly.</footer>
  </div>
  <script>window.setTimeout(function () { window.location.href = {{.AppURLJS}}; }, 250);</script>
</body>
</html>
`))

var sitePageTemplate = template.Must(template.New("site-page").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <link rel="canonical" href="{{.CanonicalURL}}">
  <meta name="description" content="{{.Description}}">
  <meta property="og:title" content="{{.Title}}">
  <meta property="og:site_name" content="{{.AppName}}">
  <meta property="og:description" content="{{.Description}}">
  <meta property="og:url" content="{{.CanonicalURL}}">
  <meta name="twitter:card" content="summary">
  <style>
    :root {
      color-scheme: light;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", Roboto, Helvetica, Arial, sans-serif;
      --blue: #229ed9;
      --blue-strong: #168ad4;
      --ink: #10233f;
      --text: #243b5a;
      --muted: #6d7e95;
      --line: #dce8f4;
      --soft: #f4f9fe;
      --white: #fff;
    }
    * { box-sizing: border-box; }
    body { margin: 0; min-width: 320px; color: var(--text); background: var(--white); -webkit-font-smoothing: antialiased; }
    a { color: inherit; }
    .site-header { position: sticky; top: 0; z-index: 5; background: rgba(255,255,255,.94); border-bottom: 1px solid var(--line); backdrop-filter: blur(14px); }
    .nav { width: min(100%, 1320px); min-height: 74px; margin: 0 auto; padding: 0 34px; display: flex; align-items: center; justify-content: space-between; gap: 24px; }
    .brand { display: inline-flex; align-items: center; gap: 12px; color: var(--ink); text-decoration: none; font-size: 28px; font-weight: 800; white-space: nowrap; }
    .brand-mark { width: 30px; height: 50px; display: block; object-fit: contain; }
    .nav-links { display: flex; align-items: center; justify-content: center; gap: 28px; color: var(--ink); font-size: 15px; font-weight: 700; }
    .nav-links a { min-height: 44px; display: inline-flex; align-items: center; color: var(--ink); text-decoration: none; border-bottom: 3px solid transparent; }
    .nav-links a.active, .nav-links a:hover, .nav-links a:focus-visible { color: var(--blue); border-bottom-color: var(--blue); outline: 0; }
    .nav-actions { display: inline-flex; align-items: center; gap: 14px; white-space: nowrap; }
    .lang { color: var(--muted); font-size: 14px; font-weight: 700; }
    .button { display: inline-flex; align-items: center; justify-content: center; min-height: 46px; padding: 0 22px; border: 1px solid var(--line); border-radius: 8px; text-decoration: none; font-size: 15px; font-weight: 800; color: var(--blue-strong); background: var(--white); box-shadow: 0 1px 2px rgba(16,35,63,.04); }
    .button.primary { border-color: var(--blue); color: #fff; background: var(--blue); box-shadow: 0 8px 18px rgba(34,158,217,.18); }
    .button:hover, .button:focus-visible { transform: translateY(-1px); outline: 0; }
    .hero { width: min(100%, 1120px); margin: 0 auto; padding: 66px 34px 52px; text-align: center; }
    .kicker { margin: 0 auto 16px; width: fit-content; padding: 7px 13px; border: 1px solid #cfe8fb; border-radius: 999px; color: var(--blue-strong); background: #f3faff; font-size: 14px; font-weight: 900; }
    h1 { margin: 0; color: var(--ink); font-size: clamp(44px, 6vw, 76px); line-height: 1.05; font-weight: 800; letter-spacing: 0; }
    .intro { margin: 18px auto 0; max-width: 760px; color: var(--muted); font-size: 18px; line-height: 1.75; }
    .hero-actions { margin-top: 30px; display: flex; align-items: center; justify-content: center; gap: 14px; flex-wrap: wrap; }
    .main { border-top: 1px solid var(--line); }
    .section { width: min(100%, 1120px); margin: 0 auto; padding: 46px 34px; border-bottom: 1px solid var(--line); }
    .section h2 { margin: 0; color: var(--ink); font-size: clamp(26px, 3vw, 36px); line-height: 1.25; }
    .section-intro { margin: 10px 0 0; color: var(--muted); font-size: 16px; line-height: 1.65; }
    .grid { margin-top: 26px; display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); border-top: 1px solid var(--line); border-left: 1px solid var(--line); }
    .item { min-height: 172px; padding: 26px; border-right: 1px solid var(--line); border-bottom: 1px solid var(--line); background: var(--white); }
    .item h3 { margin: 0; color: var(--ink); font-size: 19px; line-height: 1.35; }
    .item p { margin: 10px 0 0; color: var(--muted); font-size: 15px; line-height: 1.68; }
    .item a { display: inline-flex; margin-top: 14px; color: var(--blue-strong); text-decoration: none; font-weight: 800; }
    .band { background: var(--soft); }
    .footer { border-top: 1px solid var(--line); background: #fbfdff; }
    .footer-grid { width: min(100%, 1120px); margin: 0 auto; padding: 42px 34px; display: grid; grid-template-columns: minmax(220px, 2fr) repeat(5, minmax(100px, 1fr)); gap: 32px; color: var(--muted); font-size: 14px; line-height: 1.8; }
    .footer h3 { margin: 0 0 9px; color: var(--ink); font-size: 15px; }
    .footer a { display: block; color: var(--muted); text-decoration: none; }
    .footer a:hover { color: var(--blue); }
    .footer .brand { margin-bottom: 12px; font-size: 24px; }
    @media (max-width: 900px) {
      .nav { height: auto; padding: 14px 20px; align-items: flex-start; flex-wrap: wrap; }
      .brand { font-size: 24px; }
      .brand-mark { width: 26px; height: 43px; }
      .nav-links { order: 3; width: 100%; justify-content: flex-start; gap: 20px; overflow-x: auto; padding-top: 4px; font-size: 14px; }
      .nav-actions { margin-left: auto; }
      .hero { padding: 46px 20px 40px; }
      .intro { font-size: 16px; }
      .section { padding: 38px 20px; }
      .grid { grid-template-columns: 1fr; }
      .item { min-height: 0; padding: 22px 20px; }
      .footer-grid { grid-template-columns: 1fr 1fr; padding: 34px 20px; }
    }
    @media (max-width: 520px) {
      .nav-actions .lang { display: none; }
      .nav-actions .button { min-height: 40px; padding: 0 14px; font-size: 13px; }
      h1 { font-size: 42px; }
      .button { width: 100%; }
      .hero-actions { width: 100%; }
      .footer-grid { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body>
  <header class="site-header">
    <nav class="nav" aria-label="主导航">
      <a class="brand" href="/" aria-label="{{.AppName}} 首页"><img class="brand-mark" src="{{.MarkImageAttr}}" alt="" width="30" height="50"><span>{{.AppName}}</span></a>
      <div class="nav-links">
        <a href="/">首页</a>
        <a class="{{if eq .Active "faq"}}active{{end}}" href="/faq">FAQ</a>
        <a class="{{if eq .Active "apps"}}active{{end}}" href="/apps">Apps</a>
        <a class="{{if eq .Active "api"}}active{{end}}" href="/api">API</a>
        <a class="{{if eq .Active "safety"}}active{{end}}" href="/safety">Safety</a>
        <a class="{{if eq .Active "updates"}}active{{end}}" href="/updates">Updates</a>
      </div>
      <div class="nav-actions">
        <span class="lang">中文 / EN</span>
        <a class="button primary" href="{{.WebURLAttr}}">打开网页版</a>
      </div>
    </nav>
  </header>
  <main>
    <section class="hero">
      <p class="kicker">{{.Kicker}}</p>
      <h1>{{.Heading}}</h1>
      <p class="intro">{{.Intro}}</p>
      <div class="hero-actions">
        {{if .PrimaryText}}<a class="button primary" href="{{.PrimaryURL}}">{{.PrimaryText}}</a>{{end}}
        {{if .SecondaryText}}<a class="button" href="{{.SecondaryURL}}">{{.SecondaryText}}</a>{{end}}
      </div>
    </section>
    <div class="main">
      {{range .Sections}}
      <section class="section">
        <h2>{{.Title}}</h2>
        {{if .Intro}}<p class="section-intro">{{.Intro}}</p>{{end}}
        <div class="grid">
          {{range .Items}}
          <article class="item">
            <h3>{{.Title}}</h3>
            <p>{{.Body}}</p>
            {{if .LinkText}}<a href="{{.LinkURL}}">{{.LinkText}}</a>{{end}}
          </article>
          {{end}}
        </div>
      </section>
      {{end}}
    </div>
  </main>
  <footer class="footer">
    <div class="footer-grid">
      <div>
        <a class="brand" href="/"><img class="brand-mark" src="{{.MarkImageAttr}}" alt="" width="30" height="50"><span>{{.AppName}}</span></a>
        <p>安全、快速、同步的即时通讯。SafeLink keeps your own instance connected.</p>
        <p>© 2026 {{.AppName}}</p>
      </div>
      <div><h3>SafeLink</h3><a href="/faq">FAQ</a><a href="/safety">功能与安全</a><a href="/links">链接规则</a><a href="/updates">更新</a></div>
      <div><h3>客户端</h3><a href="/apps#ios">iOS</a><a href="/apps#android">Android</a><a href="/apps#macos">macOS</a><a href="/apps#windows">Windows</a><a href="{{.WebURLAttr}}">Web</a></div>
      <div><h3>平台</h3><a href="/api">API</a><a href="/translations">本地化</a><a href="/instantview">即时视图</a></div>
      <div><h3>支持</h3><a href="/support">帮助中心</a><a href="/privacy">隐私</a><a href="/terms">条款</a><a href="/press">媒体资料</a></div>
      <div><h3>连接</h3><a href="{{.CanonicalURL}}">www.safelink.chat</a><a href="{{.AppURLAttr}}">safelink://</a><a href="{{.WebURLAttr}}">网页版</a></div>
    </div>
  </footer>
</body>
</html>
`))
var usernameNotFoundTemplate = template.Must(template.New("username-not-found").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex,nofollow"><title>Username not found - {{.AppName}}</title>
<style>:root{color-scheme:dark;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}body{margin:0;min-height:100svh;display:grid;place-items:center;padding:24px;background:#0e1621;color:#f5f8fb}.card{width:min(100%,420px);padding:34px 28px;border:1px solid rgba(255,255,255,.08);border-radius:22px;background:#17212b;text-align:center}h1{margin:0 0 12px;font-size:26px}p{margin:0;color:#9fb0bf;line-height:1.55;overflow-wrap:anywhere}a{display:inline-block;margin-top:24px;color:#67bff9;text-decoration:none}</style>
</head><body><main class="card"><h1>Username not found</h1><p>{{if .Username}}@{{.Username}} is not an active public {{.AppName}} username.{{else}}This is not a valid public {{.AppName}} username.{{end}}</p><a href="{{.HomeURL}}">Back to {{.AppName}}</a></main></body></html>`))

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
      <a class="download" href="{{.AppURLAttr}}" data-open-app>Open</a>
    </header>
    <main>
      <section class="page">
        <div class="page-icon">{{if eq .Icon "stickers"}}S{{else if eq .Icon "invite"}}+{{else if eq .Icon "message"}}M{{else if eq .Icon "call"}}C{{else if eq .Icon "style"}}A{{else if eq .Icon "profile"}}@{{else if eq .Icon "folder"}}F{{else}}{{.SiteInitial}}{{end}}</div>
        <p class="kind">{{.KindLabel}}</p>
        <h1>{{.PageTitle}}</h1>
        {{if .Extra}}<p class="extra">{{.Extra}}</p>{{end}}
        <p class="desc">{{.Description}}</p>
        <a class="button" href="{{.AppURLAttr}}" data-open-app>{{.ActionText}}</a>
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
    (function () {
      var primaryUrl = {{.AppURLJS}};
      var compatUrl = {{.CompatAppURLJS}};
      var didLeave = false;
      var desktopBrowser = /Windows|Macintosh|Mac OS X|Linux|X11/i.test(navigator.userAgent || "") && !/Android|iPhone|iPad|iPod|Mobile/i.test(navigator.userAgent || "");
      var launchUrl = desktopBrowser && compatUrl ? compatUrl : primaryUrl;
      var fallbackUrl = desktopBrowser && compatUrl ? primaryUrl : compatUrl;
      function markLeave() { didLeave = true; }
      document.addEventListener("visibilitychange", function () {
        if (document.hidden) {
          markLeave();
        }
      });
      window.addEventListener("pagehide", markLeave);
      var attemptId = 0;
      function openLaunchWithFallback(fallbackDelay) {
        didLeave = false;
        var currentAttempt = ++attemptId;
        window.location.href = launchUrl;
        setTimeout(function () {
          if (fallbackUrl && currentAttempt === attemptId && !didLeave && !document.hidden) {
            window.location.href = fallbackUrl;
          }
        }, fallbackDelay);
      }
      var launchers = document.querySelectorAll("[data-open-app]");
      for (var i = 0; i < launchers.length; i++) {
        launchers[i].setAttribute("href", launchUrl);
        launchers[i].addEventListener("click", function (event) {
          event.preventDefault();
          openLaunchWithFallback(700);
        });
      }
      setTimeout(function () {
        openLaunchWithFallback(1200);
      }, 100);
    })();
  </script>
</body>
</html>
`))
