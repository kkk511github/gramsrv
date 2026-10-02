// Package linksettings stores the bounded public-domain override shared by
// the admin console and the server. It never changes transport keys or ports.
package linksettings

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Settings struct {
	PublicURL string `json:"public_url"`
	WebURL    string `json:"web_url"`
}

func (s Settings) Validate() (Settings, error) {
	for _, field := range []struct {
		name  string
		value *string
	}{{"公开域名", &s.PublicURL}, {"网页版地址", &s.WebURL}} {
		u, err := url.Parse(strings.TrimSpace(*field.value))
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") || strings.ContainsAny(u.Host, "\r\n\t $'\"\\") {
			return Settings{}, fmt.Errorf("%s必须是 HTTPS 根地址，不能包含账号、路径或参数", field.name)
		}
		u.Path = ""
		u.Host = strings.ToLower(u.Host)
		*field.value = u.String()
	}
	return s, nil
}

func Read(path string) (*Settings, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var settings Settings
	if err := json.Unmarshal(b, &settings); err != nil {
		return nil, err
	}
	settings, err = settings.Validate()
	if err != nil {
		return nil, err
	}
	return &settings, nil
}

func Save(path string, settings Settings) error {
	if path == "" {
		return fmt.Errorf("未配置域名设置存储路径")
	}
	settings, err := settings.Validate()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".link-settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
