package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var mailSignatureCmd = &cobra.Command{
	Use:   "signature",
	Short: "列出/查看邮箱签名（含默认使用信息）",
	Long: `列出或查看邮箱签名。

使用飞书 GET /open-apis/mail/v1/user_mailboxes/{mailbox_id}/settings/signatures API
读取邮箱设置里的签名列表；--detail 指定某个签名 ID 时，从列表里筛出该签名的渲染详情。

可选:
  --from         邮箱地址（默认 me）
  --detail       签名 ID（指定后只输出该签名详情；缺省则列出全部）
  --dry-run      只打印将要发送的请求，不实际调用
  --output, -o   输出格式（json）

权限:
  - User Access Token
  - mail:user_mailbox.settings:read

示例:
  feishu-cli mail signature
  feishu-cli mail signature --from me -o json
  feishu-cli mail signature --detail 7012345678901234567`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		from, _ := cmd.Flags().GetString("from")
		detail, _ := cmd.Flags().GetString("detail")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")

		mailboxID := strings.TrimSpace(from)
		if mailboxID == "" {
			mailboxID = "me"
		}

		if dryRun {
			return printJSON(map[string]any{
				"method":     "GET",
				"path":       fmt.Sprintf("/open-apis/mail/v1/user_mailboxes/%s/settings/signatures", mailboxID),
				"detail_id":  detail,
				"mailbox_id": mailboxID,
			})
		}

		token, err := requireUserToken(cmd, "mail signature")
		if err != nil {
			return err
		}

		data, err := client.ListMailSignatures(mailboxID, token)
		if err != nil {
			return err
		}

		var parsed struct {
			Signatures []mailSignatureItem `json:"signatures"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			// 解析失败时直接透传原始 data
			if output == "json" {
				return printJSON(json.RawMessage(data))
			}
			fmt.Println(string(data))
			return nil
		}

		// --detail 筛出单个签名
		if strings.TrimSpace(detail) != "" {
			match := findMailSignature(parsed.Signatures, detail)
			if match == nil {
				return fmt.Errorf("未找到签名 ID %q（用 `feishu-cli mail signature` 列出全部）", detail)
			}
			if output == "json" {
				return printJSON(match)
			}
			printMailSignatureDetail(match)
			return nil
		}

		if output == "json" {
			return printJSON(parsed.Signatures)
		}

		if len(parsed.Signatures) == 0 {
			fmt.Println("未找到任何邮箱签名。")
			return nil
		}
		fmt.Printf("邮箱签名（共 %d 条）:\n\n", len(parsed.Signatures))
		for i, s := range parsed.Signatures {
			tag := ""
			if s.IsDefault {
				tag = "  [默认]"
			}
			fmt.Printf("[%d] %s%s\n", i+1, displayMailSignatureName(s), tag)
			fmt.Printf("    签名 ID: %s\n", s.signatureID())
			if rec := strings.TrimSpace(s.RecommendedUsage); rec != "" {
				fmt.Printf("    推荐用途: %s\n", rec)
			}
			fmt.Println()
		}
		return nil
	},
}

// mailSignatureItem 邮箱签名条目（兼容飞书可能的字段名变体）。
type mailSignatureItem struct {
	ID               string `json:"id"`
	SignatureID      string `json:"signature_id"`
	Name             string `json:"name"`
	Content          string `json:"content"`
	IsDefault        bool   `json:"is_default"`
	IsReplyDefault   bool   `json:"is_reply_default"`
	RecommendedUsage string `json:"recommended_usage"`
}

// signatureID 返回该签名的有效 ID（兼容 id / signature_id 两种字段）。
func (s mailSignatureItem) signatureID() string {
	if s.ID != "" {
		return s.ID
	}
	return s.SignatureID
}

// findMailSignature 按 ID 在签名列表里查找（同时匹配 id 与 signature_id）。
func findMailSignature(items []mailSignatureItem, id string) *mailSignatureItem {
	for i := range items {
		if items[i].ID == id || items[i].SignatureID == id {
			return &items[i]
		}
	}
	return nil
}

// displayMailSignatureName 渲染签名展示名（无名字时回退为 ID）。
func displayMailSignatureName(s mailSignatureItem) string {
	if n := strings.TrimSpace(s.Name); n != "" {
		return n
	}
	if id := s.signatureID(); id != "" {
		return "(signature " + id + ")"
	}
	return "(未命名)"
}

func printMailSignatureDetail(s *mailSignatureItem) {
	fmt.Printf("签名名称: %s\n", displayMailSignatureName(*s))
	fmt.Printf("签名 ID:  %s\n", s.signatureID())
	fmt.Printf("默认签名: %t\n", s.IsDefault)
	fmt.Printf("回复默认: %t\n", s.IsReplyDefault)
	if rec := strings.TrimSpace(s.RecommendedUsage); rec != "" {
		fmt.Printf("推荐用途: %s\n", rec)
	}
	if c := strings.TrimSpace(s.Content); c != "" {
		fmt.Printf("内容:\n%s\n", c)
	}
}

func init() {
	mailCmd.AddCommand(mailSignatureCmd)
	mailSignatureCmd.Flags().String("from", "me", "邮箱地址（默认 me）")
	mailSignatureCmd.Flags().String("detail", "", "签名 ID（指定后只输出该签名详情）")
	mailSignatureCmd.Flags().Bool("dry-run", false, "只打印请求，不实际调用")
	mailSignatureCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	mailSignatureCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
}
