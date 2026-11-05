package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// msgReadStatusMaxIDs 单次 read_status 请求的消息数上限（与官方一致）。
const msgReadStatusMaxIDs = 50

var msgReadStatusCmd = &cobra.Command{
	Use:   "read-status [om_xxx ...]",
	Short: "查询当前用户对消息的已读状态（批量）",
	Long: `批量查询当前登录用户是否已读指定消息（POST /open-apis/im/v1/messages/read_status）。

与 msg read-users 的区别：read-users 查「谁读了我发的消息」；read-status 查「我是否读了这些消息」。

参数:
  om_xxx ...        消息 ID（可多个），也可用 --message-ids 逗号分隔；合计 1–50 个，须为 om_ 前缀
  --output, -o      输出格式（json）

身份: 仅 User（需先 feishu-cli auth login）
权限: im:message:readonly

输出（-o json）: 透传服务端 data，含 items[]（message_id / is_read 等）与 invalid_message_ids[]

示例:
  feishu-cli msg read-status om_xxx om_yyy
  feishu-cli msg read-status --message-ids om_xxx,om_yyy -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		idsFlag, _ := cmd.Flags().GetString("message-ids")
		output, _ := cmd.Flags().GetString("output")
		messageIDs, err := parseReadStatusMessageIDs(args, idsFlag)
		if err != nil {
			return err
		}

		if err := config.Validate(); err != nil {
			return err
		}
		token, err := requireUserToken(cmd, "msg read-status")
		if err != nil {
			return err
		}

		data, err := client.GetMessagesReadStatus(messageIDs, token)
		if err != nil {
			return err
		}
		if output == "json" {
			var v any
			if err := json.Unmarshal(data, &v); err != nil || v == nil {
				v = map[string]any{}
			}
			return printJSON(v)
		}

		var parsed struct {
			Items []struct {
				MessageID string `json:"message_id"`
				IsRead    bool   `json:"is_read"`
			} `json:"items"`
			InvalidMessageIDs []string `json:"invalid_message_ids"`
		}
		_ = json.Unmarshal(data, &parsed)
		for _, it := range parsed.Items {
			state := "未读"
			if it.IsRead {
				state = "已读"
			}
			fmt.Printf("%s  %s\n", it.MessageID, state)
		}
		if len(parsed.InvalidMessageIDs) > 0 {
			fmt.Printf("无效的消息 ID: %s\n", strings.Join(parsed.InvalidMessageIDs, ", "))
		}
		return nil
	},
}

// parseReadStatusMessageIDs 合并位置参数与 --message-ids，去重保序，并本地校验数量与 om_ 前缀（exit 2）。
func parseReadStatusMessageIDs(args []string, idsFlag string) ([]string, error) {
	seen := map[string]bool{}
	var ids []string
	add := func(raw string) {
		for _, id := range strings.Split(raw, ",") {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, a := range args {
		add(a)
	}
	add(idsFlag)
	if len(ids) == 0 {
		return nil, clierr.Usagef("请提供至少一个消息 ID（位置参数或 --message-ids）")
	}
	if len(ids) > msgReadStatusMaxIDs {
		return nil, clierr.Usagef("单次最多查询 %d 个消息 ID，当前 %d 个", msgReadStatusMaxIDs, len(ids))
	}
	for _, id := range ids {
		if !strings.HasPrefix(id, "om_") {
			return nil, clierr.Usagef("无效的消息 ID %q：read-status 需要 om_ 开头的消息 ID", id)
		}
	}
	return ids, nil
}

func init() {
	msgCmd.AddCommand(msgReadStatusCmd)
	msgReadStatusCmd.Flags().String("message-ids", "", "消息 ID 列表（逗号分隔，可与位置参数合用）")
	msgReadStatusCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	msgReadStatusCmd.Flags().String("user-access-token", "", "User Access Token（覆盖登录态）")
}
