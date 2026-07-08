package ipgeo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"telesrv/internal/domain"
)

const (
	DefaultEndpoint = "https://ipwho.is/{ip}?lang=zh-CN"
	defaultTimeout  = 800 * time.Millisecond
	defaultCacheTTL = 24 * time.Hour
)

type Options struct {
	Endpoint string
	Timeout  time.Duration
	CacheTTL time.Duration
	Client   *http.Client
}

type Resolver struct {
	endpoint string
	timeout  time.Duration
	cacheTTL time.Duration
	client   *http.Client

	mu    sync.RWMutex
	cache map[string]cacheEntry
	sf    singleflight.Group
}

type cacheEntry struct {
	location domain.IPLocation
	ok       bool
	expires  time.Time
}

type ipWhoisResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Country string `json:"country"`
	Region  string `json:"region"`
	City    string `json:"city"`
}

func NewResolver(opts Options) *Resolver {
	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	cacheTTL := opts.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultCacheTTL
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	return &Resolver{
		endpoint: endpoint,
		timeout:  timeout,
		cacheTTL: cacheTTL,
		client:   client,
		cache:    make(map[string]cacheEntry),
	}
}

func (r *Resolver) LookupIPLocation(ctx context.Context, ip string) (domain.IPLocation, bool, error) {
	addr, ok := publicIP(ip)
	if !ok {
		return domain.IPLocation{}, false, nil
	}
	key := addr.String()
	if loc, ok, hit := r.cached(key); hit {
		return loc, ok, nil
	}
	v, err, _ := r.sf.Do(key, func() (any, error) {
		if loc, ok, hit := r.cached(key); hit {
			return cacheEntry{location: loc, ok: ok}, nil
		}
		loc, ok, err := r.lookup(ctx, key)
		r.store(key, loc, ok)
		if err != nil {
			return cacheEntry{location: loc, ok: ok}, err
		}
		return cacheEntry{location: loc, ok: ok}, nil
	})
	entry, _ := v.(cacheEntry)
	return entry.location, entry.ok, err
}

func (r *Resolver) cached(key string) (domain.IPLocation, bool, bool) {
	now := time.Now()
	r.mu.RLock()
	entry, ok := r.cache[key]
	r.mu.RUnlock()
	if !ok || now.After(entry.expires) {
		return domain.IPLocation{}, false, false
	}
	return entry.location, entry.ok, true
}

func (r *Resolver) store(key string, loc domain.IPLocation, ok bool) {
	r.mu.Lock()
	r.cache[key] = cacheEntry{location: loc, ok: ok, expires: time.Now().Add(r.cacheTTL)}
	r.mu.Unlock()
}

func (r *Resolver) lookup(ctx context.Context, ip string) (domain.IPLocation, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointForIP(r.endpoint, ip), nil)
	if err != nil {
		return domain.IPLocation{}, false, err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return domain.IPLocation{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return domain.IPLocation{}, false, fmt.Errorf("ipgeo status %d", resp.StatusCode)
	}
	var body ipWhoisResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return domain.IPLocation{}, false, err
	}
	if !body.Success {
		if body.Message != "" {
			return domain.IPLocation{}, false, fmt.Errorf("ipgeo: %s", body.Message)
		}
		return domain.IPLocation{}, false, nil
	}
	loc, ok := locationFromIPWhois(body)
	return loc, ok, nil
}

func publicIP(ip string) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return netip.Addr{}, false
	}
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsUnspecified() {
		return netip.Addr{}, false
	}
	return addr, true
}

func endpointForIP(endpoint, ip string) string {
	escaped := url.PathEscape(ip)
	if strings.Contains(endpoint, "{ip}") {
		return strings.ReplaceAll(endpoint, "{ip}", escaped)
	}
	if strings.Contains(endpoint, "?") {
		return endpoint + "&ip=" + url.QueryEscape(ip)
	}
	return strings.TrimRight(endpoint, "/") + "/" + escaped
}

func locationFromIPWhois(body ipWhoisResponse) (domain.IPLocation, bool) {
	country := cleanName(body.Country)
	region := cleanName(body.Region)
	city := cleanName(body.City)
	if country == "中国" {
		state := cleanChineseAdminName(strings.TrimPrefix(region, country))
		if state == "" {
			state = cleanChineseAdminName(region)
		}
		city = cleanChineseCityName(city)
		if city == state {
			city = ""
		}
		return location(state, city)
	}
	if city == "" {
		city = region
	}
	if city == country {
		city = ""
	}
	return location(country, city)
}

func location(country, region string) (domain.IPLocation, bool) {
	country = cleanName(country)
	region = cleanName(region)
	if country == "" && region == "" {
		return domain.IPLocation{}, false
	}
	return domain.IPLocation{Country: country, Region: region}, true
}

func cleanName(v string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(v)), " ")
}

func cleanChineseAdminName(v string) string {
	v = cleanName(v)
	v = strings.TrimPrefix(v, "中国")
	for _, suffix := range []string{"特别行政区", "维吾尔自治区", "壮族自治区", "回族自治区", "自治区", "省", "市"} {
		v = strings.TrimSuffix(v, suffix)
	}
	return v
}

func cleanChineseCityName(v string) string {
	v = cleanName(v)
	for _, suffix := range []string{"地区", "盟", "市"} {
		v = strings.TrimSuffix(v, suffix)
	}
	return v
}
