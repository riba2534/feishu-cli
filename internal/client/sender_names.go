package client

import (
	"encoding/json"
	"sync"
)

// senderNameRegistry 进程内累积服务端回填的发送者显示名（sender.id → name）。
//
// 所有读消息的 raw 请求都带 with_sender_name=true，服务端会在每条消息的 sender
// 对象里回填 sender_name（含 Bot 和外部租户用户，无需通讯录权限）。larkim.Message
// 的类型化解析会丢弃该字段，因此在每个 raw body unmarshal 点旁路采集到这里。
// CLI 是一次性进程，注册表随进程消亡，不存在跨请求串数据问题。
var (
	senderNameMu       sync.Mutex
	senderNameRegistry = make(map[string]string)
)

// senderNameEnvelope 仅用于从 raw body 侧路提取 sender_name 的最小结构。
// 兼容 list（data.items）与 get（data.items 单元素）两类响应。
type senderNameEnvelope struct {
	Data struct {
		Items []struct {
			Sender struct {
				ID         string `json:"id"`
				SenderName string `json:"sender_name"`
			} `json:"sender"`
		} `json:"items"`
	} `json:"data"`
}

// harvestSenderNames 从 /im/v1/messages 系列接口的 raw body 中提取服务端回填的
// sender_name（需请求带 with_sender_name=true），累积到进程级注册表。
// 解析失败静默忽略——名字增强是 best-effort，不影响主链路。
func harvestSenderNames(body []byte) {
	var env senderNameEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return
	}
	senderNameMu.Lock()
	defer senderNameMu.Unlock()
	for _, item := range env.Data.Items {
		if item.Sender.ID != "" && item.Sender.SenderName != "" {
			senderNameRegistry[item.Sender.ID] = item.Sender.SenderName
		}
	}
}

// HarvestedSenderNames 返回当前进程已采集的发送者显示名快照（sender.id → name）。
func HarvestedSenderNames() map[string]string {
	senderNameMu.Lock()
	defer senderNameMu.Unlock()
	out := make(map[string]string, len(senderNameRegistry))
	for k, v := range senderNameRegistry {
		out[k] = v
	}
	return out
}
