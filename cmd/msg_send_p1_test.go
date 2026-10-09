package cmd

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return dir
}

func TestExpandMessageInputValue(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("note.md", []byte("\ufeff# 来自文件"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldStdin := messageInputStdin
	defer func() { messageInputStdin = oldStdin }()

	tests := []struct {
		name     string
		flag     string
		raw      string
		literal  bool
		stdin    string
		want     string
		wantErr  string
		wantHint bool
	}{
		{name: "普通文本原样", flag: "--text", raw: "hello", literal: true, want: "hello"},
		{name: "stdin", flag: "--markdown", raw: "-", literal: true, stdin: "来自 stdin", want: "来自 stdin"},
		{name: "@file 读文件并去 BOM", flag: "--markdown", raw: "@note.md", literal: true, want: "# 来自文件"},
		{name: "@@ 转义", flag: "--text", raw: "@@note.md", literal: true, want: "@note.md"},
		{name: "text 的 @张三 文件不存在按字面", flag: "--text", raw: "@张三 你好", literal: true, want: "@张三 你好"},
		{name: "text 像路径但不存在：字面+提示", flag: "--text", raw: "@missing.md", literal: true, want: "@missing.md", wantHint: true},
		{name: "content 文件不存在报错", flag: "--content", raw: "@missing.json", literal: false, wantErr: "读取文件"},
		{name: "content 空路径报错", flag: "--content", raw: "@", literal: false, wantErr: "不能为空"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messageInputStdin = strings.NewReader(tt.stdin)
			var errOut bytes.Buffer
			got, err := expandMessageInputValue(tt.flag, tt.raw, tt.literal, new(bool), &errOut)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want contains %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			if tt.wantHint != (errOut.Len() > 0) {
				t.Fatalf("hint = %q, wantHint=%v", errOut.String(), tt.wantHint)
			}
		})
	}

	// 同一次调用只能有一个参数读 stdin
	used := true
	if _, err := expandMessageInputValue("--content", "-", false, &used, nil); err == nil {
		t.Fatal("stdin 已被占用时应报错")
	}
}

func TestOptimizeMarkdownStyle(t *testing.T) {
	in := "# 一级\n## 二级\n正文\n| a | b |\n|---|---|\n| 1 | 2 |\n结尾\n\n\n\n```\n# 代码里的标题不动\n```"
	got := optimizeMarkdownStyle(in)
	for _, want := range []string{"#### 一级", "##### 二级", "正文\n\n| a | b |", "| 1 | 2 |\n\n结尾", "# 代码里的标题不动"} {
		if !strings.Contains(got, want) {
			t.Fatalf("缺少 %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\n\n\n") {
		t.Fatalf("应压缩多余空行:\n%q", got)
	}
	// 只有 H4 以下时不降级
	if got := optimizeMarkdownStyle("#### 小标题\n内容"); !strings.HasPrefix(got, "#### 小标题") {
		t.Fatalf("无 H1~H3 时不应改动标题: %q", got)
	}
}

func TestStripNonIMMarkdownImages(t *testing.T) {
	in := "![ok](img_v3_abc) ![remote](https://example.com/a.png) ![local](./a.png)\n```\n![in code](https://example.com/x.png)\n```"
	got, removed := stripNonIMMarkdownImages(in)
	if !strings.Contains(got, "![ok](img_v3_abc)") || strings.Contains(got, "example.com/a.png") || strings.Contains(got, "./a.png") {
		t.Fatalf("图片过滤结果不对: %q", got)
	}
	if !strings.Contains(got, "![in code](https://example.com/x.png)") {
		t.Fatalf("代码块内的内容不应处理: %q", got)
	}
	if len(removed) != 2 {
		t.Fatalf("removed = %v", removed)
	}
}

// TestMarkdownUploadsLocalImagesBeforeStripping --upload-images 必须在剔除非 img_ 图片之前完成上传。
func TestMarkdownUploadsLocalImagesBeforeStripping(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("pic.png", []byte("\x89PNG\r\n\x1a\n0000"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldImageUpload := uploadIMImage
	defer func() { uploadIMImage = oldImageUpload }()
	uploadIMImage = func(path, imageType string) (string, error) { return "img_uploaded_pic", nil }

	cmd, input := newMessageContentTestCommand(t, "--markdown", "# 标题\n![图](pic.png)", "--upload-images")
	cmd.SetErr(&bytes.Buffer{})
	input.errOut = &bytes.Buffer{}
	if err := input.validate(); err != nil {
		t.Fatal(err)
	}
	_, content, err := input.resolve()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "img_uploaded_pic") || !strings.Contains(content, "#### 标题") {
		t.Fatalf("本地图片应先上传再归一样式: %s", content)
	}
}

func TestMessageAttachmentsMergeIntoPost(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
		want    []string
	}{
		{name: "markdown + attachment", args: []string{"--markdown", "**hi**", "--attachment", "file_a,file_b", "--attachment", "file_a"},
			want: []string{`"files":[{"key":"file_a"},{"key":"file_b"}]`, `"tag":"md"`}},
		{name: "仅附件", args: []string{"--attachment", "file_only"}, want: []string{`"files":[{"key":"file_only"}]`}},
		{name: "post content + attachment", args: []string{"--msg-type", "post", "--content", `{"zh_cn":{"content":[]}}`, "--attachment", "file_c"},
			want: []string{`"files":[{"key":"file_c"}]`}},
		{name: "与 --text 冲突", args: []string{"--text", "hi", "--attachment", "file_a"}, wantErr: "--text"},
		{name: "与 --image 冲突", args: []string{"--image", "img_x", "--attachment", "file_a"}, wantErr: "--image"},
		{name: "content 非 post", args: []string{"--content", `{"text":"x"}`, "--attachment", "file_a"}, wantErr: "--msg-type post"},
		{name: "content 已含 files", args: []string{"--msg-type", "post", "--content", `{"files":[{"key":"file_z"}]}`, "--attachment", "file_a"}, wantErr: "files"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, input := newMessageContentTestCommand(t, tt.args...)
			err := input.validate()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("validate err = %v, want contains %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validate: %v", err)
			}
			msgType, content, err := input.resolve()
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if msgType != "post" || !json.Valid([]byte(content)) {
				t.Fatalf("type=%s content=%s", msgType, content)
			}
			for _, w := range tt.want {
				if !strings.Contains(content, w) {
					t.Fatalf("content 缺少 %s: %s", w, content)
				}
			}
		})
	}
}

// buildOggWithGranule 构造一个最小 Ogg 页（只关心最后一页的 granule position）。
func buildOggWithGranule(granule uint64) []byte {
	page := make([]byte, 27)
	copy(page, "OggS")
	binary.LittleEndian.PutUint64(page[6:], granule)
	return append([]byte("junk-before"), page...)
}

// buildMP4WithDuration 构造 ftyp + moov(mvhd v0) 的最小 MP4。
func buildMP4WithDuration(timescale, duration uint32) []byte {
	box := func(typ string, payload []byte) []byte {
		b := make([]byte, 8+len(payload))
		binary.BigEndian.PutUint32(b, uint32(len(b)))
		copy(b[4:], typ)
		copy(b[8:], payload)
		return b
	}
	mvhd := make([]byte, 20)
	binary.BigEndian.PutUint32(mvhd[12:], timescale)
	binary.BigEndian.PutUint32(mvhd[16:], duration)
	out := box("ftyp", []byte("isom0000"))
	return append(out, box("moov", box("mvhd", mvhd))...)
}

// TestAudioVideoUploadCarriesDuration 音视频上传带 duration（毫秒），--file 附件不带。
func TestAudioVideoUploadCarriesDuration(t *testing.T) {
	dir := t.TempDir()
	opus := filepath.Join(dir, "voice.opus")
	mp4 := filepath.Join(dir, "clip.mp4")
	cover := filepath.Join(dir, "cover.png")
	_ = os.WriteFile(opus, buildOggWithGranule(48000*3), 0o600) // 3s
	_ = os.WriteFile(mp4, buildMP4WithDuration(1000, 4500), 0o600)
	_ = os.WriteFile(cover, []byte("\x89PNG\r\n\x1a\n0000"), 0o600)

	oldFile, oldImage := uploadIMFileWithOptions, uploadIMImage
	defer func() { uploadIMFileWithOptions, uploadIMImage = oldFile, oldImage }()
	durations := map[string]int{}
	uploadIMFileWithOptions = func(path, name, fileType string, duration int) (string, error) {
		durations[filepath.Base(path)+"|"+fileType] = duration
		return "file_x", nil
	}
	uploadIMImage = func(path, imageType string) (string, error) { return "img_x", nil }

	for _, args := range [][]string{
		{"--audio", opus},
		{"--video", mp4, "--video-cover", cover},
		{"--file", mp4},
	} {
		_, input := newMessageContentTestCommand(t, args...)
		input.errOut = &bytes.Buffer{}
		if err := input.validate(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if _, _, err := input.resolve(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	if durations["voice.opus|opus"] != 3000 {
		t.Fatalf("opus duration = %d, want 3000 (%v)", durations["voice.opus|opus"], durations)
	}
	if durations["clip.mp4|mp4"] != 4500 {
		t.Fatalf("mp4 duration = %d, want 4500 (%v)", durations["clip.mp4|mp4"], durations)
	}
	if durations["clip.mp4|stream"] != 0 {
		t.Fatalf("--file 附件不应带 duration: %v", durations)
	}
}

func newSendTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("receive-id-type", "", "")
	cmd.Flags().String("receive-id", "", "")
	cmd.Flags().String("thread-id", "", "")
	addMessageContentFlags(cmd)
	cmd.Flags().String("idempotency-key", "", "")
	cmd.Flags().StringP("output", "o", "", "")
	cmd.Flags().String("user-access-token", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	return cmd
}

// TestSendDryRunDoesNotCallAPIOrUpload --dry-run 只输出请求预览：不发网、不上传本地文件。
func TestSendDryRunDoesNotCallAPIOrUpload(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	chdirTemp(t)
	_ = os.WriteFile("pic.png", []byte("\x89PNG\r\n\x1a\n0000"), 0o600)
	cleanup := stubCmdFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("dry-run 不应发出请求: %s", r.URL.Path)
	})
	defer cleanup()
	oldImage := uploadIMImage
	defer func() { uploadIMImage = oldImage }()
	uploadIMImage = func(path, imageType string) (string, error) {
		t.Error("dry-run 不应上传图片")
		return "", nil
	}

	cmd := newSendTestCmd()
	mustSetFlag(t, cmd, "receive-id-type", "chat_id")
	mustSetFlag(t, cmd, "receive-id", testChatID)
	mustSetFlag(t, cmd, "image", "pic.png")
	mustSetFlag(t, cmd, "dry-run", "true")
	out := captureStdout(t, func() {
		if err := sendMessageCmd.RunE(cmd, nil); err != nil {
			t.Fatalf("dry-run 返回错误: %v", err)
		}
	})
	var plan struct {
		DryRun bool `json:"dry_run"`
		API    []struct {
			Method string         `json:"method"`
			URL    string         `json:"url"`
			Body   map[string]any `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("解析预览失败: %v\n%s", err, out)
	}
	if !plan.DryRun || len(plan.API) != 1 || plan.API[0].URL != "/open-apis/im/v1/messages" {
		t.Fatalf("预览结构不对: %s", out)
	}
	if !strings.Contains(fmt.Sprint(plan.API[0].Body["content"]), "img_dryrun_1") {
		t.Fatalf("本地图片应以占位 key 预览: %s", out)
	}
}

// TestSendJSONOutputIncludesChatIDAndCreateTime msg send -o json 补 chat_id / create_time。
func TestSendJSONOutputIncludesChatIDAndCreateTime(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"message_id":"om_sent","chat_id":"oc_sent","create_time":"1700000000000","msg_type":"text"}}`)
	}))
	defer cleanup()

	cmd := newSendTestCmd()
	mustSetFlag(t, cmd, "receive-id-type", "chat_id")
	mustSetFlag(t, cmd, "receive-id", testChatID)
	mustSetFlag(t, cmd, "text", "hi")
	mustSetFlag(t, cmd, "output", "json")
	out := captureStdout(t, func() {
		if err := sendMessageCmd.RunE(cmd, nil); err != nil {
			t.Fatalf("send 返回错误: %v", err)
		}
	})
	var got map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("解析输出失败: %v\n%s", err, out)
	}
	if got["message_id"] != "om_sent" || got["chat_id"] != "oc_sent" || got["create_time"] != "1700000000000" {
		t.Fatalf("输出字段不全: %v", got)
	}
}

func newEditTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("msg-type", "text", "")
	cmd.Flags().StringP("text", "t", "", "")
	cmd.Flags().String("markdown", "", "")
	cmd.Flags().StringP("content", "c", "", "")
	cmd.Flags().Bool("upload-images", false, "")
	cmd.Flags().StringSlice("set-attachments", nil, "")
	cmd.Flags().Bool("clear-attachments", false, "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().StringP("output", "o", "", "")
	return cmd
}

// TestMsgEditUsesPutWithBotToken msg edit 走 PUT /im/v1/messages/:id，固定 Bot 身份，text 规范化 @。
func TestMsgEditUsesPutWithBotToken(t *testing.T) {
	isolateMsgTokenTestEnv(t)
	var gotMethod, gotPath, gotAuth string
	var gotBody map[string]any
	cleanup := stubCmdFeishuServer(t, tenantTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"message_id":"om_test_message","chat_id":"oc_x","update_time":"1700000000001"}}`)
	}))
	defer cleanup()

	cmd := newEditTestCmd()
	mustSetFlag(t, cmd, "text", "<at id=ou_x></at> 更正")
	mustSetFlag(t, cmd, "output", "json")
	out := captureStdout(t, func() {
		if err := msgEditCmd.RunE(cmd, []string{testMessageID}); err != nil {
			t.Fatalf("edit 返回错误: %v", err)
		}
	})
	if gotMethod != http.MethodPut || gotPath != "/open-apis/im/v1/messages/"+testMessageID || gotAuth != testTenantAuth {
		t.Fatalf("请求不符: %s %s auth=%s", gotMethod, gotPath, gotAuth)
	}
	if gotBody["msg_type"] != "text" || !strings.Contains(fmt.Sprint(gotBody["content"]), `user_id=\"ou_x\"`) {
		t.Fatalf("请求体不符: %v", gotBody)
	}
	if !strings.Contains(out, `"update_time": "1700000000001"`) {
		t.Fatalf("输出缺 update_time: %s", out)
	}
}

func TestMsgEditValidation(t *testing.T) {
	tests := []struct {
		name string
		set  map[string]string
		want string
	}{
		{"卡片不支持", map[string]string{"msg-type": "interactive", "content": "{}"}, "text|post"},
		{"缺内容", map[string]string{}, "必须指定"},
		{"内容互斥", map[string]string{"text": "a", "markdown": "b"}, "互斥"},
		{"清空与设置互斥", map[string]string{"markdown": "b", "set-attachments": "file_a", "clear-attachments": "true"}, "--clear-attachments"},
		{"附件不能配 text", map[string]string{"text": "a", "set-attachments": "file_a"}, "--text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newEditTestCmd()
			for k, v := range tt.set {
				mustSetFlag(t, cmd, k, v)
			}
			err := msgEditCmd.RunE(cmd, []string{testMessageID})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want contains %q", err, tt.want)
			}
		})
	}
}
