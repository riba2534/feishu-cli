package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newMessageContentTestCommand(t *testing.T, args ...string) (*cobra.Command, messageContentInput) {
	t.Helper()
	command := &cobra.Command{Use: "test"}
	addMessageContentFlags(command)
	if err := command.ParseFlags(args); err != nil {
		t.Fatalf("解析测试 flag 失败: %v", err)
	}
	return command, readMessageContentInput(command)
}

func TestMessageContentInputValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"text", []string{"--text", "hello"}, ""},
		{"markdown", []string{"--markdown", "**hello**"}, ""},
		{"image key", []string{"--image", "img_xxx"}, ""},
		{"file key", []string{"--file", "file_xxx"}, ""},
		{"audio key", []string{"--audio", "file_xxx"}, ""},
		{"video keys", []string{"--video", "file_xxx", "--video-cover", "img_xxx"}, ""},
		{"mutually exclusive", []string{"--text", "a", "--image", "img_xxx"}, "互斥"},
		{"missing video cover", []string{"--video", "file_xxx"}, "--video-cover"},
		{"orphan video cover", []string{"--text", "a", "--video-cover", "img_xxx"}, "只能与 --video"},
		{"explicit type conflict", []string{"--msg-type", "text", "--image", "img_xxx"}, "冲突"},
		{"invalid JSON", []string{"--content", "not-json"}, "有效 JSON"},
		{"remote image rejected", []string{"--image", "https://example.com/a.png"}, "暂不下载远程 URL"},
		{"invalid audio extension", []string{"--audio", "voice.mp3"}, "不可用"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, input := newMessageContentTestCommand(t, test.args...)
			err := input.validate()
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validate() error = %v, want contains %q", err, test.wantErr)
			}
		})
	}
}

func TestMessageContentResolveMarkdownAndExistingKeys(t *testing.T) {
	_, markdownInput := newMessageContentTestCommand(t, "--markdown", "**hello**")
	if err := markdownInput.validate(); err != nil {
		t.Fatal(err)
	}
	msgType, content, err := markdownInput.resolve()
	if err != nil {
		t.Fatal(err)
	}
	if msgType != "post" || !json.Valid([]byte(content)) || !strings.Contains(content, `"tag":"md"`) {
		t.Fatalf("Markdown 解析结果不正确: type=%s content=%s", msgType, content)
	}

	_, videoInput := newMessageContentTestCommand(t,
		"--video", "file_video", "--video-cover", "img_cover")
	if err := videoInput.validate(); err != nil {
		t.Fatal(err)
	}
	msgType, content, err = videoInput.resolve()
	if err != nil {
		t.Fatal(err)
	}
	if msgType != "media" ||
		!strings.Contains(content, `"file_key":"file_video"`) ||
		!strings.Contains(content, `"image_key":"img_cover"`) {
		t.Fatalf("视频解析结果不正确: type=%s content=%s", msgType, content)
	}
}

func TestMessageContentResolveLocalMedia(t *testing.T) {
	tempDir := t.TempDir()
	imagePath := filepath.Join(tempDir, "image.png")
	if err := os.WriteFile(imagePath, []byte("\x89PNG\r\n\x1a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mp4Path := filepath.Join(tempDir, "clip.mp4")
	if err := os.WriteFile(mp4Path, []byte("fake-mp4"), 0o600); err != nil {
		t.Fatal(err)
	}

	oldImageUpload := uploadIMImage
	oldFileUpload := uploadIMFileWithOptions
	defer func() {
		uploadIMImage = oldImageUpload
		uploadIMFileWithOptions = oldFileUpload
	}()

	var gotImagePath string
	uploadIMImage = func(path, imageType string) (string, error) {
		gotImagePath = path
		if imageType != "" {
			t.Fatalf("imageType = %q, want default", imageType)
		}
		return "img_uploaded", nil
	}
	var gotFileType string
	uploadIMFileWithOptions = func(path, name, fileType string, duration int) (string, error) {
		if path != mp4Path {
			t.Fatalf("path = %q, want %q", path, mp4Path)
		}
		gotFileType = fileType
		return "file_uploaded", nil
	}

	_, imageInput := newMessageContentTestCommand(t, "--image", imagePath)
	if err := imageInput.validate(); err != nil {
		t.Fatal(err)
	}
	msgType, content, err := imageInput.resolve()
	if err != nil {
		t.Fatal(err)
	}
	if gotImagePath != imagePath || msgType != "image" || content != `{"image_key":"img_uploaded"}` {
		t.Fatalf("图片结果异常: path=%q type=%s content=%s", gotImagePath, msgType, content)
	}

	_, fileInput := newMessageContentTestCommand(t, "--file", mp4Path)
	if err := fileInput.validate(); err != nil {
		t.Fatal(err)
	}
	msgType, content, err = fileInput.resolve()
	if err != nil {
		t.Fatal(err)
	}
	if gotFileType != "stream" || msgType != "file" || content != `{"file_key":"file_uploaded"}` {
		t.Fatalf("MP4 附件结果异常: fileType=%s type=%s content=%s", gotFileType, msgType, content)
	}
}

func TestValidateReplyMessageID(t *testing.T) {
	if err := validateReplyMessageID("om_xxx"); err != nil {
		t.Fatalf("om_xxx 应合法: %v", err)
	}
	if err := validateReplyMessageID("omt_xxx"); err == nil || !strings.Contains(err.Error(), "thread_id") {
		t.Fatalf("omt_xxx 应返回 thread_id 提示: %v", err)
	}
}

func TestValidateIdempotencyKeyUnicodeCharacters(t *testing.T) {
	if err := validateIdempotencyKey(strings.Repeat("中", 50)); err != nil {
		t.Fatalf("50 个 Unicode 字符应合法: %v", err)
	}
	if err := validateIdempotencyKey(strings.Repeat("中", 51)); err == nil {
		t.Fatal("51 个 Unicode 字符应被拒绝")
	}
}

func TestSendThreadIDFailsBeforeContentOrUpload(t *testing.T) {
	threadFlag := sendMessageCmd.Flags().Lookup("thread-id")
	imageFlag := sendMessageCmd.Flags().Lookup("image")
	oldThreadValue, oldThreadChanged := threadFlag.Value.String(), threadFlag.Changed
	oldImageValue, oldImageChanged := imageFlag.Value.String(), imageFlag.Changed
	defer func() {
		_ = threadFlag.Value.Set(oldThreadValue)
		threadFlag.Changed = oldThreadChanged
		_ = imageFlag.Value.Set(oldImageValue)
		imageFlag.Changed = oldImageChanged
	}()

	if err := threadFlag.Value.Set("omt_xxx"); err != nil {
		t.Fatal(err)
	}
	threadFlag.Changed = true
	if err := imageFlag.Value.Set("/definitely/missing.png"); err != nil {
		t.Fatal(err)
	}
	imageFlag.Changed = true

	err := sendMessageCmd.RunE(sendMessageCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "msg reply") {
		t.Fatalf("--thread-id 应在检查图片/配置前失败并提示 reply，实际: %v", err)
	}
}
