package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistrationInviteActionValidation(t *testing.T) {
	s := &server{}
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"shared dry run", `{"reason":"test","operation":"generate","count":1,"max_uses":100,"days":30,"code":"01234"}`, 200},
		{"batch dry run", `{"reason":"test","operation":"generate","count":10,"max_uses":1,"days":7}`, 200},
		{"policy dry run", `{"reason":"test","operation":"policy","enabled":true}`, 200},
		{"fixed batch", `{"reason":"test","operation":"generate","count":2,"max_uses":100,"days":30,"code":"SAFELINK-TEST"}`, 400},
		{"short code", `{"reason":"test","operation":"generate","count":1,"max_uses":100,"days":30,"code":"1234"}`, 400},
		{"non numeric", `{"reason":"test","operation":"generate","count":1,"max_uses":100,"days":30,"code":"12A45"}`, 400},
		{"missing reason", `{"operation":"policy"}`, 400},
		{"unlimited uses", `{"reason":"test","operation":"generate","count":1,"max_uses":0,"days":30}`, 400},
		{"invalid operation", `{"reason":"test","operation":"unknown"}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			s.handleRegistrationInviteActionAPI(w, httptest.NewRequest(http.MethodPost, "/api/actions/registration-invites", strings.NewReader(tc.body)))
			if w.Code != tc.status {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
			if tc.status == 200 {
				var result struct {
					DryRun bool `json:"dry_run"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || !result.DryRun {
					t.Fatalf("not dry-run: %s", w.Body.String())
				}
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("invite responses must not be cached")
			}
		})
	}
}
