package client

import "testing"

// TestFlagConstantsMatchServerSchema 锁住服务端实际枚举值，
// 防止后续修改时误写成 0/1/2 顺序（参考 lark-cli shortcuts/im/helpers.go）。
func TestFlagConstantsMatchServerSchema(t *testing.T) {
	if flagItemTypeDefault != 0 || flagItemTypeThread != 4 || flagItemTypeMsgThread != 11 {
		t.Errorf("ItemType 常量与服务端不一致: default=%d(=0) thread=%d(=4) msg_thread=%d(=11)",
			flagItemTypeDefault, flagItemTypeThread, flagItemTypeMsgThread)
	}
	if flagFlagTypeFeed != 1 || flagFlagTypeMessage != 2 {
		t.Errorf("FlagType 常量与服务端不一致: feed=%d(=1) message=%d(=2)",
			flagFlagTypeFeed, flagFlagTypeMessage)
	}
}

func TestParseFlagItemType(t *testing.T) {
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"", flagItemTypeDefault, false},
		{"default", flagItemTypeDefault, false},
		{"thread", flagItemTypeThread, false},
		{"msg_thread", flagItemTypeMsgThread, false},
		{"unknown", 0, true},
	}
	for _, c := range cases {
		got, err := ParseFlagItemType(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseFlagItemType(%q) err=%v wantErr=%v", c.in, err, c.wantErr)
			continue
		}
		if err == nil && got != c.want {
			t.Errorf("ParseFlagItemType(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseFlagFlagType(t *testing.T) {
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"", flagFlagTypeMessage, false},
		{"message", flagFlagTypeMessage, false},
		{"feed", flagFlagTypeFeed, false},
		{"unknown", 0, true},
	}
	for _, c := range cases {
		got, err := ParseFlagFlagType(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseFlagFlagType(%q) err=%v wantErr=%v", c.in, err, c.wantErr)
			continue
		}
		if err == nil && got != c.want {
			t.Errorf("ParseFlagFlagType(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
