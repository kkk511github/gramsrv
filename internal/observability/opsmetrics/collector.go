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
	summaryInterval = time.Minute
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

	connectionsActive  atomic.Int64
	connectionsOpened  atomic.Int64
	connectionsClosed  atomic.Int64
	handshakes         atomic.Int64
	handshakeNanos     atomic.Int64
	handshakeMaxNanos  atomic.Int64
	rpcErrors          atomic.Int64
	inboundQueueNanos  atomic.Int64
	inboundQueueCount  atomic.Int64
	inboundDropped     atomic.Int64
	outboundErrors     atomic.Int64
	outboundResent     atomic.Int64
	outboundDropped    atomic.Int64
	outboundQueueWaits atomic.Int64
	intakeErrors       atomic.Int64
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
	summaryTicker := time.NewTicker(summaryInterval)
	cleanupTicker := time.NewTicker(cleanupInterval)
	defer flushTicker.Stop()
	defer summaryTicker.Stop()
	defer cleanupTicker.Stop()
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		c.flush(flushCtx)
		c.logConnectionSummary()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-flushTicker.C:
			c.flush(ctx)
		case <-summaryTicker.C:
			c.logConnectionSummary()
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

func (c *Collector) ConnOpened() {
	if c == nil {
		return
	}
	c.connectionsActive.Add(1)
	c.connectionsOpened.Add(1)
}

func (c *Collector) ConnClosed() {
	if c == nil {
		return
	}
	for current := c.connectionsActive.Load(); current > 0; current = c.connectionsActive.Load() {
		if c.connectionsActive.CompareAndSwap(current, current-1) {
			break
		}
	}
	c.connectionsClosed.Add(1)
}

func (c *Collector) HandshakeDone(d time.Duration) {
	if c == nil {
		return
	}
	nanos := max(d.Nanoseconds(), 0)
	c.handshakes.Add(1)
	c.handshakeNanos.Add(nanos)
	updateMax(&c.handshakeMaxNanos, nanos)
}

func (c *Collector) RPCHandled(_ string, _ time.Duration, err error) {
	if c == nil {
		return
	}
	c.rpcRequests.Add(1)
	if err != nil {
		c.rpcErrors.Add(1)
	}
}

func (c *Collector) InboundRPCQueued(string, int, int) {}

func (c *Collector) InboundRPCStarted(_ string, queueWait time.Duration) {
	if c == nil {
		return
	}
	c.inboundQueueCount.Add(1)
	c.inboundQueueNanos.Add(max(queueWait.Nanoseconds(), 0))
}

func (c *Collector) InboundRPCDropped(string, string) {
	if c != nil {
		c.inboundDropped.Add(1)
	}
}

func (c *Collector) OutboundSend(_ uint32, _ time.Duration, _ int, err error) {
	if c != nil && err != nil {
		c.outboundErrors.Add(1)
	}
}

func (c *Collector) OutboundResend(count int, err error) {
	if c == nil {
		return
	}
	if count > 0 {
		c.outboundResent.Add(int64(count))
	}
	if err != nil {
		c.outboundErrors.Add(1)
	}
}

func (c *Collector) OutboundDropped(string) {
	if c != nil {
		c.outboundDropped.Add(1)
	}
}

func (c *Collector) OutboundQueueWait(int, int) {
	if c != nil {
		c.outboundQueueWaits.Add(1)
	}
}

func (c *Collector) ConnectionIntake(_ string, outcome string, _ time.Duration) {
	if c != nil && outcome == "error" {
		c.intakeErrors.Add(1)
	}
}

func (c *Collector) logConnectionSummary() {
	if c == nil {
		return
	}
	opened := c.connectionsOpened.Swap(0)
	closed := c.connectionsClosed.Swap(0)
	handshakes := c.handshakes.Swap(0)
	handshakeNanos := c.handshakeNanos.Swap(0)
	handshakeMax := c.handshakeMaxNanos.Swap(0)
	rpcErrors := c.rpcErrors.Swap(0)
	queueCount := c.inboundQueueCount.Swap(0)
	queueNanos := c.inboundQueueNanos.Swap(0)
	inboundDropped := c.inboundDropped.Swap(0)
	outboundErrors := c.outboundErrors.Swap(0)
	outboundResent := c.outboundResent.Swap(0)
	outboundDropped := c.outboundDropped.Swap(0)
	outboundQueueWaits := c.outboundQueueWaits.Swap(0)
	intakeErrors := c.intakeErrors.Swap(0)
	active := c.connectionsActive.Load()
	if active == 0 && opened == 0 && closed == 0 && handshakes == 0 && rpcErrors == 0 &&
		queueCount == 0 && inboundDropped == 0 && outboundErrors == 0 && outboundResent == 0 &&
		outboundDropped == 0 && outboundQueueWaits == 0 && intakeErrors == 0 {
		return
	}
	var handshakeAvg, queueAvg time.Duration
	if handshakes > 0 {
		handshakeAvg = time.Duration(handshakeNanos / handshakes)
	}
	if queueCount > 0 {
		queueAvg = time.Duration(queueNanos / queueCount)
	}
	c.logger.Info("MTProto connection summary",
		zap.Int64("active", active),
		zap.Int64("opened", opened),
		zap.Int64("closed", closed),
		zap.Int64("handshakes", handshakes),
		zap.Duration("handshake_avg", handshakeAvg),
		zap.Duration("handshake_max", time.Duration(handshakeMax)),
		zap.Int64("rpc_errors", rpcErrors),
		zap.Duration("rpc_queue_avg", queueAvg),
		zap.Int64("rpc_dropped", inboundDropped),
		zap.Int64("outbound_errors", outboundErrors),
		zap.Int64("outbound_resent", outboundResent),
		zap.Int64("outbound_dropped", outboundDropped),
		zap.Int64("outbound_queue_waits", outboundQueueWaits),
		zap.Int64("intake_errors", intakeErrors),
	)
}

func updateMax(dst *atomic.Int64, value int64) {
	for current := dst.Load(); value > current; current = dst.Load() {
		if dst.CompareAndSwap(current, value) {
			return
		}
	}
}

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
