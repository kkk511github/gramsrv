package rpc

import (
	"context"
	"testing"

	"github.com/iamxvbaba/td/clock"
	"go.uber.org/zap"

	"telesrv/internal/clientaddr"
	"telesrv/internal/domain"
	"telesrv/internal/geoip"
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

func TestTGAuthorizationCurrentDeviceHasZeroHash(t *testing.T) {
	devices := []domain.Authorization{
		{AuthKeyID: [8]byte{1}, Hash: 101},
		{AuthKeyID: [8]byte{2}, Hash: 202},
	}
	for _, current := range devices {
		for _, device := range devices {
			got := tgAuthorization(device, current.AuthKeyID, 1700000000, domain.IPLocation{})
			isCurrent := device.AuthKeyID == current.AuthKeyID
			wantHash := device.Hash
			if isCurrent {
				wantHash = 0
			}
			if got.Current != isCurrent || got.Hash != wantHash {
				t.Fatalf("device %x viewed by %x: current=%v hash=%d, want %v/%d",
					device.AuthKeyID, current.AuthKeyID, got.Current, got.Hash, isCurrent, wantHash)
			}
		}
	}
	if devices[0].Hash != 101 || devices[1].Hash != 202 {
		t.Fatal("response conversion changed stored authorization hashes")
	}
}

func TestAccountAuthorizationsKeepSafeLinkLocationAndCurrentDevice(t *testing.T) {
	key := [8]byte{1}
	resolver := &recordingIPGeo{location: domain.IPLocation{Country: "江苏", Region: "南京"}}
	r := New(Config{}, Deps{
		Auth: &captureAuthService{authorizations: []domain.Authorization{
			{AuthKeyID: key, Hash: 101, IP: "198.51.100.23", Platform: "ios"},
			{AuthKeyID: [8]byte{2}, Hash: 202, IP: "198.51.100.23", Platform: "android"},
		}},
		IPGeo: resolver,
	}, zap.NewNop(), clock.System)
	ctx := WithAuthKeyID(WithUserID(context.Background(), 42), key)
	out, err := r.onAccountGetAuthorizations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Authorizations) != 2 || resolver.calls != 1 {
		t.Fatalf("authorizations=%d location lookups=%d, want 2/1", len(out.Authorizations), resolver.calls)
	}
	for _, item := range out.Authorizations {
		if item.Country != "江苏" || item.Region != "南京" || item.IP != "198.51.100.23" {
			t.Fatalf("location changed: %+v", item)
		}
	}
	current := out.Authorizations[0]
	if !current.Current || current.Hash != 0 || current.AppName != "SafeLink iOS" {
		t.Fatalf("current SafeLink iOS device changed: %+v", current)
	}
	if out.Authorizations[1].Current || out.Authorizations[1].Hash != 202 {
		t.Fatalf("other device changed: %+v", out.Authorizations[1])
	}
}

func TestConfiguredGeoIPDoesNotFallBackToLegacyProvider(t *testing.T) {
	legacy := &recordingIPGeo{location: domain.IPLocation{Country: "江苏", Region: "南京"}}
	configured := &stubGeoIPResolver{locations: map[string]geoip.Location{
		"198.51.100.23": {Country: "Japan", Region: "Tokyo"},
	}}
	r := New(Config{}, Deps{IPGeo: legacy, GeoIP: configured}, zap.NewNop(), clock.System)
	for ip, want := range map[string]string{"198.51.100.23": "Japan", "203.0.113.7": "Unknown"} {
		got := r.tgAuthorization(context.Background(), domain.Authorization{IP: ip, Platform: "ios"}, [8]byte{}, 1700000000)
		if got.Country != want || got.AppName != "SafeLink iOS" || got.Hash != 0 {
			t.Fatalf("configured resolver response: %+v, want country %q", got, want)
		}
	}
	if legacy.calls != 0 {
		t.Fatalf("legacy provider calls=%d, want 0 when GeoIP is explicitly configured", legacy.calls)
	}
}

type recordingIPGeo struct {
	location domain.IPLocation
	calls    int
}

func (r *recordingIPGeo) LookupIPLocation(context.Context, string) (domain.IPLocation, bool, error) {
	r.calls++
	return r.location, true, nil
}

type fakeIPGeo struct {
	loc domain.IPLocation
	ok  bool
	err error
}

func (f fakeIPGeo) LookupIPLocation(context.Context, string) (domain.IPLocation, bool, error) {
	return f.loc, f.ok, f.err
}
