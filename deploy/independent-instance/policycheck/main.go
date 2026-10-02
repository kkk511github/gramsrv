// policycheck verifies persisted member-list policy without changing production data.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"telesrv/internal/domain"
	"telesrv/internal/store/postgres"
)

func main() {
	id := flag.Int64("channel", 0, "Channel ID to inspect (read-only)")
	flag.Parse()
	if *id <= 0 || os.Getenv("TELESRV_POSTGRES_DSN") == "" {
		panic("channel and TELESRV_POSTGRES_DSN are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(os.Getenv("TELESRV_POSTGRES_DSN"))
	if err != nil {
		panic("invalid database configuration")
	}
	config.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		panic(err)
	}
	defer pool.Close()
	rows, err := pool.Query(ctx, "SELECT user_id FROM channel_members WHERE channel_id=$1 AND status='active' ORDER BY user_id LIMIT 20", *id)
	if err != nil {
		panic(err)
	}
	var users []int64
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			panic(err)
		}
		users = append(users, userID)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
	rows.Close()
	if len(users) == 0 {
		panic("no active members")
	}
	store := postgres.NewChannelStore(pool)
	for _, userID := range users {
		view, err := store.GetChannel(ctx, userID, *id)
		if err != nil {
			panic(err)
		}
		list, err := store.GetParticipants(ctx, userID, *id, domain.ChannelParticipantsFilter{Kind: domain.ChannelParticipantsRecent}, 0, 100)
		if err != nil {
			panic(err)
		}
		fmt.Printf("user=%d role=%s hidden=%v returned_members=%d total=%d\n", userID, view.Self.Role, view.Channel.ParticipantsHidden, len(list.Participants), list.Count)
		if view.Channel.ParticipantsHidden && view.Self.Role == domain.ChannelRoleMember && len(list.Participants) != 0 {
			panic("hidden member list leaked")
		}
	}
}
