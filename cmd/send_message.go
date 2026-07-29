package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var sendMessageCmd = &cobra.Command{
	Use:   "send",
	Short: "发送消息",
	Long: `向飞书用户或群组发送消息。

参数:
  --receive-id-type   接收者类型（与 --thread-id 二选一）
  --receive-id        接收者标识（与 --thread-id 二选一）
  --thread-id         话题 ID（omt_xxx），在已有话题内追加一条消息（等价于 receive_id_type=thread_id）
  --msg-type          消息类型（默认: text）
  --content, -c       消息内容 JSON
  --content-file      消息内容 JSON 文件
  --text, -t          简单文本消息（快捷方式）
  --file, -f          发送本地文件（自动上传并发送，快捷方式）
  --image             发送本地图片（自动上传并发送，快捷方式）
  --upload-images     自动上传 Markdown、post image_key 与 Card V2 img_key 中的本地图片
  --idempotency-key   幂等键（≤50 字符），服务端按此键去重，防止重发
  --output, -o        输出格式（json）

接收者类型:
  email       邮箱
  open_id     Open ID
  user_id     用户 ID
  union_id    Union ID
  chat_id     群组 ID
  thread_id   话题 ID（在话题内追加消息，通常用 --thread-id 代替）

消息类型:
  text         文本消息
  post         富文本消息
  image        图片消息
  file         文件消息
  audio        音频消息
  media        媒体消息
  sticker      表情消息
  interactive  卡片消息
  share_chat   分享群消息
  share_user   分享用户消息

示例:
  # 发送文本消息
  feishu-cli msg send \
    --receive-id-type email \
    --receive-id user@example.com \
    --text "你好，这是一条测试消息"

  # 发送到群组
  feishu-cli msg send \
    --receive-id-type chat_id \
    --receive-id oc_xxx \
    --text "群消息"

  # 直接发送本地文件（自动上传）
  feishu-cli msg send \
    --receive-id-type chat_id \
    --receive-id oc_xxx \
    --file /path/to/report.pdf

  # 直接发送本地图片（自动上传）
  feishu-cli msg send \
    --receive-id-type chat_id \
    --receive-id oc_xxx \
    --image /path/to/screenshot.png

  # 发送卡片消息
  feishu-cli msg send \
    --receive-id-type email \
    --receive-id user@example.com \
    --msg-type interactive \
    --content-file card.json

  # 发送卡片消息并自动上传本地图片
  feishu-cli msg send \
    --receive-id-type email \
    --receive-id user@example.com \
    --msg-type interactive \
    --content-file card.json \
    --upload-images

  # 在已有话题内追加消息
  feishu-cli msg send \
    --thread-id omt_xxx \
    --text "话题内继续聊"

  # 使用幂等键防止重发（相同 key 重复调用只会发出一条消息）
  feishu-cli msg send \
    --receive-id-type email \
    --receive-id user@example.com \
    --text "对账通知" \
    --idempotency-key "bill-2026-07-22"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		receiveIDType, _ := cmd.Flags().GetString("receive-id-type")
		receiveID, _ := cmd.Flags().GetString("receive-id")
		threadID, _ := cmd.Flags().GetString("thread-id")
		msgType, _ := cmd.Flags().GetString("msg-type")

		if threadID != "" {
			if receiveIDType != "" || receiveID != "" {
				return fmt.Errorf("--thread-id 与 --receive-id-type/--receive-id 互斥，只能指定一组")
			}
			receiveIDType = "thread_id"
			receiveID = threadID
		} else if receiveIDType == "" || receiveID == "" {
			return fmt.Errorf("必须指定 --thread-id，或同时指定 --receive-id-type 和 --receive-id")
		}
		if err := validateSendReceiveIDType(receiveIDType); err != nil {
			return err
		}
		if err := validateSendMessageType(msgType); err != nil {
			return err
		}

		idempotencyKey, _ := cmd.Flags().GetString("idempotency-key")
		if err := validateIdempotencyKey(idempotencyKey); err != nil {
			return err
		}

		if err := config.Validate(); err != nil {
			return err
		}

		token := resolveOptionalUserToken(cmd)

		content, _ := cmd.Flags().GetString("content")
		contentFile, _ := cmd.Flags().GetString("content-file")
		text, _ := cmd.Flags().GetString("text")
		filePath, _ := cmd.Flags().GetString("file")
		imagePath, _ := cmd.Flags().GetString("image")
		uploadImages, _ := cmd.Flags().GetBool("upload-images")

		// 互斥校验：5 个内容标志只能指定一个
		var specifiedFlags []string
		if filePath != "" {
			specifiedFlags = append(specifiedFlags, "--file")
		}
		if imagePath != "" {
			specifiedFlags = append(specifiedFlags, "--image")
		}
		if contentFile != "" {
			specifiedFlags = append(specifiedFlags, "--content-file")
		}
		if content != "" {
			specifiedFlags = append(specifiedFlags, "--content")
		}
		if text != "" {
			specifiedFlags = append(specifiedFlags, "--text")
		}
		if len(specifiedFlags) > 1 {
			return fmt.Errorf("以下标志互斥，只能指定其中一个: %s", fmt.Sprintf("%v", specifiedFlags))
		}

		// 文件/图片路径预检查
		if filePath != "" {
			if _, err := os.Stat(filePath); err != nil {
				return fmt.Errorf("无法访问文件: %w", err)
			}
		}
		if imagePath != "" {
			if _, err := os.Stat(imagePath); err != nil {
				return fmt.Errorf("无法访问图片文件: %w", err)
			}
		}

		var msgContent string
		switch {
		case filePath != "":
			// Upload file via IM API, then send as file message
			fmt.Fprintf(os.Stderr, "正在上传文件: %s\n", filepath.Base(filePath))
			fileKey, err := client.UploadIMFile(filePath, "")
			if err != nil {
				return err
			}
			msgType = "file"
			contentJSON, _ := json.Marshal(map[string]string{"file_key": fileKey})
			msgContent = string(contentJSON)

		case imagePath != "":
			// Upload image via IM API, then send as image message
			fmt.Fprintf(os.Stderr, "正在上传图片: %s\n", filepath.Base(imagePath))
			imageKey, err := client.UploadIMImage(imagePath, "")
			if err != nil {
				return err
			}
			msgType = "image"
			contentJSON, _ := json.Marshal(map[string]string{"image_key": imageKey})
			msgContent = string(contentJSON)

		case contentFile != "":
			data, err := os.ReadFile(contentFile)
			if err != nil {
				return fmt.Errorf("读取内容文件失败: %w", err)
			}
			msgContent = string(data)

		case content != "":
			msgContent = content

		case text != "":
			msgType = "text"
			// 在 marshal 前规范化 @ 标签（修复 <at id=...> / <at open_id=...> 等 AI 易错格式），
			// 让 json.Marshal 自动转义产生的双引号，避免破坏 JSON 结构。
			text = client.NormalizeAtMentions(text)
			msgContent = client.CreateTextMessageContent(text)

		default:
			return fmt.Errorf("必须指定 --content、--content-file、--text、--file 或 --image")
		}

		// 自动上传本地图片（仅对 post 和 interactive 消息有效）
		if uploadImages && (msgType == "post" || msgType == "interactive") {
			// 确定 basePath：如果使用 --content-file，以其目录为 basePath；否则用当前目录
			basePath := "."
			if contentFile != "" {
				basePath = filepath.Dir(contentFile)
				if basePath == "" || basePath == "." {
					basePath = "."
				}
			}
			processedContent, uploadCount, err := processAndUploadLocalImages(msgContent, basePath)
			if err != nil {
				return err
			}
			if uploadCount > 0 {
				fmt.Fprintf(os.Stderr, "已自动上传 %d 张本地图片\n", uploadCount)
			}
			msgContent = processedContent
		}

		messageID, err := client.SendMessage(receiveIDType, receiveID, msgType, msgContent, token, idempotencyKey)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		if output == "json" {
			if err := printJSON(map[string]string{
				"message_id": messageID,
			}); err != nil {
				return err
			}
		} else {
			fmt.Printf("消息发送成功！\n")
			fmt.Printf("  消息 ID: %s\n", messageID)
		}

		return nil
	},
}

func validateSendReceiveIDType(receiveIDType string) error {
	switch receiveIDType {
	case "email", "open_id", "user_id", "union_id", "chat_id", "thread_id":
		return nil
	default:
		return fmt.Errorf("无效的 --receive-id-type: %s，有效值: email/open_id/user_id/union_id/chat_id/thread_id", receiveIDType)
	}
}

func validateSendMessageType(msgType string) error {
	switch msgType {
	case "text", "post", "image", "file", "audio", "media", "sticker", "interactive", "share_chat", "share_user":
		return nil
	default:
		return fmt.Errorf("无效的 --msg-type: %s，有效值: text/post/image/file/audio/media/sticker/interactive/share_chat/share_user", msgType)
	}
}

// validateIdempotencyKey 校验幂等键长度：飞书发消息 API 的 uuid 字段上限 50 **字符**
// （官方数据校验规则按字符计，非字节），用 rune 数校验以免中文键被误拒。
func validateIdempotencyKey(key string) error {
	if key == "" {
		return nil
	}
	if n := utf8.RuneCountInString(key); n > 50 {
		return fmt.Errorf("--idempotency-key 过长：%d 字符，上限 50 字符", n)
	}
	return nil
}

func init() {
	msgCmd.AddCommand(sendMessageCmd)
	sendMessageCmd.Flags().String("receive-id-type", "", "接收者类型（email/open_id/user_id/union_id/chat_id/thread_id）")
	sendMessageCmd.Flags().String("receive-id", "", "接收者标识")
	sendMessageCmd.Flags().String("thread-id", "", "话题 ID（omt_xxx），在已有话题内追加消息；与 --receive-id-type/--receive-id 互斥")
	sendMessageCmd.Flags().String("msg-type", "text", "消息类型（text/post/image/interactive 等）")
	sendMessageCmd.Flags().StringP("content", "c", "", "消息内容 JSON")
	sendMessageCmd.Flags().String("content-file", "", "消息内容 JSON 文件")
	sendMessageCmd.Flags().StringP("text", "t", "", "简单文本消息")
	sendMessageCmd.Flags().StringP("file", "f", "", "发送本地文件（自动上传并发送）")
	sendMessageCmd.Flags().String("image", "", "发送本地图片（自动上传并发送）")
	sendMessageCmd.Flags().Bool("upload-images", false, "自动上传 Markdown、post image_key 与 Card V2 img_key 中的本地图片")
	sendMessageCmd.Flags().String("idempotency-key", "", "幂等键（≤50 字符），服务端按此键去重；相同键重复发送返回首次消息，防止重发")
	sendMessageCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	sendMessageCmd.Flags().String("user-access-token", "", "User Access Token（用户授权令牌）")
}

// markdown 图片正则: ![alt](path)
var markdownImageRegex = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)

var uploadIMImage = client.UploadIMImage

var supportedIMImageExtensions = map[string]struct{}{
	".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {},
	".bmp": {}, ".webp": {}, ".tif": {}, ".tiff": {},
}

// isLocalPath 检测字符串是否为本地文件路径
func isLocalPath(s string) bool {
	if s == "" {
		return false
	}
	// 如果是 URL 或已上传的 image_key（img_ 开头），不是本地路径
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "img_") || strings.HasPrefix(s, "file_") {
		return false
	}
	// 只接受显式绝对/相对路径，或受支持的图片扩展名。
	// 不把任意包含 / 的字符串当成本地路径，避免误上传业务 key。
	if _, ok := supportedIMImageExtensions[strings.ToLower(filepath.Ext(s))]; ok {
		return true
	}
	if filepath.IsAbs(s) ||
		strings.HasPrefix(s, "./") ||
		strings.HasPrefix(s, "../") ||
		strings.HasPrefix(s, "~/") ||
		(len(s) >= 3 && s[1] == ':' && (s[2] == '\\' || s[2] == '/')) ||
		strings.HasPrefix(s, `\\`) {
		return true
	}
	return false
}

// resolveLocalPath 解析本地图片路径；~/ 相对于当前用户主目录，其余相对路径相对于 basePath。
func resolveLocalPath(path, basePath string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("解析用户主目录失败: %w", err)
		}
		return filepath.Join(homeDir, strings.TrimPrefix(path, "~/")), nil
	}
	return filepath.Join(basePath, path), nil
}

func validateLocalIMImage(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("不是普通文件")
	}
	ext := strings.ToLower(filepath.Ext(path))
	if _, ok := supportedIMImageExtensions[ext]; !ok {
		return fmt.Errorf("不支持的图片扩展名 %q", ext)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	header := make([]byte, 512)
	readSize, err := file.Read(header)
	if err != nil {
		return err
	}
	header = header[:readSize]
	contentType := http.DetectContentType(header)
	if len(header) >= 4 &&
		((header[0] == 'I' && header[1] == 'I' && header[2] == 0x2a && header[3] == 0x00) ||
			(header[0] == 'M' && header[1] == 'M' && header[2] == 0x00 && header[3] == 0x2a)) {
		contentType = "image/tiff"
	}
	allowed := map[string]struct{}{
		"image/png": {}, "image/jpeg": {}, "image/gif": {},
		"image/bmp": {}, "image/webp": {}, "image/tiff": {},
	}
	if _, ok := allowed[contentType]; !ok {
		return fmt.Errorf("文件内容不是支持的图片格式（检测为 %s）", contentType)
	}
	return nil
}

// uploadLocalImageForIM 解析路径、检查存在性并上传图片到 IM API
// 返回 image_key；文件不存在或上传失败时返回错误，阻止继续发送未解析的本地路径。
func uploadLocalImageForIM(imagePath, basePath string) (string, error) {
	resolvedPath, err := resolveLocalPath(imagePath, basePath)
	if err != nil {
		return "", fmt.Errorf("解析本地图片路径 %s 失败: %w", imagePath, err)
	}

	if err := validateLocalIMImage(resolvedPath); err != nil {
		return "", fmt.Errorf("本地图片不可用 %s: %w", resolvedPath, err)
	}

	fmt.Fprintf(os.Stderr, "正在上传图片: %s\n", resolvedPath)
	imageKey, err := uploadIMImage(resolvedPath, "")
	if err != nil {
		return "", fmt.Errorf("上传图片 %s 失败: %w", resolvedPath, err)
	}
	return imageKey, nil
}

// processAndUploadLocalImages 解析消息内容中的本地图片路径，上传并替换为 image_key/img_key
// basePath 用于解析相对路径：如果使用 --content-file，以其目录为 basePath；否则用当前目录
// 返回处理后的内容和上传的图片数量
func processAndUploadLocalImages(content string, basePath string) (string, int, error) {
	uploadCount := 0

	// 1. 先处理 Markdown 语法中的本地图片: ![alt](local/path.png)
	// 在 JSON 处理之前立即替换，避免 JSON 重新序列化导致字符串不匹配
	matches := markdownImageRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		fullMatch := match[0] // 完整匹配如 ![alt](local/path.png)
		imagePath := match[2] // 图片路径

		if !isLocalPath(imagePath) {
			continue
		}

		imageKey, err := uploadLocalImageForIM(imagePath, basePath)
		if err != nil {
			return "", uploadCount, err
		}

		// 立即替换，避免后续 JSON 序列化改变内容导致 ReplaceAll 失效
		content = strings.ReplaceAll(content, fullMatch, fmt.Sprintf("![%s](%s)", match[1], imageKey))
		uploadCount++
	}

	// 2. 尝试解析为 JSON，处理富文本 image_key 和 Card JSON 2.0 img_key
	var jsonData interface{}
	if err := json.Unmarshal([]byte(content), &jsonData); err == nil {
		changed, newData, count, err := processJSONLocalImages(jsonData, basePath)
		if err != nil {
			return "", uploadCount, err
		}
		if changed {
			uploadCount += count
			processed, err := json.Marshal(newData)
			if err != nil {
				return "", 0, fmt.Errorf("序列化处理后的内容失败: %w", err)
			}
			content = string(processed)
		}
	}

	return content, uploadCount, nil
}

// processJSONLocalImages 递归处理 JSON 结构中的本地图片
// basePath 用于解析相对路径
// 返回是否有修改、处理后的数据、上传的图片数量
func processJSONLocalImages(data interface{}, basePath string) (bool, interface{}, int, error) {
	switch v := data.(type) {
	case map[string]interface{}:
		changed := false
		count := 0
		newMap := make(map[string]interface{}, len(v))
		tag, _ := v["tag"].(string)
		for key, val := range v {
			// post 富文本使用 image_key；Card JSON 2.0 的 img 与 img_combination 使用 img_key。
			if tag == "img" && (key == "image_key" || key == "img_key") {
				if localPath, ok := val.(string); ok && isLocalPath(localPath) {
					imageKey, err := uploadLocalImageForIM(localPath, basePath)
					if err != nil {
						return false, v, 0, err
					}
					newMap[key] = imageKey
					changed = true
					count++
					continue
				}
			}
			if tag == "img_combination" && key == "img_list" {
				c, newVal, n, err := processImageListLocalImages(val, basePath)
				if err != nil {
					return false, v, 0, err
				}
				if c {
					newMap[key] = newVal
					changed = true
					count += n
					continue
				}
			}
			c, newVal, n, err := processJSONLocalImages(val, basePath)
			if err != nil {
				return false, v, 0, err
			}
			if c {
				changed = true
				count += n
			}
			newMap[key] = newVal
		}
		if !changed {
			return false, v, 0, nil
		}
		return true, newMap, count, nil

	case []interface{}:
		changed := false
		count := 0
		newArr := make([]interface{}, len(v))
		for i, item := range v {
			c, newItem, n, err := processJSONLocalImages(item, basePath)
			if err != nil {
				return false, v, 0, err
			}
			if c {
				changed = true
				count += n
			}
			newArr[i] = newItem
		}
		if !changed {
			return false, v, 0, nil
		}
		return true, newArr, count, nil

	default:
		return false, v, 0, nil
	}
}

// processImageListLocalImages 处理 img_combination.img_list 中没有 tag 的图片项。
func processImageListLocalImages(data interface{}, basePath string) (bool, interface{}, int, error) {
	items, ok := data.([]interface{})
	if !ok {
		return false, data, 0, nil
	}

	changed := false
	count := 0
	result := make([]interface{}, len(items))
	for index, item := range items {
		image, ok := item.(map[string]interface{})
		if !ok {
			result[index] = item
			continue
		}
		localPath, ok := image["img_key"].(string)
		if !ok || !isLocalPath(localPath) {
			result[index] = item
			continue
		}
		imageKey, err := uploadLocalImageForIM(localPath, basePath)
		if err != nil {
			return false, data, 0, err
		}
		copied := make(map[string]interface{}, len(image))
		for key, value := range image {
			copied[key] = value
		}
		copied["img_key"] = imageKey
		result[index] = copied
		changed = true
		count++
	}
	if !changed {
		return false, data, 0, nil
	}
	return true, result, count, nil
}
