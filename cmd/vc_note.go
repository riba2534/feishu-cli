package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// vc note —— 智能会议纪要（smart notes）的独立入口。
// 与 vc notes（按会议/妙记批量查产物）互补：note 子命令组按 note_id 直接操作单篇纪要。
var vcNoteCmd = &cobra.Command{
	Use:   "note",
	Short: "智能会议纪要（按 note_id 查详情 / 导出统一逐字稿）",
	Long: `智能会议纪要（smart notes）操作。note_id 可从 vc notes / vc detail 结果获取。

子命令:
  detail       查询纪要详情（展示类型、关联文档 token 等）
  transcript   导出统一逐字稿（unified transcript，自动翻页拉全量）

权限要求（User Token）: vc:note:read`,
}

var vcNoteDetailCmd = &cobra.Command{
	Use:   "detail <note_id>",
	Short: "查询智能纪要详情",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "vc note detail")
		if err != nil {
			return err
		}
		raw, err := client.GetMeetingNote(args[0], token)
		if err != nil {
			return err
		}
		var result map[string]any
		if err := json.Unmarshal(raw, &result); err != nil {
			return fmt.Errorf("解析纪要详情失败: %w", err)
		}
		return printJSON(result)
	},
}

var vcNoteTranscriptCmd = &cobra.Command{
	Use:   "transcript <note_id>",
	Short: "导出智能纪要的统一逐字稿（自动翻页拉全量）",
	Long: `导出智能纪要的统一逐字稿（unified transcript）。

参数:
  <note_id>    纪要 ID
  --output     保存文件路径（缺省打印到 stdout）
  --format     逐字稿格式: markdown（默认）/ text

权限要求（User Token）: vc:note:read`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "vc note transcript")
		if err != nil {
			return err
		}
		format, _ := cmd.Flags().GetString("format")
		if err := validateEnum(format, "--format", []string{"markdown", "text"}); err != nil {
			return err
		}
		output, _ := cmd.Flags().GetString("output")

		noteID := args[0]
		var content strings.Builder
		cursor := ""
		// 自动翻页：cursor_id 迭代直到服务端不再返回新游标（防御性上限 200 页）
		for page := 0; page < 200; page++ {
			params := map[string]string{"transcript_format": format}
			if cursor != "" {
				params["cursor_id"] = cursor
			}
			result, err := client.GetUnifiedNoteTranscript(noteID, params, token)
			if err != nil {
				return err
			}
			chunk, next, err := parseUnifiedTranscriptPage(result)
			if err != nil {
				return fmt.Errorf("纪要 %s 第 %d 页: %w", noteID, page+1, err)
			}
			content.WriteString(chunk)
			if next == "" || next == cursor {
				break
			}
			cursor = next
		}
		if content.Len() == 0 {
			return fmt.Errorf("纪要 %s 未返回逐字稿内容（可能仍在生成中，稍后重试）", noteID)
		}
		if output != "" {
			if err := os.WriteFile(output, []byte(content.String()), 0600); err != nil {
				return fmt.Errorf("写入文件失败: %w", err)
			}
			fmt.Printf("逐字稿已保存到 %s（%d 字节）\n", output, content.Len())
			return nil
		}
		fmt.Print(content.String())
		return nil
	},
}

// parseUnifiedTranscriptPage 从 unified_note_transcript 响应中提取本页文本与下一页游标。
// 响应结构服务端未完全文档化：识别不出已知文本字段时**显式报错**（列出实际字段名，
// 引导用 api 透传拿原始响应排查），绝不把响应 envelope 伪装成逐字稿内容输出——
// 那会产出垃圾文件且静默截断翻页，比失败更难发现。
func parseUnifiedTranscriptPage(data map[string]any) (content, nextCursor string, err error) {
	if data == nil {
		return "", "", fmt.Errorf("响应 data 为空")
	}
	for _, key := range []string{"transcript", "content", "text"} {
		if s, ok := data[key].(string); ok && s != "" {
			content = s
			break
		}
	}
	if content == "" {
		keys := make([]string, 0, len(data))
		for k := range data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return "", "", fmt.Errorf(
			"无法从响应中识别逐字稿字段（实际字段: %s）。响应结构可能已变更，可用 `feishu-cli api GET /open-apis/vc/v1/notes/<note_id>/unified_note_transcript` 查看原始响应并反馈 issue",
			strings.Join(keys, ", "))
	}
	for _, key := range []string{"cursor_id", "next_cursor_id", "page_token"} {
		if s, ok := data[key].(string); ok && s != "" {
			nextCursor = s
			break
		}
	}
	if hasMore, ok := data["has_more"].(bool); ok && !hasMore {
		nextCursor = ""
	}
	return content, nextCursor, nil
}

func init() {
	vcCmd.AddCommand(vcNoteCmd)
	vcNoteCmd.AddCommand(vcNoteDetailCmd, vcNoteTranscriptCmd)
	vcNoteDetailCmd.Flags().String("user-access-token", "", "User Access Token")
	vcNoteTranscriptCmd.Flags().String("user-access-token", "", "User Access Token")
	vcNoteTranscriptCmd.Flags().String("output", "", "保存文件路径（缺省打印到 stdout）")
	vcNoteTranscriptCmd.Flags().String("format", "markdown", "逐字稿格式: markdown / text")
}
