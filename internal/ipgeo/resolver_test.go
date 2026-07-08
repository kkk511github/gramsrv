package ipgeo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResolverFormatsChineseProvinceCity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"country":"中国","region":"中国江苏","city":"南京"}`))
	}))
	defer srv.Close()

	resolver := NewResolver(Options{Endpoint: srv.URL + "/{ip}", Timeout: time.Second})
	got, ok, err := resolver.LookupIPLocation(context.Background(), "36.152.44.95")
	if err != nil {
		t.Fatalf("LookupIPLocation: %v", err)
	}
	if !ok {
		t.Fatal("LookupIPLocation ok = false, want true")
	}
	if got.Country != "江苏" || got.Region != "南京" {
		t.Fatalf("location = %+v, want 江苏/南京", got)
	}
}

func TestResolverIgnoresPrivateIP(t *testing.T) {
	resolver := NewResolver(Options{})
	got, ok, err := resolver.LookupIPLocation(context.Background(), "192.168.1.9")
	if err != nil {
		t.Fatalf("LookupIPLocation private: %v", err)
	}
	if ok || got.Country != "" || got.Region != "" {
		t.Fatalf("private location = %+v ok %v, want empty false", got, ok)
	}
}
