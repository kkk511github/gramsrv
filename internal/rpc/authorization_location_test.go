package rpc

import (
	"context"
	"testing"
	"time"

	"telesrv/internal/clientaddr"
	"telesrv/internal/domain"
)

func TestAuthzFromCtxIncludesClientAddress(t *testing.T) {
	authKeyID := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	ctx := context.Background()
	ctx = WithAuthKeyID(ctx, authKeyID)
	ctx = WithLayer(ctx, 227)
	ctx = WithClientInfo(ctx, ClientInfo{
		APIID:         100,
		DeviceModel:   "SafeLink iOS",
		SystemVersion: "iOS 18",
		AppVersion:    "1.0",
	})
	ctx = clientaddr.WithContext(ctx, clientaddr.Info{
		IP:      "203.0.113.10",
		Country: "US",
		Region:  "California",
	})

	var r Router
	got := r.authzFromCtx(ctx)
	if got.AuthKeyID != authKeyID || got.Layer != 227 || got.IP != "203.0.113.10" || got.Country != "US" || got.Region != "California" {
		t.Fatalf("authzFromCtx = %+v, want auth key, layer and client location", got)
	}
}

func TestAuthorizationWithCurrentClientAddressBackfillsCurrentSession(t *testing.T) {
	current := [8]byte{9}
	other := [8]byte{8}
	ctx := clientaddr.WithContext(context.Background(), clientaddr.Info{
		IP:      "198.51.100.8",
		Country: "SG",
		Region:  "Singapore",
	})

	got := authorizationWithCurrentClientAddress(ctx, domain.Authorization{AuthKeyID: current}, current)
	if got.IP != "198.51.100.8" || got.Country != "SG" || got.Region != "Singapore" {
		t.Fatalf("current authorization = %+v, want client location", got)
	}

	unchanged := authorizationWithCurrentClientAddress(ctx, domain.Authorization{AuthKeyID: other}, current)
	if unchanged.IP != "" || unchanged.Country != "" || unchanged.Region != "" {
		t.Fatalf("other authorization changed = %+v", unchanged)
	}
}

func TestTGAuthorizationUsesStoredLocation(t *testing.T) {
	authKeyID := [8]byte{1}
	got := tgAuthorization(domain.Authorization{
		AuthKeyID: authKeyID,
		Hash:      77,
		IP:        "203.0.113.20",
		Country:   "US",
		Region:    "California",
		CreatedAt: time.Unix(10, 0),
		ActiveAt:  time.Unix(20, 0),
	}, authKeyID, 30)

	if !got.Current || got.IP != "203.0.113.20" || got.Country != "US" || got.Region != "California" {
		t.Fatalf("tgAuthorization = %+v, want stored location", got)
	}
}
