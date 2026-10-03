package main

import (
	"net/http"
	"strings"
	"time"

	"telesrv/internal/store/postgres"
)

func (s *server) handleRegistrationInvitesAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	enabled, items, err := postgres.NewUserStore(s.read.pool).RegistrationInvites(r.Context())
	if err != nil {
		writeAPIError(w, 500, "无法读取注册邀请码")
		return
	}
	writeJSON(w, 200, map[string]any{"enabled": enabled, "items": items})
}

func (s *server) handleRegistrationInviteActionAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var body struct {
		CommandID string    `json:"command_id"`
		Reason    string    `json:"reason"`
		Confirm   bool      `json:"confirm"`
		Operation string    `json:"operation"`
		Enabled   bool      `json:"enabled"`
		ID        int64     `json:"id"`
		Count     int       `json:"count"`
		MaxUses   int       `json:"max_uses"`
		Days      int       `json:"days"`
		Code      string    `json:"code"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if !decodeAction(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Reason) == "" {
		writeAPIError(w, 400, "请填写操作原因")
		return
	}
	switch body.Operation {
	case "policy":
	case "generate":
		if body.ExpiresAt.IsZero() && body.Days >= 1 && body.Days <= 365 {
			body.ExpiresAt = time.Now().Add(time.Duration(body.Days) * 24 * time.Hour)
		}
		if err := postgres.ValidateRegistrationInviteOptions(body.Count, body.MaxUses, body.ExpiresAt, body.Code); err != nil {
			writeAPIError(w, 400, err.Error())
			return
		}
	case "disable":
		if body.ID <= 0 {
			writeAPIError(w, 400, "邀请码不存在")
			return
		}
	default:
		writeAPIError(w, 400, "不支持的操作")
		return
	}
	meta := s.commandMetaFromAPI(r, body.CommandID, body.Reason, body.Confirm, "registration-invite-"+body.Operation)
	details := map[string]any{"operation": body.Operation}
	message := "校验通过，等待确认"
	var err error
	if !meta.DryRun {
		invites := postgres.NewUserStore(s.read.pool)
		switch body.Operation {
		case "policy":
			err = invites.SetRegistrationInviteRequired(r.Context(), body.Enabled)
			message = "注册限制已更新，立即生效"
			details["enabled"] = body.Enabled
		case "disable":
			err = invites.DisableRegistrationInvite(r.Context(), body.ID)
			message = "邀请码已停用"
			details["id"] = body.ID
		case "generate":
			var codes []string
			codes, err = invites.CreateRegistrationInvites(r.Context(), body.Count, body.MaxUses, body.ExpiresAt, body.Code)
			if err == nil {
				details["codes"] = strings.Join(codes, "\n")
			}
			message = "5 位邀请码已生成，可在列表中随时查看和复制"
		}
	}
	writeJSON(w, 200, serverCommandResult(meta, "server.registration_invites", err, message, details))
}
