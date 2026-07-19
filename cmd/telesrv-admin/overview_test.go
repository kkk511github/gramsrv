package main

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestAttentionItemUsesRealIssueStateAndTimestamp(t *testing.T) {
	discovered := time.Date(2026, time.July, 19, 12, 30, 0, 0, time.UTC)
	item := attentionItem("push_retries", 4, "retrying", pgtype.Timestamptz{Time: discovered, Valid: true})
	if item.State != "retrying" || item.Count != 4 || item.DiscoveredAt == nil || !item.DiscoveredAt.Equal(discovered) {
		t.Fatalf("attention item = %+v", item)
	}
}

func TestAttentionItemHealthyOmitsDiscoveryTimestamp(t *testing.T) {
	discovered := time.Date(2026, time.July, 19, 12, 30, 0, 0, time.UTC)
	item := attentionItem("stale_uploads", 0, "overdue", pgtype.Timestamptz{Time: discovered, Valid: true})
	if item.State != "healthy" || item.DiscoveredAt != nil {
		t.Fatalf("healthy attention item = %+v", item)
	}
}
