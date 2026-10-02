package help

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
)

func TestAppConfigAllowsSafeLinkWebNavigation(t *testing.T) {
	for _, mapbox := range []string{"", "test-map-token"} {
		cfg := defaultAppConfig(mapbox)
		var value struct {
			Protocols []string `json:"web_app_allowed_protocols"`
		}
		if err := json.Unmarshal(cfg.JSON, &value); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(value.Protocols, []string{"http", "https", "safelink"}) {
			t.Fatalf("unexpected app navigation protocols: %v", value.Protocols)
		}
	}
	cfg, unchanged, err := (*Service)(nil).GetAppConfig(context.Background(), 0, 30)
	if err != nil || unchanged || cfg.Hash <= 30 {
		t.Fatalf("old clients must receive refreshed protocols: hash=%d unchanged=%v err=%v", cfg.Hash, unchanged, err)
	}
}
