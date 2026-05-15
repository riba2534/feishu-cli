package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/spf13/cobra"
)

// cardContentTypeFlagDesc 是 msg get/mget/list/history 查询命令共用的 flag 帮助文本。
//
// 飞书 OAPI 不传 card_msg_content_type 时，部分 interactive 卡片只返回“请升级客户端”
// 的降级内容。CLI 默认请求 user_card_content，并额外提取 card_texts 方便阅读。
const cardContentTypeFlagDesc = "卡片消息返回格式：user / raw / rendered（默认 user）。" +
	"user → user_card_content（schema 2.0 JSON，CLI 会提取 card_texts）；" +
	"raw → raw_card_content（平台内部完整 cardDSL）；" +
	"rendered → 不传 card_msg_content_type，保留 OAPI 渲染版/降级版"

// addCardContentTypeFlag 把 --card-content-type flag 注册到目标命令上。
func addCardContentTypeFlag(c *cobra.Command) {
	c.Flags().String("card-content-type", "", cardContentTypeFlagDesc)
}

// resolveCardContentType 把 flag 值（user/raw/rendered 或完整 OAPI 名）规范化为 OAPI 接受的字符串。
// 未显式传 flag 时默认 user_card_content，避免 interactive 卡片只返回“请升级客户端”。
// 显式传 rendered/default/legacy 时保持空字符串，让 client 层走 OAPI 原有渲染版返回路径。
func resolveCardContentType(cmd *cobra.Command) (string, error) {
	v, _ := cmd.Flags().GetString("card-content-type")
	v = strings.TrimSpace(v)
	if !cmd.Flags().Changed("card-content-type") && v == "" {
		return client.CardMsgContentTypeUser, nil
	}
	switch strings.ToLower(v) {
	case "", "rendered", "default", "legacy":
		return "", nil
	case "user", client.CardMsgContentTypeUser:
		return client.CardMsgContentTypeUser, nil
	case "raw", client.CardMsgContentTypeRaw:
		return client.CardMsgContentTypeRaw, nil
	default:
		return "", fmt.Errorf("无效的 --card-content-type 取值 %q（合法值：user / raw / rendered / user_card_content / raw_card_content）", v)
	}
}
