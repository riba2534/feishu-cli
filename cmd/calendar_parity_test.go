package cmd

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/clierr"
)

// calendarChdirTemp 切到临时目录（测试结束还原），返回该目录
func calendarChdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	return dir
}

// TestPrepareRichDescriptionPaths 本地图片只接受当前目录内的相对路径；远端 URL 原样保留
func TestPrepareRichDescriptionPaths(t *testing.T) {
	dir := calendarChdirTemp(t)
	if err := os.MkdirAll(filepath.Join(dir, "img"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "img", "a.png"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "b.png")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	rd, err := prepareRichDescription("**议程** ![a]( ./img/a.png ) ![r](https://example.com/r.png) ![a2](img/a.png)")
	if err != nil {
		t.Fatalf("合法路径: %v", err)
	}
	if len(rd.Images) != 2 {
		t.Fatalf("应识别 2 张本地图片（远端 URL 不算）: %+v", rd.Images)
	}
	got := rd.rewrite([]string{"U1", "U2"})
	if got != "**议程** ![a]( U1 ) ![r](https://example.com/r.png) ![a2](U2)" {
		t.Fatalf("改写结果不对: %q", got)
	}
	if p := rd.preview(); !strings.Contains(p, "<上传后的图片 URL: ./img/a.png>") {
		t.Fatalf("预览应带占位符: %q", p)
	}

	for _, bad := range []string{
		"![x](" + outside + ")",        // 绝对路径
		"![x](../b.png)",               // 越出当前目录
		"![x](./missing.png)",          // 不存在
		"![x](file://" + outside + ")", // file:// 绝对路径
	} {
		if _, err := prepareRichDescription(bad); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Fatalf("%q 期望用法错误，得到 %v", bad, err)
		}
	}
}

// TestCreateEventDescriptionFormatMarkdownDryRun markdown 写 description_rich 并列出图片上传步骤；默认 text 不变
func TestCreateEventDescriptionFormatMarkdownDryRun(t *testing.T) {
	dir := calendarChdirTemp(t)
	if err := os.WriteFile(filepath.Join(dir, "a.png"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	rec := setupNoCredsWorkTest(t)
	base := map[string]string{"summary": "s", "start": "2026-12-01T10:00:00+08:00", "end": "2026-12-01T11:00:00+08:00", "description": "**b** ![p](./a.png)", "dry-run": "true"}

	out, err := runWorkCmd(t, createEventCmd, nil, base)
	if err != nil {
		t.Fatalf("text dry-run: %v", err)
	}
	steps := dryRunSteps(t, out)
	if steps[0].Body["description"] != "**b** ![p](./a.png)" || steps[0].Body["description_rich"] != nil {
		t.Fatalf("默认 text 应原样写 description: %+v", steps[0].Body)
	}

	flags := map[string]string{"description-format": "markdown"}
	for k, v := range base {
		flags[k] = v
	}
	out, err = runWorkCmd(t, createEventCmd, nil, flags)
	if err != nil {
		t.Fatalf("markdown dry-run: %v", err)
	}
	steps = dryRunSteps(t, out)
	if len(steps) != 2 || steps[0].URL != "/open-apis/drive/v1/medias/upload_all" || steps[0].Body["parent_type"] != "calendar" {
		t.Fatalf("应先列出图片上传: %+v", steps)
	}
	body := steps[1].Body
	if _, ok := body["description"]; ok {
		t.Fatalf("markdown 不应再写 description: %+v", body)
	}
	if rich, _ := body["description_rich"].(string); !strings.Contains(rich, "**b** ![p](<上传后的图片 URL: ./a.png>)") {
		t.Fatalf("description_rich 不对: %+v", body)
	}
	if len(rec.all()) != 0 {
		t.Fatalf("dry-run 不应联网: %+v", rec.all())
	}
}

// TestCalendarDescriptionFormatValidation 非法取值、markdown 缺描述、图片越界均为 exit 2 且不联网
func TestCalendarDescriptionFormatValidation(t *testing.T) {
	calendarChdirTemp(t)
	rec := setupWorkCmdTest(t, "u-test", nil)
	base := map[string]string{"summary": "s", "start": "2026-12-01T10:00:00+08:00", "end": "2026-12-01T11:00:00+08:00"}
	cases := []map[string]string{
		{"description": "x", "description-format": "html"},
		{"description-format": "markdown"},
		{"description": "![x](/etc/passwd)", "description-format": "markdown"},
	}
	for _, extra := range cases {
		flags := map[string]string{}
		for k, v := range base {
			flags[k] = v
		}
		for k, v := range extra {
			flags[k] = v
		}
		resetCommandTreeFlags(createEventCmd)
		_, err := runWorkCmd(t, createEventCmd, nil, flags)
		if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Fatalf("%v 期望用法错误，得到 %v", extra, err)
		}
	}
	_, err := runWorkCmd(t, updateEventCmd, []string{"evt"}, map[string]string{"description": "![x](../a.png)", "description-format": "markdown"})
	if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("update-event 越界图片期望用法错误，得到 %v", err)
	}
	if len(rec.all()) != 0 {
		t.Fatalf("参数错误不应联网: %+v", rec.all())
	}
}

// TestCreateEventMarkdownUploadsThenCreates 真实路径：先上传图片，再以 description_rich 创建日程
func TestCreateEventMarkdownUploadsThenCreates(t *testing.T) {
	dir := calendarChdirTemp(t)
	if err := os.WriteFile(filepath.Join(dir, "a.png"), []byte("not-a-real-png"), 0o600); err != nil {
		t.Fatal(err)
	}
	rec := setupWorkCmdTest(t, "", func(w http.ResponseWriter, r *http.Request, body string) {
		if strings.Contains(r.URL.Path, "/medias/upload_all") {
			_, _ = w.Write([]byte(`{"code":0,"data":{"file_token":"boxcnxxx"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"event":{"event_id":"evt_1","summary":"s","start_time":{"timestamp":"1795000000"},"end_time":{"timestamp":"1795003600"}}}}`))
	})
	out, err := runWorkCmd(t, createEventCmd, nil, map[string]string{
		"summary": "s", "start": "2026-12-01T10:00:00+08:00", "end": "2026-12-01T11:00:00+08:00",
		"description": "## 议程\n![p](./a.png)", "description-format": "markdown", "as": "bot", "output": "json",
	})
	if err != nil {
		t.Fatalf("create-event markdown: %v", err)
	}
	reqs := rec.apiReqs()
	if len(reqs) != 2 || !strings.Contains(reqs[0].Path, "/medias/upload_all") || !strings.Contains(reqs[1].Path, "/calendars/primary/events") {
		t.Fatalf("请求顺序不对: %+v", reqs)
	}
	if !strings.Contains(reqs[0].Body, "calendar") {
		t.Fatalf("上传应带 parent_type=calendar: %s", reqs[0].Body)
	}
	evBody := reqs[1].Body
	if strings.Contains(evBody, `"description":`) || !strings.Contains(evBody, `"description_rich":"## 议程\n![p](https://internal-api-drive-stream.feishu.cn/space/api/box/stream/download/preview/boxcnxxx?preview_type=16`) {
		t.Fatalf("创建请求应带改写后的 description_rich: %s", evBody)
	}
	if !strings.Contains(out, `"event_id": "evt_1"`) {
		t.Fatalf("输出不对: %s", out)
	}
}

// TestCreateEventMeetingOwner --meeting-owner-id：ou_ 前缀 + 仅 bot，写入 vchat.meeting_settings.owner_id
func TestCreateEventMeetingOwner(t *testing.T) {
	rec := setupWorkCmdTest(t, "u-test", nil)
	base := map[string]string{"summary": "s", "start": "2026-12-01T10:00:00+08:00", "end": "2026-12-01T11:00:00+08:00", "dry-run": "true"}
	for _, extra := range []map[string]string{
		{"meeting-owner-id": "ou_a"},               // 默认 auto 身份
		{"meeting-owner-id": "ou_a", "as": "user"}, // user 身份
		{"meeting-owner-id": "u_a", "as": "bot"},   // 前缀不对
		{"meeting-owner-id": "ou_", "as": "bot"},   // 空 open_id
	} {
		flags := map[string]string{}
		for k, v := range base {
			flags[k] = v
		}
		for k, v := range extra {
			flags[k] = v
		}
		resetCommandTreeFlags(createEventCmd)
		_, err := runWorkCmd(t, createEventCmd, nil, flags)
		if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Fatalf("%v 期望用法错误，得到 %v", extra, err)
		}
	}
	flags := map[string]string{"meeting-owner-id": "ou_a", "as": "bot"}
	for k, v := range base {
		flags[k] = v
	}
	resetCommandTreeFlags(createEventCmd)
	out, err := runWorkCmd(t, createEventCmd, nil, flags)
	if err != nil {
		t.Fatalf("bot dry-run: %v", err)
	}
	vchat, _ := dryRunSteps(t, out)[0].Body["vchat"].(map[string]any)
	ms, _ := vchat["meeting_settings"].(map[string]any)
	if vchat["vc_type"] != "vc" || ms["owner_id"] != "ou_a" {
		t.Fatalf("vchat 不对: %+v", vchat)
	}
	if len(rec.all()) != 0 {
		t.Fatalf("不应联网: %+v", rec.all())
	}
}

// TestUpdateEventMarkdownDryRun update-event --description-format markdown 写 description_rich
func TestUpdateEventMarkdownDryRun(t *testing.T) {
	calendarChdirTemp(t)
	setupNoCredsWorkTest(t)
	out, err := runWorkCmd(t, updateEventCmd, []string{"evt"}, map[string]string{"description": "- a\n- b", "description-format": "markdown", "dry-run": "true"})
	if err != nil {
		t.Fatalf("update-event dry-run: %v", err)
	}
	steps := dryRunSteps(t, out)
	last := steps[len(steps)-1]
	if last.Method != "PATCH" || last.Body["description_rich"] != "- a\n- b" || last.Body["description"] != nil {
		t.Fatalf("PATCH 请求体不对: %+v", last)
	}
}
