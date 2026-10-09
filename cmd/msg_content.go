package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/safefile"
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
	// attachments 是 --attachment 的取值（file_key 或本地文件路径），合并进 post 的附件区（files）。
	attachments []string
	// dryRun 为 true 时只构造请求：本地文件不上传，用 img_dryrun_N / file_dryrun_N 占位。
	dryRun       bool
	dryRunSerial int
	// errOut 诊断输出（stderr）；nil 时用 os.Stderr。
	errOut io.Writer
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
	command.Flags().StringSlice("attachment", nil, "post 附件区文件：file_key 或本地文件路径（可重复/逗号分隔；需配合 --markdown 或 post 内容，单独使用时发送仅含附件的 post）")
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
	input.attachments, _ = command.Flags().GetStringSlice("attachment")
	input.errOut = command.ErrOrStderr()
	return input
}

func (input *messageContentInput) stderr() io.Writer {
	if input.errOut != nil {
		return input.errOut
	}
	return os.Stderr
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
	if len(specified) == 0 && len(input.attachments) == 0 {
		return fmt.Errorf("必须指定 --content、--content-file、--text、--markdown、--file、--image、--audio、--video 或 --attachment")
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
	if err := input.validateAttachments(); err != nil {
		return err
	}

	switch {
	case input.content != "":
		if !json.Valid([]byte(input.content)) {
			return fmt.Errorf("--content 必须是有效 JSON")
		}
	case input.contentFile != "":
		data, err := readLocalInputFile(input.contentFile)
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
	case len(input.attachments) > 0 && input.content == "" && input.contentFile == "":
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
	return strings.HasPrefix(value, "img_") && !localRegularFileExists(value, ".")
}

func isIMFileKey(value string) bool {
	return strings.HasPrefix(value, "file_") && !localRegularFileExists(value, ".")
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
	// 文件会上传到飞书：敏感目录、不存在、是目录、无权限读取均为用法错误
	info, err := safefile.StatInputFile(path)
	if err != nil {
		return fmt.Errorf("%s %s 不可用: %w", flagName, label, err)
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
	markdownHandled := false

	switch {
	case input.contentFile != "":
		data, err := readLocalInputFile(input.contentFile)
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
		md, err := input.prepareMarkdown(input.markdown)
		if err != nil {
			return "", "", err
		}
		content = createMarkdownPostContent(md)
		markdownHandled = true
	case input.image != "":
		msgType = "image"
		key, err := input.resolveImageKey("--image", input.image)
		if err != nil {
			return "", "", err
		}
		content = marshalMessageKey("image_key", key)
	case input.file != "":
		msgType = "file"
		key, err := input.resolveFileKey("--file", input.file, fileUploadTypeForAttachment(input.file))
		if err != nil {
			return "", "", err
		}
		content = marshalMessageKey("file_key", key)
	case input.audio != "":
		msgType = "audio"
		key, err := input.resolveFileKey("--audio", input.audio, "opus")
		if err != nil {
			return "", "", err
		}
		content = marshalMessageKey("file_key", key)
	case input.video != "":
		msgType = "media"
		fileKey, err := input.resolveFileKey("--video", input.video, "mp4")
		if err != nil {
			return "", "", err
		}
		imageKey, err := input.resolveImageKey("--video-cover", input.videoCover)
		if err != nil {
			return "", "", err
		}
		data, _ := json.Marshal(map[string]string{
			"file_key":  fileKey,
			"image_key": imageKey,
		})
		content = string(data)
	case len(input.attachments) > 0:
		// 仅附件：发送一条只有附件区的 post
		msgType = "post"
		content = `{"zh_cn":{"content":[]}}`
	}

	// --content / --content-file 的 text/post 消息体同样规范化 @ 标签（对齐官方 send/reply）；
	// 必须在 JSON 层面逐字符串处理，直接对 JSON 串做正则替换会写入未转义的双引号。
	if (input.content != "" || input.contentFile != "") && (msgType == "text" || msgType == "post") {
		content = client.NormalizeAtMentionsInJSON(content)
	}

	if input.uploadImages && !markdownHandled && (msgType == "post" || msgType == "interactive") {
		if input.dryRun {
			fmt.Fprintln(input.stderr(), "[dry-run] --upload-images：预览不上传 post/interactive 中的本地图片，实际发送时会上传并替换为 image_key")
		} else {
			basePath := "."
			if input.contentFile != "" {
				basePath = filepath.Dir(input.contentFile)
			}
			processed, count, err := processAndUploadLocalImages(content, basePath)
			if err != nil {
				return "", "", err
			}
			if count > 0 {
				fmt.Fprintf(input.stderr(), "已自动上传 %d 张本地图片\n", count)
			}
			content = processed
		}
	}

	// 附件区：把 --attachment 的文件合并进 post 顶层 files（对齐官方 --attachment）。
	if len(input.attachments) > 0 {
		keys, err := input.resolveAttachmentKeys()
		if err != nil {
			return "", "", err
		}
		merged, err := mergeAttachmentsIntoPostContent(content, keys)
		if err != nil {
			return "", "", fmt.Errorf("--attachment: 合并附件到 post 内容失败: %w", err)
		}
		msgType = "post"
		content = merged
	}
	return msgType, content, nil
}

// prepareMarkdown 处理 --markdown 正文：规范化 @ 标签 → 上传本地图片（--upload-images）→
// 样式归一（标题降级、表格空行等，对齐官方）→ 删除 md 无法渲染的非 img_ 图片引用并提示。
func (input *messageContentInput) prepareMarkdown(markdown string) (string, error) {
	md := client.NormalizeAtMentions(markdown)
	if input.uploadImages {
		if input.dryRun {
			md = input.placeholderLocalMarkdownImages(md)
		} else {
			processed, count, err := processAndUploadLocalImages(md, ".")
			if err != nil {
				return "", err
			}
			if count > 0 {
				fmt.Fprintf(input.stderr(), "已自动上传 %d 张本地图片\n", count)
			}
			md = processed
		}
	}
	md = optimizeMarkdownStyle(md)
	md, removed := stripNonIMMarkdownImages(md)
	if len(removed) > 0 {
		hint := "post 的 md 只能渲染 img_xxx 图片"
		if !input.uploadImages {
			hint += "；本地图片请加 --upload-images 自动上传"
		}
		fmt.Fprintf(input.stderr(), "[提示] 已移除 %d 个无法渲染的图片引用（%s）: %s\n", len(removed), hint, strings.Join(removed, ", "))
	}
	return md, nil
}

// placeholderLocalMarkdownImages dry-run 时把 Markdown 中的本地图片替换为占位 key，不上传。
func (input *messageContentInput) placeholderLocalMarkdownImages(md string) string {
	return markdownImageRegex.ReplaceAllStringFunc(md, func(m string) string {
		sub := markdownImageRegex.FindStringSubmatch(m)
		if len(sub) < 3 || !isLocalPath(sub[2], ".") {
			return m
		}
		input.dryRunSerial++
		return fmt.Sprintf("![%s](img_dryrun_%d)", sub[1], input.dryRunSerial)
	})
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

func (input *messageContentInput) resolveImageKey(flagName, value string) (string, error) {
	if isIMImageKey(value) {
		return value, nil
	}
	path, err := resolveStandaloneMediaPath(value)
	if err != nil {
		return "", fmt.Errorf("解析 %s 路径失败: %w", flagName, err)
	}
	if input.dryRun {
		input.dryRunSerial++
		return fmt.Sprintf("img_dryrun_%d", input.dryRunSerial), nil
	}
	fmt.Fprintf(input.stderr(), "正在上传图片: %s\n", filepath.Base(path))
	key, err := uploadIMImage(path, "")
	if err != nil {
		return "", fmt.Errorf("%s 上传失败: %w", flagName, err)
	}
	if key == "" {
		return "", fmt.Errorf("%s 上传成功但未返回 image_key", flagName)
	}
	return key, nil
}

func (input *messageContentInput) resolveFileKey(flagName, value, fileType string) (string, error) {
	if isIMFileKey(value) {
		return value, nil
	}
	path, err := resolveStandaloneMediaPath(value)
	if err != nil {
		return "", fmt.Errorf("解析 %s 路径失败: %w", flagName, err)
	}
	if input.dryRun {
		input.dryRunSerial++
		return fmt.Sprintf("file_dryrun_%d", input.dryRunSerial), nil
	}
	// 语音/视频带上时长（毫秒），客户端才能显示时长（对齐官方 parseMediaDuration）；解析失败传 0。
	duration := 0
	if fileType == "opus" || fileType == "mp4" {
		duration = parseMediaDurationMs(path, fileType)
	}
	fmt.Fprintf(input.stderr(), "正在上传文件: %s\n", filepath.Base(path))
	key, err := uploadIMFileWithOptions(path, "", fileType, duration)
	if err != nil {
		return "", fmt.Errorf("%s 上传失败: %w", flagName, err)
	}
	if key == "" {
		return "", fmt.Errorf("%s 上传成功但未返回 file_key", flagName)
	}
	return key, nil
}

// parseMediaDurationMs 可在测试中替换。
var parseMediaDurationMs = client.ParseLocalMediaDurationMs

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
