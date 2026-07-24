package rpc

import (
	"strings"
	"sync/atomic"
)

// Scam/fake profile warnings surfaced in the full-profile About text.
//
// Telegram Desktop only ships the SCAM/FAKE badge strings and renders no
// warning paragraph, while iOS/Android show a localized warning. To make the
// warning visible on every client, the server injects it into the projected
// getFullUser/getFullChannel About field. The stored bio is never modified.
const (
	defaultScamWarningUser    = "\u26A0\uFE0F Warning: Many users reported this account as a scam. Please be careful, especially if it asks you for money."
	defaultFakeWarningUser    = "\u26A0\uFE0F Warning: Many users reported that this account impersonates a famous person or organization."
	defaultScamWarningChannel = "\u26A0\uFE0F Warning: Many users reported this channel as a scam. Please be careful, especially if it asks you for money."
	defaultFakeWarningChannel = "\u26A0\uFE0F Warning: Many users reported that this channel impersonates a famous person or organization."
	defaultScamWarningGroup   = "\u26A0\uFE0F Warning: Many users reported this group as a scam. Please be careful, especially if it asks you for money."
	defaultFakeWarningGroup   = "\u26A0\uFE0F Warning: Many users reported that this group impersonates a famous person or organization."
)

var moderationWarningOverrides atomic.Pointer[moderationWarningConfig]

type moderationWarningConfig struct {
	scam string
	fake string
}

// SetModerationWarnings installs operator overrides for scam/fake warnings.
// Empty strings keep the built-in per-peer-type defaults.
func SetModerationWarnings(scam, fake string) {
	moderationWarningOverrides.Store(&moderationWarningConfig{
		scam: strings.TrimSpace(scam),
		fake: strings.TrimSpace(fake),
	})
}

func moderationOverride() moderationWarningConfig {
	if cfg := moderationWarningOverrides.Load(); cfg != nil {
		return *cfg
	}
	return moderationWarningConfig{}
}

func aboutWithModerationWarning(about, scamDefault, fakeDefault string, scam, fake bool) string {
	override := moderationOverride()
	warning := ""
	switch {
	case scam:
		if warning = override.scam; warning == "" {
			warning = scamDefault
		}
	case fake:
		if warning = override.fake; warning == "" {
			warning = fakeDefault
		}
	}
	if warning == "" {
		return about
	}
	if about = strings.TrimSpace(about); about == "" {
		return warning
	}
	return warning + "\n\n" + about
}
