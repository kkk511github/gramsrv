package langpack

import (
	"context"
	"strings"

	"telesrv/internal/domain"
	"telesrv/internal/store"
)

// Service 提供客户端语言包查询。
type Service struct {
	packs         store.LangPackStore
	brandReplacer *strings.Replacer
}

type Option func(*Service)

// NewService 创建 langpack 服务。
func NewService(packs store.LangPackStore, opts ...Option) *Service {
	s := &Service{packs: packs}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

func WithBranding(branding Branding) Option {
	return func(s *Service) {
		s.brandReplacer = newBrandReplacer(branding)
	}
}

// GetLangPack 返回完整语言包。
func (s *Service) GetLangPack(ctx context.Context, langPack, langCode string) (domain.LangPack, error) {
	return s.GetDifference(ctx, langPack, langCode, 0)
}

// GetDifference 返回从 fromVersion 到当前版本的语言包差异。
func (s *Service) GetDifference(ctx context.Context, langPack, langCode string, fromVersion int) (domain.LangPack, error) {
	if s == nil || s.packs == nil {
		return domain.LangPack{LangPack: langPack, LangCode: langCode, FromVersion: fromVersion}, nil
	}
	return s.packs.GetPack(ctx, normalizePack(langPack), normalizeCode(langCode), fromVersion)
}

// GetStrings 返回指定 key 的语言包字符串。
func (s *Service) GetStrings(ctx context.Context, langPack, langCode string, keys []string) (domain.LangPack, error) {
	if s == nil || s.packs == nil {
		return domain.LangPack{LangPack: langPack, LangCode: langCode}, nil
	}
	return s.packs.GetStrings(ctx, normalizePack(langPack), normalizeCode(langCode), keys)
}

func normalizePack(langPack string) string {
	if langPack == "" {
		return "tdesktop"
	}
	return langPack
}

func normalizeCode(langCode string) string {
	if langCode == "" {
		return "en"
	}
	return langCode
}
