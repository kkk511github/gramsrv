package rpc

import (
	"context"
	"testing"

	"github.com/gotd/td/clock"
	"go.uber.org/zap"

	"telesrv/internal/clientaddr"
	"telesrv/internal/domain"
)

func TestAuthzFromCtxCapturesClientIPAndIOSMetadata(t *testing.T) {
	key := [8]byte{1, 2, 3}
	r := New(Config{}, Deps{}, zap.NewNop(), clock.System)

	ctx := WithAuthKeyID(context.Background(), key)
	ctx = WithLayer(ctx, currentClientLayer)
	ctx = WithClientInfo(ctx, ClientInfo{
		APIID:         24547280,
		DeviceModel:   "iPhone 17 Pro Max",
		SystemVersion: "26.5.1",
		AppVersion:    "12.8 (33162)",
	})
	ctx = clientaddr.WithIP(ctx, "203.0.113.9")

	got := r.authzFromCtx(ctx)
	if got.AuthKeyID != key || got.Layer != currentClientLayer {
		t.Fatalf("auth ids = %x layer %d, want %x layer %d", got.AuthKeyID, got.Layer, key, currentClientLayer)
	}
	if got.IP != "203.0.113.9" {
		t.Fatalf("IP = %q, want real client IP", got.IP)
	}
	if got.Platform != string(ClientTypeIOS) {
		t.Fatalf("Platform = %q, want %q", got.Platform, ClientTypeIOS)
	}
}

func TestTGAuthorizationUsesRealIPAndResolvedLocation(t *testing.T) {
	key := [8]byte{9}
	r := New(Config{}, Deps{IPGeo: fakeIPGeo{loc: domain.IPLocation{Country: "江苏", Region: "南京"}, ok: true}}, zap.NewNop(), clock.System)
	got := r.tgAuthorization(context.Background(), domain.Authorization{
		AuthKeyID:     key,
		DeviceModel:   "iPhone 17 Pro Max",
		Platform:      string(ClientTypeIOS),
		SystemVersion: "26.5.1",
		AppVersion:    "12.8 (33162)",
		IP:            "198.51.100.23",
	}, key, 1700000000)

	if got.IP != "198.51.100.23" {
		t.Fatalf("IP = %q, want real client IP", got.IP)
	}
	if got.Country != "江苏" || got.Region != "南京" {
		t.Fatalf("location = country %q region %q, want 江苏/南京", got.Country, got.Region)
	}
	if got.AppName != "SafeLink iOS" {
		t.Fatalf("AppName = %q, want SafeLink iOS", got.AppName)
	}

	empty := r.tgAuthorization(context.Background(), domain.Authorization{AuthKeyID: key}, key, 1700000000)
	if empty.IP != "" || empty.Country != "Unknown" || empty.Region != "Unknown" {
		t.Fatalf("empty location = ip %q country %q region %q, want Unknown fallback", empty.IP, empty.Country, empty.Region)
	}
}

func TestRouterUpdatesAuthorizationIPFromContext(t *testing.T) {
	key := [8]byte{7}
	auth := &captureAuthService{authorizations: []domain.Authorization{{
		AuthKeyID: key,
		UserID:    1000000001,
	}}}
	r := New(Config{}, Deps{Auth: auth}, zap.NewNop(), clock.System)

	r.maybeUpdateAuthorizationIP(clientaddr.WithIP(context.Background(), "203.0.113.44"), key)

	if got := auth.authorizations[0].IP; got != "203.0.113.44" {
		t.Fatalf("stored IP = %q, want real client IP", got)
	}
}

type fakeIPGeo struct {
	loc domain.IPLocation
	ok  bool
	err error
}

func (f fakeIPGeo) LookupIPLocation(context.Context, string) (domain.IPLocation, bool, error) {
	return f.loc, f.ok, f.err
}
