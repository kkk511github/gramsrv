package opsmetrics

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
)

const (
	flushInterval   = 10 * time.Second
	cleanupInterval = 6 * time.Hour
	metricRetention = 8 * 24 * time.Hour
)

type executor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// Collector aggregates bounded operational counters. It deliberately records
// no method names, user identifiers, message content, device tokens, or errors.
type Collector struct {
	db     executor
	logger *zap.Logger

	rpcRequests   atomic.Int64
	pushDelivered atomic.Int64
	pushFailed    atomic.Int64
}

func New(db executor, logger *zap.Logger) *Collector {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Collector{db: db, logger: logger}
}

func (c *Collector) Run(ctx context.Context) {
	if c == nil || c.db == nil {
		return
	}
	c.cleanup(ctx)
	flushTicker := time.NewTicker(flushInterval)
	cleanupTicker := time.NewTicker(cleanupInterval)
	defer flushTicker.Stop()
	defer cleanupTicker.Stop()
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		c.flush(flushCtx)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-flushTicker.C:
			c.flush(ctx)
		case <-cleanupTicker.C:
			c.cleanup(ctx)
		}
	}
}

func (c *Collector) flush(parent context.Context) {
	if c == nil || c.db == nil {
		return
	}
	rpcRequests := c.rpcRequests.Swap(0)
	pushDelivered := c.pushDelivered.Swap(0)
	pushFailed := c.pushFailed.Swap(0)
	if rpcRequests == 0 && pushDelivered == 0 && pushFailed == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	_, err := c.db.Exec(ctx, `
INSERT INTO operational_metric_minutes (
    bucket_at, rpc_requests, push_delivered, push_failed, updated_at
) VALUES (
    date_trunc('minute', now()), $1, $2, $3, now()
)
ON CONFLICT (bucket_at) DO UPDATE SET
    rpc_requests = operational_metric_minutes.rpc_requests + EXCLUDED.rpc_requests,
    push_delivered = operational_metric_minutes.push_delivered + EXCLUDED.push_delivered,
    push_failed = operational_metric_minutes.push_failed + EXCLUDED.push_failed,
    updated_at = now()`, rpcRequests, pushDelivered, pushFailed)
	if err == nil {
		return
	}

	// Preserve the increments for the next flush if PostgreSQL is transiently
	// unavailable. Concurrent increments remain additive.
	c.rpcRequests.Add(rpcRequests)
	c.pushDelivered.Add(pushDelivered)
	c.pushFailed.Add(pushFailed)
	c.logger.Warn("flush operational metrics failed", zap.Error(err))
}

func (c *Collector) cleanup(parent context.Context) {
	if c == nil || c.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	if _, err := c.db.Exec(ctx, `DELETE FROM operational_metric_minutes WHERE bucket_at < $1`, time.Now().UTC().Add(-metricRetention)); err != nil {
		c.logger.Warn("cleanup operational metrics failed", zap.Error(err))
	}
}

func (c *Collector) ConnOpened() {}

func (c *Collector) ConnClosed() {}

func (c *Collector) HandshakeDone(time.Duration) {}

func (c *Collector) RPCHandled(string, time.Duration, error) {
	if c != nil {
		c.rpcRequests.Add(1)
	}
}

func (c *Collector) InboundRPCQueued(string, int, int) {}

func (c *Collector) InboundRPCStarted(string, time.Duration) {}

func (c *Collector) InboundRPCDropped(string, string) {}

func (c *Collector) OutboundSend(uint32, time.Duration, int, error) {}

func (c *Collector) OutboundResend(int, error) {}

func (c *Collector) OutboundDropped(string) {}

func (c *Collector) OutboundQueueWait(int, int) {}

func (c *Collector) MessageSend(time.Duration, bool, error) {}

func (c *Collector) MessageRateLimited(int) {}

func (c *Collector) OutboxClaimed(int) {}

func (c *Collector) OutboxDelivered(time.Duration) {}

func (c *Collector) OutboxFailed(error) {}

func (c *Collector) PushDelivered() {
	if c != nil {
		c.pushDelivered.Add(1)
	}
}

func (c *Collector) PushFailed() {
	if c != nil {
		c.pushFailed.Add(1)
	}
}
