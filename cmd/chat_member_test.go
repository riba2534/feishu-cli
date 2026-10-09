package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newChatMemberListTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("member-id-type", "open_id", "")
	cmd.Flags().String("member-types", "", "")
	cmd.Flags().Int("page-size", 0, "")
	cmd.Flags().String("page-token", "", "")
	cmd.Flags().Bool("page-all", false, "")
	cmd.Flags().String("user-access-token", "", "")
	cmd.Flags().String("as", "auto", "")
	return cmd
}

// TestChatMemberListPageAllMergesUsersAndBots 验证 --page-all 合并多页的 users/bots，
// truncations 取最后一页并在 stderr 告警；旧字段 items 只含用户。
func TestChatMemberListPageAllMergesUsersAndBots(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	var pages []string
	cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/im/v1/chats/"+testChatID+"/members/list" {
			http.Error(w, "unexpected "+r.URL.Path, http.StatusNotFound)
			return
		}
		tok := r.URL.Query().Get("page_token")
		pages = append(pages, tok+"|"+r.URL.Query().Get("page_size"))
		w.Header().Set("Content-Type", "application/json")
		if tok == "" {
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"users":[{"member_id":"ou_u1","name":"甲"}],"bots":[{"member_id":"ou_b1","name":"机器人","app_id":"cli_b1"}],"truncations":[],"has_more":true,"page_token":"p2"}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"users":[{"member_id":"ou_u2","name":"乙"}],"bots":[],"truncations":[{"member_type":"user","limit":2}],"user_total":9,"bot_total":1,"has_more":false,"page_token":""}}`)
	}))
	defer cleanup()

	cmd := newChatMemberListTestCmd()
	mustSetFlag(t, cmd, "page-all", "true")
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	out := captureStdout(t, func() {
		if err := chatMemberListCmd.RunE(cmd, []string{testChatID}); err != nil {
			t.Fatalf("chat member list 返回错误: %v", err)
		}
	})

	if strings.Join(pages, ",") != "|100,p2|100" {
		t.Fatalf("翻页请求序列 = %v（--page-all 未指定 page-size 时应用 100）", pages)
	}
	var got struct {
		Items       []map[string]any `json:"items"`
		Users       []map[string]any `json:"users"`
		Bots        []map[string]any `json:"bots"`
		Truncations []map[string]any `json:"truncations"`
		UserTotal   int              `json:"user_total"`
		HasMore     bool             `json:"has_more"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("解析输出失败: %v\n%s", err, out)
	}
	if len(got.Items) != 2 || len(got.Users) != 2 || len(got.Bots) != 1 {
		t.Fatalf("items=%d users=%d bots=%d，期望 2/2/1", len(got.Items), len(got.Users), len(got.Bots))
	}
	if got.Bots[0]["app_id"] != "cli_b1" {
		t.Fatalf("bots 缺 app_id: %+v", got.Bots)
	}
	if len(got.Truncations) != 1 || got.UserTotal != 9 || got.HasMore {
		t.Fatalf("最后一页信号丢失: %+v", got)
	}
	if !strings.Contains(stderr.String(), "截断") {
		t.Fatalf("truncations 非空时应在 stderr 告警，got %q", stderr.String())
	}
}

func TestNormalizeChatMemberTypes(t *testing.T) {
	got, err := normalizeChatMemberTypes(" User, bot ,user ")
	if err != nil || got != "user,bot" {
		t.Fatalf("normalizeChatMemberTypes = %q, %v", got, err)
	}
	if _, err := normalizeChatMemberTypes("admin"); err == nil {
		t.Fatal("非法成员类型应报错")
	}
	if got, _ := normalizeChatMemberTypes(""); got != "" {
		t.Fatalf("空输入应返回空，got %q", got)
	}
}
