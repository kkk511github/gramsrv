package linksettings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsValidate(t *testing.T) {
	for _, raw := range []string{"http://example.test", "https://user:pass@example.test", "https://example.test/path", "https://example.test/?x=1", "https://example.test/#x", "https://example.test/?", "javascript:alert(1)", "https://", "https://example.test\nX=y"} {
		if _, err := (Settings{PublicURL: raw, WebURL: "https://web.example.test"}).Validate(); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	s, err := (Settings{PublicURL: " https://EXAMPLE.test/ ", WebURL: "https://web.example.test/"}).Validate()
	if err != nil || s.PublicURL != "https://example.test" || s.WebURL != "https://web.example.test" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestSaveReadPreservesPreviousOnValidationFailure(t *testing.T) {
	p := filepath.Join(t.TempDir(), "links.json")
	if s, err := Read(p); s != nil || err != nil {
		t.Fatalf("%v %v", s, err)
	}
	want := Settings{PublicURL: "https://example.test", WebURL: "https://web.example.test"}
	if err := Save(p, want); err != nil {
		t.Fatal(err)
	}
	if err := Save(p, Settings{}); err == nil {
		t.Fatal("invalid save accepted")
	}
	got, err := Read(p)
	if err != nil || *got != want {
		t.Fatalf("%v %v", got, err)
	}
	stat, _ := os.Stat(p)
	if stat.Mode().Perm() != 0600 {
		t.Fatal("settings permissions are not private")
	}
}
