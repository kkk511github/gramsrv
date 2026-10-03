package rpc

import (
	"context"
	"errors"
	"reflect"

	"github.com/iamxvbaba/td/tg"
	"github.com/iamxvbaba/td/tlprofile"
	"telesrv/internal/domain"
)

type accountFeatureService interface {
	GetMainProfileTab(context.Context, int64) (domain.ProfileTab, error)
	SetMainProfileTab(context.Context, int64, domain.ProfileTab) error
	ConfirmBotConnection(context.Context, int64, int64, int64) error
}

func (r *Router) registerAccountFeatureCompat(d *tlprofile.Dispatcher) {
	registerRPC[*tg.AccountConfirmBotConnectionRequest](d, tlprofile.SemanticMethodAccountConfirmBotConnection, func(ctx context.Context, req *tg.AccountConfirmBotConnectionRequest) (any, error) {
		return r.onAccountConfirmBotConnection(ctx, req)
	})
	registerRPC[*tg.AccountSetMainProfileTabRequest](d, tlprofile.SemanticMethodAccountSetMainProfileTab, func(ctx context.Context, req *tg.AccountSetMainProfileTabRequest) (any, error) {
		return r.onAccountSetMainProfileTab(ctx, req)
	})
}

func (r *Router) accountFeatureOwner(ctx context.Context) (int64, error) {
	userID, ok, err := r.currentUserID(ctx)
	if err != nil {
		return 0, internalErr()
	}
	if !ok || userID <= 0 {
		return 0, authKeyUnregisteredErr()
	}
	if r.deps.Users == nil {
		return 0, notImplementedErr()
	}
	user, err := r.deps.Users.Self(ctx, userID)
	if err != nil {
		return 0, internalErr()
	}
	if user.Bot {
		return 0, botMethodInvalidErr()
	}
	if user.ID != userID || user.Deleted {
		return 0, authKeyUnregisteredErr()
	}
	return userID, nil
}

func accountFeatureErr(err error) error {
	switch {
	case errors.Is(err, domain.ErrBotBusinessMissing):
		return botBusinessMissingErr()
	case errors.Is(err, domain.ErrAccountFeatureUnavailable):
		return notImplementedErr()
	case errors.Is(err, domain.ErrAccountFeatureOwnerInvalid):
		return authKeyUnregisteredErr()
	case errors.Is(err, domain.ErrProfileTabInvalid):
		return inputConstructorInvalidErr()
	default:
		return internalErr()
	}
}

func (r *Router) onAccountConfirmBotConnection(ctx context.Context, req *tg.AccountConfirmBotConnectionRequest) (bool, error) {
	userID, err := r.accountFeatureOwner(ctx)
	if err != nil {
		return false, err
	}
	if req == nil || req.BotID == nil || reflect.ValueOf(req.BotID).IsNil() {
		return false, botBusinessMissingErr()
	}
	bot, found, err := r.connectedBusinessBotFromInput(ctx, userID, req.BotID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, botBusinessMissingErr()
	}
	svc, ok := r.deps.Account.(accountFeatureService)
	if !ok {
		return false, notImplementedErr()
	}
	if err := svc.ConfirmBotConnection(ctx, userID, bot.ID, r.clock.Now().Unix()); err != nil {
		return false, accountFeatureErr(err)
	}
	return true, nil
}

func (r *Router) onAccountSetMainProfileTab(ctx context.Context, req *tg.AccountSetMainProfileTabRequest) (bool, error) {
	userID, err := r.accountFeatureOwner(ctx)
	if err != nil {
		return false, err
	}
	if req == nil {
		return false, inputConstructorInvalidErr()
	}
	tab, err := domainProfileTab(req.Tab)
	if err != nil {
		return false, err
	}
	svc, ok := r.deps.Account.(accountFeatureService)
	if !ok {
		return false, notImplementedErr()
	}
	if err := svc.SetMainProfileTab(ctx, userID, tab); err != nil {
		return false, accountFeatureErr(err)
	}
	r.invalidateRPCProjectionForUser(userID)
	return true, nil
}

func domainProfileTab(tab tg.ProfileTabClass) (domain.ProfileTab, error) {
	if tab == nil || reflect.ValueOf(tab).IsNil() {
		return "", inputConstructorInvalidErr()
	}
	switch tab.(type) {
	case *tg.ProfileTabPosts:
		return domain.ProfileTabPosts, nil
	case *tg.ProfileTabGifts:
		return domain.ProfileTabGifts, nil
	case *tg.ProfileTabMedia:
		return domain.ProfileTabMedia, nil
	case *tg.ProfileTabFiles:
		return domain.ProfileTabFiles, nil
	case *tg.ProfileTabMusic:
		return domain.ProfileTabMusic, nil
	case *tg.ProfileTabVoice:
		return domain.ProfileTabVoice, nil
	case *tg.ProfileTabLinks:
		return domain.ProfileTabLinks, nil
	case *tg.ProfileTabGifs:
		return domain.ProfileTabGIFs, nil
	default:
		return "", inputConstructorInvalidErr()
	}
}

func tgProfileTab(tab domain.ProfileTab) tg.ProfileTabClass {
	switch tab {
	case domain.ProfileTabPosts:
		return &tg.ProfileTabPosts{}
	case domain.ProfileTabGifts:
		return &tg.ProfileTabGifts{}
	case domain.ProfileTabMedia:
		return &tg.ProfileTabMedia{}
	case domain.ProfileTabFiles:
		return &tg.ProfileTabFiles{}
	case domain.ProfileTabMusic:
		return &tg.ProfileTabMusic{}
	case domain.ProfileTabVoice:
		return &tg.ProfileTabVoice{}
	case domain.ProfileTabLinks:
		return &tg.ProfileTabLinks{}
	case domain.ProfileTabGIFs:
		return &tg.ProfileTabGifs{}
	default:
		return nil
	}
}

// Read after the projection cache so changes made on another router are visible
// immediately, including to other viewers. Deleted accounts bypass this path.
func (r *Router) applyMainProfileTabToUserFull(ctx context.Context, userID int64, full *tg.UserFull) error {
	svc, ok := r.deps.Account.(accountFeatureService)
	if !ok {
		return nil
	}
	tab, err := svc.GetMainProfileTab(ctx, userID)
	if err != nil {
		return internalErr()
	}
	full.MainTab = nil
	full.Flags2.Unset(20)
	if value := tgProfileTab(tab); value != nil {
		full.SetMainTab(value)
	}
	return nil
}
