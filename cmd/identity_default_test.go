package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const defaultAsTestAppID = "test_app_id"

// defaultAsTestEnv 隔离家目录（TestMain 已整体隔离，这里再给每个用例独立 HOME），
// 写入带 default_as 的临时 config.yaml，并启动统计请求的假飞书服务端。
// token 为 nil 时不写 token.json（模拟未登录）。
type defaultAsTestEnv struct {
	home        string
	refreshHits int32
	tenantHits  int32
	bizHits     int32
	lastBizAuth atomic.Value
	refreshFail atomic.Bool // true 时刷新端点返回 invalid_grant
}

func newDefaultAsTestEnv(t *testing.T, defaultAs string, token *auth.TokenStore) *defaultAsTestEnv {
	t.Helper()
	env := &defaultAsTestEnv{home: t.TempDir()}
	t.Setenv("HOME", env.home)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	t.Setenv("FEISHU_PROFILE", "")
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")
	t.Setenv(config.EnvDefaultAs, "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPortProbe(r) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/authen/v2/oauth/token":
			atomic.AddInt32(&env.refreshHits, 1)
			if env.refreshFail.Load() {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprint(w, `{"error":"invalid_grant","error_description":"refresh token is invalid"}`)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "u-refreshed", "refresh_token": "r-next", "expires_in": 7200, "refresh_token_expires_in": 86400})
		case strings.Contains(r.URL.Path, "tenant_access_token"):
			atomic.AddInt32(&env.tenantHits, 1)
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-bot-token","expire":7200}`)
		default:
			atomic.AddInt32(&env.bizHits, 1)
			env.lastBizAuth.Store(r.Header.Get("Authorization"))
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{}}`)
		}
	}))
	t.Cleanup(srv.Close)

	if token != nil {
		dir := filepath.Join(env.home, ".feishu-cli")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(token)
		if err := os.WriteFile(filepath.Join(dir, "token.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	viper.Reset()
	cfgFile := filepath.Join(t.TempDir(), "config.yaml")
	content := fmt.Sprintf("app_id: %q\napp_secret: \"test_app_secret\"\nbase_url: %q\n", defaultAsTestAppID, srv.URL)
	if defaultAs != "" {
		content += fmt.Sprintf("default_as: %q\n", defaultAs)
	}
	if err := os.WriteFile(cfgFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(cfgFile); err != nil {
		t.Fatalf("config.Init: %v", err)
	}
	t.Cleanup(viper.Reset)
	return env
}

func validUserToken() *auth.TokenStore {
	return &auth.TokenStore{
		AccessToken:      "u-valid-login",
		RefreshToken:     "r-valid",
		TokenType:        "Bearer",
		ExpiresAt:        time.Now().Add(time.Hour),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour),
		AppID:            defaultAsTestAppID,
	}
}

func expiredUserToken() *auth.TokenStore {
	tok := validUserToken()
	tok.AccessToken = "u-expired-login"
	tok.ExpiresAt = time.Now().Add(-time.Hour)
	return tok
}

// newDefaultAsCmd 构造带 --user-access-token、可选 --as（defAs 为 nil 表示不注册）的命令。
func newDefaultAsCmd(defAs *string) *cobra.Command {
	c := &cobra.Command{Use: "x"}
	c.Flags().String("user-access-token", "", "")
	c.Flags().Bool("dry-run", false, "")
	if defAs != nil {
		c.Flags().String("as", *defAs, "")
	}
	return c
}

// 所有支持 User 的 helper，按命令类别列出；返回空串表示 Bot。
var defaultAsHelpers = map[string]func(*cobra.Command) (string, error){
	"写类 resolveOptionalUserToken":             resolveOptionalUserToken,
	"读类 resolveOptionalUserTokenWithFallback": resolveOptionalUserTokenWithFallback,
	"--as 类 resolveIdentityToken":             resolveIdentityToken,
	"auto resolveAutoUserToken":               resolveAutoUserToken,
	"perm 旧默认 resolvePermIdentity":            resolvePermIdentity,
	"slides/board 旧默认 legacyBot": func(c *cobra.Command) (string, error) {
		return resolveIdentityWithLegacyDefault(c, legacyBotUnlessExplicitUserToken)
	},
	"破坏性 resolveOptionalUserTokenForDestructive": func(c *cobra.Command) (string, error) {
		return resolveOptionalUserTokenForDestructive(c, "drive pull --delete-local")
	},
}

func helperCmd(name string) *cobra.Command {
	switch {
	case strings.Contains(name, "perm"):
		c := newDefaultAsCmd(nil)
		addPermIdentityFlags(c)
		_ = c.LocalFlags() // 与 cobra 执行时一致：把 persistent flag 合并进 Flags()
		return c
	case strings.Contains(name, "slides/board"):
		return newDefaultAsCmd(strPtr("bot"))
	case strings.Contains(name, "--as 类"):
		// okr 等命令组的 --as 默认 bot：default_as=user 同样改为 User
		return newDefaultAsCmd(strPtr("bot"))
	case strings.Contains(name, "auto"):
		return newDefaultAsCmd(strPtr("auto"))
	}
	return newDefaultAsCmd(nil)
}

func TestNormalizeDefaultAsViaConfig(t *testing.T) {
	newDefaultAsTestEnv(t, "", nil)
	if got := configuredDefaultAs(); got != config.DefaultAsAuto {
		t.Fatalf("未配置 default_as 应为 auto，得到 %q", got)
	}
	newDefaultAsTestEnv(t, "User", nil)
	if got := configuredDefaultAs(); got != config.DefaultAsUser {
		t.Fatalf("config.yaml default_as: User 应规范化为 user，得到 %q", got)
	}
}

// default_as=auto（默认）：写类不读 token.json、--as 默认 bot 的命令仍是 Bot——与改动前完全一致。
func TestDefaultAsAutoKeepsExistingDefaults(t *testing.T) {
	newDefaultAsTestEnv(t, "auto", validUserToken())

	tok, err := resolveOptionalUserToken(newDefaultAsCmd(nil))
	if err != nil || tok != "" {
		t.Fatalf("auto 下写类命令应保持 Bot（不读 token.json），得到 token=%q err=%v", tok, err)
	}
	tok, err = resolveIdentityToken(newDefaultAsCmd(strPtr("bot")))
	if err != nil || tok != "" {
		t.Fatalf("auto 下 --as 默认 bot 的命令应保持 Bot，得到 token=%q err=%v", tok, err)
	}
	tok, err = resolvePermIdentity(helperCmd("perm"))
	if err != nil || tok != "" {
		t.Fatalf("auto 下 perm 应保持 Bot，得到 token=%q err=%v", tok, err)
	}
	tok, err = resolveOptionalUserTokenWithFallback(newDefaultAsCmd(nil))
	if err != nil || tok != "u-valid-login" {
		t.Fatalf("auto 下读类命令仍应 User 优先，得到 token=%q err=%v", tok, err)
	}
}

// default_as=auto 且未登录：读类命令照旧回落 Bot、不报错。
func TestDefaultAsAutoNoTokenFallsBackToBot(t *testing.T) {
	newDefaultAsTestEnv(t, "", nil)
	for name, fn := range defaultAsHelpers {
		tok, err := fn(helperCmd(name))
		if err != nil || tok != "" {
			t.Errorf("%s：auto 且未登录应回落 Bot，得到 token=%q err=%v", name, tok, err)
		}
	}
}

// default_as=user：所有支持 User 的 helper 都用 token.json 的登录态。
func TestDefaultAsUserUsesLoginToken(t *testing.T) {
	env := newDefaultAsTestEnv(t, "user", validUserToken())
	for name, fn := range defaultAsHelpers {
		tok, err := fn(helperCmd(name))
		if err != nil {
			t.Errorf("%s：default_as=user 不应报错: %v", name, err)
			continue
		}
		if tok != "u-valid-login" {
			t.Errorf("%s：default_as=user 应使用登录态 token，得到 %q", name, tok)
		}
	}
	if hits := atomic.LoadInt32(&env.tenantHits); hits != 0 {
		t.Errorf("default_as=user 不应请求 tenant token，得到 %d 次", hits)
	}
}

// 环境变量 FEISHU_DEFAULT_AS 覆盖 config.yaml。
func TestDefaultAsEnvOverridesConfigFile(t *testing.T) {
	// newDefaultAsTestEnv 会清空该环境变量并按 config.yaml（auto）初始化；之后设置环境变量再重新 Init
	newDefaultAsTestEnv(t, "auto", validUserToken())
	t.Setenv(config.EnvDefaultAs, "user")
	cfgFile := viper.ConfigFileUsed()
	viper.Reset()
	if err := config.Init(cfgFile); err != nil {
		t.Fatal(err)
	}
	tok, err := resolveOptionalUserToken(newDefaultAsCmd(nil))
	if err != nil || tok != "u-valid-login" {
		t.Fatalf("FEISHU_DEFAULT_AS=user 应覆盖 config.yaml 的 auto，得到 token=%q err=%v", tok, err)
	}
}

// access_token 过期：default_as=user 走自动刷新并写回 token.json。
func TestDefaultAsUserRefreshesExpiredToken(t *testing.T) {
	env := newDefaultAsTestEnv(t, "user", expiredUserToken())
	tok, err := resolveOptionalUserToken(newDefaultAsCmd(nil))
	if err != nil {
		t.Fatalf("refresh_token 有效时应自动刷新: %v", err)
	}
	if tok != "u-refreshed" {
		t.Fatalf("应返回刷新后的 token，得到 %q", tok)
	}
	if hits := atomic.LoadInt32(&env.refreshHits); hits != 1 {
		t.Fatalf("应刷新 1 次，得到 %d", hits)
	}
	stored, err := auth.LoadToken()
	if err != nil || stored == nil || stored.AccessToken != "u-refreshed" {
		t.Fatalf("刷新结果应写回 token.json，得到 %+v err=%v", stored, err)
	}
}

// 未登录 / 刷新失败：default_as=user 报鉴权错误（退出码 3）并给出修复提示，绝不回落 Bot。
func TestDefaultAsUserFailsClosed(t *testing.T) {
	cases := map[string]*auth.TokenStore{
		"未登录":  nil,
		"刷新失败": expiredUserToken(),
	}
	for label, token := range cases {
		t.Run(label, func(t *testing.T) {
			env := newDefaultAsTestEnv(t, "user", token)
			env.refreshFail.Store(true)
			for name, fn := range defaultAsHelpers {
				tok, err := fn(helperCmd(name))
				if err == nil {
					t.Errorf("%s：应报错而不是回落 Bot，得到 token=%q", name, tok)
					continue
				}
				if !clierr.HasKind(err, clierr.KindAuth) {
					t.Errorf("%s：应为鉴权类错误（退出码 3），得到 %v", name, clierr.Kinds(err))
				}
				for _, want := range []string{"default_as=user", "feishu-cli auth login", "--as bot", "FEISHU_DEFAULT_AS=auto"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("%s：错误信息缺少 %q：%v", name, want, err)
					}
				}
			}
			if hits := atomic.LoadInt32(&env.tenantHits); hits != 0 {
				t.Errorf("拿不到 User Token 时不应请求 tenant token（即不回落 Bot），得到 %d 次", hits)
			}
			if hits := atomic.LoadInt32(&env.bizHits); hits != 0 {
				t.Errorf("拿不到 User Token 时不应发业务请求，得到 %d 次", hits)
			}
		})
	}
}

// 显式参数优先于 default_as：--as bot 强制 Bot；--as auto 回到 auto 语义；--user-access-token 原样使用。
func TestDefaultAsUserExplicitFlagsWin(t *testing.T) {
	newDefaultAsTestEnv(t, "user", nil)

	c := newDefaultAsCmd(strPtr("auto"))
	_ = c.Flags().Set("as", "bot")
	if tok, err := resolveIdentityToken(c); err != nil || tok != "" {
		t.Fatalf("--as bot 应优先于 default_as=user，得到 token=%q err=%v", tok, err)
	}
	c = helperCmd("perm")
	if err := c.Flags().Set("as", "bot"); err != nil {
		t.Fatal(err)
	}
	if tok, err := resolvePermIdentity(c); err != nil || tok != "" {
		t.Fatalf("perm --as bot 应优先于 default_as=user，得到 token=%q err=%v", tok, err)
	}
	c = newDefaultAsCmd(strPtr("auto"))
	_ = c.Flags().Set("as", "auto")
	if tok, err := resolveIdentityToken(c); err != nil || tok != "" {
		t.Fatalf("显式 --as auto 且未登录应回落 Bot，得到 token=%q err=%v", tok, err)
	}
	c = newDefaultAsCmd(nil)
	_ = c.Flags().Set("user-access-token", "u-explicit")
	if tok, err := resolveOptionalUserToken(c); err != nil || tok != "u-explicit" {
		t.Fatalf("显式 --user-access-token 应原样使用，得到 token=%q err=%v", tok, err)
	}
}

// 只支持 Bot 的命令、声明忽略 default_as 的命令不受影响。
func TestDefaultAsUserSkipsBotOnlyAndIgnoredCommands(t *testing.T) {
	newDefaultAsTestEnv(t, "user", nil)

	// merge-forward 等只支持 Bot 的命令只看显式 token
	if tok := resolveExplicitUserToken(newDefaultAsCmd(nil)); tok != "" {
		t.Fatalf("resolveExplicitUserToken 不应受 default_as 影响，得到 %q", tok)
	}
	// user info 声明了 ignoreDefaultAsAnnotation：保持默认 Bot
	if getUserInfoCmd.Annotations[ignoreDefaultAsAnnotation] != "1" {
		t.Fatal("user info 应声明 ignoreDefaultAsAnnotation")
	}
	parent := &cobra.Command{Use: "group", Annotations: map[string]string{ignoreDefaultAsAnnotation: "1"}}
	child := newDefaultAsCmd(strPtr("bot"))
	parent.AddCommand(child)
	if defaultAsUserApplies(child) {
		t.Fatal("父命令声明忽略 default_as 时子命令也应忽略")
	}
	if tok, err := resolveIdentityToken(child); err != nil || tok != "" {
		t.Fatalf("忽略 default_as 的命令应保持自身默认 Bot，得到 token=%q err=%v", tok, err)
	}
}

// --dry-run 预览不读凭据：default_as=user 未登录也能预览，并展示 user 身份。
func TestDefaultAsUserDryRunPreviewIsStatic(t *testing.T) {
	env := newDefaultAsTestEnv(t, "user", nil)
	c := newDefaultAsCmd(nil)
	_ = c.Flags().Set("dry-run", "true")
	tok, err := resolveOptionalUserTokenOrPreview(c)
	if err != nil {
		t.Fatalf("dry-run 预览不应读凭据或报错: %v", err)
	}
	if tok == "" {
		t.Fatal("default_as=user 的 dry-run 预览应展示 user 身份")
	}
	if hits := atomic.LoadInt32(&env.refreshHits) + atomic.LoadInt32(&env.tenantHits) + atomic.LoadInt32(&env.bizHits); hits != 0 {
		t.Fatalf("dry-run 预览不应联网，得到 %d 次请求", hits)
	}
	if got := permIdentityLabel(helperCmd("perm")); got != "user" {
		t.Fatalf("perm dry-run 身份标签应为 user，得到 %q", got)
	}
}

// 非法 default_as 在 config.Validate 报用法错误（退出码 2）。
func TestDefaultAsInvalidRejectedByValidate(t *testing.T) {
	newDefaultAsTestEnv(t, "bot", nil)
	err := config.Validate()
	if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("非法 default_as 应在 Validate 报用法错误，得到 %v", err)
	}
	if !strings.Contains(err.Error(), "auto|user") {
		t.Fatalf("错误信息应提示合法取值，得到 %v", err)
	}
}

// 命令级：api 命令（--as 默认 auto）在 default_as=user 且未登录时不发任何请求；登录后以 User Token 调用。
func TestRunAPIDefaultAsUser(t *testing.T) {
	defer resetAPIFlags()

	t.Run("未登录 fail-closed", func(t *testing.T) {
		env := newDefaultAsTestEnv(t, "user", nil)
		resetAPIFlags()
		cmd := newTestAPICmd()
		err := cmd.RunE(cmd, []string{"GET", "/open-apis/im/v1/chats"})
		if err == nil || !strings.Contains(err.Error(), "default_as=user") {
			t.Fatalf("应以 default_as=user 提示报错，得到 %v", err)
		}
		if hits := atomic.LoadInt32(&env.tenantHits) + atomic.LoadInt32(&env.bizHits); hits != 0 {
			t.Fatalf("不应以 Bot 身份发任何请求，得到 %d 次", hits)
		}
	})

	t.Run("已登录用 User Token", func(t *testing.T) {
		env := newDefaultAsTestEnv(t, "user", validUserToken())
		resetAPIFlags()
		cmd := newTestAPICmd()
		if err := cmd.RunE(cmd, []string{"GET", "/open-apis/im/v1/chats"}); err != nil {
			t.Fatalf("已登录时应成功: %v", err)
		}
		if got, _ := env.lastBizAuth.Load().(string); got != "Bearer u-valid-login" {
			t.Fatalf("业务请求应携带登录态 User Token，得到 %q", got)
		}
		if hits := atomic.LoadInt32(&env.tenantHits); hits != 0 {
			t.Fatalf("不应请求 tenant token，得到 %d 次", hits)
		}
	})
}
