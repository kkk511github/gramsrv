package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginPolicyActionValidation(t *testing.T) {
	s := &server{}
	for _, days := range []int{0, 1, 7, 30, 90, 365, 366, -1} {
		r := httptest.NewRequest("POST", "/api/actions/login-policy", strings.NewReader(fmt.Sprintf(`{"reason":"test","future_auth_enabled":true,"future_auth_days":%d,"registration_password_required":true}`, days)))
		w := httptest.NewRecorder()
		s.handleLoginPolicyActionAPI(w, r)
		if days < 1 || days > 365 {
			if w.Code != 400 {
				t.Fatalf("invalid %d: %s", days, w.Body.String())
			}
			continue
		}
		var result struct {
			DryRun bool `json:"dry_run"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || !result.DryRun {
			t.Fatalf("dry run %d: %s", days, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	s.handleLoginPolicyActionAPI(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"reason":"test","future_auth_days":30}`)))
	if w.Code != 400 {
		t.Fatal("omitted enabled flag could revoke tokens")
	}
}
