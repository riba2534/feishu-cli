package cmd

import (
	"strconv"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
)

// mailMinScheduleLead 定时发送至少提前的时长（与官方 validateSendTime 一致）。
const mailMinScheduleLead = 5 * time.Minute

// mailNow 便于测试注入当前时间。
var mailNow = time.Now

const mailSendTimeFlagHelp = "定时发送时间（Unix 秒时间戳，至少晚于当前 5 分钟；须配合 --confirm-send）"
const mailRequestReceiptFlagHelp = "请求已读回执（写入 Disposition-Notification-To: 发件人；对方客户端可能提示、自动回执或忽略）"

func addMailSendTimeFlag(cmd *cobra.Command) {
	cmd.Flags().String("send-time", "", mailSendTimeFlagHelp)
}

func addMailRequestReceiptFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("request-receipt", false, mailRequestReceiptFlagHelp)
}

// validateMailSendTime 本地校验 --send-time（不联网）：未传返回空串；
// 传了必须配合 --confirm-send、为 Unix 秒、且至少晚于当前 5 分钟，否则 exit 2。
func validateMailSendTime(cmd *cobra.Command) (string, error) {
	raw, _ := cmd.Flags().GetString("send-time")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if confirm, _ := cmd.Flags().GetBool("confirm-send"); !confirm {
		return "", clierr.Usagef("--send-time 需要同时指定 --confirm-send（定时发送也是发送操作）")
	}
	ts, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return "", clierr.Usagef("--send-time 必须是 Unix 秒时间戳，得到 %q", raw)
	}
	minTS := mailNow().Add(mailMinScheduleLead).Unix()
	if ts < minTS {
		return "", clierr.Usagef("--send-time 必须至少晚于当前 5 分钟（最早 %d，得到 %d）", minTS, ts)
	}
	return strconv.FormatInt(ts, 10), nil
}

// requireMailReceiptSender --request-receipt 需要可解析的发件人地址作为回执地址。
func requireMailReceiptSender(cmd *cobra.Command, from string) error {
	if want, _ := cmd.Flags().GetBool("request-receipt"); !want {
		return nil
	}
	if strings.TrimSpace(from) == "" {
		return clierr.Usagef("--request-receipt 需要可解析的发件人地址：请用 --from 指定，或确认邮箱 profile 可读取")
	}
	return nil
}

// mailReceiptAddress 返回应写入 Disposition-Notification-To 的地址（未开启时为空）。
func mailReceiptAddress(cmd *cobra.Command, from string) string {
	if want, _ := cmd.Flags().GetBool("request-receipt"); !want {
		return ""
	}
	return strings.TrimSpace(from)
}
