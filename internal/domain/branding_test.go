package domain

import (
	"strings"
	"testing"
)

func TestServiceIdentityAndLoginMessageUseSafeLinkBrand(t *testing.T) {
	serviceUser := OfficialSystemUser()
	if serviceUser.FirstName != "SafeLink" || serviceUser.Username != "safelink" {
		t.Fatalf("service user = %+v, want SafeLink identity", serviceUser)
	}
	message, err := OfficialLoginCodeMessage(42, "12345", 1)
	if err != nil {
		t.Fatalf("build login message: %v", err)
	}
	if !strings.Contains(message.Body, "SafeLink") || strings.Contains(strings.ToLower(message.Body), "telegram") {
		t.Fatalf("login message exposes wrong brand: %q", message.Body)
	}
}
