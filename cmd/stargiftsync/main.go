// Command stargiftsync copies the current official Telegram Star Gift catalog
// into SafeLink's durable, administrator-managed catalog.
//
// Authentication uses an existing gotd session or TELEGRAM_BOT_TOKEN. The
// SafeLink Admin API token is read from TELESRV_ADMIN_API_TOKEN. Tokens are
// never accepted as flags so they do not leak through the process list.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iamxvbaba/td/telegram"
	"github.com/iamxvbaba/td/telegram/auth"
	"github.com/iamxvbaba/td/telegram/auth/qrlogin"
	"github.com/iamxvbaba/td/telegram/downloader"
	"github.com/iamxvbaba/td/tg"
	"rsc.io/qr"
)

const (
	tdesktopAPIID   = 17349
	tdesktopAPIHash = "344583e45741c457fe1862106095a5eb"
)

type syncConfig struct {
	AdminURL          string
	AdminToken        string
	SessionPath       string
	LoginStatePath    string
	QRPath            string
	ManifestPath      string
	SourceDir         string
	BotToken          string
	Phone             string
	Code              string
	Password          string
	SendCodeOnly      bool
	QRLogin           bool
	Logout            bool
	Confirm           bool
	EnableUnavailable bool
	Limit             int
	Timeout           time.Duration
}

type phoneLoginState struct {
	Phone         string `json:"phone"`
	PhoneCodeHash string `json:"phone_code_hash"`
}

type sourceGift struct {
	ID           int64
	Title        string
	Stars        int64
	ConvertStars int64
	SortOrder    int
	SoldOut      bool
	Auction      bool
	TGS          []byte
	SHA256       string
}

type manifestGift struct {
	SafeLinkGiftID int64  `json:"safelink_gift_id"`
	Title          string `json:"title,omitempty"`
	SHA256         string `json:"sha256"`
	Stars          int64  `json:"stars"`
	ConvertStars   int64  `json:"convert_stars"`
	SortOrder      int    `json:"sort_order"`
	SoldOut        bool   `json:"sold_out,omitempty"`
	Auction        bool   `json:"auction,omitempty"`
}

type syncManifest struct {
	UpdatedAt time.Time               `json:"updated_at"`
	Gifts     map[string]manifestGift `json:"gifts"`
}

type commandResult struct {
	Status          string         `json:"status"`
	AlreadyExecuted bool           `json:"already_executed"`
	Message         string         `json:"message"`
	Details         map[string]any `json:"details"`
	Error           string         `json:"error"`
}

type importMetadata struct {
	CommandID     string `json:"command_id"`
	Actor         string `json:"actor"`
	Reason        string `json:"reason"`
	DryRun        bool   `json:"dry_run"`
	GiftID        int64  `json:"gift_id,omitempty"`
	Title         string `json:"title"`
	Stars         int64  `json:"stars"`
	ConvertStars  int64  `json:"convert_stars"`
	Enabled       bool   `json:"enabled"`
	SortOrder     int    `json:"sort_order"`
	TrustedSource bool   `json:"trusted_source"`
}

func main() {
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func parseFlags() syncConfig {
	var cfg syncConfig
	flag.StringVar(&cfg.AdminURL, "admin-url", envOr("SAFELINK_ADMIN_API_URL", "http://127.0.0.1:2399"), "SafeLink Admin API URL")
	flag.StringVar(&cfg.SessionPath, "session", envOr("SESSION", "/tmp/telegram-star-gifts.session"), "official Telegram gotd session path")
	flag.StringVar(&cfg.LoginStatePath, "login-state", strings.TrimSpace(os.Getenv("TELEGRAM_LOGIN_STATE")), "temporary phone login state path")
	flag.StringVar(&cfg.QRPath, "qr-file", envOr("TELEGRAM_QR_FILE", "/tmp/telegram-star-gifts-qr.png"), "temporary QR login image")
	flag.StringVar(&cfg.ManifestPath, "manifest", envOr("SAFELINK_STAR_GIFT_MANIFEST", "data/telegram-star-gifts-manifest.json"), "source-to-SafeLink gift ID manifest")
	flag.StringVar(&cfg.SourceDir, "source-dir", strings.TrimSpace(os.Getenv("SAFELINK_STAR_GIFT_SOURCE_DIR")), "optional private cache for official Telegram TGS files")
	flag.BoolVar(&cfg.SendCodeOnly, "send-code", false, "send a Telegram phone login code and stop")
	flag.BoolVar(&cfg.QRLogin, "qr-login", false, "use QR login when no authorized session, bot token, or phone code is available")
	flag.BoolVar(&cfg.Logout, "logout", false, "revoke the Telegram session, delete local login state, and stop")
	flag.BoolVar(&cfg.Confirm, "confirm", false, "perform imports after validation")
	flag.BoolVar(&cfg.EnableUnavailable, "enable-unavailable", true, "enable sold-out and auction source gifts in the independent SafeLink catalog")
	flag.IntVar(&cfg.Limit, "limit", 0, "maximum gifts to sync; 0 means all")
	flag.DurationVar(&cfg.Timeout, "timeout", 45*time.Minute, "whole sync timeout")
	flag.Parse()
	cfg.AdminToken = strings.TrimSpace(os.Getenv("TELESRV_ADMIN_API_TOKEN"))
	cfg.BotToken = strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	cfg.Phone = strings.TrimSpace(os.Getenv("TELEGRAM_PHONE"))
	cfg.Code = strings.TrimSpace(os.Getenv("TELEGRAM_CODE"))
	cfg.Password = os.Getenv("TELEGRAM_PASSWORD")
	if cfg.LoginStatePath == "" {
		cfg.LoginStatePath = cfg.SessionPath + ".login.json"
	}
	return cfg
}

func run(cfg syncConfig) error {
	if cfg.Limit < 0 {
		return errors.New("limit must be non-negative")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.SessionPath), 0o700); err != nil {
		return fmt.Errorf("create session directory: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	updates := tg.NewUpdateDispatcher()
	loggedIn := qrlogin.OnLoginToken(updates)
	client := telegram.NewClient(tdesktopAPIID, tdesktopAPIHash, telegram.Options{
		SessionStorage: &telegram.FileSessionStorage{Path: cfg.SessionPath},
		UpdateHandler:  updates,
	})
	loggedOut := false
	err := client.Run(ctx, func(ctx context.Context) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("check Telegram authorization: %w", err)
		}
		if cfg.Logout {
			if status.Authorized {
				if _, err := client.API().AuthLogOut(ctx); err != nil {
					return fmt.Errorf("revoke Telegram authorization: %w", err)
				}
			}
			loggedOut = true
			fmt.Println("[authorization] Telegram session revoked")
			return nil
		}
		if !status.Authorized {
			switch {
			case cfg.SendCodeOnly:
				if cfg.Phone == "" {
					return errors.New("TELEGRAM_PHONE is required with -send-code")
				}
				sent, err := client.Auth().SendCode(ctx, cfg.Phone, auth.SendCodeOptions{})
				if err != nil {
					return fmt.Errorf("send Telegram login code: %w", err)
				}
				code, ok := sent.(*tg.AuthSentCode)
				if !ok {
					return fmt.Errorf("send Telegram login code returned unsupported result %T", sent)
				}
				if err := savePhoneLoginState(cfg.LoginStatePath, phoneLoginState{
					Phone: cfg.Phone, PhoneCodeHash: code.PhoneCodeHash,
				}); err != nil {
					return err
				}
				fmt.Println("[authorization] Telegram login code sent")
				return nil
			case cfg.BotToken != "":
				if _, err := client.Auth().Bot(ctx, cfg.BotToken); err != nil {
					return fmt.Errorf("authorize Telegram bot: %w", err)
				}
			case cfg.Code != "":
				state, err := loadPhoneLoginState(cfg.LoginStatePath)
				if err != nil {
					return err
				}
				if cfg.Phone != "" && cfg.Phone != state.Phone {
					return errors.New("TELEGRAM_PHONE does not match the pending login state")
				}
				if _, err := client.Auth().SignIn(ctx, state.Phone, cfg.Code, state.PhoneCodeHash); err != nil {
					if !errors.Is(err, auth.ErrPasswordAuthNeeded) {
						return fmt.Errorf("authorize Telegram phone login: %w", err)
					}
					if cfg.Password == "" {
						return errors.New("Telegram two-step verification is enabled; set TELEGRAM_PASSWORD and retry with the same TELEGRAM_CODE")
					}
					if _, err := client.Auth().Password(ctx, cfg.Password); err != nil {
						return fmt.Errorf("authorize Telegram two-step verification: %w", err)
					}
				}
				_ = os.Remove(cfg.LoginStatePath)
				fmt.Println("[authorization] Telegram phone login completed")
			case cfg.QRLogin:
				if _, err := client.QR().Auth(ctx, loggedIn, func(_ context.Context, token qrlogin.Token) error {
					if err := writeQR(cfg.QRPath, token); err != nil {
						return err
					}
					fmt.Printf("[authorization] scan %s before %s\n", cfg.QRPath, token.Expires().Local().Format(time.RFC3339))
					return nil
				}); err != nil {
					return fmt.Errorf("authorize Telegram QR login: %w", err)
				}
				_ = os.Remove(cfg.QRPath)
			default:
				return errors.New("Telegram authorization required: use TELEGRAM_PHONE with -send-code, then TELEGRAM_CODE; or provide TELEGRAM_BOT_TOKEN; or explicitly use -qr-login")
			}
		} else if cfg.SendCodeOnly {
			fmt.Println("[authorization] existing Telegram session is already authorized")
			return nil
		}
		if cfg.AdminToken == "" {
			return errors.New("TELESRV_ADMIN_API_TOKEN is required for catalog synchronization")
		}

		api := client.API()
		catalog, err := api.PaymentsGetStarGifts(ctx, 0)
		if err != nil {
			return fmt.Errorf("payments.getStarGifts: %w", err)
		}
		full, ok := catalog.(*tg.PaymentsStarGifts)
		if !ok {
			return fmt.Errorf("payments.getStarGifts returned %T", catalog)
		}
		gifts, err := fetchSourceGifts(ctx, api, full.Gifts, cfg.Limit, cfg.SourceDir)
		if err != nil {
			return err
		}
		fmt.Printf("[catalog] fetched=%d hash=%d\n", len(gifts), full.Hash)

		manifest, err := loadManifest(cfg.ManifestPath)
		if err != nil {
			return err
		}
		httpClient := &http.Client{Timeout: 90 * time.Second}
		for i, gift := range gifts {
			key := fmt.Sprintf("%d", gift.ID)
			previous := manifest.Gifts[key]
			if previous.SafeLinkGiftID != 0 && previous.SHA256 == gift.SHA256 &&
				previous.Stars == gift.Stars && previous.ConvertStars == gift.ConvertStars &&
				previous.SortOrder == gift.SortOrder {
				fmt.Printf("[%d/%d] source=%d unchanged safelink=%d\n", i+1, len(gifts), gift.ID, previous.SafeLinkGiftID)
				continue
			}
			enabled := cfg.EnableUnavailable || (!gift.SoldOut && !gift.Auction)
			safeLinkID, err := syncOneGift(ctx, httpClient, cfg, gift, previous.SafeLinkGiftID, enabled)
			if err != nil {
				return fmt.Errorf("sync source gift %d: %w", gift.ID, err)
			}
			if !cfg.Confirm {
				fmt.Printf("[%d/%d] source=%d validated\n", i+1, len(gifts), gift.ID)
				continue
			}
			manifest.Gifts[key] = manifestGift{
				SafeLinkGiftID: safeLinkID, Title: gift.Title, SHA256: gift.SHA256,
				Stars: gift.Stars, ConvertStars: gift.ConvertStars, SortOrder: gift.SortOrder,
				SoldOut: gift.SoldOut, Auction: gift.Auction,
			}
			manifest.UpdatedAt = time.Now().UTC()
			if err := saveManifest(cfg.ManifestPath, manifest); err != nil {
				return err
			}
			fmt.Printf("[%d/%d] source=%d imported safelink=%d\n", i+1, len(gifts), gift.ID, safeLinkID)
		}
		return nil
	})
	if loggedOut {
		_ = os.Remove(cfg.SessionPath)
		_ = os.Remove(cfg.LoginStatePath)
		_ = os.Remove(cfg.QRPath)
	}
	return err
}

func loadPhoneLoginState(path string) (phoneLoginState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return phoneLoginState{}, fmt.Errorf("read Telegram phone login state: %w", err)
	}
	var state phoneLoginState
	if err := json.Unmarshal(raw, &state); err != nil {
		return phoneLoginState{}, fmt.Errorf("decode Telegram phone login state: %w", err)
	}
	if state.Phone == "" || state.PhoneCodeHash == "" {
		return phoneLoginState{}, errors.New("Telegram phone login state is incomplete")
	}
	return state, nil
}

func savePhoneLoginState(path string, state phoneLoginState) error {
	if state.Phone == "" || state.PhoneCodeHash == "" {
		return errors.New("Telegram phone login state is incomplete")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create Telegram login state directory: %w", err)
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write Telegram phone login state: %w", err)
	}
	return nil
}

func writeQR(path string, token qrlogin.Token) error {
	code, err := qr.Encode(token.URL(), qr.M)
	if err != nil {
		return fmt.Errorf("render Telegram login QR: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create QR directory: %w", err)
	}
	if err := os.WriteFile(path, code.PNG(), 0o600); err != nil {
		return fmt.Errorf("write QR image: %w", err)
	}
	return nil
}

func fetchSourceGifts(ctx context.Context, api *tg.Client, classes []tg.StarGiftClass, limit int, sourceDir string) ([]sourceGift, error) {
	dl := downloader.NewDownloader()
	out := make([]sourceGift, 0, len(classes))
	for _, class := range classes {
		gift, ok := class.(*tg.StarGift)
		if !ok {
			continue
		}
		doc, ok := gift.Sticker.(*tg.Document)
		if !ok || doc.ID == 0 {
			return nil, fmt.Errorf("gift %d has invalid sticker %T", gift.ID, gift.Sticker)
		}
		if doc.MimeType != "application/x-tgsticker" {
			return nil, fmt.Errorf("gift %d uses unsupported sticker MIME %q", gift.ID, doc.MimeType)
		}
		if doc.Size <= 0 || doc.Size > 4<<20 {
			return nil, fmt.Errorf("gift %d sticker size %d is outside the SafeLink import limit", gift.ID, doc.Size)
		}
		if gift.Stars <= 0 || gift.ConvertStars < 0 || gift.ConvertStars > gift.Stars {
			return nil, fmt.Errorf("gift %d has unsupported price %d/%d", gift.ID, gift.Stars, gift.ConvertStars)
		}
		data, err := loadGiftDocument(ctx, dl, api, sourceDir, gift.ID, doc)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		title, _ := gift.GetTitle()
		out = append(out, sourceGift{
			ID: gift.ID, Title: title, Stars: gift.Stars, ConvertStars: gift.ConvertStars,
			SoldOut: gift.SoldOut, Auction: gift.Auction, TGS: data,
			SHA256: hex.EncodeToString(sum[:]),
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	for i := range out {
		out[i].SortOrder = i
	}
	return out, nil
}

func loadGiftDocument(ctx context.Context, dl *downloader.Downloader, api *tg.Client, sourceDir string, giftID int64, doc *tg.Document) ([]byte, error) {
	var cachePath string
	if sourceDir != "" {
		cachePath = filepath.Join(sourceDir, fmt.Sprintf("%d.tgs", giftID))
		cached, err := os.ReadFile(cachePath)
		if err == nil && int64(len(cached)) == doc.Size {
			return cached, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read cached gift %d: %w", giftID, err)
		}
	}

	var data bytes.Buffer
	location := &tg.InputDocumentFileLocation{ID: doc.ID, AccessHash: doc.AccessHash, FileReference: doc.FileReference}
	if _, err := dl.Download(api, location).Stream(ctx, &data); err != nil {
		return nil, fmt.Errorf("download gift %d document %d: %w", giftID, doc.ID, err)
	}
	if int64(data.Len()) != doc.Size {
		return nil, fmt.Errorf("gift %d document size = %d, want %d", giftID, data.Len(), doc.Size)
	}
	raw := append([]byte(nil), data.Bytes()...)
	if cachePath != "" {
		if err := os.MkdirAll(sourceDir, 0o700); err != nil {
			return nil, fmt.Errorf("create gift source cache: %w", err)
		}
		if err := os.WriteFile(cachePath, raw, 0o600); err != nil {
			return nil, fmt.Errorf("cache gift %d: %w", giftID, err)
		}
	}
	return raw, nil
}

func syncOneGift(ctx context.Context, client *http.Client, cfg syncConfig, gift sourceGift, safeLinkID int64, enabled bool) (int64, error) {
	if len(gift.SHA256) < 12 {
		return 0, errors.New("source gift SHA-256 is invalid")
	}
	baseID := fmt.Sprintf("tg-gift-v2-%d-%s", gift.ID, gift.SHA256[:12])
	validate := importMetadata{
		CommandID: baseID + "-validate", Actor: "safelink-gift-sync", Reason: "sync official gift catalog",
		DryRun: true, GiftID: safeLinkID, Title: gift.Title, Stars: gift.Stars,
		ConvertStars: gift.ConvertStars, Enabled: enabled, SortOrder: gift.SortOrder, TrustedSource: true,
	}
	if _, err := importGift(ctx, client, cfg.AdminURL, cfg.AdminToken, validate, gift); err != nil {
		return 0, fmt.Errorf("validate: %w", err)
	}
	if !cfg.Confirm {
		return safeLinkID, nil
	}
	execute := validate
	execute.CommandID = baseID + "-import"
	execute.DryRun = false
	result, err := importGift(ctx, client, cfg.AdminURL, cfg.AdminToken, execute, gift)
	if err != nil {
		return 0, fmt.Errorf("import: %w", err)
	}
	importedID, err := resultGiftID(result)
	if err != nil {
		return 0, err
	}
	if safeLinkID != 0 && importedID != safeLinkID {
		return 0, fmt.Errorf("admin returned gift id %d, want existing %d", importedID, safeLinkID)
	}
	return importedID, nil
}

func importGift(ctx context.Context, client *http.Client, adminURL, token string, metadata importMetadata, gift sourceGift) (commandResult, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	rawMetadata, err := json.Marshal(metadata)
	if err != nil {
		return commandResult{}, err
	}
	if err := writer.WriteField("metadata", string(rawMetadata)); err != nil {
		return commandResult{}, err
	}
	file, err := writer.CreateFormFile("file", fmt.Sprintf("telegram-star-gift-%d.tgs", gift.ID))
	if err != nil {
		return commandResult{}, err
	}
	if _, err := file.Write(gift.TGS); err != nil {
		return commandResult{}, err
	}
	if err := writer.Close(); err != nil {
		return commandResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(adminURL, "/")+"/v1/gifts/import", &body)
	if err != nil {
		return commandResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		return commandResult{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return commandResult{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var result commandResult
	if err := dec.Decode(&result); err != nil {
		return commandResult{}, fmt.Errorf("admin status %d returned invalid JSON: %w", resp.StatusCode, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || result.Status != "completed" {
		message := result.Error
		if message == "" {
			message = result.Message
		}
		return result, fmt.Errorf("admin status %d: %s", resp.StatusCode, message)
	}
	return result, nil
}

func resultGiftID(result commandResult) (int64, error) {
	value, ok := result.Details["gift_id"]
	if !ok {
		return 0, errors.New("admin result omitted gift_id")
	}
	switch typed := value.(type) {
	case json.Number:
		id, err := typed.Int64()
		if err == nil && id > 0 {
			return id, nil
		}
	case float64:
		if typed > 0 && typed == float64(int64(typed)) {
			return int64(typed), nil
		}
	}
	return 0, fmt.Errorf("admin returned invalid gift_id %v", value)
}

func loadManifest(path string) (syncManifest, error) {
	manifest := syncManifest{Gifts: map[string]manifestGift{}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return manifest, nil
	}
	if err != nil {
		return syncManifest{}, fmt.Errorf("read manifest: %w", err)
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return syncManifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.Gifts == nil {
		manifest.Gifts = map[string]manifestGift{}
	}
	return manifest, nil
}

func saveManifest(path string, manifest syncManifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create manifest directory: %w", err)
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace manifest: %w", err)
	}
	return nil
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
