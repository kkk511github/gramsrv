package rpc

import "testing"

func TestChannelInviteHashFromLinkSupportsSafeLinkPrefixes(t *testing.T) {
	tests := []struct {
		name string
		link string
		want string
	}{
		{name: "bare hash", link: "abc123", want: "abc123"},
		{name: "plus hash", link: "+abc123", want: "abc123"},
		{name: "safe link public invite", link: "https://safelink.chat/+abc123", want: "abc123"},
		{name: "safe link join deep link", link: "safelink://join?invite=abc123", want: "abc123"},
		{name: "sali join deep link", link: "sali://join?invite=abc123", want: "abc123"},
		{name: "telegram join deep link", link: "tg://join?invite=abc123", want: "abc123"},
		{name: "legacy joinchat path", link: "https://t.me/joinchat/abc123", want: "abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := channelInviteHashFromLink(tt.link)
			if err != nil {
				t.Fatalf("channelInviteHashFromLink(%q) error = %v", tt.link, err)
			}
			if got != tt.want {
				t.Fatalf("channelInviteHashFromLink(%q) = %q, want %q", tt.link, got, tt.want)
			}
		})
	}
}
