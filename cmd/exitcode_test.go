package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

func TestExitCodeFor(t *testing.T) {
	dialErr := &url.Error{Op: "Post", URL: "https://open.feishu.cn/x", Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")}}
	policyErr := &url.Error{Op: "Get", URL: "https://evil.example.com", Err: errors.New("拒绝自定义远端 host \"evil.example.com\"")}

	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil 成功", nil, 0},
		{"普通错误", errors.New("获取文档失败: something"), 1},
		{"资源级无权限仍是业务错误", errors.New("获取文档失败: code=1770032, msg=forbidden"), 1},
		{"99991672 应用未开通 scope", errors.New("获取文档失败: code=99991672, msg=Access denied"), 3},
		{"99991679 用户未授权 scope（HTTP 400 body 形态）", errors.New(`HTTP 400, body: {"code":99991679,"msg":"Unauthorized"}`), 3},
		{"99991668 user token 不支持", fmt.Errorf("查询失败: code=99991668, msg=user access token not support"), 3},
		{"refresh_token 终态 20037", errors.New(`token 端点返回 HTTP 400: {"code":20037,"error":"invalid_grant"}`), 3},
		{"log_id 含同数字串不误判", errors.New("失败: code=10020, msg=x, log_id=2026999916720000"), 1},
		{"未登录哨兵", fmt.Errorf("搜索需要 User Token: %w", auth.ErrNoUserTokenConfigured), 3},
		{"类型化鉴权错误", clierr.Authf("缺少 app_id"), 3},
		{"鉴权包装网络错误按网络处理", clierr.Auth(fmt.Errorf("自动刷新 Access Token 失败: %w", dialErr)), 4},
		{"context 超时", fmt.Errorf("调用失败: %w", context.DeadlineExceeded), 4},
		{"DNS 失败", &net.DNSError{Err: "no such host", Name: "open.feishu.cn", IsNotFound: true}, 4},
		{"SDK 拨号失败类型", fmt.Errorf("API 调用失败: %w", &larkcore.DialFailedError{}), 4},
		{"%v 拼接丢链的超时文本", errors.New("请求失败: Get \"https://open.feishu.cn\": net/http: TLS handshake timeout"), 4},
		{"host 白名单拒绝不是网络错误", fmt.Errorf("请求失败: %w", policyErr), 1},
		{"需要确认", clierr.ConfirmationRequiredf("需要 --yes"), 10},
		{"用户取消", clierr.Cancelledf("操作已取消"), 1},
		{"类型化用法错误", clierr.Usagef("bad"), 2},
		{"业务 msg 含 invalid argument 不算用法错误", errors.New("失败: code=1, msg=invalid argument \"x\""), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exitCodeFor(tc.err); got != tc.want {
				t.Fatalf("exitCodeFor(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

// TestExitCodeForCobraUsageErrors 用真实 cobra 树验证各类用法错误都映射为退出码 2。
func TestExitCodeForCobraUsageErrors(t *testing.T) {
	newTree := func() *cobra.Command {
		root := &cobra.Command{Use: "root", SilenceErrors: true, SilenceUsage: true}
		group := &cobra.Command{Use: "doc"}
		get := &cobra.Command{Use: "get", Args: cobra.ExactArgs(1), RunE: func(*cobra.Command, []string) error { return nil }}
		get.Flags().String("type", "", "")
		del := &cobra.Command{Use: "delete", RunE: func(*cobra.Command, []string) error { return nil }}
		del.Flags().String("id", "", "")
		_ = del.MarkFlagRequired("id")
		group.AddCommand(get, del)
		root.AddCommand(group)
		installUnknownSubcommandGuard(root)
		root.SetFlagErrorFunc(flagSuggestionErrorFunc)
		return root
	}
	cases := map[string][]string{
		"未知根命令":    {"notacmd"},
		"未知子命令":    {"doc", "gett"},
		"未知 flag":  {"doc", "get", "x", "--typo"},
		"参数个数错误":   {"doc", "get"},
		"缺必填 flag": {"doc", "delete"},
		"flag 缺值":  {"doc", "get", "x", "--type"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			root := newTree()
			root.SetArgs(args)
			err := root.Execute()
			if err == nil {
				t.Fatalf("期望用法错误，实际成功")
			}
			if got := exitCodeFor(err); got != clierr.ExitUsage {
				t.Fatalf("exitCodeFor(%q) = %d, want 2", err, got)
			}
		})
	}
}
