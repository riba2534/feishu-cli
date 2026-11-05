package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/apidiag"
	"github.com/riba2534/feishu-cli/internal/auth"
)

// --error-format json：仍以 fmt.Errorf("code=%d...") 返回的旧错误也要在信封里带 code/log_id；
// apidiag 有记录时与文本模式同源，带 missing_scopes 与 hint。
func TestErrorFormatJSON_LegacyErrorFields(t *testing.T) {
	withErrorFormat(t, errorFormatJSON)
	apidiag.Reset()
	t.Cleanup(apidiag.Reset)

	t.Run("从文本取 code 与 log_id", func(t *testing.T) {
		var buf bytes.Buffer
		writeCommandError(&buf, fmt.Errorf("妙搭 API 失败: code=99991679, msg=Unauthorized, log_id=2026ABCDEF"))
		e := decodeEnvelope(t, buf.String())
		if e["code"] != float64(99991679) || e["log_id"] != "2026ABCDEF" {
			t.Fatalf("旧式错误应带 code/log_id: %v", e)
		}
		if e["exit_code"] != float64(3) {
			t.Fatalf("exit_code=%v", e["exit_code"])
		}
	})

	t.Run("SDK CodeError 形态", func(t *testing.T) {
		var buf bytes.Buffer
		writeCommandError(&buf, errors.New("API 调用失败: msg:换取 tenant_access_token 失败,code:99991663"))
		if e := decodeEnvelope(t, buf.String()); e["code"] != float64(99991663) {
			t.Fatalf("SDK 形态应带 code: %v", e)
		}
	})

	t.Run("HTTP 状态码不当业务码", func(t *testing.T) {
		var buf bytes.Buffer
		writeCommandError(&buf, errors.New("下载失败: unexpected status code: 500"))
		e := decodeEnvelope(t, buf.String())
		if _, ok := e["code"]; ok {
			t.Fatalf("status code 不应解析为业务码: %v", e)
		}
		if _, ok := e["log_id"]; ok {
			t.Fatalf("不应有 log_id: %v", e)
		}
	})

	t.Run("apidiag 记录与文本模式同源", func(t *testing.T) {
		info, ok := apidiag.Parse(400, nil, []byte(`{"code":99991679,"msg":"Unauthorized","error":{"log_id":"2026REGLOG","permission_violations":[{"subject":"spark:app:read"}]}}`))
		if !ok {
			t.Fatal("测试数据应能解析")
		}
		apidiag.Record(info)
		err := fmt.Errorf("妙搭 API 失败: code=99991679, msg=Unauthorized")
		var buf bytes.Buffer
		writeCommandError(&buf, err)
		e := decodeEnvelope(t, buf.String())
		if e["code"] != float64(99991679) || e["log_id"] != "2026REGLOG" {
			t.Fatalf("应取 apidiag 记录: %v", e)
		}
		if s, _ := e["missing_scopes"].([]any); len(s) != 1 || s[0] != "spark:app:read" {
			t.Fatalf("missing_scopes 不符: %v", e["missing_scopes"])
		}
		hint, _ := e["hint"].(string)
		if !strings.Contains(hint, "spark:app:read") {
			t.Fatalf("hint 应与文本模式一致: %q", hint)
		}
		// 文本模式不变：同一错误的文本输出逐字节等于主文本 + 诊断行
		withErrorFormat(t, errorFormatText)
		var text bytes.Buffer
		writeCommandError(&text, err)
		if text.String() != legacyErrorOutput(err) || !strings.Contains(text.String(), hint) {
			t.Fatalf("文本模式变化: %q", text.String())
		}
	})
}

// noRefreshTokenStore 返回一个仍有效、但登录时未下发 refresh_token 的 token（签发应用 cli_issuer）。
func noRefreshTokenStore() *auth.TokenStore {
	return &auth.TokenStore{
		AccessToken: "u-no-refresh",
		ExpiresAt:   time.Now().Add(time.Hour),
		Scope:       "offline_access",
		AppID:       "cli_issuer",
	}
}

const issuerSettingsURL = "https://open.feishu.cn/app/cli_issuer/safe"

// settings_url 指向签发 token 的应用，而不是当前覆盖的 app_id。
func TestAuthStatus_NoRefreshSettingsURLUsesIssuerApp(t *testing.T) {
	writeVerifyToken(t, noRefreshTokenStore())
	t.Setenv("FEISHU_APP_ID", "cli_override")
	t.Setenv("FEISHU_APP_SECRET", "secret_x")

	stdout, stderr, err := runCLI(t, "auth", "status", "-o", "json")
	if err != nil {
		t.Fatalf("auth status: %v\n%s", err, stderr)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout 非 JSON: %v", err)
	}
	info, _ := out["no_refresh_token"].(map[string]any)
	if info == nil || info["settings_url"] != issuerSettingsURL {
		t.Fatalf("settings_url 应指向签发应用: %v", out["no_refresh_token"])
	}
	if fix, _ := info["fix"].(string); !strings.Contains(fix, issuerSettingsURL) {
		t.Fatalf("fix 应带签发应用链接: %q", fix)
	}
}

func TestAuthLoginEvent_NoRefreshSettingsURLUsesIssuerApp(t *testing.T) {
	event := buildAuthorizationCompleteEvent(noRefreshTokenStore(), buildLoginScopeSummary("a", "a"), "cli_override", "")
	info, _ := event["no_refresh_token"].(map[string]any)
	if info == nil || info["settings_url"] != issuerSettingsURL {
		t.Fatalf("login 事件 settings_url 应指向签发应用: %v", event["no_refresh_token"])
	}
	if hint, _ := event["hint"].(string); !strings.Contains(hint, issuerSettingsURL) {
		t.Fatalf("login hint 应带签发应用链接: %q", hint)
	}
	// token 未记录签发应用时回退到传入的 app_id
	store := noRefreshTokenStore()
	store.AppID = ""
	event = buildAuthorizationCompleteEvent(store, buildLoginScopeSummary("a", "a"), "cli_override", "")
	if info, _ := event["no_refresh_token"].(map[string]any); info["settings_url"] != "https://open.feishu.cn/app/cli_override/safe" {
		t.Fatalf("应回退到当前 app_id: %v", info)
	}
}

// doctor：token 有效但从未下发 refresh_token 时仍为 pass，但要给出与 auth status 一致的提示。
func TestDoctor_ValidTokenWithoutRefreshHasHint(t *testing.T) {
	writeVerifyToken(t, noRefreshTokenStore())
	t.Setenv("FEISHU_APP_ID", "cli_override")

	for _, r := range []checkResult{checkUserToken(), checkUserIdentity()} {
		if r.Status != "pass" {
			t.Fatalf("%s: 判定规则不应改变，got %s", r.Name, r.Status)
		}
		if !strings.Contains(r.Hint, "refresh_token") || !strings.Contains(r.Hint, issuerSettingsURL) {
			t.Fatalf("%s: 应提示未下发 refresh_token 并指向签发应用: %q", r.Name, r.Hint)
		}
	}

	// 有 refresh_token 时不加提示
	store := noRefreshTokenStore()
	store.RefreshToken = "r-ok"
	store.RefreshExpiresAt = time.Now().Add(24 * time.Hour)
	writeVerifyToken(t, store)
	for _, r := range []checkResult{checkUserToken(), checkUserIdentity()} {
		if r.Status != "pass" || r.Hint != "" {
			t.Fatalf("%s: 正常 token 不应有提示: %+v", r.Name, r)
		}
	}
}
