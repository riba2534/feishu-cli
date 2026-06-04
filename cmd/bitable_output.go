package cmd

import (
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/output"
	"github.com/spf13/cobra"
)

// bitableReq 描述一次 bitable API 请求（供 dry-run 预览与实调共用）。
type bitableReq struct {
	method string         // GET/POST/PUT/PATCH/DELETE
	path   string         // 已用 BaseV3Path / BitableV1Path 构造的完整路径
	params map[string]any // query 参数
	body   any            // 请求体
	useV1  bool           // true → bitable/v1（BitableV1Call）；false → base/v3（BaseV3Call）
}

// renderBitableResult 渲染 bitable 返回的 data：
// 命令注册了 --format（本次新增命令）→ 走 output 包（支持 --jq/--format）；否则回退 printJSON。
func renderBitableResult(cmd *cobra.Command, data any) error {
	if cmd.Flags().Lookup("format") != nil {
		o, err := output.ParseOptions(cmd)
		if err != nil {
			return err
		}
		return output.Render(o, data)
	}
	return printJSON(data)
}

// bitableRun 封装新增 bitable 命令的统一执行流程：
// config 校验 → 解析必需 User Token → 解析 base-token → 构造请求描述符 →
// 若 --dry-run 仅预览描述符不发请求；否则按 useV1 路由调用并渲染。
func bitableRun(cmd *cobra.Command, build func(baseToken string) bitableReq) error {
	if err := config.Validate(); err != nil {
		return err
	}
	// base-token 用于构造路径（dry-run 预览也要展示），故先解析。
	baseToken, err := resolveBaseToken(cmd)
	if err != nil {
		return err
	}
	req := build(baseToken)

	// --dry-run：未注册该 flag 的（只读）命令 GetBool 返回 false，正常执行。
	// 放在 User Token 解析之前——dry-run 不发请求，不应因缺登录态而失败。
	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		apiVer := "base/v3"
		if req.useV1 {
			apiVer = "bitable/v1"
		}
		// dry-run 预览也尊重用户的 --format/--jq（与实调路径 renderBitableResult 一致），
		// 避免 help 列了 --format/--jq 却在 dry-run 时静默失效。
		o, oerr := output.ParseOptions(cmd)
		if oerr != nil {
			return oerr
		}
		return output.Render(o, map[string]any{
			"api":    apiVer,
			"method": req.method,
			"path":   req.path,
			"params": req.params,
			"body":   req.body,
		})
	}

	token, err := resolveRequiredUserToken(cmd)
	if err != nil {
		return err
	}

	var data map[string]any
	if req.useV1 {
		data, err = client.BitableV1Call(req.method, req.path, req.params, req.body, token)
	} else {
		data, err = client.BaseV3Call(req.method, req.path, req.params, req.body, token)
	}
	if err != nil {
		return err
	}
	return renderBitableResult(cmd, data)
}

// addBitableCommonFlags 给新增 bitable 子命令注册通用 flag：
// --base-token / --user-access-token / --format / --jq。
func addBitableCommonFlags(cmd *cobra.Command) {
	addBaseTokenFlag(cmd)
	cmd.Flags().String("user-access-token", "", "User Access Token")
	output.AddFormatFlags(cmd)
}

// addBitableWriteFlags 在通用 flag 基础上给写命令追加 --dry-run（mutating 操作预览）。
func addBitableWriteFlags(cmd *cobra.Command) {
	addBitableCommonFlags(cmd)
	output.AddDryRunFlag(cmd)
}
