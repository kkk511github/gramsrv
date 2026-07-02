package langpack

import (
	"strings"

	"telesrv/internal/brand"
	"telesrv/internal/domain"
)

// Branding rewrites visible language-pack values to the configured app brand.
// Keys are intentionally never rewritten because clients use them as stable IDs.
type Branding struct {
	AppName     string
	SourceNames []string
}

func newBrandReplacer(branding Branding) *strings.Replacer {
	appName := strings.TrimSpace(branding.AppName)
	if appName == "" {
		return nil
	}
	sourceNames := branding.SourceNames
	if len(sourceNames) == 0 {
		sourceNames = brand.SourceNames()
	}
	args := make([]string, 0, len(sourceNames)*2)
	seen := make(map[string]struct{}, len(sourceNames))
	for _, source := range sourceNames {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		if _, ok := seen[source]; ok {
			continue
		}
		seen[source] = struct{}{}
		args = append(args, source, appName)
	}
	if len(args) == 0 {
		return nil
	}
	return strings.NewReplacer(args...)
}

func applyBrandingToPack(pack *domain.LangPack, replacer *strings.Replacer) {
	if pack == nil || replacer == nil {
		return
	}
	for i := range pack.Strings {
		applyBrandingToString(&pack.Strings[i], replacer)
	}
}

func applyBrandingToString(item *domain.LangPackString, replacer *strings.Replacer) {
	item.Value = replacer.Replace(item.Value)
	item.ZeroValue = replacer.Replace(item.ZeroValue)
	item.OneValue = replacer.Replace(item.OneValue)
	item.TwoValue = replacer.Replace(item.TwoValue)
	item.FewValue = replacer.Replace(item.FewValue)
	item.ManyValue = replacer.Replace(item.ManyValue)
	item.OtherValue = replacer.Replace(item.OtherValue)
}
