package cmd

import (
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// @_user_1 不得误伤 @_user_10：较长的 key 优先匹配。
func TestReplaceMentionKeys_PrefixKeys(t *testing.T) {
	str := func(s string) *string { return &s }
	mentions := []*larkim.Mention{
		{Key: str("@_user_1"), Name: str("Alice")},
		{Key: str("@_user_10"), Name: str("Bob")},
	}
	got := replaceMentionKeys("hi @_user_1 and @_user_10", mentions)
	if got != "hi @Alice and @Bob" {
		t.Fatalf("got %q", got)
	}
}
