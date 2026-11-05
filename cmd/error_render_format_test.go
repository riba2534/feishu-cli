package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/apidiag"
	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
)

// legacyErrorOutput 复刻改动前 Execute 的两段输出，用于逐字节对比。
func legacyErrorOutput(err error) string {
	var b bytes.Buffer
	if msg := err.Error(); msg != "" {
		fmt.Fprintln(&b, msg)
	}
	for _, line := range renderErrorDiagnostics(err) {
		fmt.Fprintln(&b, line)
	}
	return b.String()
}

func withErrorFormat(t *testing.T, v errorFormatValue, args ...string) {
	t.Helper()
	oldFmt, oldArgs := errorFormat, os.Args
	errorFormat = v
	os.Args = append([]string{"feishu-cli"}, args...)
	t.Cleanup(func() { errorFormat, os.Args = oldFmt, oldArgs })
}

func sampleErrors() map[string]error {
	missingScope := &client.APIError{Action: "搜索文档", Info: apidiag.Info{
		HTTPStatus: 400, Code: 99991679, Msg: "Unauthorized", LogID: "202601010000000000000000000000",
		MissingScopes: []string{"search:docs:read"},
	}}
	return map[string]error{
		"usage":       clierr.Usagef("--page-size 必须在 1-50 之间"),
		"auth":        clierr.Auth(auth.ErrNoUserTokenConfigured),
		"api":         fmt.Errorf("搜索失败: %w", missingScope),
		"general":     errors.New("普通错误"),
		"empty":       errors.New(""),
		"api_biz_err": &apiBizError{apiErr: &client.APIError{Info: apidiag.Info{Code: 1254404, Msg: "not found"}}, hint: "提示：确认资源 ID"},
	}
}

// 不开启 json 时与原两段输出逐字节一致。
func TestErrorRenderTextUnchanged(t *testing.T) {
	withErrorFormat(t, errorFormatText, "search", "docs", "x")
	for name, err := range sampleErrors() {
		var got bytes.Buffer
		writeCommandError(&got, err)
		if want := legacyErrorOutput(err); got.String() != want {
			t.Fatalf("%s: 文本输出变化\n got: %q\nwant: %q", name, got.String(), want)
		}
	}
}

func decodeEnvelope(t *testing.T, s string) map[string]any {
	t.Helper()
	if strings.Count(strings.TrimSpace(s), "\n") != 0 {
		t.Fatalf("应为单行 JSON: %q", s)
	}
	var out struct {
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal([]byte(s), &out); err != nil || out.Error == nil {
		t.Fatalf("非 JSON 信封: %v %q", err, s)
	}
	return out.Error
}

func TestErrorRenderJSONEnvelope(t *testing.T) {
	withErrorFormat(t, errorFormatJSON)
	errs := sampleErrors()
	cases := []struct {
		name     string
		exitCode float64
		kind     string
		check    func(t *testing.T, e map[string]any)
	}{
		{"usage", 2, "usage", func(t *testing.T, e map[string]any) {
			if _, ok := e["code"]; ok {
				t.Fatal("非 API 错误不应有 code")
			}
		}},
		{"auth", 3, "auth", nil},
		{"api", 3, "auth", func(t *testing.T, e map[string]any) {
			if e["code"] != float64(99991679) || e["log_id"] != "202601010000000000000000000000" {
				t.Fatalf("code/log_id 不符: %v", e)
			}
			if s, _ := e["missing_scopes"].([]any); len(s) != 1 || s[0] != "search:docs:read" {
				t.Fatalf("missing_scopes 不符: %v", e["missing_scopes"])
			}
			if h, _ := e["hint"].(string); h == "" {
				t.Fatal("应带修复建议")
			}
		}},
		{"general", 1, "general", func(t *testing.T, e map[string]any) {
			if _, ok := e["hint"]; ok {
				t.Fatal("无建议时应省略 hint")
			}
			if e["retryable"] != false {
				t.Fatalf("retryable=%v", e["retryable"])
			}
		}},
		{"api_biz_err", 1, "general", func(t *testing.T, e map[string]any) {
			if e["code"] != float64(1254404) || e["hint"] != "提示：确认资源 ID" {
				t.Fatalf("领域建议/code 不符: %v", e)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			writeCommandError(&buf, errs[c.name])
			e := decodeEnvelope(t, buf.String())
			if e["exit_code"] != c.exitCode || e["kind"] != c.kind || e["message"] != errs[c.name].Error() {
				t.Fatalf("基本字段不符: %v", e)
			}
			if c.check != nil {
				c.check(t, e)
			}
		})
	}

	var buf bytes.Buffer
	writeCommandError(&buf, clierr.Network(errors.New("dial tcp: i/o timeout")))
	if e := decodeEnvelope(t, buf.String()); e["retryable"] != true || e["kind"] != "network" {
		t.Fatalf("网络错误应可重试: %v", e)
	}
	buf.Reset()
	writeCommandError(&buf, &client.APIError{Info: apidiag.Info{Code: 99991400, Msg: "rate limited"}})
	if e := decodeEnvelope(t, buf.String()); e["retryable"] != true {
		t.Fatalf("限流应可重试: %v", e)
	}
}

// flag 解析失败时（未知 flag 在前）仍按原始参数识别 --error-format json；"--" 之后不算。
func TestErrorFormatArgsFallback(t *testing.T) {
	cases := map[string]struct {
		args []string
		want bool
	}{
		"等号":      {[]string{"--bogus", "--error-format=json"}, true},
		"空格":      {[]string{"--bogus", "--error-format", "JSON"}, true},
		"text":    {[]string{"--error-format", "text"}, false},
		"双横线之后不算": {[]string{"api", "--", "--error-format=json"}, false},
		"未传":      {[]string{"search", "docs"}, false},
	}
	t.Cleanup(func() { flagsParsed = false })
	for name, c := range cases {
		withErrorFormat(t, errorFormatText)
		flagsParsed = false
		if got := errorFormatIsJSON(c.args); got != c.want {
			t.Fatalf("%s: got %v", name, got)
		}
	}
}

// flag 正常解析时不再扫描原始参数：别的 flag 的取值里出现 --error-format=json 不得切换格式。
func TestErrorFormatIgnoresFlagValuesWhenParsed(t *testing.T) {
	withErrorFormat(t, errorFormatText)
	flagsParsed = true
	t.Cleanup(func() { flagsParsed = false })
	if errorFormatIsJSON([]string{"msg", "send", "--text", "--error-format=json"}) {
		t.Fatal("--text 的取值不应切换为 JSON 错误信封")
	}
}

// 端到端：经根命令解析的用法错误在 json 模式下输出信封；非法取值为用法错误。
func TestErrorRenderJSONViaCLI(t *testing.T) {
	withErrorFormat(t, errorFormatText)
	_, _, err := runCLI(t, "--error-format", "xml", "auth", "status")
	if err == nil || exitCodeFor(err) != clierr.ExitUsage {
		t.Fatalf("非法 --error-format 应为用法错误: %v", err)
	}
	_, _, err = runCLI(t, "--error-format", "json", "api", "FOO", "/open-apis/x")
	if err == nil {
		t.Fatal("期望报错")
	}
	if errorFormat != errorFormatJSON {
		t.Fatalf("flag 未生效: %q", errorFormat)
	}
	var buf bytes.Buffer
	writeCommandError(&buf, err)
	if e := decodeEnvelope(t, buf.String()); e["exit_code"] != float64(2) || e["kind"] != "usage" {
		t.Fatalf("信封不符: %v", e)
	}
}
