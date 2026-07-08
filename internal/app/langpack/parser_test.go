package langpack

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"telesrv/internal/domain"
	"telesrv/internal/store/memory"
)

func TestParseTDesktopFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tdesktop_en_v42.strings")
	if err := os.WriteFile(path, []byte(`
"lng_plain" = "Plain value";
"lng_escape" = "Line\nTwo";
"lng_items#one" = "{count} item";
"lng_items#other" = "{count} items";
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	pack, err := ParseTDesktopFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if pack.LangPack != "tdesktop" || pack.LangCode != "en" || pack.Version != 42 {
		t.Fatalf("pack meta = %+v", pack)
	}
	if len(pack.Strings) != 3 {
		t.Fatalf("strings count = %d, want 3", len(pack.Strings))
	}
	if got := pack.Strings[1].Value; got != "Line\nTwo" {
		t.Fatalf("escape value = %q", got)
	}
	plural := pack.Strings[2]
	if !plural.Pluralized || plural.Key != "lng_items" || plural.OneValue == "" || plural.OtherValue == "" {
		t.Fatalf("plural string = %+v", plural)
	}
}

func TestParseClientLangPackFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "weba_en_v12000000.strings")
	if err := os.WriteFile(path, []byte(`
"NewMessageTitle" = "New Message";
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	pack, err := ParseTDesktopFile(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if pack.LangPack != "weba" || pack.LangCode != "en" || pack.Version != 12000000 {
		t.Fatalf("pack meta = %+v", pack)
	}
	if len(pack.Strings) != 1 || pack.Strings[0].Key != "NewMessageTitle" {
		t.Fatalf("strings = %+v", pack.Strings)
	}
}

func TestSeedDirectoryWalksClientSubdirs(t *testing.T) {
	root := t.TempDir()
	for _, item := range []struct {
		dir  string
		file string
		key  string
	}{
		{dir: "tdesktop", file: "tdesktop_en_v1.strings", key: "lng_language_name"},
		{dir: "weba", file: "weba_en_v2.strings", key: "NewMessageTitle"},
	} {
		dir := filepath.Join(root, item.dir)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir fixture: %v", err)
		}
		content := []byte(`"` + item.key + `" = "value";`)
		if err := os.WriteFile(filepath.Join(dir, item.file), content, 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	store := memory.NewLangPackStore()
	service := NewService(store)
	seeded, err := service.SeedDirectory(context.Background(), root)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if seeded != 2 {
		t.Fatalf("seeded = %d, want 2", seeded)
	}
	pack, err := service.GetLangPack(context.Background(), "weba", "en")
	if err != nil {
		t.Fatalf("get weba pack: %v", err)
	}
	if pack.Version != 2 || len(pack.Strings) != 1 || pack.Strings[0].Key != "NewMessageTitle" {
		t.Fatalf("weba pack = %+v", pack)
	}
}

func TestSeedDirectorySkipsSameVersionSameContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "tdesktop_en_v1.strings")
	if err := os.WriteFile(path, []byte(`"lng_app_name" = "SafeLink";`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	store := memory.NewLangPackStore()
	service := NewService(store)
	if seeded, err := service.SeedDirectory(context.Background(), root); err != nil || seeded != 1 {
		t.Fatalf("first seed = %d, %v; want 1, nil", seeded, err)
	}
	if seeded, err := service.SeedDirectory(context.Background(), root); err != nil || seeded != 0 {
		t.Fatalf("second seed = %d, %v; want 0, nil", seeded, err)
	}
	pack, err := service.GetLangPack(context.Background(), "tdesktop", "en")
	if err != nil {
		t.Fatalf("get pack: %v", err)
	}
	if pack.Version != 1 {
		t.Fatalf("version = %d, want 1", pack.Version)
	}
}

func TestSeedDirectoryBumpsVersionForSameVersionChangedContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "tdesktop_en_v1.strings")
	if err := os.WriteFile(path, []byte(`"lng_app_name" = "Telegram";`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	store := memory.NewLangPackStore()
	service := NewService(store)
	if seeded, err := service.SeedDirectory(context.Background(), root); err != nil || seeded != 1 {
		t.Fatalf("first seed = %d, %v; want 1, nil", seeded, err)
	}
	if err := os.WriteFile(path, []byte(`"lng_app_name" = "SafeLink";`), 0o600); err != nil {
		t.Fatalf("rewrite fixture: %v", err)
	}
	if seeded, err := service.SeedDirectory(context.Background(), root); err != nil || seeded != 1 {
		t.Fatalf("changed seed = %d, %v; want 1, nil", seeded, err)
	}

	pack, err := service.GetLangPack(context.Background(), "tdesktop", "en")
	if err != nil {
		t.Fatalf("get pack: %v", err)
	}
	if pack.Version != 2 {
		t.Fatalf("version = %d, want 2", pack.Version)
	}
	if len(pack.Strings) != 1 || pack.Strings[0].Value != "SafeLink" {
		t.Fatalf("strings = %+v, want SafeLink", pack.Strings)
	}
}

func TestSeedDirectoryAppliesBrandingAndBumpsWhenAppNameChanges(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "tdesktop_en_v1.strings")
	if err := os.WriteFile(path, []byte(`
"TelegramPremium" = "Telegram Premium";
"lng_business" = "SafeLink Business";
"lng_tidings" = "Tidings Desktop";
"lng_link" = "https://telegram.org/faq and https://t.me/example and tg://resolve?domain=test";
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	store := memory.NewLangPackStore()
	service := NewService(store, WithBranding(Branding{AppName: "Safelink"}))
	if seeded, err := service.SeedDirectory(context.Background(), root); err != nil || seeded != 4 {
		t.Fatalf("first seed = %d, %v; want 4, nil", seeded, err)
	}
	pack, err := service.GetLangPack(context.Background(), "tdesktop", "en")
	if err != nil {
		t.Fatalf("get first pack: %v", err)
	}
	if pack.Version != 1 ||
		langPackTestValue(pack, "TelegramPremium") != "Safelink Premium" ||
		langPackTestValue(pack, "lng_business") != "Safelink Business" ||
		langPackTestValue(pack, "lng_tidings") != "Safelink Desktop" ||
		langPackTestValue(pack, "lng_link") != "https://safelink.chat/faq and https://safelink.chat/example and safelink://resolve?domain=test" {
		t.Fatalf("first pack = %+v", pack)
	}

	service = NewService(store, WithBranding(Branding{AppName: "NewName"}))
	if seeded, err := service.SeedDirectory(context.Background(), root); err != nil || seeded != 4 {
		t.Fatalf("second seed = %d, %v; want 4, nil", seeded, err)
	}
	pack, err = service.GetLangPack(context.Background(), "tdesktop", "en")
	if err != nil {
		t.Fatalf("get second pack: %v", err)
	}
	if pack.Version != 2 ||
		langPackTestValue(pack, "TelegramPremium") != "NewName Premium" ||
		langPackTestValue(pack, "lng_business") != "NewName Business" ||
		langPackTestValue(pack, "lng_tidings") != "NewName Desktop" ||
		langPackTestValue(pack, "lng_link") != "https://safelink.chat/faq and https://safelink.chat/example and safelink://resolve?domain=test" {
		t.Fatalf("second pack = %+v", pack)
	}
}

func TestBundledAndroidPersianLangPackParses(t *testing.T) {
	path := filepath.Join("..", "..", "..", "data", "langpack", "android", "android_fa_v59634849.strings")
	pack, err := ParseTDesktopFile(path)
	if err != nil {
		t.Fatalf("parse bundled android fa pack: %v", err)
	}
	if pack.LangPack != "android" || pack.LangCode != "fa" || pack.Version != 59634849 {
		t.Fatalf("pack meta = %+v, want android/fa v59634849", pack)
	}
	if len(pack.Strings) < 10000 {
		t.Fatalf("strings count = %d, want full android fa pack", len(pack.Strings))
	}
	wantPersian := "\u0641\u0627\u0631\u0633\u06cc"
	for _, item := range pack.Strings {
		if item.Key == "TranslateLanguageFA" {
			if item.Value != wantPersian {
				t.Fatalf("TranslateLanguageFA = %q, want %q", item.Value, wantPersian)
			}
			return
		}
	}
	t.Fatalf("TranslateLanguageFA not found in bundled android fa pack")
}

func TestBundledLangPacksBrandingRemovesLegacyPublicValues(t *testing.T) {
	root := filepath.Join("..", "..", "..", "data", "langpack")
	if _, err := os.Stat(root); err != nil {
		t.Skipf("bundled langpack dir unavailable: %v", err)
	}
	replacer := newBrandReplacer(Branding{AppName: "SafeLink"})
	if replacer == nil {
		t.Fatal("brand replacer is nil")
	}
	checked := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".strings" {
			return nil
		}
		pack, err := ParseTDesktopFile(path)
		if err != nil {
			return err
		}
		applyBrandingToPack(&pack, replacer)
		for _, item := range pack.Strings {
			for _, value := range langPackStringValues(item) {
				if forbidden := legacyPublicBrandToken(value); forbidden != "" {
					t.Fatalf("%s %s value still contains %q: %q", path, item.Key, forbidden, value)
				}
			}
			checked++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk bundled langpacks: %v", err)
	}
	if checked == 0 {
		t.Fatal("checked no bundled langpack strings")
	}
}

func langPackTestValue(pack domain.LangPack, key string) string {
	for _, item := range pack.Strings {
		if item.Key == key {
			return item.Value
		}
	}
	return ""
}

func langPackStringValues(item domain.LangPackString) []string {
	return []string{
		item.Value,
		item.ZeroValue,
		item.OneValue,
		item.TwoValue,
		item.FewValue,
		item.ManyValue,
		item.OtherValue,
	}
}

func legacyPublicBrandToken(value string) string {
	lower := strings.ToLower(value)
	for _, token := range []string{
		"telegram",
		"tidings",
		"tiding",
		"telesrv",
		"t.me",
		"tg://",
		"telegram.org",
		"telegram.me",
		"tidings.org",
		"tidings.me",
		"core.telegram",
		"core.tidings",
	} {
		if strings.Contains(lower, token) {
			return token
		}
	}
	return ""
}
