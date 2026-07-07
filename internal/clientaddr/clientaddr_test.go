package clientaddr

import (
	"net/http"
	"testing"
)

func TestFromHTTPPrefersForwardedClientIP(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/apiws", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.RemoteAddr = "127.0.0.1:51420"
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 10.0.0.2")
	req.Header.Set("CF-IPCountry", "US")
	req.Header.Set("X-Geo-Region", "California")

	got := FromHTTP(req)
	if got.IP != "203.0.113.10" || got.Country != "US" || got.Region != "California" {
		t.Fatalf("FromHTTP = %+v, want forwarded IP with geo headers", got)
	}
}

func TestFromHTTPFallsBackToRemoteAddr(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/apiws", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.RemoteAddr = "10.0.0.8:4141"

	got := FromHTTP(req)
	if got.IP != "10.0.0.8" || got.Country != "Local Network" {
		t.Fatalf("FromHTTP = %+v, want local remote address", got)
	}
}

func TestFromRemoteAddrNormalizesIPv6MappedIPv4(t *testing.T) {
	got := FromRemoteAddr("[::ffff:192.0.2.9]:443")
	if got.IP != "192.0.2.9" {
		t.Fatalf("FromRemoteAddr IP = %q, want mapped IPv4", got.IP)
	}
}
