package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"telesrv/internal/admin"
	"telesrv/internal/procctl"
)

func TestUpdateServerEnvUsesSafeLinkBrand(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte("TELESRV_BRAND_PRODUCT_NAME=SafeLink\n"), 0600); err != nil {
		t.Fatal(err)
	}
	srv := &server{envCtl: procctl.NewManager(root)}
	for _, confirm := range []bool{false, true} {
		body, err := json.Marshal(map[string]any{
			"reason": "brand test", "confirm": confirm,
			"values": map[string]string{"TELESRV_BRAND_PRODUCT_NAME": "SafeLink"},
		})
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		srv.handleUpdateServerEnvAPI(rec, httptest.NewRequest(http.MethodPost, "/api/actions/update-server-env", strings.NewReader(string(body))))
		var result admin.CommandResult
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK || result.Status != "completed" || result.Error != "" || result.DryRun != !confirm {
			t.Fatalf("confirm=%v status=%d result=%+v", confirm, rec.Code, result)
		}
		if !strings.Contains(result.Message, "SafeLink") || strings.Contains(strings.ToLower(result.Message), "telesrv") {
			t.Fatalf("unbranded save message: %q", result.Message)
		}
	}
}
