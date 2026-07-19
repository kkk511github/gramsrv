package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

const overviewActivityLimit = 4

type overviewResponse struct {
	CheckedAt time.Time            `json:"checked_at"`
	Trend     []overviewTrendPoint `json:"trend"`
	Attention []overviewAttention  `json:"attention"`
	Activity  []overviewActivity   `json:"activity"`
}

type overviewTrendPoint struct {
	BucketAt             time.Time `json:"bucket_at"`
	MessagesPerMinute    float64   `json:"messages_per_minute"`
	RPCRequestsPerMinute *float64  `json:"rpc_requests_per_minute,omitempty"`
	PushSuccessRate      *float64  `json:"push_success_rate,omitempty"`
	MessageCount         int64     `json:"message_count"`
	RPCRequestCount      int64     `json:"rpc_request_count"`
	PushDelivered        int64     `json:"push_delivered"`
	PushFailed           int64     `json:"push_failed"`
}

type overviewAttention struct {
	ID           string     `json:"id"`
	State        string     `json:"state"`
	Count        int64      `json:"count"`
	DiscoveredAt *time.Time `json:"discovered_at,omitempty"`
}

type overviewActivity struct {
	ID             int64     `json:"id"`
	Action         string    `json:"action"`
	Actor          string    `json:"actor"`
	Status         string    `json:"status"`
	DryRun         bool      `json:"dry_run"`
	TargetUserID   int64     `json:"target_user_id,omitempty"`
	TargetPeerType string    `json:"target_peer_type,omitempty"`
	TargetPeerID   int64     `json:"target_peer_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

func (s *server) handleOverviewAPI(w http.ResponseWriter, r *http.Request) {
	if s.read == nil || s.read.pool == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "read store is not configured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	trend, err := s.read.overviewTrend(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	attention, err := s.read.overviewAttention(ctx, time.Now().Add(-s.cfg.UploadPartTTL))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	activity, err := s.read.overviewActivity(ctx, overviewActivityLimit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, overviewResponse{
		CheckedAt: time.Now().UTC(),
		Trend:     trend,
		Attention: attention,
		Activity:  activity,
	})
}

func (s *readStore) overviewTrend(ctx context.Context) ([]overviewTrendPoint, error) {
	rows, err := s.pool.Query(ctx, `
WITH limits AS (
    SELECT now() AS now_at,
           date_trunc('hour', now()) - interval '23 hours' AS start_at,
           date_trunc('hour', now()) AS end_at
),
buckets AS (
    SELECT generate_series(start_at, end_at, interval '1 hour') AS bucket_at
    FROM limits
),
message_events AS (
    SELECT date_trunc('hour', pm.created_at) AS bucket_at
    FROM private_messages pm, limits l
    WHERE pm.created_at >= l.start_at
    UNION ALL
    SELECT date_trunc('hour', cm.created_at) AS bucket_at
    FROM channel_messages cm, limits l
    WHERE cm.created_at >= l.start_at
),
message_counts AS (
    SELECT bucket_at, COUNT(*)::bigint AS message_count
    FROM message_events
    GROUP BY bucket_at
),
metric_counts AS (
    SELECT date_trunc('hour', om.bucket_at) AS bucket_at,
           SUM(om.rpc_requests)::bigint AS rpc_request_count,
           SUM(om.push_delivered)::bigint AS push_delivered,
           SUM(om.push_failed)::bigint AS push_failed
    FROM operational_metric_minutes om, limits l
    WHERE om.bucket_at >= l.start_at
    GROUP BY date_trunc('hour', om.bucket_at)
)
SELECT b.bucket_at,
       ROUND((COALESCE(mc.message_count, 0)::numeric / duration.minutes)::numeric, 2)::double precision AS messages_per_minute,
       CASE WHEN oc.bucket_at IS NULL THEN NULL
            ELSE ROUND((oc.rpc_request_count::numeric / duration.minutes)::numeric, 2)::double precision
       END AS rpc_requests_per_minute,
       CASE WHEN COALESCE(oc.push_delivered, 0) + COALESCE(oc.push_failed, 0) = 0 THEN NULL
            ELSE ROUND((100.0 * oc.push_delivered::numeric /
                       (oc.push_delivered + oc.push_failed)::numeric), 1)::double precision
       END AS push_success_rate,
       COALESCE(mc.message_count, 0)::bigint,
       COALESCE(oc.rpc_request_count, 0)::bigint,
       COALESCE(oc.push_delivered, 0)::bigint,
       COALESCE(oc.push_failed, 0)::bigint
FROM buckets b
CROSS JOIN limits l
CROSS JOIN LATERAL (
    SELECT CASE WHEN b.bucket_at = l.end_at
                THEN GREATEST(1.0, LEAST(60.0, EXTRACT(EPOCH FROM (l.now_at - b.bucket_at)) / 60.0))
                ELSE 60.0
           END AS minutes
) duration
LEFT JOIN message_counts mc USING (bucket_at)
LEFT JOIN metric_counts oc USING (bucket_at)
ORDER BY b.bucket_at`)
	if err != nil {
		return nil, fmt.Errorf("query overview trend: %w", err)
	}
	defer rows.Close()

	points := make([]overviewTrendPoint, 0, 24)
	for rows.Next() {
		var point overviewTrendPoint
		var rpcRate, pushRate pgtype.Float8
		if err := rows.Scan(
			&point.BucketAt,
			&point.MessagesPerMinute,
			&rpcRate,
			&pushRate,
			&point.MessageCount,
			&point.RPCRequestCount,
			&point.PushDelivered,
			&point.PushFailed,
		); err != nil {
			return nil, fmt.Errorf("scan overview trend: %w", err)
		}
		if rpcRate.Valid {
			value := rpcRate.Float64
			point.RPCRequestsPerMinute = &value
		}
		if pushRate.Valid {
			value := pushRate.Float64
			point.PushSuccessRate = &value
		}
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate overview trend: %w", err)
	}
	return points, nil
}

func (s *readStore) overviewAttention(ctx context.Context, staleUploadCutoff time.Time) ([]overviewAttention, error) {
	var (
		frozenCount int64
		pushCount   int64
		mediaCount  int64
		frozenAt    pgtype.Timestamptz
		pushAt      pgtype.Timestamptz
		mediaAt     pgtype.Timestamptz
	)
	err := s.pool.QueryRow(ctx, `
SELECT
    (SELECT COUNT(*) FROM account_restrictions WHERE frozen),
    (SELECT MIN(COALESCE(frozen_since, updated_at)) FROM account_restrictions WHERE frozen),
    (SELECT COUNT(*) FROM push_notification_outbox WHERE attempts > 0),
    (SELECT MIN(created_at) FROM push_notification_outbox WHERE attempts > 0),
    (SELECT COUNT(*) FROM (
        SELECT owner_user_id, file_id
        FROM upload_parts
        WHERE created_at < $1
        GROUP BY owner_user_id, file_id
    ) stale_uploads),
    (SELECT MIN(created_at) FROM upload_parts WHERE created_at < $1)`, staleUploadCutoff).Scan(
		&frozenCount, &frozenAt,
		&pushCount, &pushAt,
		&mediaCount, &mediaAt,
	)
	if err != nil {
		return nil, fmt.Errorf("query overview attention: %w", err)
	}
	return []overviewAttention{
		attentionItem("frozen_accounts", frozenCount, "needs_review", frozenAt),
		attentionItem("push_retries", pushCount, "retrying", pushAt),
		attentionItem("stale_uploads", mediaCount, "overdue", mediaAt),
	}, nil
}

func attentionItem(id string, count int64, issueState string, discovered pgtype.Timestamptz) overviewAttention {
	state := "healthy"
	var discoveredAt *time.Time
	if count > 0 {
		state = issueState
		if discovered.Valid {
			value := discovered.Time
			discoveredAt = &value
		}
	}
	return overviewAttention{ID: id, State: state, Count: count, DiscoveredAt: discoveredAt}
}

func (s *readStore) overviewActivity(ctx context.Context, limit int) ([]overviewActivity, error) {
	if limit <= 0 || limit > 20 {
		limit = overviewActivityLimit
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, action, actor, status, dry_run,
       target_user_id, target_peer_type, target_peer_id, created_at
FROM admin_audit_logs
ORDER BY id DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("query overview activity: %w", err)
	}
	defer rows.Close()

	activity := make([]overviewActivity, 0, limit)
	for rows.Next() {
		var item overviewActivity
		if err := rows.Scan(
			&item.ID, &item.Action, &item.Actor, &item.Status, &item.DryRun,
			&item.TargetUserID, &item.TargetPeerType, &item.TargetPeerID, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan overview activity: %w", err)
		}
		activity = append(activity, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate overview activity: %w", err)
	}
	return activity, nil
}
