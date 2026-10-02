package push

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"telesrv/internal/domain"
)

func TestAPNSLiveCredentials(t *testing.T) {
	path := os.Getenv("SAFELINK_APNS_LIVE_KEY")
	if path == "" {
		t.Skip("live APNs credentials check is opt-in")
	}
	sender, err := newAPNSSender(Config{
		APNSTopic:          os.Getenv("SAFELINK_APNS_LIVE_TOPIC"),
		APNSTeamID:         os.Getenv("SAFELINK_APNS_LIVE_TEAM"),
		APNSKeyID:          os.Getenv("SAFELINK_APNS_LIVE_KEY_ID"),
		APNSPrivateKeyPath: path,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	invalid, err := sender.Send(ctx, domain.PushDevice{Token: strings.Repeat("0", 64)}, domain.PushNotificationJob{Title: "SafeLink", Body: "Credential check"})
	if err != nil {
		t.Fatal(err)
	}
	if !invalid {
		t.Fatal("APNs must reject the deliberately invalid device token")
	}
	t.Log("APNs returned invalid-device response without a provider authentication error")
}
