package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/profile"
	"github.com/spf13/cobra"
)

// profileSnapshot 是一套 Bot 的离线身份卡（不输出 secret / 裸 token）。
type profileSnapshot struct {
	Name      string `json:"name,omitempty"`
	Source    string `json:"source"`
	Path      string `json:"path"`
	AppID     string `json:"app_id,omitempty"`
	BaseURL   string `json:"base_url,omitempty"`
	HasConfig bool   `json:"has_config"`
	HasSecret bool   `json:"has_secret"`
	HasToken  bool   `json:"has_token"`
	// HasConfigUserToken 表示该目录 config.yaml 里配了静态 user_access_token（不输出明文）。
	HasConfigUserToken bool   `json:"has_config_user_token,omitempty"`
	TokenStatus        string `json:"token_status,omitempty"`
	UserName           string `json:"user_name,omitempty"`
	UserOpenID         string `json:"user_open_id,omitempty"`
	Active             bool   `json:"active"`
	SelectWith         string `json:"select_with,omitempty"`
	// 按来源分开记录读取失败原因：--config 只替换 config.yaml，
	// token.json / user_profile.json 仍来自该目录，它们的错误必须保留。
	ConfigError string `json:"config_error,omitempty"`
	TokenError  string `json:"token_error,omitempty"`
	CacheError  string `json:"cache_error,omitempty"`
}

// profileInventory 是 Agent 的 Bot 清单契约。
type profileInventory struct {
	Mode               string `json:"mode"`
	Active             string `json:"active"`
	ActiveSource       string `json:"active_source"`
	TokenFrom          string `json:"token_from_profile"`
	UserTokenOverride  string `json:"user_token_override"`
	HasConfigUserToken bool   `json:"has_config_user_token"`
	// EffectiveError 是当前生效那套的读取问题；inactive profile 的问题只留在 profiles[] 里。
	EffectiveError string                   `json:"effective_error,omitempty"`
	EnvOverrides   profile.EnvOverrideFlags `json:"env_overrides"`
	FlagOverrides  profile.EnvOverrideFlags `json:"flag_overrides"`
	Effective      profileSnapshot          `json:"effective"`
	Profiles       []profileSnapshot        `json:"profiles"`
	Hint           string                   `json:"hint,omitempty"`
}

// fileExists 判断路径是否存在，用于与基线一致的 has_config / has_token 语义。
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// snapshotFromDir 读一套 Bot 的离线身份卡。单个文件坏掉只记录到 snap.Error，
// 不返回 error——一个损坏的 inactive profile 不该让整张清单打不开（基线的
// profile.Describe 只做文件存在性检查，同样不会因内容非法而失败）。
func snapshotFromDir(name, source, dir string, active bool) profileSnapshot {
	snap := profileSnapshot{
		Name:   name,
		Source: source,
		Path:   dir,
		Active: active,
	}
	if name != "" {
		snap.SelectWith = "--profile " + name
	}
	// has_config 与基线语义一致：文件存在即为 true，与内容是否合法无关
	snap.HasConfig = fileExists(filepath.Join(dir, profile.ConfigFileName))
	fields, err := profile.ReadConfigFields(dir)
	if err != nil {
		snap.ConfigError = fmt.Sprintf("config.yaml 解析失败: %v", err)
	} else {
		snap.AppID = fields.AppID
		snap.BaseURL = profile.EffectiveBaseURL(fields.BaseURL)
		snap.HasSecret = fields.AppSecret != ""
		snap.HasConfigUserToken = fields.UserAccessToken != ""
	}

	tokenPath := filepath.Join(dir, profile.TokenFileName)
	snap.HasToken = fileExists(tokenPath)
	token, err := auth.LoadTokenFrom(tokenPath)
	if err != nil {
		snap.TokenError = fmt.Sprintf("token.json 解析失败: %v", err)
	} else if token != nil {
		snap.TokenStatus = token.TokenStatus()
	}

	cache, err := auth.LoadUserCacheFrom(filepath.Join(dir, profile.UserCacheFileName))
	if err != nil {
		snap.CacheError = fmt.Sprintf("user_profile.json 解析失败: %v", err)
	} else if cache != nil {
		snap.UserName = cache.Name
		snap.UserOpenID = cache.OpenID
	}

	return snap
}

// activeConfigFields 读当前真正生效的配置字段：--config 指定时以它为准，否则读所选
// 目录的 config.yaml——与 config.Init 的取文件规则保持一致，避免 auth status 显示的
// app_id 和 doctor / 实际请求用的不是同一个。
func activeConfigFields(dir string) (profile.ConfigFields, error) {
	if cfgFile != "" {
		return profile.ReadConfigFieldsFile(cfgFile)
	}
	return profile.ReadConfigFields(dir)
}

// fileCredSource 描述「文件」这一层的实际来源，--config 时要点名是它。
func fileCredSource() credSource {
	if cfgFile != "" {
		return credSource("--config 指定的配置文件")
	}
	return credFromFile
}

func flagOverrideFlags() profile.EnvOverrideFlags {
	return profile.EnvOverrideFlags{
		AppID:     config.BotFlagAppID() != "",
		AppSecret: config.BotFlagAppSecretSet(),
		Profile:   profile.CommandOverride() != "",
	}
}

func applyEnvToEffective(snap profileSnapshot) profileSnapshot {
	if id := config.BotFlagAppID(); id != "" {
		snap.AppID = id
	} else if profile.EnvOverrides().AppID {
		snap.AppID = profile.EnvAppID()
	}
	if config.BotFlagAppSecretSet() || profile.EnvOverrides().AppSecret {
		snap.HasSecret = true
	}
	if envURL := profile.EnvBaseURL(); envURL != "" {
		snap.BaseURL = strings.TrimRight(envURL, "/")
	}
	return snap
}

func buildProfileInventory() (profileInventory, error) {
	active, source, err := profile.ResolveActive()
	if err != nil {
		return profileInventory{}, err
	}
	names, err := profile.List()
	if err != nil {
		return profileInventory{}, err
	}

	inv := profileInventory{
		Active:            active,
		ActiveSource:      source,
		TokenFrom:         tokenFromProfile(active),
		UserTokenOverride: userTokenOverride(),
		EnvOverrides:      profile.EnvOverrides(),
		FlagOverrides:     flagOverrideFlags(),
		Profiles:          []profileSnapshot{},
	}
	if len(names) > 0 {
		inv.Mode = "profile"
	} else {
		inv.Mode = "legacy"
	}

	for _, name := range names {
		dir, err := profile.ProfileDir(name)
		if err != nil {
			return profileInventory{}, err
		}
		inv.Profiles = append(inv.Profiles, snapshotFromDir(name, "profile", dir, name == active))
	}

	activeDir, err := profile.ActiveDir()
	if err != nil {
		return profileInventory{}, err
	}
	effSource := source
	if active != "" {
		effSource = "profile"
	} else if source == profile.SourceNone {
		effSource = "legacy"
	}
	effective := snapshotFromDir(active, effSource, activeDir, true)
	// profiles[] 各读各自目录，但 effective 必须反映 --config 覆盖后的那份。
	// --config 显式给了有效凭证时，所选目录 YAML 是否损坏与本次命令无关，
	// 连它的 Error 一起清掉，否则「配置坏了但已用 --config 绕开」会被误诊成故障。
	if cfgFile != "" {
		// 只替换 config 这一层：token.json / user_profile.json 仍来自所选目录，
		// 它们的解析错误依然成立，不能被 --config 一并抹掉。
		effective.ConfigError = ""
		effective.HasConfig = fileExists(cfgFile)
		if !effective.HasConfig {
			// 用户点名的文件不存在：config.Init 会直接失败，诊断输出必须同步这个事实
			effective.ConfigError = fmt.Sprintf("--config 指定的 %s 不存在", cfgFile)
			effective.AppID, effective.HasSecret, effective.HasConfigUserToken = "", false, false
		}
		fields, ferr := profile.ReadConfigFieldsFile(cfgFile)
		if ferr != nil {
			effective.ConfigError = fmt.Sprintf("--config %s 解析失败: %v", cfgFile, ferr)
			effective.AppID, effective.HasSecret, effective.HasConfigUserToken = "", false, false
		} else {
			effective.AppID = fields.AppID
			effective.HasSecret = fields.AppSecret != ""
			effective.BaseURL = profile.EffectiveBaseURL(fields.BaseURL)
			effective.HasConfigUserToken = fields.UserAccessToken != ""
		}
	}
	inv.Effective = applyEnvToEffective(effective)
	inv.HasConfigUserToken = inv.Effective.HasConfigUserToken
	inv.EffectiveError = inv.Effective.problems()
	inv.Hint = inventoryHint(inv)
	return inv, nil
}

// problems 汇总该条目的全部读取错误，供人类输出与 profile_error 字段复用。
func (s profileSnapshot) problems() string {
	var parts []string
	for _, e := range []string{s.ConfigError, s.TokenError, s.CacheError} {
		if e != "" {
			parts = append(parts, e)
		}
	}
	return strings.Join(parts, "; ")
}

// displayProfileName 把空 profile 名渲染成旧布局标记。
func displayProfileName(name string) string {
	if name == "" {
		return "(legacy)"
	}
	return name
}

func printProfileContextHuman() {
	inv, err := buildProfileInventory()
	if err != nil {
		// config.yaml 坏掉时更要说清当前在哪个 profile——这正是排查最需要的信息
		if name, source, rerr := profile.ResolveActive(); rerr == nil {
			fmt.Printf("  Profile:        %s（source=%s）\n", displayProfileName(name), source)
		}
		return
	}
	fmt.Printf("  Profile:        %s（source=%s）\n", displayProfileName(inv.Active), inv.ActiveSource)
	if inv.Effective.AppID != "" {
		fmt.Printf("  App ID:         %s\n", inv.Effective.AppID)
	}
	if inv.EffectiveError != "" {
		fmt.Printf("  ⚠️ 配置问题:      %s\n", inv.EffectiveError)
	}
	if inv.Hint != "" {
		fmt.Printf("  提示:           %s\n", inv.Hint)
	}
}

func attachProfileContext(result map[string]any) {
	inv, err := buildProfileInventory()
	if err != nil {
		result["profile_error"] = err.Error()
		// 清单构建失败也要保留身份线索，否则 JSON 消费方连当前 profile 都拿不到
		if name, source, rerr := profile.ResolveActive(); rerr == nil {
			result["profile"] = name
			result["profile_source"] = source
			result["token_from_profile"] = tokenFromProfile(name)
		}
		return
	}
	result["profile"] = inv.Active
	result["profile_source"] = inv.ActiveSource
	if inv.EffectiveError != "" {
		result["profile_error"] = inv.EffectiveError
	}
	result["token_from_profile"] = inv.TokenFrom
	if inv.Effective.AppID != "" {
		result["app_id"] = inv.Effective.AppID
	}
	result["env_overrides"] = inv.EnvOverrides
	result["flag_overrides"] = inv.FlagOverrides
	if inv.Hint != "" {
		result["hint"] = inv.Hint
	}
}

func inventoryHint(inv profileInventory) string {
	var parts []string
	if inv.FlagOverrides.AppID || inv.FlagOverrides.AppSecret {
		parts = append(parts, "本命令使用 --bot-app-id/--bot-app-secret 覆盖对应 App 凭证，不写盘。")
		if inv.FlagOverrides.AppID != inv.FlagOverrides.AppSecret {
			parts = append(parts, "只传了其中一个 flag，另一半仍走环境变量或配置文件。")
		}
	} else if inv.EnvOverrides.AppID || inv.EnvOverrides.AppSecret {
		if len(inv.Profiles) > 0 {
			parts = append(parts, "FEISHU_APP_ID/FEISHU_APP_SECRET 中非空项会覆盖所选 profile 的 App 凭证；要使用 profile YAML 中的凭证，请先 unset 对应环境变量。")
		} else {
			parts = append(parts, "App 凭证来自 FEISHU_APP_ID/FEISHU_APP_SECRET。")
		}
	}
	if inv.FlagOverrides.AppID || inv.FlagOverrides.AppSecret || inv.EnvOverrides.AppID || inv.EnvOverrides.AppSecret {
		parts = append(parts, userTokenPhrase(inv.Active)+"；App 凭证与 User Token 来源不会自动绑定。")
	}
	if inv.Mode == "legacy" && len(inv.Profiles) == 0 {
		parts = append(parts, "尚未创建 profile。当前只用环境变量/旧布局的一套 Bot。要管理多个: feishu-cli profile add <name> --app-id ... --app-secret ...")
	}
	return strings.Join(parts, " ")
}

func tokenFromProfile(active string) string {
	if active == "" {
		return "(legacy)"
	}
	return active
}

// credSource 描述某个 App 凭证字段最终来自哪一层。
type credSource string

const (
	credFromFlag credSource = "--bot-app-id/--bot-app-secret"
	credFromEnv  credSource = "FEISHU_APP_ID/FEISHU_APP_SECRET"
	credFromFile credSource = "config.yaml"
)

// profileLabel 把 profile 名渲染成可读位置，空名表示旧布局。
func profileLabel(active string) string {
	if active == "" {
		return "旧布局 ~/.feishu-cli"
	}
	return fmt.Sprintf("profile %q", active)
}

// appCredentialWarning 只在 App 凭证「真错配」时返回告警文案，其余情况返回空串。
//
// 会告警的两种情况：
//  1. 覆盖来的 app_id 与所选目录 config.yaml 里的 app_id 不是同一个应用——User Token 仍读
//     该目录的 token.json，App 身份与 User 身份分属两个应用。
//  2. app_id 与 app_secret 落在不同层且无法确认同属一个应用（例如只传了 --bot-app-secret，
//     app_id 仍走 config.yaml）——两半凭证拼在一起会换不到 tenant_access_token。
//
// 单应用 export FEISHU_APP_ID/FEISHU_APP_SECRET（没有 profile，或与所选 profile 是同一个
// 应用）是文档推荐用法，不产生任何输出——否则每条命令都要刷一行无效警告。
func appCredentialWarning() string {
	active, _, err := profile.ResolveActive()
	if err != nil {
		return ""
	}
	dir, err := profile.ActiveDir()
	if err != nil {
		return ""
	}
	fields, err := activeConfigFields(dir)
	if err != nil {
		return ""
	}
	return credentialWarningFrom(active, fields, fileCredSource(), configOrigin(profileLabel(active)))
}

// userTokenFlagValue 保存本次命令 --user-access-token 的实际取值（各命令的 local flag）。
// 用值而不是 Changed：显式传空串时 resolver 会跳过它继续走 env/file。
var userTokenFlagValue string

// setUserTokenFlagPresence 在 PersistentPreRunE 中记录 --user-access-token 的取值。
func setUserTokenFlagPresence(cmd *cobra.Command) {
	userTokenFlagValue = ""
	if cmd == nil {
		return
	}
	if f := cmd.Flags().Lookup("user-access-token"); f != nil {
		// 不 TrimSpace：auth.ResolveUserAccessToken 判的是原始 != ""，
		// 这里做额外清洗会让报告与 resolver 的实际行为对不上
		userTokenFlagValue = f.Value.String()
	}
}

// userTokenOverride 报告是否存在压过 profile token.json 的显式 User Token 设置。
//
// 它只陈述「设置了什么」，绝不断言本次命令实际用了哪个 token：本项目按命令分四类
// token helper（cmd/utils.go），`--as bot`、默认 Bot 身份的写命令、只认 flag 的
// vc bot meeting-join、固定读本地文件的 auth status 各有各的解析路径，root 层
// 无权替它们下结论。返回空串表示没有任何显式覆盖。
func userTokenOverride() string {
	if userTokenFlagValue != "" {
		return "--user-access-token"
	}
	if os.Getenv("FEISHU_USER_ACCESS_TOKEN") != "" {
		return "FEISHU_USER_ACCESS_TOKEN"
	}
	return ""
}

// userTokenPhrase 描述 User Token 的候选来源，措辞限定在「使用 User Token 的命令」，
// 不断言本次命令是否走 User 身份。
func userTokenPhrase(active string) string {
	if origin := userTokenOverride(); origin != "" {
		return fmt.Sprintf("%s 已设置，使用 User Token 的命令会优先用它，而不是回退到 %s 的 token.json（该文件仍可能被读取用于自动刷新）", origin, profileLabel(active))
	}
	return fmt.Sprintf("使用 User Token 的命令会读 %s 的 token.json", profileLabel(active))
}

// appCredentialWarningForProfile 强制以目标 profile 自己的 config.yaml 为基准。
// profile use 必须走这个入口：它讲的是「写完指针后的持久状态」，既不能被本次
// --profile 带偏，也不能被本次 --config 这种一次性覆盖劫持。
func appCredentialWarningForProfile(name, dir string) string {
	fields, err := profile.ReadConfigFields(dir)
	if err != nil {
		return ""
	}
	return credentialWarningFrom(name, fields, credFromFile, profileLabel(name)+" 的 config.yaml")
}

// credentialWarningFrom 按给定的「文件层」凭证判定错配。fromFile 是该层的来源标签，
// origin 是它在文案里的可读位置。
func credentialWarningFrom(active string, fields profile.ConfigFields, fromFile credSource, origin string) string {
	flags := flagOverrideFlags()
	env := profile.EnvOverrides()
	if !flags.AppID && !flags.AppSecret && !env.AppID && !env.AppSecret {
		return ""
	}

	idSource, effectiveID := fromFile, fields.AppID
	switch {
	case flags.AppID:
		idSource, effectiveID = credFromFlag, config.BotFlagAppID()
	case env.AppID:
		idSource, effectiveID = credFromEnv, profile.EnvAppID()
	}
	secretSource, hasSecret := fromFile, fields.AppSecret != ""
	switch {
	case flags.AppSecret:
		secretSource, hasSecret = credFromFlag, true
	case env.AppSecret:
		secretSource, hasSecret = credFromEnv, true
	}

	// app_secret 落在配置文件且该文件的 app_id 就是最终 app_id 时，两半必属同一应用
	sameApp := idSource == secretSource ||
		(secretSource == fromFile && fields.AppID != "" && fields.AppID == effectiveID)

	switch {
	case idSource != fromFile && fields.AppID != "" && effectiveID != fields.AppID:
		return fmt.Sprintf(
			"⚠️  App 凭证被 %s 覆盖为 %s，与 %s（%s）不是同一个应用；%s。",
			idSource, effectiveID, origin, fields.AppID, userTokenPhrase(active))
	case effectiveID != "" && hasSecret && !sameApp:
		return fmt.Sprintf(
			"⚠️  app_id 来自 %s、app_secret 来自 %s；两半凭证若不属于同一应用会换不到 tenant_access_token。",
			idSource, secretSource)
	}
	return ""
}

// configOrigin 描述凭证文件的位置，供告警文案使用。
func configOrigin(where string) string {
	if cfgFile != "" {
		return fmt.Sprintf("--config 指定的 %s", cfgFile)
	}
	return where + " 的 config.yaml"
}

func warnAppCredentialTokenSource() {
	if msg := appCredentialWarning(); msg != "" {
		fmt.Fprintln(os.Stderr, msg)
	}
}
