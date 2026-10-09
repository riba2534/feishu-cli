package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/v2/internal/auth"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/runctx"
	"golang.org/x/term"
)

// credentialProbeTimeout 是凭证探测（换一次 tenant_access_token）的总超时。
const credentialProbeTimeout = 5 * time.Second

// probeTenantTokenFn 换取 tenant_access_token，测试可替换。
var probeTenantTokenFn = auth.FetchTenantAccessTokenResult

// readAppSecretFromStdin 读取 --app-secret-stdin 的 App Secret，避免 secret 进入 shell 历史与 ps 输出。
//
// stdin 是终端时不回显地提示输入；否则读取第一行（管道 / 重定向），去掉首尾空白。
func readAppSecretFromStdin(in io.Reader) (string, error) {
	if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprint(os.Stderr, "App Secret（输入不回显）: ")
		raw, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", clierr.Usagef("读取 App Secret 失败: %v", err)
		}
		return requireNonEmptySecret(string(raw))
	}
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", clierr.Usagef("从 stdin 读取 App Secret 失败: %v", err)
	}
	return requireNonEmptySecret(line)
}

func requireNonEmptySecret(raw string) (string, error) {
	secret := strings.TrimSpace(raw)
	if secret == "" {
		return "", clierr.Usagef("stdin 中没有读到 App Secret。用法示例: printf '%%s' \"$FEISHU_APP_SECRET\" | feishu-cli profile add work --app-id cli_xxx --app-secret-stdin")
	}
	return secret, nil
}

// resolveSecretFlags 合并 --app-secret 与 --app-secret-stdin（二者互斥）。
func resolveSecretFlags(plain string, fromStdin bool, in io.Reader) (string, error) {
	if fromStdin {
		if plain != "" {
			return "", clierr.Usagef("--app-secret 与 --app-secret-stdin 不能同时使用")
		}
		return readAppSecretFromStdin(in)
	}
	return plain, nil
}

// probeAppCredentials 用 app_id / app_secret 换取一次 tenant_access_token，校验凭证是否有效
// （对齐官方 cmd/config/init_probe.go）。
//
//   - 服务端确定性拒绝（invalid_client，如 secret 错误 20002、应用不存在 20048）→ 返回错误（退出码 3）
//   - 网络、超时、5xx 等无法判定的情况 → 返回告警文本，不阻断（不让上游抖动影响配置）
func probeAppCredentials(appID, appSecret, baseURL string) (warning string, err error) {
	ctx, cancel := context.WithTimeout(runctx.Root(), credentialProbeTimeout)
	defer cancel()
	if _, err := probeTenantTokenFn(ctx, appID, appSecret, baseURL); err != nil {
		if auth.IsCredentialRejected(err) {
			return "", clierr.Auth(fmt.Errorf("App 凭证校验失败（app_id 或 app_secret 不正确，配置未写入）: %w", err))
		}
		return fmt.Sprintf("凭证探测未完成（%v），已跳过校验", err), nil
	}
	return "", nil
}
