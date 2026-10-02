package main

import (
	"net/http"
	"strings"

	"telesrv/internal/store"
	"telesrv/internal/store/postgres"
)

func (s *server) handleLoginPolicyAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	policy, err := postgres.NewUserStore(s.read.pool).LoginPolicy(r.Context())
	if err != nil {
		writeAPIError(w, 500, "无法读取登录设置")
		return
	}
	writeJSON(w, 200, policy)
}

func (s *server) handleLoginPolicyActionAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var body struct {
		CommandID                    string `json:"command_id"`
		Reason                       string `json:"reason"`
		Confirm                      bool   `json:"confirm"`
		FutureAuthEnabled            *bool  `json:"future_auth_enabled"`
		FutureAuthDays               int    `json:"future_auth_days"`
		RegistrationPasswordRequired *bool  `json:"registration_password_required"`
	}
	if !decodeAction(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Reason) == "" || body.FutureAuthEnabled == nil || body.RegistrationPasswordRequired == nil {
		writeAPIError(w, 400, "请填写操作原因及完整登录设置")
		return
	}
	policy := store.LoginPolicy{FutureAuthEnabled: *body.FutureAuthEnabled, FutureAuthDays: body.FutureAuthDays, RegistrationPasswordRequired: *body.RegistrationPasswordRequired}
	if err := policy.Validate(); err != nil {
		writeAPIError(w, 400, err.Error())
		return
	}
	meta := s.commandMetaFromAPI(r, body.CommandID, body.Reason, body.Confirm, "login-policy")
	details := map[string]any{"future_auth_enabled": policy.FutureAuthEnabled, "future_auth_days": policy.FutureAuthDays, "registration_password_required": policy.RegistrationPasswordRequired}
	message := "校验通过；关闭会撤销已有快捷登录凭证，缩短有效期会同步限制已有凭证，延长只适用于新凭证"
	var err error
	if !meta.DryRun {
		err = postgres.NewUserStore(s.read.pool).SaveLoginPolicy(r.Context(), policy)
		message = "登录设置已保存，立即生效"
		if err != nil {
			message = "登录设置保存失败"
		}
	}
	writeJSON(w, 200, serverCommandResult(meta, "server.login_policy", err, message, details))
}
