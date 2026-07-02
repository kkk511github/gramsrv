package langpack

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"telesrv/internal/domain"
)

// SeedDirectory 将导出的 .strings 文件导入 LangPackStore。
// root 可直接指向 data/langpack，也可指向包含 .strings 的具体平台目录。
func (s *Service) SeedDirectory(ctx context.Context, root string) (int, error) {
	if s == nil || s.packs == nil || root == "" {
		return 0, nil
	}
	dir := filepath.Clean(root)
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("stat langpack seed dir: %w", err)
	}

	seeded := 0
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".strings") {
			return nil
		}
		pack, err := ParseTDesktopFile(path)
		if err != nil {
			return err
		}
		applyBrandingToPack(&pack, s.brandReplacer)
		existing, err := s.packs.GetPack(ctx, pack.LangPack, pack.LangCode, 0)
		if err != nil {
			return err
		}
		if existing.Version >= pack.Version {
			if langPackStringsEqual(existing.Strings, pack.Strings) {
				return nil
			}
			pack.Version = existing.Version + 1
		}
		if err := s.packs.UpsertPack(ctx, pack); err != nil {
			return err
		}
		seeded += len(pack.Strings)
		return nil
	})
	if err != nil {
		return seeded, fmt.Errorf("walk langpack seed dir: %w", err)
	}
	return seeded, nil
}

func langPackStringsEqual(a, b []domain.LangPackString) bool {
	if len(a) != len(b) {
		return false
	}
	byKey := make(map[string]domain.LangPackString, len(a))
	for _, item := range a {
		byKey[item.Key] = item
	}
	for _, item := range b {
		if existing, ok := byKey[item.Key]; !ok || !langPackStringEqual(existing, item) {
			return false
		}
	}
	return true
}

func langPackStringEqual(a, b domain.LangPackString) bool {
	return a.Key == b.Key &&
		a.Value == b.Value &&
		a.Pluralized == b.Pluralized &&
		a.ZeroValue == b.ZeroValue &&
		a.OneValue == b.OneValue &&
		a.TwoValue == b.TwoValue &&
		a.FewValue == b.FewValue &&
		a.ManyValue == b.ManyValue &&
		a.OtherValue == b.OtherValue &&
		a.Deleted == b.Deleted
}
