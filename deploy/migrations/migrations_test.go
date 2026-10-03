package migrations

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var migrationFileRE = regexp.MustCompile(`^([0-9]{4})_.+\.(up|down)\.sql$`)

func TestBotPaymentMigrationsFollowSafeLinkLoginPolicy(t *testing.T) {
	for _, name := range []string{
		"0223_bot_stars_wallet",
		"0224_bot_invoices",
		"0225_bot_api_pre_checkout_payload",
		"0226_bot_api_pre_checkout_shape",
	} {
		for _, direction := range []string{"up", "down"} {
			if _, err := os.Stat(name + "." + direction + ".sql"); err != nil {
				t.Fatalf("missing post-0222 bot payment migration %s.%s: %v", name, direction, err)
			}
		}
	}
}

func TestMigrationFilesHaveUniqueVersionDirections(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}

	seen := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		match := migrationFileRE.FindStringSubmatch(name)
		if match == nil {
			continue
		}
		key := match[1] + "." + match[2]
		if prev, ok := seen[key]; ok {
			t.Fatalf("duplicate migration version/direction %s: %s and %s", key, prev, name)
		}
		seen[key] = name
	}
}

func TestMigrationsAvoidPostgres17OnlyTransactionTimeout(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		data, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		if strings.Contains(string(data), "transaction_timeout") {
			t.Fatalf("%s uses transaction_timeout, which breaks PostgreSQL 16 deployments", entry.Name())
		}
	}
}

func TestMigrationsDoNotRequireSuperuserReplicationRole(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		data, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		if strings.Contains(string(data), "session_replication_role") {
			t.Fatalf("%s uses session_replication_role, which requires a PostgreSQL superuser", entry.Name())
		}
	}
}
