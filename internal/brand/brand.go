package brand

import (
	"net/url"
	"strconv"
	"strings"
)

const (
	DefaultAppName       = "SafeLink"
	DefaultAppScheme     = "safelink"
	DefaultPublicHost    = "safelink.chat"
	DefaultPublicBaseURL = "https://" + DefaultPublicHost
)

func SourceNames() []string {
	return []string{
		"Telegram",
		"TELEGRAM",
		"telegram",
		"Tidings",
		"TIDINGS",
		"tidings",
		"Tiding",
		"TIDING",
		"tiding",
		"Telesrv",
		"TELESRV",
		"telesrv",
		"SafeLink",
		"SAFELINK",
		"safelink",
		"Safelink",
	}
}

func SourceReplacements(appName string) []string {
	appName = strings.TrimSpace(appName)
	if appName == "" {
		appName = DefaultAppName
	}
	lowerAppName := strings.ToLower(appName)
	return []string{
		"translations.telegram.org",
		DefaultPublicHost,
		"translations.tidings.org",
		DefaultPublicHost,
		"getdesktop.telegram.org",
		DefaultPublicHost,
		"desktop.telegram.org",
		DefaultPublicHost,
		"web.telegram.org",
		DefaultPublicHost,
		"ads.telegram.org",
		DefaultPublicHost,
		"core.telegram.org",
		DefaultPublicHost,
		"core.tidings.org",
		DefaultPublicHost,
		"telegram.org",
		DefaultPublicHost,
		"telegram.me",
		DefaultPublicHost,
		"tidings.org",
		DefaultPublicHost,
		"tidings.me",
		DefaultPublicHost,
		"https://t.me",
		DefaultPublicBaseURL,
		"http://t.me",
		DefaultPublicBaseURL,
		"t.me",
		DefaultPublicHost,
		"tg://",
		DefaultAppScheme + "://",
		"Telegram",
		appName,
		"TELEGRAM",
		appName,
		"telegram",
		lowerAppName,
		"Tidings",
		appName,
		"TIDINGS",
		appName,
		"tidings",
		lowerAppName,
		"Tiding",
		appName,
		"TIDING",
		appName,
		"tiding",
		lowerAppName,
		"Telesrv",
		appName,
		"TELESRV",
		appName,
		"telesrv",
		lowerAppName,
		"SafeLink",
		appName,
		"SAFELINK",
		appName,
		"safelink",
		lowerAppName,
		"Safelink",
		appName,
	}
}

func PublicURL(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return DefaultPublicBaseURL
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return DefaultPublicBaseURL + path
}

func PublicURLPrefix() string {
	return DefaultPublicBaseURL + "/"
}

func UserURL(username string) string {
	username = strings.Trim(strings.TrimSpace(username), " /@")
	if username == "" {
		return ""
	}
	return PublicURL(url.PathEscape(username))
}

func InviteURL(hash string) string {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return ""
	}
	return PublicURL("/+" + hash)
}

func ChannelMessageURL(username string, messageID int) string {
	username = strings.Trim(strings.TrimSpace(username), " /@")
	if username == "" || messageID <= 0 {
		return ""
	}
	return PublicURL(url.PathEscape(username) + "/" + strconv.Itoa(messageID))
}

func PrivateChannelMessageURL(channelID int64, messageID int) string {
	if channelID == 0 || messageID <= 0 {
		return ""
	}
	return PublicURL("c/" + strconv.FormatInt(channelID, 10) + "/" + strconv.Itoa(messageID))
}

func BusinessChatURL(slug string) string {
	slug = strings.Trim(strings.TrimSpace(slug), " /")
	if slug == "" {
		return ""
	}
	return PublicURL("m/" + url.PathEscape(slug))
}

func StickerSetURL(kind, shortName string) string {
	kind = strings.Trim(strings.TrimSpace(kind), " /")
	shortName = strings.Trim(strings.TrimSpace(shortName), " /")
	if kind == "" || shortName == "" {
		return ""
	}
	return PublicURL(kind + "/" + url.PathEscape(shortName))
}

func GroupCallInviteURL(slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	escaped := url.PathEscape(slug)
	return PublicURL("call/" + escaped + "?slug=" + url.QueryEscape(slug))
}

func StoryURL(peerType string, peerID int64, storyID int) string {
	peerType = strings.Trim(strings.TrimSpace(peerType), " /")
	if peerType == "" || peerID == 0 || storyID <= 0 {
		return ""
	}
	return PublicURL("story/" + url.PathEscape(peerType) + "/" + strconv.FormatInt(peerID, 10) + "/" + strconv.Itoa(storyID))
}

func BoostURL(channelID int64) string {
	if channelID == 0 {
		return ""
	}
	return PublicURL("boost?c=" + strconv.FormatInt(channelID, 10))
}

func KnownPublicHost(host string) bool {
	switch strings.Trim(strings.ToLower(host), ".") {
	case DefaultPublicHost:
		return true
	default:
		return false
	}
}
