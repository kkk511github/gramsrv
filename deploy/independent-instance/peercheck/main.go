// peercheck verifies contact safety flags without modifying production data.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/iamxvbaba/td/bin"
	"github.com/iamxvbaba/td/clock"
	"github.com/iamxvbaba/td/tg"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"telesrv/internal/app/contacts"
	"telesrv/internal/app/users"
	"telesrv/internal/domain"
	"telesrv/internal/rpc"
	"telesrv/internal/store/postgres"
)

func main() {
	owner := flag.Int64("owner", 0, "Viewer user ID")
	peer := flag.Int64("peer", 0, "Peer user ID")
	flag.Parse()
	if *owner <= 0 || *peer <= 0 || *owner == *peer {
		panic("distinct owner and peer IDs are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(os.Getenv("TELESRV_POSTGRES_DSN"))
	if err != nil {
		panic("invalid database configuration")
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		panic(err)
	}
	defer pool.Close()
	store := postgres.NewContactStore(pool)
	service := contacts.NewService(store)
	userStore := postgres.NewUserStore(pool)
	router := rpc.New(rpc.Config{}, rpc.Deps{Contacts: service, Users: users.NewService(userStore)}, zap.NewNop(), clock.System)
	for _, pair := range [][2]int64{{*owner, *peer}, {*peer, *owner}} {
		contact, found, err := store.Get(ctx, pair[0], pair[1])
		if err != nil {
			panic(err)
		}
		blocked, err := store.IsBlocked(ctx, pair[0], pair[1])
		if err != nil {
			panic(err)
		}
		settings, err := service.GetPeerSettings(ctx, pair[0], domain.Peer{Type: domain.PeerTypeUser, ID: pair[1]})
		if err != nil {
			panic(err)
		}
		fmt.Printf("viewer=%d peer=%d contact=%v mutual=%v blocked=%v block_prompt=%v\n", pair[0], pair[1], found, contact.Mutual, blocked, settings.BlockContact)
		if settings.BlockContact != (!found && !blocked) {
			panic("unexpected safety prompt")
		}
		user, exists, err := userStore.ByID(ctx, pair[1])
		if err != nil || !exists {
			panic("peer lookup failed")
		}
		var request bin.Buffer
		if err := (&tg.UsersGetFullUserRequest{ID: &tg.InputUser{UserID: user.ID, AccessHash: user.AccessHash}}).Encode(&request); err != nil {
			panic(err)
		}
		response, err := router.Dispatch(rpc.WithUserID(ctx, pair[0]), [8]byte{}, 0, &request)
		if err != nil {
			panic(err)
		}
		full, ok := response.(*tg.UsersUserFull)
		if !ok {
			panic(fmt.Sprintf("unexpected response %T", response))
		}
		fmt.Printf("profile blocked=%v stories_blocked=%v block_prompt=%v\n", full.FullUser.Blocked, full.FullUser.BlockedMyStoriesFrom, full.FullUser.Settings.BlockContact)
		if full.FullUser.Blocked != blocked || full.FullUser.BlockedMyStoriesFrom != blocked || full.FullUser.Settings.BlockContact != settings.BlockContact {
			panic("profile flags disagree with persisted contact relationship")
		}
	}
}
