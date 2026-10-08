package cmd

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/safefile"
	"github.com/spf13/cobra"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
	"golang.org/x/term"
)

// flagString 读取字符串 flag，忽略错误（未注册 flag 返回空串）。
func flagString(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

// flagInt 读取整型 flag，忽略错误。
func flagInt(cmd *cobra.Command, name string) int {
	v, _ := cmd.Flags().GetInt(name)
	return v
}

// resolveOptionalUserToken 解析显式指定的 user_access_token（可选）
// 仅检查 --user-access-token 参数和 FEISHU_USER_ACCESS_TOKEN 环境变量，
// 不自动从 token.json 加载，确保能用 App Token 的 API 默认使用 App Token（租户身份）
func resolveOptionalUserToken(cmd *cobra.Command) string {
	if flagToken, _ := cmd.Flags().GetString("user-access-token"); flagToken != "" {
		return flagToken
	}
	if envToken := os.Getenv("FEISHU_USER_ACCESS_TOKEN"); envToken != "" {
		return envToken
	}
	return ""
}

// resolveFlagUserToken 仅解析命令行显式传入的 user_access_token。
// 适用于默认应使用 App/Tenant Token，仅在用户明确指定时才切换到 User Token 的命令。
func resolveFlagUserToken(cmd *cobra.Command) string {
	flagToken, _ := cmd.Flags().GetString("user-access-token")
	return flagToken
}

// resolveAutoUserToken 在 auto 模式下解析 User Token。
// 规则：
// 1. 若未配置任何 User 身份（无 flag、无 env、无 token.json、无 config），返回 "", nil（安全回退到 Tenant/Bot）
// 2. 若配置了 User 身份且解析/刷新成功，返回 token, nil
// 3. 若检测到 User 身份配置，但解析或刷新失败，必须 fail-closed 返回具体错误，禁止切回 Bot
func resolveAutoUserToken(cmd *cobra.Command) (string, error) {
	flagToken, _ := cmd.Flags().GetString("user-access-token")
	cfg := config.Get()
	token, err := auth.ResolveUserAccessToken(flagToken, cfg.UserAccessToken, cfg.AppID, cfg.AppSecret, cfg.BaseURL)
	if err != nil {
		if auth.IsNoUserTokenConfigured(err) {
			return "", nil
		}
		return "", err
	}
	return token, nil
}

// resolveOptionalUserTokenForDestructive 与 resolveOptionalUserTokenWithFallback 同源，
// 但用于**不可逆操作**（drive pull --delete-local / drive push --delete-remote 等）：
// 「已配置 User Token 却无法使用」时 fail-closed 报错，而不是降级成 Bot 身份继续。
//
// 原因：身份决定了「远端有哪些文件」。以 Bot 身份看到的远端视图往往更小，
// 差集计算随之变大，--delete-local 会把用户本地文件当作"远端已不存在"而删掉。
// 未配置 User Token（纯 Bot 场景）仍正常放行，属预期用法。
func resolveOptionalUserTokenForDestructive(cmd *cobra.Command, opName string) (string, error) {
	flagToken, _ := cmd.Flags().GetString("user-access-token")
	cfg := config.Get()
	token, err := auth.ResolveUserAccessToken(flagToken, cfg.UserAccessToken, cfg.AppID, cfg.AppSecret, cfg.BaseURL)
	if err != nil {
		if auth.IsNoUserTokenConfigured(err) {
			return "", nil // 未配置 User 身份：按 Bot 执行，属预期
		}
		return "", fmt.Errorf("%s 是不可逆操作，但已配置的 User Token 无法使用，拒绝降级为 Bot 身份执行"+
			"（Bot 看到的远端文件更少，差集会误删本地文件）：%w", opName, err)
	}
	return token, nil
}

// resolveOptionalUserTokenWithFallback 尝试完整优先级链解析 User Token（可选）
// 与 resolveOptionalUserToken 不同，会额外尝试从 token.json 和 config 中读取
// 找不到时返回空字符串（回退到 App Token），而非报错
// 适用于 msg/chat/doc export 等希望自动使用 User Token 的场景
//
// 注意「未配置」与「配置了但解析失败」的区别：
//   - 未配置（ErrNoUserTokenConfigured）：静默回退 Bot，属正常路径
//   - 配置了但失败（token.json 未绑定 app_id、app_id 不匹配、读取或刷新失败）：
//     仍回退 Bot 以保持读命令可用，但必须在 stderr 明确告警。
//     否则用户会拿到 Bot 视角的空结果或「无权限」，却以为是自己的 User 身份在查
//     （同一台机器上 --as 类命令对同样的 token 会 fail-closed，两种行为互相矛盾）。
func resolveOptionalUserTokenWithFallback(cmd *cobra.Command) string {
	flagToken, _ := cmd.Flags().GetString("user-access-token")
	cfg := config.Get()
	token, err := auth.ResolveUserAccessToken(flagToken, cfg.UserAccessToken, cfg.AppID, cfg.AppSecret, cfg.BaseURL)
	if err != nil {
		if !auth.IsNoUserTokenConfigured(err) {
			// 检测到 User 身份配置但无法使用：告警后按 Bot 身份继续
			fmt.Fprintf(os.Stderr, "⚠️  已配置 User Token 但无法使用，本次改用 Bot 身份执行（结果可能不含你的个人数据）：%v\n", err)
		} else if cfg.Debug {
			fmt.Fprintf(os.Stderr, "[Debug] 未配置 User Token，使用 App Token\n")
		}
		return ""
	}
	if cfg.Debug {
		source := "token.json/config"
		if flagToken != "" {
			source = "--user-access-token 参数"
		} else if os.Getenv("FEISHU_USER_ACCESS_TOKEN") != "" {
			source = "FEISHU_USER_ACCESS_TOKEN 环境变量"
		}
		fmt.Fprintf(os.Stderr, "[Debug] 使用 User Access Token (来源: %s)\n", source)
	}
	return token
}

// resolveRequiredUserToken 解析 user_access_token（必需）
// 用于搜索等必须使用 User Access Token 的 API，解析失败时返回错误
func resolveRequiredUserToken(cmd *cobra.Command) (string, error) {
	flagToken, _ := cmd.Flags().GetString("user-access-token")
	cfg := config.Get()
	return auth.ResolveUserAccessToken(flagToken, cfg.UserAccessToken, cfg.AppID, cfg.AppSecret, cfg.BaseURL)
}

// requireUserToken 封装 resolveRequiredUserToken，失败时返回带命令名和 auth login 提示的统一错误。
// 新命令（vc/minutes/mail/drive/...）都走这个 helper，保持错误信息一致。
func requireUserToken(cmd *cobra.Command, cmdName string) (string, error) {
	token, err := resolveRequiredUserToken(cmd)
	if err != nil {
		return "", fmt.Errorf("%s 需要 User Access Token（请先 `feishu-cli auth login`）: %w", cmdName, err)
	}
	return token, nil
}

// resolveIdentityToken 根据命令的 --as flag 解析应使用的 token。
// 返回的字符串为空表示使用 Tenant(App) Token；非空表示 User Token。
// 适用于底层飞书 API 同时支持 user / tenant 身份的命令组（如 bitable，
// 其 base/v3 与 bitable/v1 client 都声明 SupportedAccessTokenTypes:[User, Tenant]）。
//
//	auto（默认）: User 优先、Tenant 兜底——已登录用 User Token，未配置自动回落 App Token，
//	             检测到 User 身份但刷新/解析失败时 fail-closed 报错，防止未授权切 Bot；
//	bot/tenant/app: 强制 App Token（返回空字符串）；
//	user: 强制 User Token，缺失则报错。
func resolveIdentityToken(cmd *cobra.Command) (string, error) {
	as, _ := cmd.Flags().GetString("as")
	switch strings.ToLower(strings.TrimSpace(as)) {
	case "", "auto":
		return resolveAutoUserToken(cmd)
	case "bot", "tenant", "app":
		return "", nil
	case "user":
		token, err := resolveRequiredUserToken(cmd)
		if err != nil {
			return "", fmt.Errorf("--as user 需要 User Access Token（请先 `feishu-cli auth login`，或改用 --as bot 走 App Token）: %w", err)
		}
		return token, nil
	default:
		return "", clierr.Usagef("--as 仅支持 bot|user|auto，得到 %q", as)
	}
}

const identityAsFlagHelp = "身份: bot(App Token) | user(User Token) | auto(User 优先；未配置回退 Bot；已配置但解析/刷新失败 fail-closed)"

func addAsFlag(cmd *cobra.Command) {
	cmd.Flags().String("as", "auto", identityAsFlagHelp)
}

func addAsPersistentFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().String("as", "auto", identityAsFlagHelp)
}

// validateIdentityAs 只校验 --as 枚举，不解析/刷新 token。dry-run 前调用。
func validateIdentityAs(cmd *cobra.Command) error {
	as, _ := cmd.Flags().GetString("as")
	switch strings.ToLower(strings.TrimSpace(as)) {
	case "", "auto", "bot", "tenant", "app", "user":
		return nil
	default:
		return clierr.Usagef("--as 仅支持 bot|user|auto，得到 %q", as)
	}
}

// resolveCurrentAuthedUserID returns the current logged-in user's ID for the requested type.
func resolveCurrentAuthedUserID(cmd *cobra.Command, userIDType string) (string, error) {
	token, err := resolveRequiredUserToken(cmd)
	if err != nil {
		return "", err
	}

	cfg := config.Get()
	cachePath, _ := auth.UserCachePath()
	cached, cacheErr := auth.LoadCurrentUserCache()
	switch {
	case cacheErr != nil:
		if cfg.Debug {
			fmt.Fprintf(os.Stderr, "[Debug] [cache:%s] 读取当前登录用户缓存失败，回源 user_info: %v\n", cachePath, cacheErr)
		}
	case cached != nil && cached.MatchesToken(token):
		if cfg.Debug {
			fmt.Fprintf(os.Stderr, "[Debug] [cache:%s] 命中当前登录用户缓存\n", cachePath)
		}
		return currentUserIDFromInfo(currentUserIDCacheToInfo(cached), userIDType)
	case cached != nil:
		if cfg.Debug {
			fmt.Fprintf(os.Stderr, "[Debug] [cache:%s] 当前登录 token 已变化，忽略旧缓存并回源 user_info\n", cachePath)
		}
	default:
		if cfg.Debug {
			fmt.Fprintf(os.Stderr, "[Debug] [cache:%s] 未命中当前登录用户缓存，回源 user_info\n", cachePath)
		}
	}

	info, err := client.GetCurrentUserInfo(token)
	if err != nil {
		return "", err
	}

	cache := &auth.CurrentUserCache{
		OpenID:           info.OpenID,
		UserID:           info.UserID,
		UnionID:          info.UnionID,
		Name:             info.Name,
		TokenFingerprint: auth.UserTokenFingerprint(token),
	}
	if err := auth.SaveCurrentUserCache(cache); err != nil {
		if cfg.Debug {
			fmt.Fprintf(os.Stderr, "[Debug] [cache:%s] 写入当前登录用户缓存失败: %v\n", cachePath, err)
		}
	} else if cfg.Debug {
		fmt.Fprintf(os.Stderr, "[Debug] [cache:%s] 已更新当前登录用户缓存\n", cachePath)
	}

	return currentUserIDFromInfo(info, userIDType)
}

func currentUserIDCacheToInfo(cache *auth.CurrentUserCache) *client.UserInfo {
	if cache == nil {
		return &client.UserInfo{}
	}

	return &client.UserInfo{
		OpenID:  cache.OpenID,
		UserID:  cache.UserID,
		UnionID: cache.UnionID,
		Name:    cache.Name,
	}
}

func currentUserIDFromInfo(info *client.UserInfo, userIDType string) (string, error) {
	if info == nil {
		return "", fmt.Errorf("当前登录用户信息为空")
	}

	switch userIDType {
	case "open_id":
		if info.OpenID != "" {
			return info.OpenID, nil
		}
	case "user_id":
		if info.UserID != "" {
			return info.UserID, nil
		}
	case "union_id":
		if info.UnionID != "" {
			return info.UnionID, nil
		}
	default:
		return "", fmt.Errorf("不支持的 user-id-type: %s", userIDType)
	}

	return "", fmt.Errorf("当前登录用户缺少 %s，无法自动推断当前登录用户身份", userIDType)
}

// validateEnum validates that value is one of the allowed values.
func validateEnum(value, fieldName string, allowedValues []string) error {
	for _, allowed := range allowedValues {
		if value == allowed {
			return nil
		}
	}
	return clierr.Usagef("不支持的%s %q，可选值: %s", fieldName, value, strings.Join(allowedValues, ", "))
}

// mustMarkFlagRequired 标记 flag 为必填，如果失败则 panic
// 用于 init() 函数中，确保配置错误在启动时被发现
func mustMarkFlagRequired(cmd *cobra.Command, flags ...string) {
	for _, flag := range flags {
		if err := cmd.MarkFlagRequired(flag); err != nil {
			panic(fmt.Sprintf("标记必填 flag '%s' 失败: %v", flag, err))
		}
	}
}

// loadJSONInput 统一处理 --xxx 和 --xxx-file 两种 JSON 输入方式。
func loadJSONInput(inlineValue, filePath, inlineFlag, fileFlag, label string) (string, error) {
	if inlineValue != "" && filePath != "" {
		return "", clierr.Usagef("--%s 和 --%s 不能同时使用", inlineFlag, fileFlag)
	}

	if filePath != "" {
		data, err := readLocalInputFile(filePath)
		if err != nil {
			return "", fmt.Errorf("读取 %s 文件失败: %w", label, err)
		}
		inlineValue = string(data)
	}

	if strings.TrimSpace(inlineValue) == "" {
		return "", clierr.Usagef("请通过 --%s 或 --%s 提供%s", inlineFlag, fileFlag, label)
	}

	return inlineValue, nil
}

// printJSON 把 v 编码为带 2 空格缩进的 JSON 写到 stdout，禁用 HTML 转义。
// 用于命令一次性输出结构化结果（auth status、search docs -o json 等）。
func printJSON(v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("JSON 序列化失败: %w", err)
	}
	fmt.Print(buf.String())
	return nil
}

// printJSONLine 把 v 编码为单行紧凑 JSON 写到 stdout，禁用 HTML 转义。
// 用于事件流（JSONL），例如 `auth login --json` 一次授权中依次输出
// device_authorization / authorization_success 两行事件。
func printJSONLine(v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("JSON 序列化失败: %w", err)
	}
	// json.Encoder.Encode 已经追加了 \n，直接写出即可。
	fmt.Print(buf.String())
	return nil
}

// 确认门禁的输入源与交互判定，供测试注入。
//
// 交互判定必须用真正的 TTY 检测（ioctl），不能复用 isTerminal 的字符设备判断：
// /dev/null 也是字符设备，`cmd </dev/null` 会被误判为交互终端，读到 EOF 后
// 又落回"已取消"分支。isTerminal 保持原语义（event consume 的 stdin EOF 协议依赖它）。
var (
	confirmInput         io.Reader = os.Stdin
	confirmPromptOut     io.Writer = os.Stderr
	confirmIsInteractive           = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
)

// confirmDangerousAction 危险操作（删除等不可逆写）的确认门禁。返回 nil 表示可以继续执行。
//
//   - 带 --yes（根命令全局 flag，或命令自身的同名 flag）或 --force → 直接放行；
//     仅用于 --force 语义就是"跳过确认"的命令。
//   - stdin 不是终端（AI Agent / 管道 / cron）→ 不读 stdin，返回"需要确认"错误（退出码 10）。
//     过去这里读到 EOF 就打印"操作已取消"并 exit 0，调用方会把"什么都没做"误判为删除成功。
//   - 交互式终端：提示写 stderr（不污染 stdout），仅输入 y/yes 放行，否则返回"已取消"错误（退出码 1）。
//
// 调用方必须在 --dry-run 提前返回之后再调用：预览不执行写操作，不需要确认。
func confirmDangerousAction(cmd *cobra.Command, prompt string) error {
	if confirmationBypassed(cmd) {
		return nil
	}
	if !confirmIsInteractive() {
		return clierr.ConfirmationRequiredf("需要确认：%s\n当前为非交互环境（stdin 不是终端），未执行任何操作。确认执行请追加 --yes 后重新运行", prompt)
	}
	fmt.Fprintf(confirmPromptOut, "%s (y/N): ", prompt)
	// 读到 EOF 也按已读内容判断：只有明确输入 y/yes 才放行
	response, _ := bufio.NewReader(confirmInput).ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))
	if response == "y" || response == "yes" {
		return nil
	}
	return clierr.Cancelledf("操作已取消，未执行任何操作")
}

// confirmationBypassed 报告本次调用是否已显式确认（--yes 或 --force）。
func confirmationBypassed(cmd *cobra.Command) bool {
	if assumeYes {
		return true
	}
	if cmd == nil {
		return false
	}
	for _, name := range []string{"yes", "force"} {
		if f := cmd.Flags().Lookup(name); f != nil && f.Value.Type() == "bool" && f.Value.String() == "true" {
			return true
		}
	}
	return false
}

// validateOutputPath 验证用户指定的输出路径是否安全：
//   - 相对路径按路径段拒绝 ".."（越出当前目录），report..v2.json 这类文件名放行；
//   - 解析符号链接后拒绝敏感目录（~/.ssh、~/.aws、~/.feishu-cli、/etc 等，见 internal/safefile）；
//   - allowedDir 非空时要求路径（解析符号链接后）位于该目录内，判断带路径分隔符边界。
func validateOutputPath(outputPath string, allowedDir string) error {
	if err := safefile.ValidateOutputPath(outputPath); err != nil {
		return err
	}
	if allowedDir != "" {
		within, err := safefile.IsWithinResolved(outputPath, allowedDir)
		if err != nil {
			return fmt.Errorf("无法解析输出路径: %w", err)
		}
		if !within {
			return clierr.Usagef("输出路径必须在 %s 目录下", allowedDir)
		}
	}
	return nil
}

// readLocalInputFile 读取用户指定的本地输入文件，先拒绝敏感目录（防止把凭证当请求体发往远端）。
func readLocalInputFile(path string) ([]byte, error) {
	if err := safefile.ValidateInputPath(path); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// unescapeSheetRange 处理 shell 转义的范围字符串
// 在某些 shell（如 zsh）中，! 字符会被自动转义为 \!
// 此函数将 \! 转换回 !
func unescapeSheetRange(rangeStr string) string {
	return strings.ReplaceAll(rangeStr, "\\!", "!")
}

// safeOutputPath 生成安全的输出路径
// 移除不安全的字符，防止路径遍历
func safeOutputPath(baseName string, ext string) string {
	// 移除路径分隔符和不安全字符
	safeName := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '_'
		}
		return r
	}, baseName)

	// 限制文件名长度
	if len(safeName) > 200 {
		safeName = safeName[:200]
	}

	if ext != "" && !strings.HasSuffix(safeName, ext) {
		safeName += ext
	}

	return safeName
}

// isValidToken 验证飞书 token 格式
// 飞书 token 通常由字母和数字组成，长度在 10-50 之间
func isValidToken(token string) bool {
	if len(token) < 5 || len(token) > 100 {
		return false
	}
	// 只允许字母、数字和部分特殊字符
	for _, r := range token {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// normalizePermMemberType normalizes member type aliases for the Drive permission API.
// The IM API uses underscore-separated identifiers (open_id, user_id, union_id, chat_id),
// while the Drive permission API uses concatenated identifiers (openid, userid, unionid, openchat).
// This function accepts both styles so users don't have to remember which API uses which format.
func normalizePermMemberType(memberType string) string {
	switch memberType {
	case "open_id":
		return "openid"
	case "user_id":
		return "userid"
	case "union_id":
		return "unionid"
	case "chat_id":
		return "openchat"
	default:
		return memberType
	}
}

// splitAndTrim 按逗号分割字符串并去除空白
func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// translateChatError 翻译群操作相关的飞书业务错误码，给出可操作建议。
// 调用方拿到 SDK 或 API 返回的 error 后，过一遍此 helper 再 return 给 cobra，
// 让用户/Agent 直接看到中文解决方案，不用自己去 grep 错误码文档。
//
// 已知翻译的错误码:
//   - 232033: 外部群权限不足（最常见，提示开「对外共享能力」+ 切换 App）
//   - 232011: 操作者不在群里
//   - 232006: chat_id 不存在或无效
//
// 未识别的错误原样返回。
func translateChatError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case client.HasAPICode(err, 232033):
		return fmt.Errorf(`%w

📌 这是飞书外部群权限错误（232033）。

外部群（external=true）的「群信息/群成员/群配置」类 API 默认禁用，
需要同时满足两个条件:
  1. 当前 App 必须开启「对外共享能力」
     → 飞书开放平台 → 应用 → 凭证与基础信息 → 检查"应用市场分发能力"
  2. 该 App 的 Bot 必须实际加入此群（让群管理员邀请）

如果你已经有另一个开了对外共享能力的 App，临时切换调用即可:
  FEISHU_APP_ID=cli_xxx FEISHU_APP_SECRET=xxx feishu-cli <命令>

或者用 profile 永久保存:
  feishu-cli profile add ext-bot --app-id cli_xxx --app-secret xxx
  feishu-cli profile use ext-bot

详见 skills/feishu-cli-messaging/references/workflows/chat/references/external-chat.md`, err)

	case client.HasAPICode(err, 232011):
		return fmt.Errorf(`%w

📌 操作者不在群里（232011）。当前 Bot/用户没加入这个群:
  - 让群管理员邀请进群: feishu-cli chat member add <chat_id> --id-list <id>
  - 或者主动入群（需邀请链接）: feishu-cli chat link <chat_id>`, err)

	case client.HasAPICode(err, 232006):
		return fmt.Errorf(`%w

📌 chat_id 无效（232006）。请检查 ID 是否正确，可通过以下方式重新获取:
  feishu-cli msg search-chats --query "群名关键词"`, err)
	}

	return err
}

// decodeImagePixelSize 读取图片真实像素尺寸，用于显式指定飞书图片块/画板节点的显示宽高。
// 只读文件头（image.DecodeConfig），不解整张图；支持 png/jpeg/gif/webp/bmp/tiff
// （后三种经 golang.org/x/image 注册，均为飞书接受的图片格式）。
//
// JPEG 带 EXIF Orientation 5-8（显示时宽高转置的旋转）时返回 (0, 0)：DecodeConfig
// 返回的是转置前的存储尺寸，直接下发会把显示框的宽高比写反，退回服务端推断更安全。
// 任何失败都返回 (0, 0)，调用方据此退回服务端推断或要求显式指定，不影响主流程成功。
func decodeImagePixelSize(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	cfg, format, err := image.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0
	}
	if format == "jpeg" {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return 0, 0
		}
		if o := jpegEXIFOrientation(f); o >= 5 && o <= 8 {
			return 0, 0
		}
	}
	return cfg.Width, cfg.Height
}

// jpegEXIFOrientation 从 JPEG 字节流读取 EXIF Orientation（1-8），读不到返回 0。
// 只顺序扫描 SOS（图像数据）之前的段头与 APP1 段内容，不解码图像数据。
func jpegEXIFOrientation(r io.Reader) int {
	br := bufio.NewReader(r)
	var soi [2]byte
	if _, err := io.ReadFull(br, soi[:]); err != nil || soi[0] != 0xFF || soi[1] != 0xD8 {
		return 0
	}
	for {
		b, err := br.ReadByte()
		if err != nil || b != 0xFF {
			return 0 // 读尽或段边界错乱，放弃
		}
		marker, err := br.ReadByte()
		if err != nil {
			return 0
		}
		for marker == 0xFF { // 跳过填充字节
			if marker, err = br.ReadByte(); err != nil {
				return 0
			}
		}
		if marker == 0xDA || marker == 0xD9 { // SOS/EOI：EXIF 只会出现在此之前
			return 0
		}
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) { // 无长度的独立标记
			continue
		}
		var lenBuf [2]byte
		if _, err := io.ReadFull(br, lenBuf[:]); err != nil {
			return 0
		}
		segLen := int(binary.BigEndian.Uint16(lenBuf[:]))
		if segLen < 2 {
			return 0
		}
		payload := make([]byte, segLen-2)
		if _, err := io.ReadFull(br, payload); err != nil {
			return 0
		}
		if marker == 0xE1 {
			if o := exifOrientationFromAPP1(payload); o != 0 {
				return o
			}
		}
	}
}

// exifOrientationFromAPP1 解析 APP1/EXIF 段负载（不含 marker 和长度字节）中
// IFD0 的 Orientation 标签（tag 0x0112, 类型 SHORT），无效返回 0。
func exifOrientationFromAPP1(payload []byte) int {
	const exifHeader = "Exif\x00\x00"
	if len(payload) < len(exifHeader)+8 || string(payload[:len(exifHeader)]) != exifHeader {
		return 0
	}
	tiff := payload[len(exifHeader):]
	var bo binary.ByteOrder
	switch {
	case tiff[0] == 'I' && tiff[1] == 'I':
		bo = binary.LittleEndian
	case tiff[0] == 'M' && tiff[1] == 'M':
		bo = binary.BigEndian
	default:
		return 0
	}
	if bo.Uint16(tiff[2:4]) != 42 {
		return 0
	}
	ifdOff := int(bo.Uint32(tiff[4:8]))
	if ifdOff < 8 || ifdOff+2 > len(tiff) {
		return 0
	}
	count := int(bo.Uint16(tiff[ifdOff : ifdOff+2]))
	for i := 0; i < count; i++ {
		entry := ifdOff + 2 + i*12
		if entry+12 > len(tiff) {
			return 0
		}
		tag := bo.Uint16(tiff[entry : entry+2])
		typ := bo.Uint16(tiff[entry+2 : entry+4])
		if tag != 0x0112 || typ != 3 { // Orientation 为 SHORT 类型
			continue
		}
		// SHORT 值内联在 value 字段前 2 字节（按 TIFF 字节序）
		if o := int(bo.Uint16(tiff[entry+8 : entry+10])); o >= 1 && o <= 8 {
			return o
		}
		return 0
	}
	return 0
}

var unsafeResourceChars = regexp.MustCompile(`[?#%\x00-\x1f\x7f]`)

// validateResourceIdentifier 严格校验资源标识符（nodeToken, spaceID, taskID, ticket, fileToken 等）：
// 拒绝空、.. 路径穿越、URL 元字符 (?#%)、百分号编码绕过 (%2e%2e)、ASCII 控制字符 (0x00-0x1f, 0x7f)、空白与斜杠。
func validateResourceIdentifier(val, name string) error {
	val = strings.TrimSpace(val)
	if val == "" {
		return fmt.Errorf("%s 不能为空", name)
	}
	if strings.Contains(val, "..") {
		return fmt.Errorf("%s 不能包含 '..' 路径穿越", name)
	}
	if strings.ContainsAny(val, " \t\r\n/") {
		return fmt.Errorf("%s 不能包含空白字符或路径分隔符 '/'", name)
	}
	if unsafeResourceChars.MatchString(val) {
		return fmt.Errorf("%s 包含非法字符（禁止包含 ? # %% 及控制字符）", name)
	}
	return nil
}
