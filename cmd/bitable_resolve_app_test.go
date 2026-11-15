package cmd

import (
	"encoding/json"
	"sync/atomic"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// /app/ 链接离线解析出 app_token / workspace_token / page_id（对齐官方 base +url-resolve），无凭据也能跑。
func TestBitableResolveBaseAppURLOffline(t *testing.T) {
	hits := isolateBitableNoCredEnv(t)
	stdout, stderr, err := runCLI(t, "bitable", "resolve", "--url",
		"https://example.larkoffice.com/app/app_123?pre_pathname=%2Fbase%2Fworkspace%2Fws_123&pageId=pge_123")
	if err != nil {
		t.Fatalf("BaseApp 链接应离线解析成功: %v %s", err, stderr)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("输出不是 JSON: %s", stdout)
	}
	if got["input_type"] != "baseapp_url" || got["resource_type"] != "baseapp" ||
		got["app_token"] != "app_123" || got["workspace_token"] != "ws_123" || got["page_id"] != "pge_123" {
		t.Errorf("BaseApp 坐标不对: %v", got)
	}
	if n := atomic.LoadInt32(hits); n != 0 {
		t.Errorf("离线解析不应联网，命中 %d 次", n)
	}

	stdout, _, err = runCLI(t, "bitable", "resolve", "--url", "https://example.feishu.cn/app/app_123", "--jq", ".app_token")
	if err != nil || stdout != "\"app_123\"\n" {
		t.Errorf("只带 app_token 的链接: out=%q err=%v", stdout, err)
	}
}

func TestBitableResolveBaseAppURLRejectsBadToken(t *testing.T) {
	isolateBitableNoCredEnv(t)
	_, _, err := runCLI(t, "bitable", "resolve", "--url", "https://example.feishu.cn/app/app_1?pageId=pg%22x")
	if got := exitCodeFor(err); got != clierr.ExitUsage {
		t.Fatalf("非法 page_id 应 exit 2，得到 %d (%v)", got, err)
	}
}

// 不带 table= 的 /base/ 链接、表单分享链接都是离线解析：无凭据 exit 0（原先先做 config.Validate 而 exit 3）。
func TestBitableResolveOfflineURLsWithoutCredentials(t *testing.T) {
	for _, url := range []string{
		"https://example.feishu.cn/base/bascnX",
		"https://example.feishu.cn/share/base/form/shrF",
	} {
		hits := isolateBitableNoCredEnv(t)
		_, stderr, err := runCLI(t, "bitable", "resolve", "--url", url)
		if err != nil {
			t.Errorf("%s: 离线解析应成功: %v %s", url, err, stderr)
		}
		if n := atomic.LoadInt32(hits); n != 0 {
			t.Errorf("%s: 不应联网，命中 %d 次", url, n)
		}
	}
	// 需要联网的（带 table= 判型）仍要求凭据
	isolateBitableNoCredEnv(t)
	_, _, err := runCLI(t, "bitable", "resolve", "--url", "https://example.feishu.cn/base/bascnX?table=tblA")
	if got := exitCodeFor(err); got != clierr.ExitAuth {
		t.Errorf("带 table= 需判型，无凭据应 exit 3，得到 %d (%v)", got, err)
	}
}
