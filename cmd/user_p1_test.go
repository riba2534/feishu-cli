package cmd

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newUserInfoTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("user-id-type", "open_id", "")
	cmd.Flags().String("department-id-type", "", "")
	cmd.Flags().StringP("output", "o", "", "")
	cmd.Flags().String("user-access-token", "", "")
	cmd.Flags().String("as", "bot", "")
	return cmd
}

// TestUserInfoIdentity 默认 bot 走 contact/v3/users/:id（旧行为）；--as user 走 basic_batch。
func TestUserInfoIdentity(t *testing.T) {
	for _, as := range []string{"bot", "user"} {
		t.Run(as, func(t *testing.T) {
			isolateMsgTokenTestEnv(t)
			var gotPath, gotAuth string
			cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/basic_batch") {
					_, _ = fmt.Fprint(w, `{"code":0,"data":{"users":[{"user_id":"ou_x","name":"张三"}]}}`)
					return
				}
				_, _ = fmt.Fprint(w, `{"code":0,"data":{"user":{"open_id":"ou_x","name":"张三"}}}`)
			}))
			defer cleanup()
			cmd := newUserInfoTestCmd()
			mustSetFlag(t, cmd, "as", as)
			if as == "user" {
				mustSetFlag(t, cmd, "user-access-token", testUserToken)
			}
			out := captureStdout(t, func() {
				if err := getUserInfoCmd.RunE(cmd, []string{"ou_x"}); err != nil {
					t.Fatalf("user info 返回错误: %v", err)
				}
			})
			if !strings.Contains(out, "张三") {
				t.Fatalf("输出缺姓名: %s", out)
			}
			if as == "bot" && (gotPath != "/open-apis/contact/v3/users/ou_x" || gotAuth != testTenantAuth) {
				t.Fatalf("bot 路径不符: %s %s", gotPath, gotAuth)
			}
			if as == "user" && (gotPath != "/open-apis/contact/v3/users/basic_batch" || gotAuth != "Bearer "+testUserToken) {
				t.Fatalf("user 路径不符: %s %s", gotPath, gotAuth)
			}
		})
	}
}

func TestUserSearchBotRequiresQuery(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("query", "", "")
	cmd.Flags().String("chat-ids", "oc_1", "")
	cmd.Flags().Bool("has-chatted", false, "")
	cmd.Flags().Int("page-size", 20, "")
	cmd.Flags().String("page-token", "", "")
	cmd.Flags().StringP("output", "o", "", "")
	cmd.Flags().String("user-access-token", "", "")
	if err := userSearchBotCmd.RunE(cmd, nil); err == nil || !strings.Contains(err.Error(), "--query") {
		t.Fatalf("缺 --query 应报错，got %v", err)
	}
}
