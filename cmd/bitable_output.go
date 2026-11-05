package cmd

import (
	"strings"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
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
	// preview 追加到 dry-run 预览顶层的说明字段（如 auto_paginate）。
	preview map[string]any
	// steps 非空时为多步编排：dry-run 预览输出 {"api","steps"}，method/path/params/body 不再单独展示。
	steps []map[string]any
}

// bitableCaller 执行一次已构造好的请求，返回待渲染的数据（对象、数组或 nil）。
type bitableCaller func(req bitableReq, token string) (any, error)

// bitableDefaultCall 按 useV1 路由到 BitableV1Call / BaseV3Call。
func bitableDefaultCall(req bitableReq, token string) (any, error) {
	var data map[string]any
	var err error
	if req.useV1 {
		data, err = client.BitableV1Call(req.method, req.path, req.params, req.body, token)
	} else {
		data, err = client.BaseV3Call(req.method, req.path, req.params, req.body, token)
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

// bitableOutputOptions 在发任何请求之前解析 --format/--jq（未注册时返回默认 JSON）。
// 非法取值属于用法错误（exit 2），不应等到请求成功后才报。
// 未注册 --format 的命令（含 -o 是下载路径或 "json" 开关的旧命令）不读 -o，避免把它当结果文件。
func bitableOutputOptions(cmd *cobra.Command) (*output.Options, error) {
	if cmd.Flags().Lookup("format") == nil {
		return &output.Options{Format: output.FormatJSON}, nil
	}
	o, err := output.ParseOptions(cmd)
	if err != nil {
		return nil, clierr.Usage(err)
	}
	return o, nil
}

// renderBitableWith 渲染 bitable 返回的数据：未要求 --format/--jq 时走 printJSON，
// 与改造前逐字节一致；要求时走 output 包。
func renderBitableWith(o *output.Options, data any) error {
	if o == nil || ((o.Format == "" || o.Format == output.FormatJSON) && strings.TrimSpace(o.JQ) == "" && o.OutputFile == "") {
		return printJSON(data)
	}
	return output.Render(o, data)
}

// renderBitableResult 渲染 bitable 返回的 data（供自带执行流程的命令使用）。
func renderBitableResult(cmd *cobra.Command, data any) error {
	o, err := bitableOutputOptions(cmd)
	if err != nil {
		return err
	}
	return renderBitableWith(o, data)
}

// bitableDryRunPreview 构造 dry-run 预览：{"api","method","path","params","body"} + preview 追加字段。
func bitableDryRunPreview(req bitableReq) map[string]any {
	apiVer := "base/v3"
	if req.useV1 {
		apiVer = "bitable/v1"
	}
	preview := map[string]any{
		"api":    apiVer,
		"method": req.method,
		"path":   req.path,
		"params": req.params,
		"body":   req.body,
	}
	if len(req.steps) > 0 {
		preview = map[string]any{"api": apiVer, "steps": req.steps}
	}
	for k, v := range req.preview {
		preview[k] = v
	}
	return preview
}

// bitableIsDryRun 读取 --dry-run；命令未注册该 flag 时返回 false。
func bitableIsDryRun(cmd *cobra.Command) bool {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	return dryRun
}

// validateBaseTokenOffline 本地校验 --base-token（必填、链接类型），不联网。
// 命令未注册 --base-token（如模板中心命令）时跳过。
func validateBaseTokenOffline(cmd *cobra.Command) error {
	if cmd.Flags().Lookup("base-token") == nil {
		return nil
	}
	raw, _ := cmd.Flags().GetString("base-token")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return clierr.Usagef("--base-token 必填")
	}
	if client.LooksLikeURL(raw) {
		if _, err := parseBaseTokenURL(raw); err != nil {
			return err
		}
	}
	return nil
}

// bitablePrepared 是 bitablePrepare 的结果：dryRun=true 表示预览已打印、调用方直接返回。
type bitablePrepared struct {
	dryRun bool
	req    bitableReq
	token  string
	opts   *output.Options
}

// bitablePrepare 按 CLI 标准顺序完成执行前的全部步骤：
//
//  1. 本地校验：--format/--jq、--as、--base-token（离线解析），不联网、不读凭据；
//  2. --dry-run：打印请求预览（wiki 链接以占位符代替，不解析）并返回 dryRun=true；
//  3. config.Validate；
//  4. 身份解析；
//  5. --base-token 解析（wiki 链接经 node_by_token 换出 base_token）并构造请求。
//
// 命令自己的参数校验必须在调用之前完成。命令未注册 --base-token 时 build 收到空串。
func bitablePrepare(cmd *cobra.Command, build func(baseToken string) bitableReq) (*bitablePrepared, error) {
	o, err := bitableOutputOptions(cmd)
	if err != nil {
		return nil, err
	}
	if err := validateIdentityAs(cmd); err != nil {
		return nil, err
	}
	if err := validateBaseTokenOffline(cmd); err != nil {
		return nil, err
	}
	hasBaseToken := cmd.Flags().Lookup("base-token") != nil

	if bitableIsDryRun(cmd) {
		baseToken := ""
		if hasBaseToken {
			// dry-run 下 resolveBaseToken 不联网：wiki 链接返回 <wiki:token> 占位符
			if baseToken, err = resolveBaseToken(cmd); err != nil {
				return nil, err
			}
		}
		// dry-run 预览只打 stdout，不写 -o 文件
		po := *o
		po.OutputFile = ""
		if err := renderBitableWith(&po, bitableDryRunPreview(build(baseToken))); err != nil {
			return nil, err
		}
		return &bitablePrepared{dryRun: true}, nil
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}
	token, err := resolveIdentityToken(cmd)
	if err != nil {
		return nil, err
	}
	baseToken := ""
	if hasBaseToken {
		if baseToken, err = resolveBaseToken(cmd); err != nil {
			return nil, err
		}
	}
	return &bitablePrepared{req: build(baseToken), token: token, opts: o}, nil
}

// bitableExecute 是 bitable 命令的统一执行骨架：bitablePrepare → call → 渲染
// （未传 --format/--jq 时输出与 printJSON 逐字节一致）。call 为 nil 时按 useV1 路由。
func bitableExecute(cmd *cobra.Command, build func(baseToken string) bitableReq, call bitableCaller) error {
	p, err := bitablePrepare(cmd, build)
	if err != nil || p.dryRun {
		return err
	}
	if call == nil {
		call = bitableDefaultCall
	}
	data, err := call(p.req, p.token)
	if err != nil {
		return err
	}
	return renderBitableWith(p.opts, data)
}

// bitableExecuteConfirmed 同 bitableExecute，但在身份解析之后、执行之前走确认门禁
// （--yes 跳过；非交互且未确认时 exit 10、不发写请求）。用于新增的删除 / 覆盖类高危命令。
func bitableExecuteConfirmed(cmd *cobra.Command, build func(baseToken string) bitableReq, prompt func(req bitableReq) string, call bitableCaller) error {
	p, err := bitablePrepare(cmd, build)
	if err != nil || p.dryRun {
		return err
	}
	if err := confirmDangerousAction(cmd, prompt(p.req)); err != nil {
		return err
	}
	if call == nil {
		call = bitableDefaultCall
	}
	data, err := call(p.req, p.token)
	if err != nil {
		return err
	}
	return renderBitableWith(p.opts, data)
}

// bitableRun 封装 bitable 命令的统一执行流程（见 bitableExecute）。
func bitableRun(cmd *cobra.Command, build func(baseToken string) bitableReq) error {
	return bitableRunWithTransform(cmd, build, nil)
}

// bitableRunWithTransform 同 bitableRun，实调成功后先用 transform 加工 data 再渲染（dry-run 不经过 transform）。
func bitableRunWithTransform(cmd *cobra.Command, build func(baseToken string) bitableReq, transform func(map[string]any) map[string]any) error {
	return bitableExecute(cmd, build, func(req bitableReq, token string) (any, error) {
		data, err := bitableDefaultCall(req, token)
		if err != nil {
			return nil, err
		}
		m, _ := data.(map[string]any)
		if transform != nil && m != nil {
			m = transform(m)
		}
		return m, nil
	})
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

// addBitablePreviewFlags 给存量命令补 --dry-run / --format / --jq（不重复注册）。
// 带 -o/--output 的旧命令（create/get/copy）只补 --dry-run，避免 -o 被当作结果文件路径。
func addBitablePreviewFlags(cmds ...*cobra.Command) {
	for _, c := range cmds {
		output.AddDryRunFlag(c)
		if c.Flags().Lookup("output") == nil {
			output.AddFormatFlags(c)
		}
	}
}
