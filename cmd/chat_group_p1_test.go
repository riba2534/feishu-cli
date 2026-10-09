package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
)

func TestValidateSearchChatsFilters(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		members []string
		modes   []string
		sort    string
		wantErr bool
	}{
		{"只按成员", "", []string{"ou_1"}, nil, "", false},
		{"成员+话题+排序", "", []string{"ou_1"}, []string{"topic"}, "member_count", false},
		{"非 open_id", "", []string{"u_1"}, nil, "", true},
		{"非法模式", "x", nil, []string{"p2p"}, "", true},
		{"非法排序", "x", nil, nil, "name", true},
		{"无条件却要过滤", "", nil, []string{"group"}, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateSearchChatsFilters(c.query, c.members, c.modes, c.sort)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if err != nil && !clierr.HasKind(err, clierr.KindUsage) {
				t.Fatalf("应为用法错误: %v", err)
			}
		})
	}
}

func TestResolveChatListTypesForIdentity(t *testing.T) {
	var errOut bytes.Buffer
	if _, err := resolveChatListTypesForIdentity(&errOut, []string{"p2p"}, ""); err == nil {
		t.Fatal("Bot 只列单聊应报错")
	}
	got, err := resolveChatListTypesForIdentity(&errOut, []string{"p2p", "group"}, "")
	if err != nil || strings.Join(got, ",") != "group" || !strings.Contains(errOut.String(), "去掉 p2p") {
		t.Fatalf("Bot 混合类型应去掉 p2p 并提示: %v %v %q", got, err, errOut.String())
	}
	got, _ = resolveChatListTypesForIdentity(&errOut, []string{"p2p"}, "u-x")
	if strings.Join(got, ",") != "p2p" {
		t.Fatalf("User 身份应保留 p2p: %v", got)
	}
	if _, err := normalizeChatListTypes("group,dm"); err == nil {
		t.Fatal("非法类型应报错")
	}
}

func TestFetchMutedChatSet(t *testing.T) {
	old := batchGetMuteStatus
	defer func() { batchGetMuteStatus = old }()
	batchGetMuteStatus = func(ids []string, token string) (map[string]bool, []string, error) {
		return map[string]bool{"oc_a": true, "oc_b": false}, []string{"oc_c"}, nil
	}
	var errOut bytes.Buffer
	if set := fetchMutedChatSet(&errOut, []string{"oc_a"}, ""); set != nil || !strings.Contains(errOut.String(), "仅对用户身份生效") {
		t.Fatalf("Bot 身份应跳过并提示: %v %q", set, errOut.String())
	}
	set := fetchMutedChatSet(&errOut, []string{"oc_a", "oc_b", "oc_c"}, "u-x")
	if !set["oc_a"] || set["oc_b"] || set["oc_c"] {
		t.Fatalf("只有免打扰的会话应在集合里: %v", set)
	}
}

func TestValidateCreateChatOptions(t *testing.T) {
	ok := client.CreateChatOptions{Name: "fp-test", ChatMode: "topic", BotIDs: []string{"cli_x"}}
	if err := validateCreateChatOptions(ok); err != nil {
		t.Fatalf("合法参数报错: %v", err)
	}
	bad := []client.CreateChatOptions{
		{Name: "x", ChatType: "public"},
		{Name: strings.Repeat("长", 61)},
		{Name: "fp", ChatMode: "p2p"},
		{Name: "fp", BotIDs: []string{"ou_x"}},
		{Name: "fp", BotIDs: []string{"cli_1", "cli_2", "cli_3", "cli_4", "cli_5", "cli_6"}},
	}
	for i, b := range bad {
		if err := validateCreateChatOptions(b); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Fatalf("case %d 应为用法错误: %v", i, err)
		}
	}
}
