// Package clierr 定义 CLI 统一的错误分类与进程退出码。
//
// 设计约束：
//   - 分类通过类型化错误 + errors.As 实现，Error() 原样返回被包装错误的文本，
//     保证给错误"打标签"不会改变用户 / Agent 看到的错误信息；
//   - 本包是叶子包（只依赖标准库），internal/auth、internal/config、internal/client
//     与 cmd 都可以在错误产生处直接打标签；
//   - 依赖业务码（client.HasAPICode）或 cobra 文案的兜底判定放在 cmd 层完成，
//     避免本包反向依赖 client。
package clierr

import "fmt"

// 进程退出码。对外契约：脚本 / AI Agent 依据退出码区分失败类型并决定是否重试。
const (
	ExitOK      = 0 // 成功
	ExitGeneral = 1 // 一般错误 / 业务错误（含资源不存在、无资源权限、限流等）
	ExitUsage   = 2 // 用法错误：未知命令 / 未知 flag / 参数个数或必填 flag 校验失败
	ExitAuth    = 3 // 鉴权 / 权限：未登录、token 失效或刷新失败、应用或用户缺 scope、App 凭证缺失
	ExitNetwork = 4 // 网络错误：超时、连接失败、DNS 解析失败
	// ExitConfirmationRequired 危险操作需要确认：非交互环境下未带 --yes。
	// 调用方（Agent）应在获得用户同意后追加 --yes 重新执行。
	ExitConfirmationRequired = 10
	// ExitInterrupted 收到 SIGINT/SIGTERM 后取消（沿用 shell 约定 128+SIGINT）。
	ExitInterrupted = 130
)

// Kind 错误分类。
type Kind int

const (
	// KindGeneral 一般错误（退出码 1），仅作为零值占位，通常无需显式标注。
	KindGeneral Kind = iota
	// KindUsage 用法错误（退出码 2）。
	KindUsage
	// KindAuth 鉴权 / 权限 / App 凭证配置错误（退出码 3）。
	KindAuth
	// KindNetwork 网络错误（退出码 4）。
	KindNetwork
	// KindConfirmationRequired 需要用户确认（退出码 10）。
	KindConfirmationRequired
	// KindCancelled 用户在交互确认中明确拒绝（退出码 1：未执行，但不是"需要确认"）。
	KindCancelled
)

// String 返回分类的稳定英文名（用于测试与调试输出）。
func (k Kind) String() string {
	switch k {
	case KindUsage:
		return "usage"
	case KindAuth:
		return "auth"
	case KindNetwork:
		return "network"
	case KindConfirmationRequired:
		return "confirmation_required"
	case KindCancelled:
		return "cancelled"
	default:
		return "general"
	}
}

// ExitCode 返回分类对应的进程退出码。
func (k Kind) ExitCode() int {
	switch k {
	case KindUsage:
		return ExitUsage
	case KindAuth:
		return ExitAuth
	case KindNetwork:
		return ExitNetwork
	case KindConfirmationRequired:
		return ExitConfirmationRequired
	default:
		return ExitGeneral
	}
}

// Error 带分类的错误。Error() 原样返回被包装错误的文本。
type Error struct {
	Kind Kind
	Err  error
}

func (e *Error) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

// Unwrap 让 errors.Is / errors.As 继续穿透到被包装的原始错误。
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Wrap 给 err 打上分类标签；err 为 nil 时返回 nil。
func Wrap(kind Kind, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Kind: kind, Err: err}
}

// Usage 标记为用法错误（退出码 2）。
func Usage(err error) error { return Wrap(KindUsage, err) }

// Usagef 构造用法错误。
func Usagef(format string, a ...any) error { return Usage(fmt.Errorf(format, a...)) }

// Auth 标记为鉴权 / 权限错误（退出码 3）。
func Auth(err error) error { return Wrap(KindAuth, err) }

// Authf 构造鉴权 / 权限错误。
func Authf(format string, a ...any) error { return Auth(fmt.Errorf(format, a...)) }

// Network 标记为网络错误（退出码 4）。
func Network(err error) error { return Wrap(KindNetwork, err) }

// ConfirmationRequired 标记为"需要确认"（退出码 10）。
func ConfirmationRequired(err error) error { return Wrap(KindConfirmationRequired, err) }

// ConfirmationRequiredf 构造"需要确认"错误。
func ConfirmationRequiredf(format string, a ...any) error {
	return ConfirmationRequired(fmt.Errorf(format, a...))
}

// Cancelledf 构造"用户已取消"错误（退出码 1）。
func Cancelledf(format string, a ...any) error {
	return Wrap(KindCancelled, fmt.Errorf(format, a...))
}

// Kinds 返回错误链上出现的全部分类（外层在前，去重）。
// 支持 fmt.Errorf("%w") 单链与 errors.Join 多链。
func Kinds(err error) []Kind {
	var out []Kind
	seen := map[Kind]bool{}
	walk(err, func(e error) {
		if ce, ok := e.(*Error); ok && ce != nil && !seen[ce.Kind] {
			seen[ce.Kind] = true
			out = append(out, ce.Kind)
		}
	})
	return out
}

// HasKind 报告错误链上是否出现指定分类。
func HasKind(err error, kind Kind) bool {
	for _, k := range Kinds(err) {
		if k == kind {
			return true
		}
	}
	return false
}

// walk 深度优先遍历错误链（含 errors.Join 的多分支），对每个节点调用 fn。
func walk(err error, fn func(error)) {
	const maxDepth = 64 // 防御自引用错误链造成死循环
	var visit func(error, int)
	visit = func(e error, depth int) {
		if e == nil || depth > maxDepth {
			return
		}
		fn(e)
		switch u := e.(type) {
		case interface{ Unwrap() []error }:
			for _, inner := range u.Unwrap() {
				visit(inner, depth+1)
			}
		case interface{ Unwrap() error }:
			visit(u.Unwrap(), depth+1)
		}
	}
	visit(err, 0)
}
