package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

// resolveBaseToken 读取 --base-token：接受裸 base_token，也接受多维表格链接
// （/base/<token>、/bitable/<token>）与挂在知识库里的多维表格链接（/wiki/<node_token>，
// 通过 node_by_token 换出底层 base_token 并校验 obj_type=bitable）。
// --dry-run 时不发请求：wiki 链接以占位符代替，stderr 说明实际执行时会解析。
func resolveBaseToken(cmd *cobra.Command) (string, error) {
	raw, _ := cmd.Flags().GetString("base-token")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", clierr.Usagef("--base-token 必填")
	}
	if !client.LooksLikeURL(raw) {
		return raw, nil
	}
	ref, err := parseBaseTokenURL(raw)
	if err != nil {
		return "", err
	}
	if ref.Type == client.ResourceTypeBitable {
		return ref.Token, nil
	}
	// wiki 链接：需要 node_by_token 换出底层多维表格 token
	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		fmt.Fprintf(os.Stderr, "提示: dry-run 不解析知识库链接；实际执行时会通过 node_by_token 把 wiki 节点 %s 换成底层 base_token\n", ref.Token)
		return "<wiki:" + ref.Token + ">", nil
	}
	token, err := resolveIdentityToken(cmd)
	if err != nil {
		return "", err
	}
	node, err := resolveBitableWikiNode(ref.Token, token)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(os.Stderr, "已将 wiki 节点 %s 解析为多维表格: %s\n", ref.Token, node.ObjToken)
	return node.ObjToken, nil
}

// parseBaseTokenURL 离线解析 --base-token 传入的链接，只接受 /base/、/bitable/、/wiki/ 路径。
func parseBaseTokenURL(raw string) (client.ResourceRef, error) {
	ref, err := client.ParseResourceURL(raw)
	if err != nil {
		if kind := classifyBaseShareURL(raw); kind != "" {
			return client.ResourceRef{}, clierr.Usagef("--base-token 不接受%s链接；先用 `feishu-cli bitable resolve --url <链接>` 解析出 base_token 等坐标", kind)
		}
		return client.ResourceRef{}, clierr.Usagef("--base-token 解析失败: %v", err)
	}
	switch ref.Type {
	case client.ResourceTypeBitable, client.ResourceTypeWiki:
		return ref, nil
	default:
		return client.ResourceRef{}, clierr.Usagef("--base-token 需要多维表格链接（/base/ 或 /wiki/），当前链接是 %s 类型", ref.Type)
	}
}

// resolveBitableWikiNode 通过 node_by_token 解析 wiki 节点，并要求底层是多维表格。
func resolveBitableWikiNode(nodeToken, userAccessToken string) (*client.WikiNode, error) {
	node, err := client.ResolveWikiNode(nodeToken, userAccessToken)
	if err != nil {
		return nil, err
	}
	objType := client.NormalizeResourceType(node.ObjType)
	if objType != client.ResourceTypeBitable {
		if objType == "" {
			objType = "未知类型"
		}
		return nil, clierr.Usagef("知识库节点 %s 的底层文档是 %s，不是多维表格；请改用对应命令，或提供多维表格链接", nodeToken, objType)
	}
	if strings.TrimSpace(node.ObjToken) == "" {
		return nil, fmt.Errorf("知识库节点 %s 的响应缺少 obj_token", nodeToken)
	}
	return node, nil
}

// addBaseTokenFlag 给命令添加 --base-token flag 并标记为必填
func addBaseTokenFlag(cmd *cobra.Command) {
	cmd.Flags().String("base-token", "", "多维表格 base_token，或 /base/、/wiki/ 链接（必填）")
	mustMarkFlagRequired(cmd, "base-token")
}

// bitableCreateCmd 创建多维表格
var bitableCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建多维表格",
	Long: `创建一个新的多维表格（base）。

必填:
  --name    多维表格名称

可选:
  --folder-token  目标文件夹 token（默认根目录）
  --time-zone     时区（如 Asia/Shanghai）
  --table-name    第一张数据表的名称（单独使用时重命名默认表）
  --fields        第一张数据表的字段 JSON 数组（与 +field create 的字段 JSON 同形）；
                  指定后先按该 schema 新建数据表，再删除平台自动创建的默认表

以 Bot 身份创建时，自动给当前 CLI 登录用户授予 full_access（输出 permission_grant）。

示例:
  feishu-cli bitable create --name "项目管理"
  feishu-cli bitable create --name "销售数据" --folder-token fldxxx --time-zone Asia/Shanghai
  feishu-cli bitable create --name "任务跟踪" --table-name "任务" \
    --fields '[{"name":"标题","type":"text"},{"name":"状态","type":"select","options":[{"name":"Todo"},{"name":"Done"}]}]'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		name, _ := cmd.Flags().GetString("name")
		folderToken, _ := cmd.Flags().GetString("folder-token")
		timeZone, _ := cmd.Flags().GetString("time-zone")
		tableName, _ := cmd.Flags().GetString("table-name")
		fieldsJSON, _ := cmd.Flags().GetString("fields")
		output, _ := cmd.Flags().GetString("output")

		if strings.TrimSpace(name) == "" {
			return clierr.Usagef("--name 必填")
		}
		// --fields 先离线校验，避免建出多维表格后才发现 schema 写错
		var firstTableFields []any
		if strings.TrimSpace(fieldsJSON) != "" {
			parsed, err := parseBitableFieldsArray(fieldsJSON, "--fields")
			if err != nil {
				return err
			}
			firstTableFields = parsed
		}

		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}

		body := map[string]any{"name": name}
		if folderToken != "" {
			body["folder_token"] = folderToken
		}
		if timeZone != "" {
			body["time_zone"] = timeZone
		}

		data, err := client.BaseV3Call("POST", client.BaseV3Path("bases"), nil, body, token)
		if err != nil {
			return err
		}
		if data == nil {
			data = map[string]any{}
		}
		newBaseToken := extractBaseTokenFromResponse(data)

		// --as bot（或 auto 未登录）创建时，自动给当前 CLI 登录用户授予 full_access
		grant := autoGrantCurrentUser(token, newBaseToken, client.ResourceTypeBitable)
		withPermissionGrant(data, grant)

		// 第一张数据表：--fields 新建自定义表并删默认表；仅 --table-name 时重命名默认表
		if firstTableFields != nil || strings.TrimSpace(tableName) != "" {
			if err := setupFirstBitableTable(data, newBaseToken, strings.TrimSpace(tableName), firstTableFields, token); err != nil {
				// 多维表格已建好：把已创建的坐标留在 stderr，避免用户重复创建
				fmt.Fprintf(os.Stderr, "多维表格已创建（base_token: %s），但初始化第一张数据表失败\n", newBaseToken)
				return err
			}
		}

		if output == "json" {
			return printJSON(data)
		}

		fmt.Printf("多维表格创建成功!\n")
		printBaseSummary(data, "base_token")
		printPermissionGrantText(os.Stdout, grant)
		return nil
	},
}

// bitableCreateDefaultTableDeleteDelay 新建自定义表后删除默认表前的等待（对齐官方 1s，避开写后读延迟）。
var bitableCreateDefaultTableDeleteDelay = time.Second

// setupFirstBitableTable 处理 bitable create 的 --table-name / --fields（对齐官方 base +base-create）：
//   - 有 --fields：按 schema 新建数据表 → 等待 1s → 删除平台自动创建的默认表；
//   - 仅 --table-name：重命名默认表。
//
// 结果写回 data：table、fields、default_table_deleted / default_table_renamed 等字段。
func setupFirstBitableTable(data map[string]any, baseToken, tableName string, fields []any, token string) error {
	if baseToken == "" {
		return fmt.Errorf("创建响应中缺少 base_token，无法初始化第一张数据表")
	}
	defaultTableID, err := findDefaultBitableTableID(baseToken, token)
	if err != nil {
		return err
	}
	if fields != nil {
		if tableName == "" {
			tableName = "数据表 1"
		}
		created, err := client.BaseV3Call("POST", client.BaseV3Path("bases", baseToken, "tables"), nil,
			map[string]any{"name": tableName, "fields": fields}, token)
		if err != nil {
			return fmt.Errorf("按 --fields 创建数据表失败: %w", err)
		}
		if bitableStr(created, "id", "table_id") == "" {
			return fmt.Errorf("创建数据表的响应缺少 table id")
		}
		data["table"] = created
		if f, ok := created["fields"]; ok {
			data["fields"] = f
		}
		time.Sleep(bitableCreateDefaultTableDeleteDelay)
		if _, err := client.BaseV3Call("DELETE", client.BaseV3Path("bases", baseToken, "tables", defaultTableID), nil, nil, token); err != nil {
			return fmt.Errorf("删除默认数据表 %s 失败（自定义表已创建）: %w", defaultTableID, err)
		}
		data["default_table_deleted"] = true
		data["deleted_default_table_id"] = defaultTableID
		return nil
	}
	renamed, err := client.BaseV3Call("PATCH", client.BaseV3Path("bases", baseToken, "tables", defaultTableID), nil,
		map[string]any{"name": tableName}, token)
	if err != nil {
		return fmt.Errorf("重命名默认数据表失败: %w", err)
	}
	data["table"] = renamed
	data["default_table_renamed"] = true
	data["renamed_default_table_id"] = defaultTableID
	return nil
}

// findDefaultBitableTableID 取新建多维表格自带的默认数据表 ID（列表第一张表）。
func findDefaultBitableTableID(baseToken, token string) (string, error) {
	tables, _, err := listBaseV3Items(client.BaseV3Path("bases", baseToken, "tables"), "tables", token)
	if err != nil {
		return "", fmt.Errorf("读取默认数据表失败: %w", err)
	}
	for _, t := range tables {
		if m, ok := t.(map[string]any); ok {
			if id := bitableStr(m, "id", "table_id"); id != "" {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("新建的多维表格中没有找到默认数据表")
}

// parseBitableFieldsArray 解析字段 JSON 数组（每项必须是对象）。
func parseBitableFieldsArray(raw, flagName string) ([]any, error) {
	var items []any
	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	dec.UseNumber()
	if err := dec.Decode(&items); err != nil {
		return nil, clierr.Usagef("%s 需要字段 JSON 数组，如 '[{\"name\":\"标题\",\"type\":\"text\"}]': %v", flagName, err)
	}
	if len(items) == 0 {
		return nil, clierr.Usagef("%s 至少需要一个字段定义（空数组会让数据表使用平台默认 schema）", flagName)
	}
	for i, item := range items {
		if _, ok := item.(map[string]any); !ok {
			return nil, clierr.Usagef("%s 第 %d 项必须是对象", flagName, i+1)
		}
	}
	return items, nil
}

// printBaseSummary 打印 create/copy 的文本摘要。base/v3 实测响应是平铺的
// {base_token,name,url,folder_token}；同时兼容旧形状 {base:{...}}。
func printBaseSummary(data map[string]any, tokenLabel string) {
	m := data
	if nested, ok := data["base"].(map[string]any); ok {
		if _, flat := data["name"]; !flat {
			m = nested
		}
	}
	width := len(tokenLabel) + 1
	line := func(label, value string) {
		fmt.Printf("  %-*s %s\n", width, label+":", value)
	}
	if t := extractBaseTokenFromResponse(data); t != "" {
		line(tokenLabel, t)
	}
	if n, _ := m["name"].(string); n != "" {
		line("name", n)
	}
	if u, _ := m["url"].(string); u != "" {
		line("URL", u)
	}
	if t, ok := data["table"].(map[string]any); ok {
		if id := bitableStr(t, "id", "table_id"); id != "" {
			line("table_id", id)
		}
	}
}

// bitableStr 依次读取 m 中的字符串键，返回第一个非空值。
func bitableStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, _ := m[k].(string); strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// bitableGetCmd 获取多维表格信息
var bitableGetCmd = &cobra.Command{
	Use:   "get",
	Short: "获取多维表格信息",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		baseToken, err := resolveBaseToken(cmd)
		if err != nil {
			return err
		}
		data, err := client.BaseV3Call("GET", client.BaseV3Path("bases", baseToken), nil, nil, token)
		if err != nil {
			return err
		}
		return printJSON(data)
	},
}

// bitableCopyCmd 复制多维表格
var bitableCopyCmd = &cobra.Command{
	Use:   "copy",
	Short: "复制多维表格",
	Long: `复制一个已有的多维表格。

必填:
  --base-token  源多维表格 token
  --name        新表格名称

可选:
  --folder-token    目标文件夹
  --without-content bool  只复制结构（不含数据）`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		token, err := resolveIdentityToken(cmd)
		if err != nil {
			return err
		}
		baseToken, err := resolveBaseToken(cmd)
		if err != nil {
			return err
		}
		name, _ := cmd.Flags().GetString("name")
		folderToken, _ := cmd.Flags().GetString("folder-token")
		withoutContent, _ := cmd.Flags().GetBool("without-content")
		output, _ := cmd.Flags().GetString("output")

		if strings.TrimSpace(name) == "" {
			return clierr.Usagef("--name 必填")
		}

		body := map[string]any{"name": name, "without_content": withoutContent}
		if folderToken != "" {
			body["folder_token"] = folderToken
		}

		data, err := client.BaseV3Call("POST", client.BaseV3Path("bases", baseToken, "copy"), nil, body, token)
		if err != nil {
			return err
		}
		// --as bot（或 auto 未登录）复制时，自动给当前 CLI 登录用户授予副本 full_access
		grant := autoGrantCurrentUser(token, extractBaseTokenFromResponse(data), client.ResourceTypeBitable)
		if data == nil {
			data = map[string]any{}
		}
		withPermissionGrant(data, grant)
		if output == "json" {
			return printJSON(data)
		}
		fmt.Printf("多维表格复制成功!\n")
		printBaseSummary(data, "new base_token")
		printPermissionGrantText(os.Stdout, grant)
		return nil
	},
}

func init() {
	bitableCmd.AddCommand(bitableCreateCmd)
	bitableCreateCmd.Flags().String("name", "", "多维表格名称（必填）")
	bitableCreateCmd.Flags().String("folder-token", "", "目标文件夹 token")
	bitableCreateCmd.Flags().String("time-zone", "", "时区（如 Asia/Shanghai）")
	bitableCreateCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	bitableCreateCmd.Flags().String("user-access-token", "", "User Access Token")
	bitableCreateCmd.Flags().String("table-name", "", "第一张数据表名称（单独使用时重命名默认表）")
	bitableCreateCmd.Flags().String("fields", "", "第一张数据表的字段 JSON 数组（新建该表并删除默认表）")
	mustMarkFlagRequired(bitableCreateCmd, "name")

	bitableCmd.AddCommand(bitableGetCmd)
	addBaseTokenFlag(bitableGetCmd)
	bitableGetCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	bitableGetCmd.Flags().String("user-access-token", "", "User Access Token")

	bitableCmd.AddCommand(bitableCopyCmd)
	addBaseTokenFlag(bitableCopyCmd)
	bitableCopyCmd.Flags().String("name", "", "新表格名称（必填）")
	bitableCopyCmd.Flags().String("folder-token", "", "目标文件夹 token")
	bitableCopyCmd.Flags().Bool("without-content", false, "只复制结构")
	bitableCopyCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	bitableCopyCmd.Flags().String("user-access-token", "", "User Access Token")
	mustMarkFlagRequired(bitableCopyCmd, "name")
}

// extractBaseTokenFromResponse 从 base/v3 创建/复制响应中取出新表的 token。
// 兼容 base 对象直接位于 data 顶层与嵌套在 data.base 两种形状，字段名兼容 base_token / app_token。
func extractBaseTokenFromResponse(data map[string]any) string {
	candidates := []map[string]any{data}
	if nested, ok := data["base"].(map[string]any); ok {
		candidates = append(candidates, nested)
	}
	for _, m := range candidates {
		for _, key := range []string{"base_token", "app_token"} {
			if t, _ := m[key].(string); strings.TrimSpace(t) != "" {
				return strings.TrimSpace(t)
			}
		}
	}
	return ""
}
