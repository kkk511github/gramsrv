package push

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"testing"
)

func TestNewFCMSenderAcceptsRawAndBase64ServiceAccount(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	account, err := json.Marshal(fcmServiceAccount{
		ProjectID:   "safelink-test",
		ClientEmail: "push@safelink-test.iam.gserviceaccount.com",
		PrivateKey:  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})),
		TokenURI:    "https://oauth2.googleapis.com/token",
	})
	if err != nil {
		t.Fatal(err)
	}

	for name, value := range map[string]string{
		"raw":    string(account),
		"base64": base64.StdEncoding.EncodeToString(account),
	} {
		t.Run(name, func(t *testing.T) {
			sender, err := newFCMSender(Config{FCMServiceAccountJSON: value})
			if err != nil {
				t.Fatalf("newFCMSender: %v", err)
			}
			if sender.projectID != "safelink-test" || sender.clientEmail == "" || sender.privateKey == nil {
				t.Fatalf("sender not initialized: %+v", sender)
			}
		})
	}
}
