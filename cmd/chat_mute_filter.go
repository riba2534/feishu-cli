package cmd

import (
	"fmt"
	"io"

	"github.com/riba2534/feishu-cli/v2/internal/client"
)

// batchGetMuteStatus 可在测试中替换。
var batchGetMuteStatus = client.BatchGetMuteStatus

// fetchMutedChatSet 为 --exclude-muted 查询免打扰会话集合（对齐官方 mute_filter）。
// 免打扰是个人设置：Bot 身份没有该数据，直接跳过并提示；查询失败时告警并不过滤，
// 返回 nil 表示"不做过滤"。
func fetchMutedChatSet(errOut io.Writer, chatIDs []string, userToken string) map[string]bool {
	if userToken == "" {
		fmt.Fprintln(errOut, "[提示] --exclude-muted 仅对用户身份生效（免打扰是个人设置，Bot 没有该数据），本次返回全部结果；如需过滤请用 --as user")
		return nil
	}
	if len(chatIDs) == 0 {
		return map[string]bool{}
	}
	muted, _, err := batchGetMuteStatus(chatIDs, userToken)
	if err != nil {
		fmt.Fprintf(errOut, "[提示] 查询免打扰状态失败，本次不过滤: %v\n", err)
		return nil
	}
	set := map[string]bool{}
	for id, isMuted := range muted {
		if isMuted {
			set[id] = true
		}
	}
	return set
}

func printMuteFilterResult(errOut io.Writer, filtered, remaining int, hasMore bool) {
	if filtered == 0 {
		return
	}
	tail := "已无更多页"
	if hasMore {
		tail = "还有更多页，可带 --page-token 继续"
	}
	fmt.Fprintf(errOut, "[提示] --exclude-muted 已过滤 %d 个免打扰会话（剩余 %d 个，%s）\n", filtered, remaining, tail)
}
