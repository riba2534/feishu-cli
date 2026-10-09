package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
)

// post 附件区（对齐官方 --attachment / +messages-edit --set-attachments/--clear-attachments）：
// post 消息体顶层 "files": [{"key": "file_xxx"}] 会渲染为消息底部的附件区。
// 服务端以文件服务元信息回填 name/size，客户端只传 key。

// validateAttachments 校验 --attachment 与其他内容参数的组合。
func (input messageContentInput) validateAttachments() error {
	if len(input.attachments) == 0 {
		return nil
	}
	for _, v := range input.attachments {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("--attachment 的取值不能为空")
		}
	}
	switch {
	case input.text != "":
		return fmt.Errorf("--attachment 把文件放进 post 的附件区，不能与 --text 同时使用；正文请改用 --markdown（或 --msg-type post + --content）")
	case input.file != "" || input.image != "" || input.audio != "" || input.video != "":
		return fmt.Errorf("--attachment 不能与 --file/--image/--audio/--video 同时使用")
	}
	if (input.content != "" || input.contentFile != "") && input.msgType != "post" {
		return fmt.Errorf("--attachment 只适用于 post 消息：配合 --content/--content-file 时需指定 --msg-type post")
	}
	if input.msgTypeChanged && input.markdown == "" && input.msgType != "post" {
		return fmt.Errorf("--attachment 会发送 post 消息，与 --msg-type %q 冲突", input.msgType)
	}
	if input.content != "" && postContentHasFiles(input.content) {
		return fmt.Errorf("--content 已包含 files 附件区，不能再与 --attachment 同时使用")
	}
	for _, v := range input.attachments {
		v = strings.TrimSpace(v)
		if isIMFileKey(v) {
			continue
		}
		if err := validateFileInput("--attachment", v, "附件"); err != nil {
			return err
		}
	}
	return nil
}

// resolveAttachmentKeys 把 --attachment 的取值解析为 file_key：已是 file_xxx 原样使用，
// 本地路径按普通附件上传（mp4/opus 同 --file，按 stream 上传避免 230055）。
func (input *messageContentInput) resolveAttachmentKeys() ([]string, error) {
	keys := make([]string, 0, len(input.attachments))
	for _, v := range input.attachments {
		v = strings.TrimSpace(v)
		key, err := input.resolveFileKey("--attachment", v, fileUploadTypeForAttachment(v))
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func decodePostContentObject(content string) (map[string]any, error) {
	parsed := map[string]any{}
	if strings.TrimSpace(content) == "" {
		return parsed, nil
	}
	dec := json.NewDecoder(strings.NewReader(content))
	dec.UseNumber()
	if err := dec.Decode(&parsed); err != nil {
		return nil, err
	}
	if parsed == nil { // JSON null
		parsed = map[string]any{}
	}
	return parsed, nil
}

func postContentHasFiles(content string) bool {
	parsed, err := decodePostContentObject(content)
	if err != nil {
		return false
	}
	files, ok := parsed["files"].([]any)
	return ok && len(files) > 0
}

// mergeAttachmentsIntoPostContent 追加附件到 post 顶层 files，按 key 去重保序。
func mergeAttachmentsIntoPostContent(content string, keys []string) (string, error) {
	parsed, err := decodePostContentObject(content)
	if err != nil {
		return "", err
	}
	var files []any
	seen := map[string]bool{}
	if existing, ok := parsed["files"].([]any); ok {
		for _, e := range existing {
			if m, ok := e.(map[string]any); ok {
				if k, _ := m["key"].(string); k != "" {
					if seen[k] {
						continue
					}
					seen[k] = true
				}
			}
			files = append(files, e)
		}
	}
	for _, k := range keys {
		if seen[k] {
			continue
		}
		seen[k] = true
		files = append(files, map[string]any{"key": k})
	}
	parsed["files"] = files
	return marshalCompactJSON(parsed)
}

// setPostAttachments 用给定 keys 覆盖 post 的附件区；keys 为空表示清空（files: []）。
func setPostAttachments(content string, keys []string) (string, error) {
	parsed, err := decodePostContentObject(content)
	if err != nil {
		return "", err
	}
	files := make([]any, 0, len(keys))
	seen := map[string]bool{}
	for _, k := range keys {
		if seen[k] {
			continue
		}
		seen[k] = true
		files = append(files, map[string]any{"key": k})
	}
	parsed["files"] = files
	return marshalCompactJSON(parsed)
}

func marshalCompactJSON(v any) (string, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimRight(b.String(), "\n"), nil
}
