package main

import (
	"time"

	"telesrv/internal/mtprotoedge"
	obsmetrics "telesrv/internal/observability/metrics"
	"telesrv/internal/observability/opsmetrics"
	"telesrv/internal/rpc"
)

// runtimeMetricsFanout keeps the bounded Prometheus exporter and the durable
// admin-dashboard minute counters on the same production event stream.
type runtimeMetricsFanout struct {
	runtime    *obsmetrics.Registry
	operations *opsmetrics.Collector
}

var (
	_ mtprotoedge.Metrics                 = (*runtimeMetricsFanout)(nil)
	_ mtprotoedge.RPCResultMetrics        = (*runtimeMetricsFanout)(nil)
	_ mtprotoedge.LogicalOutboxMetrics    = (*runtimeMetricsFanout)(nil)
	_ mtprotoedge.ConnectionIntakeMetrics = (*runtimeMetricsFanout)(nil)
	_ rpc.Metrics                         = (*runtimeMetricsFanout)(nil)
)

func newRuntimeMetricsFanout(runtime *obsmetrics.Registry, operations *opsmetrics.Collector) *runtimeMetricsFanout {
	return &runtimeMetricsFanout{runtime: runtime, operations: operations}
}

func (m *runtimeMetricsFanout) ConnOpened() {
	m.runtime.ConnOpened()
	m.operations.ConnOpened()
}

func (m *runtimeMetricsFanout) ConnClosed() {
	m.runtime.ConnClosed()
	m.operations.ConnClosed()
}

func (m *runtimeMetricsFanout) HandshakeDone(d time.Duration) {
	m.runtime.HandshakeDone(d)
	m.operations.HandshakeDone(d)
}

func (m *runtimeMetricsFanout) RPCHandled(method string, d time.Duration, err error) {
	m.runtime.RPCHandled(method, d, err)
	m.operations.RPCHandled(method, d, err)
}

func (m *runtimeMetricsFanout) InboundRPCQueued(method string, length, capacity int) {
	m.runtime.InboundRPCQueued(method, length, capacity)
	m.operations.InboundRPCQueued(method, length, capacity)
}

func (m *runtimeMetricsFanout) InboundRPCStarted(method string, queueWait time.Duration) {
	m.runtime.InboundRPCStarted(method, queueWait)
	m.operations.InboundRPCStarted(method, queueWait)
}

func (m *runtimeMetricsFanout) InboundRPCDropped(method, reason string) {
	m.runtime.InboundRPCDropped(method, reason)
	m.operations.InboundRPCDropped(method, reason)
}

func (m *runtimeMetricsFanout) OutboundSend(typeID uint32, queueWait time.Duration, bytes int, err error) {
	m.runtime.OutboundSend(typeID, queueWait, bytes, err)
	m.operations.OutboundSend(typeID, queueWait, bytes, err)
}

func (m *runtimeMetricsFanout) OutboundResend(count int, err error) {
	m.runtime.OutboundResend(count, err)
	m.operations.OutboundResend(count, err)
}

func (m *runtimeMetricsFanout) OutboundDropped(reason string) {
	m.runtime.OutboundDropped(reason)
	m.operations.OutboundDropped(reason)
}

func (m *runtimeMetricsFanout) OutboundQueueWait(length, capacity int) {
	m.runtime.OutboundQueueWait(length, capacity)
	m.operations.OutboundQueueWait(length, capacity)
}

func (m *runtimeMetricsFanout) RPCResultPrepared(method, priority string, innerBytes, wireBytes int, compressed bool) {
	m.runtime.RPCResultPrepared(method, priority, innerBytes, wireBytes, compressed)
}

func (m *runtimeMetricsFanout) RPCResultDelivered(method string, egressLatency time.Duration, wireBytes int, err error) {
	m.runtime.RPCResultDelivered(method, egressLatency, wireBytes, err)
}

func (m *runtimeMetricsFanout) LogicalOutboxAcknowledged(bytes int, retainedFor time.Duration, rpcResult bool) {
	m.runtime.LogicalOutboxAcknowledged(bytes, retainedFor, rpcResult)
}

func (m *runtimeMetricsFanout) ConnectionIntake(stage, outcome string, d time.Duration) {
	m.runtime.ConnectionIntake(stage, outcome, d)
	m.operations.ConnectionIntake(stage, outcome, d)
}

func (m *runtimeMetricsFanout) MessageSend(d time.Duration, duplicate bool, err error) {
	m.runtime.MessageSend(d, duplicate, err)
	m.operations.MessageSend(d, duplicate, err)
}

func (m *runtimeMetricsFanout) MessageRateLimited(retryAfterSeconds int) {
	m.runtime.MessageRateLimited(retryAfterSeconds)
	m.operations.MessageRateLimited(retryAfterSeconds)
}

func (m *runtimeMetricsFanout) OutboxClaimed(count int) {
	m.runtime.OutboxClaimed(count)
	m.operations.OutboxClaimed(count)
}

func (m *runtimeMetricsFanout) OutboxDelivered(d time.Duration) {
	m.runtime.OutboxDelivered(d)
	m.operations.OutboxDelivered(d)
}

func (m *runtimeMetricsFanout) OutboxFailed(err error) {
	m.runtime.OutboxFailed(err)
	m.operations.OutboxFailed(err)
}
