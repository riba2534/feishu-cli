package cmd

import (
	"errors"
	"net"
	"net/http"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
)

// driveBatchFailure 是 drive push/pull 单项失败的分类结果（对齐官方 drive_push.go 的批量失败分级）。
type driveBatchFailure struct {
	Class    string // 分类标签，写入 JSON items[].error_class
	Code     int    // 飞书业务码（无则 0）
	Terminal bool   // 是否终止整批：缺 scope、无权限、限流、参数错误等重跑同一批也不会成功
	Hint     string // 中文处理建议
}

// 本地文件在扫描后发生变化（push 上传前快照校验失败）。
var errDriveLocalFileChanged = errors.New("本地文件在扫描后发生变化")

// classifyDriveBatchFailure 按业务码/HTTP 状态对单项失败分级。
//
// push=true 时额外把冲突、配额、网络错误视为终止（与官方 driveClassifyPushFailure 一致）；
// pull 中网络错误只影响单个文件（下载流已在分片级重试过）。
// 1062507（父目录子节点超 1500）**不**终止：本项目按目录隔离，只跳过发往已满目录的条目。
func classifyDriveBatchFailure(err error, push bool) driveBatchFailure {
	d := driveBatchFailure{Class: "unknown"}
	if err == nil {
		return d
	}
	if errors.Is(err, errDriveLocalFileChanged) {
		d.Class = "local_file_changed"
		d.Hint = "本地文件在推送过程中被修改，请重新扫描后重跑该项"
		return d
	}
	status := 0
	if apiErr, ok := client.AsAPIError(err); ok {
		d.Code = apiErr.Code
		status = apiErr.HTTPStatus
	}
	has := func(codes ...int) bool {
		for _, c := range codes {
			if d.Code == c || client.HasAPICode(err, c) {
				d.Code = c
				return true
			}
		}
		return false
	}

	switch {
	case has(99991672):
		d.Class, d.Terminal = "app_scope_missing", true
		d.Hint = "应用未开通所需 scope：请应用管理员在开放平台开通并发布后重跑（重新 auth login 无法解决）"
	case has(99991679, 99991676):
		d.Class, d.Terminal = "user_scope_missing", true
		d.Hint = "User Token 未授权所需 scope：运行 `feishu-cli auth login --scope \"<所需 scope>\"` 补充授权后重跑，或改用 --as bot"
	case has(1061004, 1063002, 1069902) || status == http.StatusForbidden:
		d.Class, d.Terminal = "permission_denied", true
		d.Hint = "当前身份对目标文件夹/文件无权限：确认身份（--as user|bot）与协作者权限后重跑"
	case has(99991400) || status == http.StatusTooManyRequests || client.IsRateLimitError(err):
		d.Class, d.Terminal = "rate_limited", true
		d.Hint = "请求被限流：停止立即重试，稍后带退避重跑"
	case has(1061002, 99992402, 9499):
		d.Class, d.Terminal = "invalid_parameters", true
		d.Hint = "请求参数错误：检查 --folder-token、覆盖模式、文件名等参数，不要用同一组参数反复重试"
	case has(1062507):
		d.Class = "parent_sibling_limit"
		d.Hint = driveFolderChildLimitAdvice
	case has(1061045):
		d.Class, d.Terminal = "conflict", push
		d.Hint = "目标目录正被并发修改：停止本次推送，稍后带退避重跑，避免对同一目录并发 push"
	case has(1061043):
		d.Class = "file_size_limit"
		d.Hint = "文件超过云盘单文件大小上限：拆分文件或改用其他存储，原样重试不会成功"
	case has(1061061, 1061101):
		d.Class, d.Terminal = "quota_exceeded", push
		d.Hint = "云盘容量/文件数配额不足：清理空间或更换目标目录后重跑"
	case has(1061044):
		d.Class, d.Terminal = "parent_node_missing", true
		d.Hint = "目标父文件夹不存在或不可见：确认 --folder-token、文件夹权限，以及推送期间父目录是否被删除"
	case has(1061007):
		d.Class = "remote_not_found"
	case has(1062009):
		d.Class = "upload_size_mismatch"
		d.Hint = "上传字节数与声明大小不一致：本地文件可能在上传中被修改，请重新扫描后重跑"
	case has(1061001, 2200, 1663, 233523001) || status >= 500:
		d.Class, d.Terminal = "server_error", true
		d.Hint = "服务端错误（已自动重试仍失败）：停止本次批量操作，稍后重跑"
	case isNetworkFailure(err):
		d.Class, d.Terminal = "network", push
		d.Hint = "网络异常：检查网络后重跑"
	case clierr.HasKind(err, clierr.KindAuth):
		d.Class, d.Terminal = "auth", true
		d.Hint = "身份凭证不可用：运行 `feishu-cli auth status` 检查"
	}
	return d
}

func isNetworkFailure(err error) bool {
	if clierr.HasKind(err, clierr.KindNetwork) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}
