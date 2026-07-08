package clientaddr

import (
	"context"
	"net"
	"strings"
)

type ipKey struct{}

func WithIP(ctx context.Context, ip string) context.Context {
	ip = NormalizeIP(ip)
	if ip == "" {
		return ctx
	}
	return context.WithValue(ctx, ipKey{}, ip)
}

func IPFrom(ctx context.Context) (string, bool) {
	ip, ok := ctx.Value(ipKey{}).(string)
	if !ok || ip == "" {
		return "", false
	}
	return ip, true
}

func IPFromAddr(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	if tcp, ok := addr.(*net.TCPAddr); ok && tcp.IP != nil {
		return NormalizeIP(tcp.IP.String())
	}
	if udp, ok := addr.(*net.UDPAddr); ok && udp.IP != nil {
		return NormalizeIP(udp.IP.String())
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		host = addr.String()
	}
	return NormalizeIP(host)
}

func NormalizeIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	return parsed.String()
}
