package clientaddr

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type ctxKey struct{}

// Info carries client network metadata from the edge layer into RPC handlers.
type Info struct {
	IP      string
	Country string
	Region  string
}

// Provider is implemented by wrapped net.Conn values that can expose metadata
// richer than RemoteAddr, such as WebSocket connections behind a reverse proxy.
type Provider interface {
	ClientAddressInfo() Info
}

func WithContext(ctx context.Context, info Info) context.Context {
	info = Normalize(info)
	if info == (Info{}) {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, info)
}

func FromContext(ctx context.Context) (Info, bool) {
	info, ok := ctx.Value(ctxKey{}).(Info)
	if !ok || info == (Info{}) {
		return Info{}, false
	}
	return info, true
}

func FromProvider(conn any) (Info, bool) {
	p, ok := conn.(Provider)
	if !ok {
		return Info{}, false
	}
	info := Normalize(p.ClientAddressInfo())
	return info, info != (Info{})
}

func FromNetAddr(addr net.Addr) Info {
	if addr == nil {
		return Info{}
	}
	return FromRemoteAddr(addr.String())
}

func FromRemoteAddr(remote string) Info {
	return Normalize(Info{IP: hostFromRemoteAddr(remote)})
}

func FromHTTP(r *http.Request) Info {
	if r == nil {
		return Info{}
	}
	info := FromRemoteAddr(r.RemoteAddr)
	if ip := firstHeaderIP(r.Header); ip != "" {
		info.IP = ip
	}
	if country := firstHeaderValue(r.Header,
		"CF-IPCountry",
		"X-Country-Code",
		"X-Geo-Country",
		"X-Forwarded-Country",
		"X-Client-Country",
	); country != "" && !strings.EqualFold(country, "XX") {
		info.Country = country
	}
	if region := firstHeaderValue(r.Header,
		"CF-Region",
		"X-Region",
		"X-Geo-Region",
		"X-Forwarded-Region",
		"X-Client-Region",
	); region != "" {
		info.Region = region
	}
	return Normalize(info)
}

func Normalize(info Info) Info {
	info.IP = normalizeIP(info.IP)
	info.Country = strings.TrimSpace(info.Country)
	info.Region = strings.TrimSpace(info.Region)
	if info.IP == "" {
		info.Country = ""
		info.Region = ""
		return info
	}
	if info.Country == "" && isLocalIP(info.IP) {
		info.Country = "Local Network"
	}
	return info
}

func firstHeaderIP(h http.Header) string {
	for _, name := range []string{"CF-Connecting-IP", "True-Client-IP", "X-Real-IP", "X-Forwarded-For"} {
		for _, raw := range h.Values(name) {
			for _, part := range strings.Split(raw, ",") {
				if ip := normalizeIP(part); ip != "" {
					return ip
				}
			}
		}
	}
	for _, raw := range h.Values("Forwarded") {
		if ip := forwardedForIP(raw); ip != "" {
			return ip
		}
	}
	return ""
}

func forwardedForIP(raw string) string {
	for _, part := range strings.Split(raw, ",") {
		for _, pair := range strings.Split(part, ";") {
			k, v, ok := strings.Cut(strings.TrimSpace(pair), "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(k), "for") {
				continue
			}
			if ip := normalizeIP(v); ip != "" {
				return ip
			}
		}
	}
	return ""
}

func firstHeaderValue(h http.Header, names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(h.Get(name)); v != "" {
			return v
		}
	}
	return ""
}

func hostFromRemoteAddr(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(remote); err == nil {
		return host
	}
	return remote
}

func normalizeIP(raw string) string {
	raw = strings.Trim(strings.TrimSpace(raw), `"`)
	raw = strings.TrimPrefix(raw, "for=")
	if raw == "" || strings.EqualFold(raw, "unknown") {
		return ""
	}
	if host, _, err := net.SplitHostPort(raw); err == nil {
		raw = host
	}
	raw = strings.Trim(raw, "[]")
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return ""
	}
	return addr.Unmap().String()
}

func isLocalIP(ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast()
}
