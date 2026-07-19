package opsmetrics

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap/zaptest"
)

type captureExecutor struct {
	mu        sync.Mutex
	arguments [][]any
	err       error
}

func (e *captureExecutor) Exec(_ context.Context, _ string, arguments ...any) (pgconn.CommandTag, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.arguments = append(e.arguments, arguments)
	return pgconn.NewCommandTag("INSERT 0 1"), e.err
}

func TestCollectorFlushesAggregatedCounters(t *testing.T) {
	db := &captureExecutor{}
	collector := New(db, zaptest.NewLogger(t))
	collector.RPCHandled("messages.sendMessage", time.Millisecond, nil)
	collector.RPCHandled("updates.getState", time.Millisecond, nil)
	collector.PushDelivered()
	collector.PushFailed()

	collector.flush(context.Background())

	db.mu.Lock()
	defer db.mu.Unlock()
	if len(db.arguments) != 1 {
		t.Fatalf("exec calls = %d, want 1", len(db.arguments))
	}
	got := db.arguments[0]
	if len(got) != 3 || got[0] != int64(2) || got[1] != int64(1) || got[2] != int64(1) {
		t.Fatalf("flush arguments = %#v, want [2 1 1]", got)
	}
}

func TestCollectorRestoresCountersAfterFlushFailure(t *testing.T) {
	db := &captureExecutor{err: errors.New("postgres unavailable")}
	collector := New(db, zaptest.NewLogger(t))
	collector.RPCHandled("help.getConfig", time.Millisecond, nil)
	collector.PushDelivered()
	collector.flush(context.Background())

	if got := collector.rpcRequests.Load(); got != 1 {
		t.Fatalf("rpc requests after failed flush = %d, want 1", got)
	}
	if got := collector.pushDelivered.Load(); got != 1 {
		t.Fatalf("push delivered after failed flush = %d, want 1", got)
	}
}
