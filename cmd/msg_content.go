package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// messageContentInput 是 msg send / msg reply 共用的内容输入模型。
// 快捷参数会推断消息类型；--content / --content-file 则保留显式 --msg-type。
type messageContentInput struct {
	msgType        string
	msgTypeChanged bool
	content        string
	contentFile    string
	text           string
	markdown       string
	file           string
	image          string
	audio          string
	video          string
	videoCover     string
	uploadImages   bool
}

var uploadIMFileWithOptions = client.UploadIMFileWithOptions

func addMessageContentFlags(command *cobra.Command) {
	command.Flags().String("msg-type", "text", "消息类型（text/post/image/file/audio/media/interactive 等）")
	command.Flags().StringP("content", "c", "", "消息内容 JSON")
	command.Flags().String("content-file", "", "消息内容 JSON 文件")
	command.Flags().StringP("text", "t", "", "简单文本消息")
	command.Flags().String("markdown", "", "Markdown 消息（自动包装为 post）")
	command.Flags().StringP("file", "f", "", "本地文件路径或 file_key（作为附件发送）")
	command.Flags().String("image", "", "本地图片路径或 image_key")
	command.Flags().String("audio", "", "本地 Opus/Ogg Opus 路径或 file_key")
	command.Flags().String("video", "", "本地 MP4 路径或 file_key（需同时指定 --video-cover）")
	command.Flags().String("video-cover", "", "视频封面本地图片路径或 image_key（仅与 --video 一起使用）")
	command.Flags().Bool("upload-images", false, "自动上传 post/interactive 中的 Markdown 与图片字段本地路径")
}

func readMessageContentInput(command *cobra.Command) messageContentInput {
	input := messageContentInput{}
	input.msgType, _ = command.Flags().GetString("msg-type")
	input.msgTypeChanged = command.Flags().Changed("msg-type")
	input.content, _ = command.Flags().GetString("content")
	input.contentFile, _ = command.Flags().GetString("content-file")
	input.text, _ = command.Flags().GetString("text")
	input.markdown, _ = command.Flags().GetString("markdown")
	input.file, _ = command.Flags().GetString("file")
	input.image, _ = command.Flags().GetString("image")
	input.audio, _ = command.Flags().GetString("audio")
	input.video, _ = command.Flags().GetString("video")
	input.videoCover, _ = command.Flags().GetString("video-cover")
	input.uploadImages, _ = command.Flags().GetBool("upload-images")
	return input
}

func (input messageContentInput) validate() error {
	if err := validateSendMessageType(input.msgType); err != nil {
		return err
	}

	sources := []struct {
		name  string
		value string
	}{
		{"--content", input.content},
		{"--content-file", input.contentFile},
		{"--text", input.text},
		{"--markdown", input.markdown},
		{"--file", input.file},
		{"--image", input.image},
		{"--audio", input.audio},
		{"--video", input.video},
	}
	var specified []string
	for _, source := range sources {
		if source.value != "" {
			specified = append(specified, source.name)
		}
	}
	if len(specified) == 0 {
		return fmt.Errorf("必须指定 --content、--content-file、--text、--markdown、--file、--image、--audio 或 --video")
	}
	if len(specified) > 1 {
		return fmt.Errorf("以下内容标志互斥，只能指定其中一个: %s", strings.Join(specified, ", "))
	}
	if input.video != "" && input.videoCover == "" {
		return fmt.Errorf("使用 --video 时必须同时指定 --video-cover")
	}
	if input.video == "" && input.videoCover != "" {
		return fmt.Errorf("--video-cover 只能与 --video 一起使用")
	}

	inferredType := input.inferredMessageType()
	if input.msgTypeChanged && inferredType != "" && input.msgType != inferredType {
		return fmt.Errorf("--msg-type %q 与内容快捷参数推断出的消息类型 %q 冲突", input.msgType, inferredType)
	}

	switch {
	case input.content != "":
		if !json.Valid([]byte(input.content)) {
			return fmt.Errorf("--content 必须是有效 JSON")
		}
	case input.contentFile != "":
		data, err := os.ReadFile(input.contentFile)
		if err != nil {
			return fmt.Errorf("读取内容文件失败: %w", err)
		}
		if !json.Valid(data) {
			return fmt.Errorf("--content-file %s 的内容必须是有效 JSON", input.contentFile)
		}
	case input.image != "":
		if err := validateImageInput("--image", input.image); err != nil {
			return err
		}
	case input.file != "":
		if err := validateFileInput("--file", input.file, "file"); err != nil {
			return err
		}
	case input.audio != "":
		if err := validateFileInput("--audio", input.audio, "audio"); err != nil {
			return err
		}
		if !isIMFileKey(input.audio) {
			ext := strings.ToLower(filepath.Ext(input.audio))
			if ext != ".opus" && ext != ".ogg" {
				return fmt.Errorf("--audio 仅支持 Opus（.opus）或 Ogg Opus（.ogg）；其他音频请用 --file 作为附件发送")
			}
		}
	case input.video != "":
		if err := validateFileInput("--video", input.video, "video"); err != nil {
			return err
		}
		if !isIMFileKey(input.video) && strings.ToLower(filepath.Ext(input.video)) != ".mp4" {
			return fmt.Errorf("--video 仅支持 MP4 文件；其他视频请先转换为 MP4，或用 --file 作为附件发送")
		}
		if err := validateImageInput("--video-cover", input.videoCover); err != nil {
			return err
		}
	}
	return nil
}

func (input messageContentInput) inferredMessageType() string {
	switch {
	case input.text != "":
		return "text"
	case input.markdown != "":
		return "post"
	case input.image != "":
		return "image"
	case input.file != "":
		return "file"
	case input.audio != "":
		return "audio"
	case input.video != "":
		return "media"
	default:
		return ""
	}
}

func isIMImageKey(value string) bool {
	return strings.HasPrefix(value, "img_")
}

func isIMFileKey(value string) bool {
	return strings.HasPrefix(value, "file_")
}

func rejectRemoteMediaURL(flagName, value string) error {
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return fmt.Errorf("%s 暂不下载远程 URL；请使用本地路径，或先上传后传入飞书资源 key", flagName)
	}
	return nil
}

func resolveStandaloneMediaPath(value string) (string, error) {
	path, err := resolveLocalPath(value, ".")
	if err != nil {
		return "", err
	}
	return filepath.Clean(path), nil
}

func validateImageInput(flagName, value string) error {
	if isIMImageKey(value) {
		return nil
	}
	if err := rejectRemoteMediaURL(flagName, value); err != nil {
		return err
	}
	path, err := resolveStandaloneMediaPath(value)
	if err != nil {
		return fmt.Errorf("解析 %s 路径失败: %w", flagName, err)
	}
	if err := validateLocalIMImage(path); err != nil {
		return fmt.Errorf("%s 图片不可用 %s: %w", flagName, path, err)
	}
	return nil
}

func validateFileInput(flagName, value, label string) error {
	if isIMFileKey(value) {
		return nil
	}
	if err := rejectRemoteMediaURL(flagName, value); err != nil {
		return err
	}
	path, err := resolveStandaloneMediaPath(value)
	if err != nil {
		return fmt.Errorf("解析 %s 路径失败: %w", flagName, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s %s 不可用 %s: %w", flagName, label, path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s %s 不是普通文件: %s", flagName, label, path)
	}
	if info.Size() == 0 {
		return fmt.Errorf("%s %s 不能为空文件: %s", flagName, label, path)
	}
	return nil
}

func (input messageContentInput) resolve() (string, string, error) {
	msgType := input.msgType
	var content string

	switch {
	case input.contentFile != "":
		data, err := os.ReadFile(input.contentFile)
		if err != nil {
			return "", "", fmt.Errorf("读取内容文件失败: %w", err)
		}
		content = string(data)
	case input.content != "":
		content = input.content
	case input.text != "":
		msgType = "text"
		content = client.CreateTextMessageContent(client.NormalizeAtMentions(input.text))
	case input.markdown != "":
		msgType = "post"
		content = createMarkdownPostContent(input.markdown)
	case input.image != "":
		msgType = "image"
		key, err := resolveImageKey("--image", input.image)
		if err != nil {
			return "", "", err
		}
		content = marshalMessageKey("image_key", key)
	case input.file != "":
		msgType = "file"
		key, err := resolveFileKey("--file", input.file, fileUploadTypeForAttachment(input.file))
		if err != nil {
			return "", "", err
		}
		content = marshalMessageKey("file_key", key)
	case input.audio != "":
		msgType = "audio"
		key, err := resolveFileKey("--audio", input.audio, "opus")
		if err != nil {
			return "", "", err
		}
		content = marshalMessageKey("file_key", key)
	case input.video != "":
		msgType = "media"
		fileKey, err := resolveFileKey("--video", input.video, "mp4")
		if err != nil {
			return "", "", err
		}
		imageKey, err := resolveImageKey("--video-cover", input.videoCover)
		if err != nil {
			return "", "", err
		}
		data, _ := json.Marshal(map[string]string{
			"file_key":  fileKey,
			"image_key": imageKey,
		})
		content = string(data)
	}

	if input.uploadImages && (msgType == "post" || msgType == "interactive") {
		basePath := "."
		if input.contentFile != "" {
			basePath = filepath.Dir(input.contentFile)
		}
		processed, count, err := processAndUploadLocalImages(content, basePath)
		if err != nil {
			return "", "", err
		}
		if count > 0 {
			fmt.Fprintf(os.Stderr, "已自动上传 %d 张本地图片\n", count)
		}
		content = processed
	}
	return msgType, content, nil
}

func createMarkdownPostContent(markdown string) string {
	data, _ := json.Marshal(map[string]interface{}{
		"zh_cn": map[string]interface{}{
			"title": "",
			"content": [][]map[string]string{
				{{"tag": "md", "text": markdown}},
			},
		},
	})
	return string(data)
}

func marshalMessageKey(name, value string) string {
	data, _ := json.Marshal(map[string]string{name: value})
	return string(data)
}

func resolveImageKey(flagName, value string) (string, error) {
	if isIMImageKey(value) {
		return value, nil
	}
	path, err := resolveStandaloneMediaPath(value)
	if err != nil {
		return "", fmt.Errorf("解析 %s 路径失败: %w", flagName, err)
	}
	fmt.Fprintf(os.Stderr, "正在上传图片: %s\n", filepath.Base(path))
	key, err := uploadIMImage(path, "")
	if err != nil {
		return "", fmt.Errorf("%s 上传失败: %w", flagName, err)
	}
	if key == "" {
		return "", fmt.Errorf("%s 上传成功但未返回 image_key", flagName)
	}
	return key, nil
}

func resolveFileKey(flagName, value, fileType string) (string, error) {
	if isIMFileKey(value) {
		return value, nil
	}
	path, err := resolveStandaloneMediaPath(value)
	if err != nil {
		return "", fmt.Errorf("解析 %s 路径失败: %w", flagName, err)
	}
	fmt.Fprintf(os.Stderr, "正在上传文件: %s\n", filepath.Base(path))
	key, err := uploadIMFileWithOptions(path, "", fileType, 0)
	if err != nil {
		return "", fmt.Errorf("%s 上传失败: %w", flagName, err)
	}
	if key == "" {
		return "", fmt.Errorf("%s 上传成功但未返回 file_key", flagName)
	}
	return key, nil
}

func fileUploadTypeForAttachment(value string) string {
	if isIMFileKey(value) {
		return ""
	}
	switch strings.ToLower(filepath.Ext(value)) {
	case ".opus", ".ogg", ".mp4":
		// 飞书要求 opus/mp4 类型的 file_key 分别用于 audio/media。
		// 用户显式使用 --file 时，应按普通附件上传，避免发送阶段报 230055。
		return "stream"
	default:
		return ""
	}
}
