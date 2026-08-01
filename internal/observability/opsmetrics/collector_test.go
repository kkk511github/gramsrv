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

func TestCollectorTracksConnectionSummaryCounters(t *testing.T) {
	collector := New(&captureExecutor{}, zaptest.NewLogger(t))
	collector.ConnOpened()
	collector.ConnOpened()
	collector.ConnClosed()
	collector.HandshakeDone(40 * time.Millisecond)
	collector.HandshakeDone(60 * time.Millisecond)
	collector.RPCHandled("help.getConfig", time.Millisecond, errors.New("rpc failed"))
	collector.InboundRPCStarted("help.getConfig", 10*time.Millisecond)
	collector.InboundRPCDropped("help.getConfig", "queue_timeout")
	collector.OutboundResend(2, nil)
	collector.OutboundDropped("queue_full")
	collector.OutboundQueueWait(10, 10)
	collector.ConnectionIntake("mux_sniff", "error", time.Second)

	if got := collector.connectionsActive.Load(); got != 1 {
		t.Fatalf("active connections = %d, want 1", got)
	}
	if got := collector.handshakeNanos.Load(); got != int64(100*time.Millisecond) {
		t.Fatalf("handshake duration = %s, want 100ms", time.Duration(got))
	}
	if got := collector.handshakeMaxNanos.Load(); got != int64(60*time.Millisecond) {
		t.Fatalf("max handshake duration = %s, want 60ms", time.Duration(got))
	}
	if got := collector.rpcErrors.Load(); got != 1 {
		t.Fatalf("rpc errors = %d, want 1", got)
	}
	if got := collector.inboundDropped.Load(); got != 1 {
		t.Fatalf("inbound dropped = %d, want 1", got)
	}
	if got := collector.outboundResent.Load(); got != 2 {
		t.Fatalf("outbound resent = %d, want 2", got)
	}
	if got := collector.intakeErrors.Load(); got != 1 {
		t.Fatalf("intake errors = %d, want 1", got)
	}
}
