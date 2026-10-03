package domain

import "errors"

type ProfileTab string

const (
	ProfileTabPosts ProfileTab = "posts"
	ProfileTabGifts ProfileTab = "gifts"
	ProfileTabMedia ProfileTab = "media"
	ProfileTabFiles ProfileTab = "files"
	ProfileTabMusic ProfileTab = "music"
	ProfileTabVoice ProfileTab = "voice"
	ProfileTabLinks ProfileTab = "links"
	ProfileTabGIFs  ProfileTab = "gifs"
)

func (t ProfileTab) Valid() bool {
	switch t {
	case ProfileTabPosts, ProfileTabGifts, ProfileTabMedia, ProfileTabFiles, ProfileTabMusic, ProfileTabVoice, ProfileTabLinks, ProfileTabGIFs:
		return true
	default:
		return false
	}
}

var (
	ErrProfileTabInvalid          = errors.New("profile tab invalid")
	ErrAccountFeatureUnavailable  = errors.New("account feature unavailable")
	ErrAccountFeatureOwnerInvalid = errors.New("account feature owner invalid")
)
