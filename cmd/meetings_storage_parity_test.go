package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// ---------- perm：URL 解析 / --member-kind / 非 JSON 错误脱敏 ----------

func newPermTargetTestCmd(docType string) *cobra.Command {
	c := &cobra.Command{Use: "x"}
	c.Flags().String("doc-type", "docx", "")
	if docType != "" {
		_ = c.Flags().Set("doc-type", docType)
	}
	return c
}

func TestPermResolveTarget(t *testing.T) {
	cases := []struct {
		raw, docType       string
		wantToken, wantTyp string
		wantUsage          bool
	}{
		{raw: "doxBare", wantToken: "doxBare", wantTyp: "docx"},
		{raw: "shtBare", docType: "sheet", wantToken: "shtBare", wantTyp: "sheet"},
		{raw: "https://example.feishu.cn/docx/doxAbc", wantToken: "doxAbc", wantTyp: "docx"},
		{raw: "https://example.feishu.cn/wiki/wikAbc?from=x", wantToken: "wikAbc", wantTyp: "wiki"},
		{raw: "https://example.feishu.cn/file/boxAbc", wantToken: "boxAbc", wantTyp: "file"},
		{raw: "https://example.feishu.cn/sheets/shtAbc", wantToken: "shtAbc", wantTyp: "sheet"},
		{raw: "https://example.feishu.cn/minutes/obcnAbc", wantToken: "obcnAbc", wantTyp: "minutes"},
		{raw: "https://example.feishu.cn/sheets/shtAbc", docType: "sheet", wantToken: "shtAbc", wantTyp: "sheet"},
		{raw: "https://example.feishu.cn/sheets/shtAbc", docType: "docx", wantUsage: true},
		{raw: "https://example.feishu.cn/minutes/obcnAbc", docType: "docx", wantUsage: true},
		{raw: "https://example.feishu.cn/unknown/abc", wantUsage: true},
		{raw: "https://evil.example.com/docx/doxAbc", wantUsage: true},
	}
	for _, tc := range cases {
		tok, typ, err := resolvePermTarget(newPermTargetTestCmd(tc.docType), tc.raw)
		if tc.wantUsage {
			if err == nil || exitCodeFor(err) != clierr.ExitUsage {
				t.Errorf("%s (doc-type=%q) 应为用法错误，got %v", tc.raw, tc.docType, err)
			}
			continue
		}
		if err != nil || tok != tc.wantToken || typ != tc.wantTyp {
			t.Errorf("%s (doc-type=%q) = (%q,%q,%v)，期望 (%q,%q)", tc.raw, tc.docType, tok, typ, err, tc.wantToken, tc.wantTyp)
		}
	}
}

func TestPermMemberKind(t *testing.T) {
	if k, err := resolvePermMemberKind("wikispaceid", "WIKI_SPACE_VIEWER", ""); err != nil || k != "wiki_space_viewer" {
		t.Fatalf("wikispaceid + member-kind 应通过并规范化，got %q %v", k, err)
	}
	if k, err := resolvePermMemberKind("openid", "", ""); err != nil || k != "" {
		t.Fatalf("非 wikispaceid 不带 member-kind 应通过，got %q %v", k, err)
	}
	for _, tc := range [][3]string{
		{"wikispaceid", "", ""},                             // 缺 member-kind
		{"wikispaceid", "bogus", ""},                        // 非法取值
		{"wikispaceid", "wiki_space_member", "single_page"}, // 与 perm-type 冲突
		{"openid", "wiki_space_member", ""},                 // 非 wikispaceid 传了 member-kind
	} {
		if _, err := resolvePermMemberKind(tc[0], tc[1], tc[2]); err == nil || exitCodeFor(err) != clierr.ExitUsage {
			t.Errorf("%v 应为用法错误，got %v", tc, err)
		}
	}
}

func TestPermSanitizeNonJSONResponseError(t *testing.T) {
	raw := errors.New("获取权限列表失败: response content-type not json, response: StatusCode: 404, Header:map[Eagleid:[abc] X-Tt-Trace-Host:[xyz]], Content: Body: 404 page not found")
	got := sanitizeNonJSONResponseError(raw).Error()
	for _, leak := range []string{"Eagleid", "X-Tt-Trace-Host", "Header:map"} {
		if strings.Contains(got, leak) {
			t.Fatalf("错误不应外泄传输头 %q: %s", leak, got)
		}
	}
	if !strings.Contains(got, "HTTP 404") || !strings.Contains(got, "获取权限列表失败") || !strings.Contains(got, "404 page not found") {
		t.Fatalf("应保留动作、状态码与 body: %s", got)
	}
	plain := errors.New("code=1063001")
	if sanitizeNonJSONResponseError(plain) != plain {
		t.Fatal("非该类错误应原样返回")
	}
	if !strings.Contains(wrapPermError(raw, "").Error(), "HTTP 404") || strings.Contains(wrapPermError(raw, "").Error(), "Eagleid") {
		t.Fatal("wrapPermError 应接入脱敏")
	}
}

func TestPermDryRunBodies(t *testing.T) {
	clearCmdCredentials(t)
	out, _, err := runCLI(t, "perm", "add", "https://example.feishu.cn/wiki/wikAbc", "--member-type", "wikispaceid",
		"--member-id", "7123", "--member-kind", "wiki_space_editor", "--perm", "edit", "--dry-run")
	if err != nil {
		t.Fatalf("perm add --dry-run: %v", err)
	}
	for _, want := range []string{`"type": "wiki_space_editor"`, `"/open-apis/drive/v1/permissions/wikAbc/members"`, `"type": "wiki"`, `"as": "bot"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("perm add 预览缺 %s:\n%s", want, out)
		}
	}
	out, _, err = runCLI(t, "perm", "delete", "doxAbc", "--member-type", "wikispaceid", "--member-id", "7123", "--member-kind", "wiki_space_member", "--dry-run")
	if err != nil || !strings.Contains(out, `"DELETE"`) || !strings.Contains(out, `"type": "wiki_space_member"`) {
		t.Fatalf("perm delete 预览应带 body.type: %v\n%s", err, out)
	}
	out, _, err = runCLI(t, "perm", "public-update", "doxAbc", "--link-share-entity", "tenant_readable", "--dry-run")
	if err != nil || !strings.Contains(out, `"link_share_entity": "tenant_readable"`) || strings.Contains(out, "external_access") {
		t.Fatalf("public-update 预览只应包含已指定字段: %v\n%s", err, out)
	}
}

// ---------- 无凭据 --dry-run：A2 + 新命令 ----------

// clearCmdCredentials 清空进程内 App 凭证：同包其他测试可能把 mock 凭证留在全局 config 里。
func clearCmdCredentials(t *testing.T) {
	t.Helper()
	// 同包其他测试可能把 viper 指向带 app_id 的临时 config.yaml 且未复位
	viper.Reset()
	t.Cleanup(viper.Reset)
	cfg := config.Get()
	origID, origSecret, origUAT := cfg.AppID, cfg.AppSecret, cfg.UserAccessToken
	cfg.AppID, cfg.AppSecret, cfg.UserAccessToken = "", "", ""
	t.Cleanup(func() { cfg.AppID, cfg.AppSecret, cfg.UserAccessToken = origID, origSecret, origUAT })
}

func TestMeetingsStorage_DryRunWithoutCredentials(t *testing.T) {
	clearCmdCredentials(t)
	cases := [][]string{
		{"perm", "list", "https://example.feishu.cn/docx/doxAbc"},
		{"perm", "auth", "doxAbc", "--action", "view"},
		{"perm", "public-get", "doxAbc"},
		{"perm", "add", "doxAbc", "--member-type", "email", "--member-id", "user@example.com", "--perm", "view"},
		{"perm", "update", "doxAbc", "--member-type", "openid", "--member-id", "ou_x", "--perm", "edit"},
		{"perm", "delete", "doxAbc", "--member-type", "openid", "--member-id", "ou_x"},
		{"perm", "transfer-owner", "doxAbc", "--member-type", "openid", "--member-id", "ou_x"},
		{"perm", "public-update", "doxAbc", "--external-access=false"},
		{"perm", "password", "create", "doxAbc"},
		{"perm", "password", "update", "doxAbc"},
		{"perm", "password", "delete", "doxAbc"},
		{"drive", "move", "--file-token", "doxAbc", "--type", "docx", "--folder-token", "fldAbc"},
		{"drive", "export", "--token", "doxAbc", "--doc-type", "docx", "--file-extension", "pdf"},
		{"drive", "update-title", "--token", "doxAbc", "--type", "docx", "--title", "t"},
		{"drive", "apply-permission", "--token", "doxAbc", "--type", "docx", "--perm", "view"},
		{"drive", "preview", "boxAbc", "--list-only"},
		{"drive", "preview", "boxAbc", "--type", "source_file", "-o", filepath.Join(t.TempDir(), "src")},
		{"drive", "cover", "boxAbc", "--spec", "default", "-o", filepath.Join(t.TempDir(), "cov.png")},
		{"file", "delete", "doxAbc", "--type", "docx"},
		{"comment", "reply", "update", "doxAbc", "c1", "r1", "--text", "x"},
		{"comment", "reply", "react", "doxAbc", "r1", "--emoji", "THUMBSUP"},
		{"vc", "bot", "meeting-join", "--meeting-number", "123456789"},
		{"vc", "bot", "meeting-leave", "--meeting-id", "6911188411932033028"},
		{"vc", "bot", "meeting-events", "--meeting-id", "6911188411932033028", "--as", "bot"},
		{"vc", "meeting", "list-active"},
		{"vc", "bot", "meeting-countdown", "--meeting-id", "6911188411932033028", "--action", "set", "--duration", "5"},
		{"vc", "bot", "meeting-message-send", "--meeting-id", "6911188411932033028", "--text", "hi"},
		{"vc", "bot", "meeting-screenshot", "--meeting-id", "6911188411932033028", "--output", filepath.Join(t.TempDir(), "s.jpg")},
		{"vc", "bot", "meeting-invite", "--meeting-id", "6911188411932033028", "--type", "all_suggested"},
		{"vc", "bot", "meeting-end", "--meeting-id", "6911188411932033028"},
		{"wiki", "member", "add", "spc1", "--member-type", "openid", "--member-id", "ou_x", "--role", "member"},
		{"wiki", "member", "remove", "spc1", "--member-type", "openid", "--member-id", "ou_x", "--role", "member"},
		{"wiki", "node-copy", "--space-id", "spc1", "--node-token", "wikAbc", "--target-space-id", "spc2"},
		{"wiki", "space-create", "--name", "n"},
	}
	for _, args := range cases {
		dry := append(append([]string{}, args...), "--dry-run")
		out, _, err := runCLI(t, dry...)
		if err != nil {
			t.Errorf("%v --dry-run 无凭据应 exit 0，got %d: %v", args, exitCodeFor(err), err)
			continue
		}
		if !strings.Contains(out, "dry_run") && !strings.Contains(out, `"method"`) && !strings.Contains(out, `"mode": "list"`) {
			t.Errorf("%v --dry-run 输出不像预览:\n%s", args, out)
		}
		if args[1] == "preview" && args[3] == "--list-only" {
			continue // 下面单独断言
		}
		if _, _, err := runCLI(t, args...); err == nil || exitCodeFor(err) != clierr.ExitAuth {
			t.Errorf("%v 去掉 --dry-run 无凭据应 exit 3，got %d: %v", args, exitCodeFor(err), err)
		}
	}
	// drive cover --list-only 是本地列表：无凭据也直接成功
	if out, _, err := runCLI(t, "drive", "cover", "boxAbc", "--list-only"); err != nil || !strings.Contains(out, `"square"`) {
		t.Fatalf("drive cover --list-only 无凭据应成功: %v\n%s", err, out)
	}
}

func TestMeetingsStorage_UsageErrorsExit2(t *testing.T) {
	clearCmdCredentials(t)
	cases := [][]string{
		{"perm", "list", "https://example.feishu.cn/unknown/abc"},
		{"perm", "add", "doxAbc", "--member-type", "wikispaceid", "--member-id", "7", "--perm", "view"},
		{"perm", "public-update", "doxAbc"},
		{"drive", "search", "--mine", "--creator-ids", "ou_x"},
		{"drive", "search", "--created-by-me", "--original-creator-ids", "ou_x"},
		{"drive", "search", "--opened-since", "400d"},
		{"drive", "search", "--created-since", "notatime"},
		{"drive", "search", "--created-since", "2026-10-02", "--created-until", "2026-10-01"},
		{"drive", "preview", "boxAbc"},
		{"drive", "preview", "boxAbc", "--type", "pdf"},
		{"drive", "preview", "boxAbc", "--list-only", "--type", "pdf"},
		{"drive", "cover", "boxAbc", "--spec", "huge", "-o", "x.png"},
		{"vc", "bot", "meeting-countdown", "--meeting-id", "6911188411932033028", "--action", "close_window", "--duration", "3"},
		{"vc", "bot", "meeting-countdown", "--meeting-id", "6911188411932033028", "--action", "set", "--duration", "5", "--reminder-before-end", "5"},
		{"vc", "bot", "meeting-countdown", "--meeting-id", "123456789", "--action", "set", "--duration", "5"},
		{"vc", "bot", "meeting-message-send", "--meeting-id", "6911188411932033028"},
		{"vc", "bot", "meeting-message-send", "--meeting-id", "6911188411932033028", "--text", "a", "--emoji-type", "LOVE", "--msg-type", "text"},
		{"vc", "bot", "meeting-invite", "--meeting-id", "6911188411932033028", "--type", "selected"},
		{"vc", "bot", "meeting-invite", "--meeting-id", "6911188411932033028", "--type", "selected", "--open-ids", "oc_x"},
		{"vc", "bot", "meeting-invite", "--meeting-id", "6911188411932033028", "--type", "all_suggested", "--open-ids", "ou_x"},
		{"vc", "bot", "meeting-invite", "--meeting-id", "6911188411932033028", "--type", "all_suggested", "--user-access-token", "u-x"},
		{"vc", "bot", "meeting-end", "--meeting-id", "6911188411932033028", "--user-access-token", "u-x"},
		{"drive", "move", "--file-token", "doxAbc", "--type", "bogus"},
		{"wiki", "space-create", "--name", ""},
	}
	for _, args := range cases {
		if _, _, err := runCLI(t, args...); err == nil || exitCodeFor(err) != clierr.ExitUsage {
			t.Errorf("%v 应 exit 2（无凭据也先报参数错误），got %d: %v", args, exitCodeFor(err), err)
		}
	}
}

// ---------- vc bot 会中控制 ----------

func TestVCBotScreenshot_SensitiveOutputRejectedBeforeNetwork(t *testing.T) {
	home, _ := os.UserHomeDir()
	_, _, err := runCLI(t, "vc", "bot", "meeting-screenshot", "--meeting-id", "6911188411932033028",
		"--output", filepath.Join(home, ".ssh", "x.png"), "--bot-app-id", "cli_x", "--bot-app-secret", "y")
	if err == nil || exitCodeFor(err) != clierr.ExitUsage || !strings.Contains(err.Error(), ".ssh") {
		t.Fatalf("输出到 ~/.ssh 应在联网前 exit 2，got %d: %v", exitCodeFor(err), err)
	}
	existing := filepath.Join(t.TempDir(), "s.jpg")
	if err := os.WriteFile(existing, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, "vc", "bot", "meeting-screenshot", "--meeting-id", "6911188411932033028", "--output", existing, "--dry-run"); err == nil || exitCodeFor(err) != clierr.ExitUsage {
		t.Fatalf("目标已存在且未 --overwrite 应 exit 2，got %v", err)
	}
	if _, _, err := runCLI(t, "vc", "bot", "meeting-screenshot", "--meeting-id", "6911188411932033028", "--output", existing, "--overwrite", "--dry-run"); err != nil {
		t.Fatalf("--overwrite 应放行: %v", err)
	}
}

func TestVCBotEnd_RequiresConfirmation(t *testing.T) {
	_, _, err := runCLI(t, "vc", "bot", "meeting-end", "--meeting-id", "6911188411932033028", "--bot-app-id", "cli_x", "--bot-app-secret", "y")
	if err == nil || exitCodeFor(err) != clierr.ExitConfirmationRequired {
		t.Fatalf("非交互未 --yes 应 exit 10，got %d: %v", exitCodeFor(err), err)
	}
}

func TestVCBotBodies(t *testing.T) {
	body, err := buildVCCountdownBody(vcCountdownInput{MeetingID: "1", Action: "SET", Duration: 10, DurationSet: true, Reminder: 2, ReminderSet: true, PlayAudio: false, PlayAudioSet: true})
	if err != nil || body["action"] != "set" || body["duration"] != 10 || body["reminder_before_end"] != 2 || body["need_play_audio_at_end"] != false {
		t.Fatalf("countdown set body = %v, %v", body, err)
	}
	body, err = buildVCCountdownBody(vcCountdownInput{MeetingID: "1", Action: "end_in_advance"})
	if err != nil || len(body) != 2 {
		t.Fatalf("end_in_advance 只应有 meeting_id/action: %v %v", body, err)
	}
	if _, err := buildVCCountdownBody(vcCountdownInput{MeetingID: "1", Action: "prolong", PlayAudioSet: true, Duration: 1, DurationSet: true}); err == nil {
		t.Fatal("prolong + need-play-audio-at-end 应报错")
	}
	body, err = buildVCMessageBody("1", "", "  hi ", "", "u1")
	if err != nil || body["msg_type"] != "text" || body["content"] != "hi" || body["uuid"] != "u1" {
		t.Fatalf("message body = %v %v", body, err)
	}
	if _, err := buildVCMessageBody("1", "", strings.Repeat("a", vcMessageMaxTextBytes+1), "", ""); err == nil {
		t.Fatal("超长文本应报错")
	}
	if _, err := buildVCMessageBody("1", "", "a", "", strings.Repeat("u", 129)); err == nil {
		t.Fatal("超长 uuid 应报错")
	}
	body, err = buildVCInviteBody("1", "SELECTED", []string{"ou_a", " ou_a", "ou_b", ""})
	if err != nil || body["invite_type"] != 2 || len(body["invitees"].([]map[string]any)) != 2 {
		t.Fatalf("invite selected 应去重: %v %v", body, err)
	}
	many := make([]string, 0, 201)
	for i := 0; i < 201; i++ {
		many = append(many, fmt.Sprintf("ou_%d", i))
	}
	if _, err := buildVCInviteBody("1", "selected", many); err == nil {
		t.Fatal("超过 200 人应报错")
	}
	vcScreenshotNow = func() time.Time { return time.Date(2026, 11, 5, 1, 2, 3, 0, time.UTC) }
	t.Cleanup(func() { vcScreenshotNow = time.Now })
	if got := vcScreenshotOutputPath("", "42"); got != filepath.Join("meeting-screenshots", "42-20261105T010203.000Z.jpg") {
		t.Fatalf("默认截图路径 = %s", got)
	}
}

// ---------- vc note transcript --overwrite ----------

func TestTranscriptOverwriteWarning(t *testing.T) {
	newCmd := func() (*cobra.Command, *bytes.Buffer) {
		c := &cobra.Command{Use: "x"}
		c.Flags().Bool("overwrite", false, "")
		buf := &bytes.Buffer{}
		c.SetErr(buf)
		return c, buf
	}
	path := filepath.Join(t.TempDir(), "t.md")
	c, buf := newCmd()
	warnTranscriptOverwrite(c, path)
	if buf.Len() != 0 {
		t.Fatalf("目标不存在不应告警: %s", buf.String())
	}
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, buf = newCmd()
	warnTranscriptOverwrite(c, path)
	if !strings.Contains(buf.String(), "已存在") || !strings.Contains(buf.String(), "--overwrite") {
		t.Fatalf("目标已存在未传 --overwrite 应告警: %q", buf.String())
	}
	c, buf = newCmd()
	_ = c.Flags().Set("overwrite", "true")
	warnTranscriptOverwrite(c, path)
	if buf.Len() != 0 {
		t.Fatalf("--overwrite 不应告警: %s", buf.String())
	}
	if f := vcNoteTranscriptCmd.Flags().Lookup("overwrite"); f == nil || f.DefValue != "false" {
		t.Fatal("transcript 应注册 --overwrite 且默认 false")
	}
}

// ---------- drive search 时间窗 ----------

func TestDriveSearchTimeRanges(t *testing.T) {
	now := time.Date(2026, 11, 5, 10, 30, 0, 0, time.Local)
	if v, err := parseDriveSearchTime("7d", now); err != nil || v != now.Add(-7*24*time.Hour).Unix() {
		t.Fatalf("7d = %d %v", v, err)
	}
	if v, err := parseDriveSearchTime("1793677476000", now); err != nil || v != 1793677476 {
		t.Fatalf("毫秒应归一化为秒: %d %v", v, err)
	}
	if _, err := parseDriveSearchTime("20261105", now); err == nil {
		t.Fatal("8 位数字不应当作 unix 秒")
	}

	ranges, notices, err := buildDriveSearchTimeRanges(map[string]driveSearchTimeInput{
		"edited":  {Since: "2026-11-01 09:15:00"},
		"created": {Since: "2026-10-01", Until: "2026-10-31"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := ranges["my_edit_time"]["start"]; got%3600 != 0 || got > time.Date(2026, 11, 1, 9, 15, 0, 0, time.Local).Unix() {
		t.Fatalf("edited 起点应向下取整到整点: %d", got)
	}
	if len(notices) != 1 || !strings.Contains(notices[0], "按小时聚合") {
		t.Fatalf("应有一条取整提示: %v", notices)
	}
	if ranges["create_time"]["start"] != time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local).Unix() {
		t.Fatalf("created 起点不对: %v", ranges["create_time"])
	}

	ranges, notices, err = buildDriveSearchTimeRanges(map[string]driveSearchTimeInput{"opened": {Since: "200d"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	rng := ranges["open_time"]
	if rng["end"] != now.Unix() || rng["end"]-rng["start"] != 90*86400 {
		t.Fatalf("200d 应收窄为最近 90 天: %v", rng)
	}
	if len(notices) != 1 || !strings.Contains(notices[0], "共 3 个切片") {
		t.Fatalf("应提示 3 个切片: %v", notices)
	}
	if _, _, err := buildDriveSearchTimeRanges(map[string]driveSearchTimeInput{"opened": {Since: "400d"}}, now); err == nil || exitCodeFor(err) != clierr.ExitUsage {
		t.Fatalf("400d 应为用法错误: %v", err)
	}
	if r, n, err := buildDriveSearchTimeRanges(map[string]driveSearchTimeInput{"opened": {Since: "30d"}}, now); err != nil || len(n) != 0 || r["open_time"]["end"] != 0 {
		t.Fatalf("≤90 天不应切片也不应补 end: %v %v %v", r, n, err)
	}
}

func TestDriveSearchBodyFilters(t *testing.T) {
	body := client.BuildDocWikiSearchBody(client.DriveSearchOptions{
		Query:              "q",
		OriginalCreatorIDs: []string{"ou_me"},
		TimeRanges:         map[string]map[string]int64{"create_time": {"start": 100}},
	})
	for _, k := range []string{"doc_filter", "wiki_filter"} {
		f := body[k].(map[string]interface{})
		if fmt.Sprint(f["original_creator_ids"]) != "[ou_me]" || fmt.Sprint(f["create_time"]) != "map[start:100]" {
			t.Fatalf("%s 缺新 filter: %v", k, f)
		}
	}
	plain := client.BuildDocWikiSearchBody(client.DriveSearchOptions{Query: "q"})
	if f := plain["doc_filter"].(map[string]interface{}); len(f) != 0 {
		t.Fatalf("不传新 flag 时 filter 应与旧版一致（为空）: %v", f)
	}
}

// ---------- drive push/pull --dry-run 差集计划 ----------

func TestDrivePushPullPlan(t *testing.T) {
	root := t.TempDir()
	write := func(rel string, mod time.Time) string {
		p := filepath.Join(root, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(rel), 0o644); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(p, mod, mod)
		return p
	}
	old := time.Unix(1700000000, 0)
	newer := time.Unix(1800000000, 0)
	localFiles := map[string]string{"a.txt": write("a.txt", old), "sub/b.txt": write("sub/b.txt", newer), "c.txt": write("c.txt", old)}
	snaps := map[string]os.FileInfo{}
	for rel, abs := range localFiles {
		info, _ := os.Stat(abs)
		snaps[rel] = info
	}
	remote := map[string]driveMirrorFile{
		"a.txt":     {FileToken: "ta", ModifiedTime: "1750000000"}, // 远端更新 → smart 跳过
		"sub/b.txt": {FileToken: "tb", ModifiedTime: "1750000000"}, // 本地更新 → smart 覆盖
		"gone.txt":  {FileToken: "tg", ModifiedTime: "1750000000"}, // 本地无 → delete_remote
	}
	plan := planDrivePush([]string{"sub"}, localFiles, snaps, remote, map[string]string{"": "fld"}, driveMirrorIfExistsSmart, true)
	got := map[string]string{}
	for _, it := range plan.Items {
		got[it.RelPath] = it.Action
	}
	want := map[string]string{"sub": drivePlanCreateFolder, "a.txt": drivePlanSkip, "sub/b.txt": drivePlanOverwrite, "c.txt": drivePlanUpload, "gone.txt": drivePlanDeleteRemote}
	if fmt.Sprint(got) != fmt.Sprint(want) || plan.Note == "" {
		t.Fatalf("push 计划 = %v（note=%q），期望 %v", got, plan.Note, want)
	}

	pullRemote := map[string]driveMirrorFile{
		"a.txt":   {FileToken: "ta", ModifiedTime: "1750000000"}, // 本地更旧 → overwrite
		"new.txt": {FileToken: "tn", ModifiedTime: "1750000000"}, // 本地无 → download
	}
	paths := map[string]struct{}{"a.txt": {}, "new.txt": {}, "online-doc": {}}
	pplan, err := planDrivePull(root, pullRemote, paths, driveMirrorIfExistsSmart, true)
	if err != nil {
		t.Fatal(err)
	}
	got = map[string]string{}
	for _, it := range pplan.Items {
		got[it.RelPath] = it.Action
	}
	want = map[string]string{"a.txt": drivePlanOverwrite, "new.txt": drivePlanDownload, "sub/b.txt": drivePlanDeleteLocal, "c.txt": drivePlanDeleteLocal}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("pull 计划 = %v，期望 %v", got, want)
	}
	// 计划阶段不得写盘 / 删文件
	for rel, abs := range localFiles {
		if _, err := os.Stat(abs); err != nil {
			t.Fatalf("dry-run 计划不应删除本地文件 %s", rel)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); !os.IsNotExist(err) {
		t.Fatal("dry-run 计划不应下载文件")
	}
}

func TestDrivePushDryRun_NoYesRequired(t *testing.T) {
	clearCmdCredentials(t)
	dir, err := os.MkdirTemp(".", "push-dry-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	// 带 --delete-remote 的 --dry-run 不要求 --yes：无凭据时走到凭据检查（exit 3），而不是确认门禁（exit 10）
	_, _, err = runCLI(t, "drive", "push", "--folder-token", "fldx", "--local-dir", dir, "--delete-remote", "--dry-run")
	if err == nil || exitCodeFor(err) != clierr.ExitAuth {
		t.Fatalf("push --delete-remote --dry-run 无凭据应 exit 3，got %d: %v", exitCodeFor(err), err)
	}
	_, _, err = runCLI(t, "drive", "push", "--folder-token", "fldx", "--local-dir", dir, "--delete-remote")
	if err == nil || exitCodeFor(err) != clierr.ExitConfirmationRequired {
		t.Fatalf("push --delete-remote 未 --yes 仍应 exit 10，got %d: %v", exitCodeFor(err), err)
	}
	_, _, err = runCLI(t, "drive", "pull", "--folder-token", "fldx", "--local-dir", dir, "--delete-local", "--dry-run")
	if err == nil || exitCodeFor(err) != clierr.ExitAuth {
		t.Fatalf("pull --delete-local --dry-run 无凭据应 exit 3，got %d: %v", exitCodeFor(err), err)
	}
}

// ---------- drive preview / cover ----------

type previewRecorder struct {
	mu   sync.Mutex
	reqs []string
}

func (r *previewRecorder) add(s string) { r.mu.Lock(); r.reqs = append(r.reqs, s); r.mu.Unlock() }

func newDrivePreviewServer(t *testing.T) *previewRecorder {
	t.Helper()
	rec := &previewRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		rec.add(r.Method + " " + r.URL.Path + "?" + r.URL.RawQuery)
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/preview_result"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"version":"77","preview_results":[{"preview_type":0,"preview_status":0},{"preview_type":7,"preview_status":1},{"preview_type":1,"preview_status":0}]}}`)
		case strings.HasSuffix(r.URL.Path, "/preview_download") && r.URL.Query().Get("bus_type") == "nope":
			w.WriteHeader(http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "/preview_download") && r.URL.Query().Get("preview_type") == "0":
			w.Header().Set("Content-Length", "0")
			w.WriteHeader(http.StatusOK) // 实测：PDF 候选 READY 但返回空 body
		case strings.HasSuffix(r.URL.Path, "/preview_download"):
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(fakePNG)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	initDocUpdateTestConfig(t, srv.URL)
	return rec
}

func TestDrivePreview_CandidatesAndEmptyBody(t *testing.T) {
	rec := newDrivePreviewServer(t)
	dir := t.TempDir()

	out, err := runMediaCmd(t, drivePreviewCmd, map[string]string{"list-only": "true"}, "boxAbc")
	if err != nil || !strings.Contains(out, `"type": "pdf"`) || !strings.Contains(out, `"status": "PROCESSING"`) {
		t.Fatalf("--list-only: %v\n%s", err, out)
	}

	pdfOut := filepath.Join(dir, "p")
	_, err = runMediaCmd(t, drivePreviewCmd, map[string]string{"type": "pdf", "output": pdfOut}, "boxAbc")
	if err == nil || !strings.Contains(err.Error(), "为空") {
		t.Fatalf("空 body 应报错: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("空 body 不应写文件: %v", entries)
	}
	last := rec.reqs[len(rec.reqs)-1]
	if !strings.Contains(last, "preview_type=0") || !strings.Contains(last, "version=77") {
		t.Fatalf("应带候选 type_code 与 preview_result 的 version: %s", last)
	}

	// image 别名跳过 PROCESSING 的 jpg，选 READY 的 png
	imgOut := filepath.Join(dir, "img")
	if _, err := runMediaCmd(t, drivePreviewCmd, map[string]string{"type": "image", "output": imgOut}, "boxAbc"); err != nil {
		t.Fatalf("image 下载失败: %v", err)
	}
	if data, err := os.ReadFile(imgOut + ".png"); err != nil || !bytes.Equal(data, fakePNG) {
		t.Fatalf("应保存 png: %v", err)
	}
	if !strings.Contains(rec.reqs[len(rec.reqs)-1], "preview_type=1") {
		t.Fatalf("image 应选 READY 的 png（type=1）: %v", rec.reqs)
	}

	if _, err := runMediaCmd(t, drivePreviewCmd, map[string]string{"type": "html", "output": filepath.Join(dir, "h")}, "boxAbc"); err == nil || !strings.Contains(err.Error(), "没有") {
		t.Fatalf("不存在的类型应报错并列出可用类型: %v", err)
	}
}

func TestDriveCover_SpecParamsAnd404(t *testing.T) {
	rec := newDrivePreviewServer(t)
	dir := t.TempDir()
	if _, err := runMediaCmd(t, driveCoverCmd, map[string]string{"spec": "square", "output": filepath.Join(dir, "sq")}, "boxAbc"); err != nil {
		t.Fatalf("cover square: %v", err)
	}
	last := rec.reqs[len(rec.reqs)-1]
	for _, want := range []string{"preview_type=1", "width=360", "height=360", "policy=near"} {
		if !strings.Contains(last, want) {
			t.Fatalf("square 规格 query 缺 %s: %s", want, last)
		}
	}
	if p := driveCoverParams(driveCoverSpec{Name: "x", BusType: "nope"}, ""); p.Get("bus_type") != "nope" {
		t.Fatal("bus_type 应下发")
	}
	orig := driveCoverSpecs
	driveCoverSpecs = append([]driveCoverSpec{{Name: "missing", BusType: "nope"}}, orig...)
	t.Cleanup(func() { driveCoverSpecs = orig })
	_, err := runMediaCmd(t, driveCoverCmd, map[string]string{"spec": "missing", "output": filepath.Join(dir, "m.png")}, "boxAbc")
	if err == nil || !strings.Contains(err.Error(), "没有 missing 规格的封面") {
		t.Fatalf("404 应给中文提示: %v", err)
	}
}

func TestVCBotScreenshot_ClientParsesJPEGAndJSONError(t *testing.T) {
	jpeg := []byte{0xff, 0xd8, 0x01, 0x02, 0xff, 0xd9}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/open-apis/vc/v1/bots/screenshot" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(r.Body)
		if strings.Contains(buf.String(), `"meeting_id":"2"`) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"code":121001,"msg":"bot not in meeting"}`)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(jpeg)
	}))
	t.Cleanup(srv.Close)
	initDocUpdateTestConfig(t, srv.URL)
	img, ct, err := client.VCBotScreenshot("1", "")
	if err != nil || ct != "image/jpeg" || !bytes.Equal(img, jpeg) {
		t.Fatalf("JPEG 响应: %v %q %v", img, ct, err)
	}
	if _, _, err := client.VCBotScreenshot("2", ""); err == nil || !client.HasAPICode(err, 121001) {
		t.Fatalf("HTTP 400 业务信封应解析出业务码: %v", err)
	}
}
