package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// cardContentTypeFlagDesc 是 msg get/mget/list 三个查询命令共用的 flag 帮助文本。
//
// 飞书 OAPI 对 interactive 卡片消息默认返回**渲染后**的文本（形如
// `<card title="...">...</card>`）。要拿到原始 schema JSON，必须传
// card_msg_content_type 参数。这里支持简写（user/raw）和完整 OAPI 值，便于在 CLI
// 里少敲几个字符。
const cardContentTypeFlagDesc = "卡片消息返回格式：user / raw（默认空，返回渲染版）。" +
	"user → user_card_content（开发者构建时的 schema 2.0 JSON，便于偷师/调试）；" +
	"raw → raw_card_content（平台内部完整 cardDSL）"

// addCardContentTypeFlag 把 --card-content-type flag 注册到目标命令上。
func addCardContentTypeFlag(c *cobra.Command) {
	c.Flags().String("card-content-type", "", cardContentTypeFlagDesc)
}

// resolveCardContentType 把 flag 值（user/raw 或完整 OAPI 名）规范化为 OAPI 接受的字符串。
// 空字符串保持空，让 client 层走原有渲染版返回路径，向后兼容。
func resolveCardContentType(cmd *cobra.Command) (string, error) {
	v, _ := cmd.Flags().GetString("card-content-type")
	v = strings.TrimSpace(v)
	switch strings.ToLower(v) {
	case "":
		return "", nil
	case "user", client.CardMsgContentTypeUser:
		return client.CardMsgContentTypeUser, nil
	case "raw", client.CardMsgContentTypeRaw:
		return client.CardMsgContentTypeRaw, nil
	default:
		return "", fmt.Errorf("无效的 --card-content-type 取值 %q（合法值：user / raw / user_card_content / raw_card_content）", v)
	}
}
