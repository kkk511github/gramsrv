package main

import (
	"net/http"
	"telesrv/internal/linksettings"
)

func (s *server) handleServerDomainsAPI(w http.ResponseWriter, r *http.Request) {
	saved, err := linksettings.Read(s.cfg.PublicLinkSettingsFile)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	settings := linksettings.Settings{PublicURL: s.cfg.PublicBaseURL, WebURL: s.cfg.PublicWebBaseURL}
	if saved != nil {
		settings = *saved
	}
	writeJSON(w, http.StatusOK, struct {
		linksettings.Settings
		Editable      bool `json:"editable"`
		OverrideSaved bool `json:"override_saved"`
	}{settings, s.cfg.PublicLinkSettingsFile != "", saved != nil})
}

func (s *server) handleSetServerDomainsAPI(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CommandID string `json:"command_id"`
		Reason    string `json:"reason"`
		Confirm   bool   `json:"confirm"`
		linksettings.Settings
	}
	if !decodeAction(w, r, &body) {
		return
	}
	if s.cfg.PublicLinkSettingsFile == "" {
		writeAPIError(w, http.StatusConflict, "当前部署尚未启用后台域名配置")
		return
	}
	settings, err := body.Settings.Validate()
	if err != nil {
		writeAPIError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	meta := s.commandMetaFromAPI(r, body.CommandID, body.Reason, body.Confirm, "set-server-domains")
	details := map[string]any{"public_url": settings.PublicURL, "web_url": settings.WebURL, "app_scheme": "safelink", "restart_required": true, "dns_tls_verified": false}
	message := "域名格式校验通过；尚未检查 DNS 和 HTTPS 部署"
	if !meta.DryRun {
		err = linksettings.Save(s.cfg.PublicLinkSettingsFile, settings)
		message = "配置已保存，重启 SafeLink 服务后生效；DNS、HTTPS 证书及反向代理需单独配置"
		if err != nil {
			message = "域名配置保存失败，原配置保持不变"
		}
	}
	writeJSON(w, http.StatusOK, serverCommandResult(meta, "server.set_domains", err, message, details))
}
