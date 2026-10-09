package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

// sheet_common.go —— sheet 命令组的公共能力：
//   - 身份：命令组级 --as bot|user|auto（不传时保持旧行为）；
//   - 表格参数：裸 token、/sheets/ 或 /spreadsheets/ URL、/wiki/ URL（node_by_token 换出底层表格）；
//   - 子表与范围：范围前缀既可以是 sheetId，也可以是子表名（"Sheet1!A1:C10"），统一换算成 sheetId。

const sheetAsFlagHelp = "身份: bot(App Token) | user(User Token) | auto(User 优先；未配置回退 Bot；已配置但不可用时报错)。" +
	"不传时沿用旧行为：User 优先，User Token 不可用时告警后回退 Bot"

// resolveSheetUserToken 解析 sheet 命令使用的身份，返回空串表示 Bot（App Token）。
//
//   - 未传 --as：保持旧行为（resolveOptionalUserTokenWithFallback：User 优先，Token 损坏时 stderr 告警后回退 Bot）；
//   - --as bot：强制 App Token（已登录也不用 User Token，适合 Bot 自有表格与无人值守任务）；
//   - --as user：强制 User Token，缺失或不可用时报错；
//   - --as auto：User 优先，未配置回退 Bot，已配置但解析/刷新失败 fail-closed。
func resolveSheetUserToken(cmd *cobra.Command) (string, error) {
	f := cmd.Flags().Lookup("as")
	if f == nil || !f.Changed {
		return resolveOptionalUserTokenWithFallback(cmd), nil
	}
	if err := validateIdentityAs(cmd); err != nil {
		return "", err
	}
	return resolveIdentityToken(cmd)
}

// sheetListFunc 便于测试替换子表列表请求。
var sheetListFunc = func(ctx context.Context, spreadsheetToken, userAccessToken string) ([]*client.SheetInfo, error) {
	return client.QuerySheets(ctx, spreadsheetToken, userAccessToken)
}

// sheetTarget 是解析后的表格目标：表格 token + 身份 + 懒加载的子表列表。
type sheetTarget struct {
	Token string // 电子表格 token（wiki 已解包）
	UAT   string // User Access Token；空串表示 Bot（App Token）
	// URLSheetID 来自表格 URL 的 ?sheet= 参数，范围没有子表前缀且未指定 --sheet-id/--sheet-name 时作为默认子表
	URLSheetID string

	sheets       []*client.SheetInfo
	sheetsLoaded bool
	sheetsErr    error
}

// parseSpreadsheetArg 离线解析表格参数（裸 token 或 URL），不发网络请求，可在 --dry-run 前调用。
// wiki URL 返回 isWiki=true 与未解包的 node_token。
func parseSpreadsheetArg(raw, argName string) (token, urlSheetID string, isWiki bool, err error) {
	res, err := parseResourceArg(raw, sheetResourceArgOptions(argName, ""))
	if err != nil {
		return "", "", false, clierr.Usage(err)
	}
	return res.Token, urlSheetParam(raw), res.InputType == client.ResourceTypeWiki, nil
}

func sheetResourceArgOptions(argName, uat string) resourceArgOptions {
	if argName == "" {
		argName = "<spreadsheet_token>"
	}
	return resourceArgOptions{
		ArgName:         argName,
		DefaultType:     client.ResourceTypeSheet,
		Allowed:         []string{client.ResourceTypeSheet},
		ResolveWiki:     true,
		UserAccessToken: uat,
	}
}

// urlSheetParam 取表格 URL 中的 ?sheet=<sheetId>；非 URL 或无该参数时返回空串。
func urlSheetParam(raw string) string {
	if !client.LooksLikeURL(strings.TrimSpace(raw)) {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(u.Query().Get("sheet"))
	if !client.IsSafeResourceToken(v) {
		return ""
	}
	return v
}

// newSheetTarget 解析身份与表格参数：先定身份（wiki 解包需要用同一身份），再把 URL / wiki 换成表格 token。
func newSheetTarget(cmd *cobra.Command, raw string) (*sheetTarget, error) {
	return newSheetTargetNamed(cmd, raw, "<spreadsheet_token>")
}

func newSheetTargetNamed(cmd *cobra.Command, raw, argName string) (*sheetTarget, error) {
	uat, err := resolveSheetUserToken(cmd)
	if err != nil {
		return nil, err
	}
	return newSheetTargetWithToken(raw, argName, uat)
}

func newSheetTargetWithToken(raw, argName, uat string) (*sheetTarget, error) {
	opts := sheetResourceArgOptions(argName, uat)
	res, err := parseResourceArg(raw, opts)
	if err != nil {
		return nil, clierr.Usage(err)
	}
	if err := resolveWikiInResource(res, opts); err != nil {
		return nil, err
	}
	noteWikiResolved(res)
	return &sheetTarget{Token: res.Token, UAT: uat, URLSheetID: urlSheetParam(raw)}, nil
}

// listSheets 懒加载子表列表（同一命令内只请求一次）。
func (t *sheetTarget) listSheets() ([]*client.SheetInfo, error) {
	if !t.sheetsLoaded {
		t.sheets, t.sheetsErr = sheetListFunc(client.Context(), t.Token, t.UAT)
		t.sheetsLoaded = true
	}
	return t.sheets, t.sheetsErr
}

// describeSheets 生成「sheetId(子表名)」列表，用于报错提示。
func describeSheets(sheets []*client.SheetInfo) string {
	parts := make([]string, 0, len(sheets))
	for _, s := range sheets {
		parts = append(parts, fmt.Sprintf("%s(%s)", s.SheetID, s.Title))
	}
	return strings.Join(parts, ", ")
}

// lookupSheet 在子表列表中按 sheetId 优先、子表名其次匹配；未找到返回 nil。
func lookupSheet(sheets []*client.SheetInfo, ref string) *client.SheetInfo {
	for _, s := range sheets {
		if s.SheetID == ref {
			return s
		}
	}
	for _, s := range sheets {
		if s.Title == ref {
			return s
		}
	}
	for _, s := range sheets {
		if strings.EqualFold(strings.TrimSpace(s.Title), strings.TrimSpace(ref)) {
			return s
		}
	}
	return nil
}

// resolveSheetRef 把 sheetId 或子表名解析为 sheetId。子表列表获取失败时原样返回（不引入新的失败模式，
// 交给后续接口报错）；列表获取成功但找不到时返回用法错误并列出全部子表。
func (t *sheetTarget) resolveSheetRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", clierr.Usagef("子表 ID / 名称不能为空")
	}
	sheets, err := t.listSheets()
	if err != nil {
		if config.Get().Debug {
			fmt.Fprintf(os.Stderr, "[Debug] 获取子表列表失败，按原样使用 %q: %v\n", ref, err)
		}
		return ref, nil
	}
	if s := lookupSheet(sheets, ref); s != nil {
		return s.SheetID, nil
	}
	return "", clierr.Usagef("子表 %q 不存在（既不是 sheetId 也不是子表名）；该表格的子表: %s", ref, describeSheets(sheets))
}

// defaultSheetID 在调用方未指定子表时选择默认子表：URL ?sheet= 参数优先；表格只有一个子表时用它；
// 多个子表时报用法错误并列出子表。子表列表获取失败时返回空串（交给接口报错，与旧行为一致）。
func (t *sheetTarget) defaultSheetID() (string, error) {
	if t.URLSheetID != "" {
		return t.URLSheetID, nil
	}
	sheets, err := t.listSheets()
	if err != nil || len(sheets) == 0 {
		return "", nil
	}
	if len(sheets) == 1 {
		return sheets[0].SheetID, nil
	}
	return "", clierr.Usagef("范围缺少子表前缀，且表格有 %d 个子表，无法确定写入/读取哪一个；"+
		"请写成 <sheetId>!A1:C10 或 <子表名>!A1:C10，或指定 --sheet-id / --sheet-name。子表: %s", len(sheets), describeSheets(sheets))
}

// qualifyRange 把范围参数规范成「<sheetId>!<A1>」。
//
//	"<sheetId>!A1:C10"  → 原样（前缀等于 knownSheetID 时不发请求）
//	"Sheet1!A1:C10"     → 子表名换成 sheetId
//	"A1:C10"            → 用 --sheet-id / --sheet-name / URL ?sheet= / 唯一子表补前缀
//	"<sheetId 或子表名>" → 整个子表（换成 sheetId）
//
// knownSheetID 为调用方已确定的 sheetId（--sheet-id 或位置参数），sheetName 为 --sheet-name。
func (t *sheetTarget) qualifyRange(rangeStr, knownSheetID, sheetName string) (string, error) {
	rangeStr = strings.TrimSpace(unescapeSheetRange(rangeStr))
	knownSheetID = strings.TrimSpace(knownSheetID)
	sheetName = strings.TrimSpace(sheetName)
	if rangeStr == "" {
		return "", clierr.Usagef("范围不能为空")
	}
	if prefix, rest, ok := client.SplitSheetRangePrefix(rangeStr); ok {
		if prefix == knownSheetID {
			return knownSheetID + "!" + rest, nil
		}
		id, err := t.resolveSheetRef(prefix)
		if err != nil {
			return "", err
		}
		return id + "!" + rest, nil
	}
	if strings.ContainsAny(rangeStr, "!！") {
		return "", clierr.Usagef("范围 %q 格式错误：子表前缀或单元格部分为空", rangeStr)
	}
	if rangeStr == knownSheetID {
		// 整个子表（已知 sheetId）
		return rangeStr, nil
	}
	if !client.LooksLikeA1Range(rangeStr) {
		// 没有 "!" 且不是 A1 写法：视为整个子表（sheetId 或子表名）
		return t.resolveSheetRef(rangeStr)
	}
	if knownSheetID == "" && sheetName == "" && t.URLSheetID == "" {
		// 形如 A1 的串也可能恰好是某个子表的 sheetId / 名称（如 "abc123"）：精确命中时按整个子表处理，保持旧行为
		if sheets, err := t.listSheets(); err == nil {
			for _, s := range sheets {
				if s.SheetID == rangeStr || s.Title == rangeStr {
					return s.SheetID, nil
				}
			}
		}
	}
	sheetID, err := t.pickSheet(knownSheetID, sheetName)
	if err != nil {
		return "", err
	}
	if sheetID == "" {
		// 无法确定子表（列表获取失败）：保持旧行为，原样交给接口
		return rangeStr, nil
	}
	return sheetID + "!" + rangeStr, nil
}

// pickSheet 按 --sheet-id > --sheet-name > URL ?sheet= > 唯一子表 的顺序确定子表。
func (t *sheetTarget) pickSheet(sheetID, sheetName string) (string, error) {
	if sheetID != "" {
		return sheetID, nil
	}
	if sheetName != "" {
		return t.resolveSheetRef(sheetName)
	}
	return t.defaultSheetID()
}

// qualifyRanges 对多个范围逐个调用 qualifyRange。
func (t *sheetTarget) qualifyRanges(ranges []string, knownSheetID string) ([]string, error) {
	out := make([]string, len(ranges))
	for i, r := range ranges {
		q, err := t.qualifyRange(r, knownSheetID, "")
		if err != nil {
			return nil, err
		}
		out[i] = q
	}
	return out, nil
}

// addSheetNameFlag 为带 --sheet-id 的命令补充 --sheet-name。
func addSheetNameFlag(cmd *cobra.Command) {
	cmd.Flags().String("sheet-name", "", "子表名称（范围未带子表前缀时使用，与 --sheet-id 二选一；自动换算为 sheetId）")
}

// sheetSelectorFlags 读取 --sheet-id / --sheet-name，并校验二者不能同时使用。
func sheetSelectorFlags(cmd *cobra.Command) (sheetID, sheetName string, err error) {
	sheetID = strings.TrimSpace(flagString(cmd, "sheet-id"))
	sheetName = strings.TrimSpace(flagString(cmd, "sheet-name"))
	if sheetID != "" && sheetName != "" {
		return "", "", clierr.Usagef("--sheet-id 与 --sheet-name 不能同时使用")
	}
	return sheetID, sheetName, nil
}

// readSheetDataInput 读取 --data / --data-file 指定的 JSON 文本。
func readSheetDataInput(cmd *cobra.Command) ([]byte, error) {
	dataStr := flagString(cmd, "data")
	dataFile := flagString(cmd, "data-file")
	switch {
	case dataFile != "":
		data, err := readLocalInputFile(dataFile)
		if err != nil {
			return nil, fmt.Errorf("读取数据文件失败: %w", err)
		}
		return data, nil
	case dataStr != "":
		return []byte(dataStr), nil
	default:
		return nil, clierr.Usagef("请通过 --data 或 --data-file 指定数据")
	}
}
