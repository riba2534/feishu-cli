package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newMailSendTimeTestCmd(confirm bool, sendTime string) *cobra.Command {
	c := &cobra.Command{}
	c.Flags().Bool("confirm-send", false, "")
	addMailSendTimeFlag(c)
	if confirm {
		_ = c.Flags().Set("confirm-send", "true")
	}
	if sendTime != "" {
		_ = c.Flags().Set("send-time", sendTime)
	}
	return c
}

func TestValidateMailSendTime(t *testing.T) {
	fixed := time.Unix(1_800_000_000, 0)
	old := mailNow
	mailNow = func() time.Time { return fixed }
	defer func() { mailNow = old }()

	if got, err := validateMailSendTime(newMailSendTimeTestCmd(false, "")); err != nil || got != "" {
		t.Fatalf("未传 --send-time 应返回空，got %q %v", got, err)
	}
	ok := fmt.Sprint(fixed.Add(5 * time.Minute).Unix())
	if got, err := validateMailSendTime(newMailSendTimeTestCmd(true, ok)); err != nil || got != ok {
		t.Fatalf("恰好 5 分钟后应通过，got %q %v", got, err)
	}
	cases := map[string]*cobra.Command{
		"缺 --confirm-send": newMailSendTimeTestCmd(false, ok),
		"非数字":              newMailSendTimeTestCmd(true, "2026-11-05T10:00:00Z"),
		"不足 5 分钟":          newMailSendTimeTestCmd(true, fmt.Sprint(fixed.Add(4*time.Minute).Unix())),
	}
	for name, c := range cases {
		if _, err := validateMailSendTime(c); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("%s: 应为用法错误，got %v", name, err)
		}
	}
}

// 不带 --request-receipt 时 EML 与旧版一致（无回执 header）；带上时写入 Disposition-Notification-To。
func TestBuildEMLReceiptHeader(t *testing.T) {
	base := mailMessageInput{From: "me@example.com", To: []string{"user@example.com"}, Subject: "hi", BodyText: "body"}
	raw, err := buildEMLBytes(base)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Disposition-Notification-To") {
		t.Fatal("未开启回执时不应写入 Disposition-Notification-To")
	}
	withReceipt := base
	withReceipt.DispositionNotificationTo = "me@example.com"
	raw2, err := buildEMLBytes(withReceipt)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw2), "Disposition-Notification-To: <me@example.com>\r\n") &&
		!strings.Contains(string(raw2), "Disposition-Notification-To: me@example.com\r\n") {
		t.Fatalf("应写入回执 header:\n%s", raw2)
	}
	// 除回执 header 外其余内容一致（Date 行除外）
	strip := func(s string) string {
		var out []string
		for _, l := range strings.Split(s, "\r\n") {
			if strings.HasPrefix(l, "Date:") || strings.HasPrefix(l, "Disposition-Notification-To:") {
				continue
			}
			out = append(out, l)
		}
		return strings.Join(out, "\r\n")
	}
	if strip(string(raw)) != strip(string(raw2)) {
		t.Fatal("回执 header 不应影响其他内容")
	}
	bad := base
	bad.DispositionNotificationTo = "a@example.com\r\nBcc: x@example.com"
	if _, err := buildEMLBytes(bad); err == nil {
		t.Fatal("回执地址含 CR/LF 应拒绝")
	}
}

func TestRequireMailReceiptSender(t *testing.T) {
	c := &cobra.Command{}
	addMailRequestReceiptFlag(c)
	if err := requireMailReceiptSender(c, ""); err != nil {
		t.Fatalf("未开启回执时不校验: %v", err)
	}
	_ = c.Flags().Set("request-receipt", "true")
	if err := requireMailReceiptSender(c, " "); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
		t.Fatalf("无发件人地址应为用法错误，got %v", err)
	}
	if got := mailReceiptAddress(c, "me@example.com"); got != "me@example.com" {
		t.Fatalf("回执地址 = %q", got)
	}
}

func newMailDraftSendTestCmd(sendTime string) *cobra.Command {
	c := &cobra.Command{}
	c.Flags().String("mailbox", "me", "")
	c.Flags().String("draft-id", "", "")
	c.Flags().Bool("confirm-send", false, "")
	addMailSendTimeFlag(c)
	c.Flags().StringP("output", "o", "", "")
	c.Flags().String("user-access-token", "", "")
	_ = c.Flags().Set("draft-id", "d1")
	_ = c.Flags().Set("confirm-send", "true")
	_ = c.Flags().Set("output", "json")
	_ = c.Flags().Set("user-access-token", "u-test")
	if sendTime != "" {
		_ = c.Flags().Set("send-time", sendTime)
	}
	return c
}

// draft-send：不带 --send-time 时请求体为空、输出不含 send_time（与旧版一致）；带上时 body={"send_time":...}。
func TestMailDraftSendScheduledBody(t *testing.T) {
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(raw))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"message_id":"m1","thread_id":"t1"}}`)
	}))
	defer srv.Close()
	setupMailAttendanceCmdTestConfig(t, srv.URL)

	var err error
	out := captureMailStdout(t, func() { err = mailDraftSendCmd.RunE(newMailDraftSendTestCmd(""), nil) })
	if err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 1 || strings.TrimSpace(bodies[0]) != "" && strings.TrimSpace(bodies[0]) != "null" && strings.TrimSpace(bodies[0]) != "{}" {
		t.Fatalf("立即发送不应带 send_time body，got %q", bodies)
	}
	if strings.Contains(bodies[0], "send_time") {
		t.Fatalf("立即发送不应带 send_time: %q", bodies[0])
	}
	if strings.Contains(out, "send_time") {
		t.Fatalf("默认输出不应新增 send_time 字段: %s", out)
	}

	ts := fmt.Sprint(time.Now().Add(time.Hour).Unix())
	out = captureMailStdout(t, func() { err = mailDraftSendCmd.RunE(newMailDraftSendTestCmd(ts), nil) })
	if err != nil {
		t.Fatal(err)
	}
	var sent map[string]any
	if len(bodies) != 2 || json.Unmarshal([]byte(bodies[1]), &sent) != nil || sent["send_time"] != ts {
		t.Fatalf("定时发送 body 应为 send_time=%s，got %q", ts, bodies)
	}
	if !strings.Contains(out, `"send_time": "`+ts+`"`) {
		t.Fatalf("定时发送输出应含 send_time: %s", out)
	}
}

func newMailShareTestCmd(kv map[string]string) *cobra.Command {
	c := &cobra.Command{}
	for _, f := range []string{"mailbox", "message-id", "thread-id", "receive-id", "user-access-token"} {
		c.Flags().String(f, "", "")
	}
	c.Flags().String("receive-id-type", "chat_id", "")
	c.Flags().Bool("dry-run", false, "")
	c.Flags().StringP("output", "o", "", "")
	for k, v := range kv {
		_ = c.Flags().Set(k, v)
	}
	return c
}

func TestMailShareToChatValidationAndDryRun(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	bad := []map[string]string{
		{"receive-id": "oc_x"},
		{"message-id": "m", "thread-id": "t", "receive-id": "oc_x"},
		{"message-id": "m"},
		{"message-id": "m", "receive-id": "oc_x", "receive-id-type": "thread_id"},
	}
	for i, kv := range bad {
		if err := mailShareToChatCmd.RunE(newMailShareTestCmd(kv), nil); err == nil || !clierr.HasKind(err, clierr.KindUsage) {
			t.Errorf("case %d 应为用法错误，got %v", i, err)
		}
	}
	var err error
	out := captureMailStdout(t, func() {
		err = mailShareToChatCmd.RunE(newMailShareTestCmd(map[string]string{"thread-id": "t1", "receive-id": "oc_x", "dry-run": "true"}), nil)
	})
	if err != nil {
		t.Fatalf("无凭据 dry-run 应成功: %v", err)
	}
	for _, want := range []string{"/messages/share_token", `"thread_id": "t1"`, "/share_tokens/<card_id>/send", `"receive_id_type": "chat_id"`} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run 输出缺少 %q:\n%s", want, out)
		}
	}
}

// share-to-chat 实调：两步串联，第二步失败时错误带 card_id。
func TestMailShareToChatTwoSteps(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/share_token") {
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"card_id":"card_1"}}`)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":1234,"msg":"send failed"}`)
	}))
	defer srv.Close()
	setupMailAttendanceCmdTestConfig(t, srv.URL)
	err := mailShareToChatCmd.RunE(newMailShareTestCmd(map[string]string{"message-id": "m1", "receive-id": "oc_x", "user-access-token": "u-test"}), nil)
	if err == nil || !strings.Contains(err.Error(), "card_id=card_1") {
		t.Fatalf("投递失败应带 card_id，got %v", err)
	}
	if len(paths) != 2 || !strings.Contains(paths[1], "/share_tokens/card_1/send") || !strings.Contains(paths[1], "receive_id_type=chat_id") {
		t.Fatalf("请求路径不符: %v", paths)
	}
}

// 无凭据下静态预览类 --dry-run exit 0；去掉 --dry-run 仍 exit 3（A2：config.Validate 移到 dry-run 之后）。
func TestImMailDryRunWithoutCredentials(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	viper.Reset() // 清掉其他用例 stub 服务器留下的 app_id/base_url，模拟真正无配置的环境
	t.Cleanup(viper.Reset)
	cases := [][]string{
		{"mail", "rule-create", "--name", "e2e", "--condition", "from:contains:a@example.com", "--action", "star"},
		{"mail", "rule-update", "--rule-id", "1", "--name", "x"},
		{"mail", "rule-enable", "--rule-id", "1"},
		{"mail", "rule-delete", "--rule-id", "1"},
		{"mail", "rule-reorder", "--rule-ids", "1,2"},
		{"mail", "signature"},
		{"mail", "template", "delete", "--template-id", "1"},
		{"mail", "thread-modify", "--thread-ids", "t1", "--add-label-ids", "FLAGGED"},
		{"mail", "thread-trash", "--thread-ids", "t1"},
		{"mail", "share-to-chat", "--message-id", "m1", "--receive-id", "oc_x"},
		{"msg", "forward", "omt_x", "--receive-id", "oc_x", "--receive-id-type", "chat_id"},
		{"msg", "merge-forward", "--receive-id", "oc_x", "--receive-id-type", "chat_id", "--message-ids", "om_a"},
		{"event", "consume", "approval.instance.status_changed_v4"},
	}
	for _, args := range cases {
		label := strings.Join(args, " ")
		if _, stderr, err := runCLI(t, append(append([]string{}, args...), "--dry-run")...); err != nil {
			t.Errorf("%s --dry-run 无凭据应 exit 0，got exit %d: %v\n%s", label, exitCodeFor(err), err, stderr)
		}
		if args[0] == "event" || args[1] == "rule-create" {
			// event consume 实跑会建连；rule-create 的可重复 flag 无法被 runCLI 的复位逻辑清空
			continue
		}
		_, _, err := runCLI(t, append(append([]string{}, args...), "--yes")...)
		if exitCodeFor(err) != 3 {
			t.Errorf("%s 无凭据实跑应 exit 3，got exit %d: %v", label, exitCodeFor(err), err)
		}
	}
}
