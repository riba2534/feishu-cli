package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/riba2534/feishu-cli/internal/registry"
	"github.com/spf13/cobra"
)

// writeJSON 把 v 编码为 2 空格缩进 JSON 写入 w（HTML 不转义）。
// 与全局 printJSON 等价但目标 writer 可注入，便于测试断言 JSON 输出。
func writeJSON(w io.Writer, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("JSON 序列化失败: %w", err)
	}
	_, err := w.Write(buf.Bytes())
	return err
}

var (
	schemaFormat string
)

func normalizeSchemaFormat(format string) (string, error) {
	if format == "" {
		return "pretty", nil
	}
	switch format {
	case "pretty", "json":
		return format, nil
	default:
		return "", fmt.Errorf("未知输出格式 %q（支持: pretty, json）", format)
	}
}

var schemaCmd = &cobra.Command{
	Use:   "schema [service.resource.method]",
	Short: "查询 OpenAPI Schema（path/verb/参数/scope）",
	Long: `查询飞书开放平台 OpenAPI 的方法 schema：HTTP 路径、动词、参数、请求体、响应体、scope。

数据来源：编译期 embed 的 internal/registry/meta_data.json 永远是离线 baseline；
运行时可从官方 public api_definition?protocol=meta 拉取 overlay（5s 超时、10MB 上限、24h TTL、无凭证）。
远端失败时静默回退 embedded。FEISHU_CLI_REMOTE_META=off 可关闭 overlay。

路径格式: <service>.<resource>.<method>
  - service:  域名（im / docs / drive / bitable / calendar / vc / mail / ...）
  - resource: 资源（messages / events / records / ...，可含 .，如 chat.members；支持嵌套 resources）
  - method:   动作（create / get / list / update / delete / ...）

子命令:
  schema list                列出所有可用 service
  schema list --service im   列出 im 域下所有 resource.method
  schema status              显示 catalog 来源（embedded/cache/runtime）、版本、service/method 数
  schema <path>              查询某个具体 method（默认 pretty 输出）
  schema <path> --format json   JSON 格式输出（AI Agent 推荐）

示例:
  feishu-cli schema list
  feishu-cli schema list --service im
  feishu-cli schema status --format json
  feishu-cli schema im.messages.delete
  feishu-cli schema im.messages.delete --format json
  feishu-cli schema calendar.events
  feishu-cli schema drive.files

注意事项:
  - 查询 schema 不需要 token；overlay 请求也不携带 App/User 凭证
  - 内置 baseline 覆盖 12 个 service；overlay 可能增加官方新增 service/method
  - 完整 OpenAPI 文档见 docUrl 字段或 https://open.feishu.cn/`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := ""
		if len(args) > 0 {
			path = args[0]
		}
		return runSchema(os.Stdout, path, schemaFormat)
	},
}

func init() {
	rootCmd.AddCommand(schemaCmd)
	schemaCmd.Flags().StringVar(&schemaFormat, "format", "pretty", "输出格式: pretty (默认) | json")
	schemaCmd.AddCommand(schemaStatusCmd)
	schemaStatusCmd.Flags().StringVar(&schemaFormat, "format", "pretty", "输出格式: pretty (默认) | json")
}

var schemaStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "显示 OpenAPI catalog 来源、版本与规模",
	Long: `显示当前进程使用的 OpenAPI catalog：

  source            embedded（仅编译期基线）/ cache（磁盘 overlay）/ runtime（本次远端拉取）
  embedded_version  编译进二进制的 meta 版本
  cache_version     本地 ~/.feishu-cli/cache/remote_meta.json 版本
  runtime_version   当前合并后实际使用的版本
  service_count / method_count  含嵌套 resource 的统计

overlay 从官方 public api_definition?protocol=meta 拉取，不携带凭证；失败回退 embedded。`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSchemaStatus(os.Stdout, schemaFormat)
	},
}

func runSchemaStatus(w io.Writer, format string) error {
	var err error
	format, err = normalizeSchemaFormat(format)
	if err != nil {
		return err
	}
	info := registry.Status()
	if format == "json" {
		return writeJSON(w, info)
	}
	fmt.Fprintln(w, formatCatalogPretty(info))
	return nil
}

func formatCatalogPretty(info registry.CatalogInfo) string {
	return fmt.Sprintf("Catalog: source=%s brand=%s embedded=%s runtime=%s cache=%s services=%d methods=%d remote=%v",
		info.Source, info.Brand, info.EmbeddedVersion, info.RuntimeVersion, info.CacheVersion,
		info.ServiceCount, info.MethodCount, info.RemoteEnabled)
}

// runSchema dispatches based on path depth.
//
//	"" / no args                 → list services
//	"service"                    → list resources & methods in service
//	"service.resource"           → list methods in resource
//	"service.resource.method"    → method detail
func runSchema(w io.Writer, path, format string) error {
	var err error
	format, err = normalizeSchemaFormat(format)
	if err != nil {
		return err
	}
	if path == "" {
		return printServices(w, format)
	}
	parts := strings.Split(path, ".")
	serviceName := parts[0]
	spec := registry.LoadFromMeta(serviceName)
	if spec == nil {
		available := strings.Join(registry.ListFromMetaProjects(), ", ")
		return fmt.Errorf("未知 service: %s\n可用 service: %s", serviceName, available)
	}
	if len(parts) == 1 {
		if format == "json" {
			return writeJSON(w, spec)
		}
		return printResourceList(w, spec)
	}
	resources, _ := spec["resources"].(map[string]interface{})
	resource, resName, remaining := findResourceByPath(resources, parts[1:])
	if resource == nil {
		var names []string
		for k := range resources {
			names = append(names, k)
		}
		sort.Strings(names)
		return fmt.Errorf("未知 resource: %s.%s\n可用 resource: %s",
			serviceName, strings.Join(parts[1:], "."), strings.Join(names, ", "))
	}
	if len(remaining) == 0 {
		if format == "json" {
			return writeJSON(w, resource)
		}
		return printResourceDetail(w, serviceName, resName, resource)
	}
	if len(remaining) > 1 {
		// remaining 多于 1 段时不能静默吞掉 — 视为用户输入路径过深
		return fmt.Errorf("路径过深: %s（期望格式 <service>.<resource>.<method>，多余片段: %s）",
			path, strings.Join(remaining[1:], "."))
	}
	methodName := remaining[0]
	methods, _ := resource["methods"].(map[string]interface{})
	method, ok := methods[methodName].(map[string]interface{})
	if !ok {
		var names []string
		for k := range methods {
			names = append(names, k)
		}
		sort.Strings(names)
		return fmt.Errorf("未知 method: %s.%s.%s\n可用 method: %s",
			serviceName, resName, methodName, strings.Join(names, ", "))
	}
	if format == "json" {
		return writeJSON(w, method)
	}
	return printMethodDetail(w, spec, resName, methodName, method)
}

// findResourceByPath tries longest-prefix match for dotted resource names.
// For example parts = ["chat", "members", "create"] should match resource
// "chat.members" then leave ["create"] as remaining (the method).
func findResourceByPath(resources map[string]interface{}, parts []string) (map[string]interface{}, string, []string) {
	for i := len(parts); i >= 1; i-- {
		candidate := strings.Join(parts[:i], ".")
		res, ok := resources[candidate]
		if !ok {
			continue
		}
		resMap, ok := res.(map[string]interface{})
		if !ok {
			continue
		}
		remaining := parts[i:]
		if len(remaining) > 0 {
			if nested, ok := resMap["resources"].(map[string]interface{}); ok && len(nested) > 0 {
				if sub, subName, subRem := findResourceByPath(nested, remaining); sub != nil {
					full := candidate
					if subName != "" {
						full = candidate + "." + subName
					}
					return sub, full, subRem
				}
			}
		}
		return resMap, candidate, remaining
	}
	return nil, "", nil
}

func printServices(w io.Writer, format string) error {
	services := registry.ListFromMetaProjects()
	info := registry.Status()
	if format == "json" {
		list := make([]map[string]interface{}, 0, len(services))
		for _, s := range services {
			spec := registry.LoadFromMeta(s)
			list = append(list, map[string]interface{}{
				"name":        s,
				"version":     registry.GetStrFromMap(spec, "version"),
				"title":       registry.GetStrFromMap(spec, "title"),
				"servicePath": registry.GetStrFromMap(spec, "servicePath"),
			})
		}
		return writeJSON(w, list)
	}
	fmt.Fprintln(w, formatCatalogPretty(info))
	fmt.Fprintln(w, "可用 service（共", len(services), "个）：")
	fmt.Fprintln(w)
	for _, s := range services {
		spec := registry.LoadFromMeta(s)
		title := registry.GetStrFromMap(spec, "title")
		version := registry.GetStrFromMap(spec, "version")
		resources, _ := spec["resources"].(map[string]interface{})
		fmt.Fprintf(w, "  %-12s %-6s %d resources  %s\n", s, version, len(resources), title)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "用法: feishu-cli schema <service>.<resource>.<method>")
	fmt.Fprintln(w, "示例: feishu-cli schema im.messages.delete")
	return nil
}

func printResourceList(w io.Writer, spec map[string]interface{}) error {
	name := registry.GetStrFromMap(spec, "name")
	version := registry.GetStrFromMap(spec, "version")
	title := registry.GetStrFromMap(spec, "title")
	servicePath := registry.GetStrFromMap(spec, "servicePath")

	fmt.Fprintf(w, "%s (%s) — %s\n", name, version, title)
	fmt.Fprintf(w, "Base path: %s\n\n", servicePath)

	resources, _ := spec["resources"].(map[string]interface{})
	printNamedResources(w, resources, "")
	fmt.Fprintf(w, "用法: feishu-cli schema %s.<resource>.<method>\n", name)
	return nil
}

func printNamedResources(w io.Writer, resources map[string]interface{}, prefix string) {
	for _, resName := range sortedKeys(resources) {
		resMap, _ := resources[resName].(map[string]interface{})
		full := resName
		if prefix != "" {
			full = prefix + "." + resName
		}
		methods, _ := resMap["methods"].(map[string]interface{})
		if len(methods) > 0 {
			fmt.Fprintf(w, "  %s\n", full)
			for _, methodName := range sortedKeys(methods) {
				m, _ := methods[methodName].(map[string]interface{})
				httpMethod := registry.GetStrFromMap(m, "httpMethod")
				desc := schemaTruncate(registry.GetStrFromMap(m, "description"), 70)
				danger := ""
				if d, _ := m["danger"].(bool); d {
					danger = " [danger]"
				}
				fmt.Fprintf(w, "    %-7s %-22s %s%s\n", httpMethod, methodName, desc, danger)
			}
			fmt.Fprintln(w)
		}
		if nested, ok := resMap["resources"].(map[string]interface{}); ok {
			printNamedResources(w, nested, full)
		}
	}
}

func printResourceDetail(w io.Writer, serviceName, resName string, resource map[string]interface{}) error {
	fmt.Fprintf(w, "%s.%s\n\n", serviceName, resName)
	methods, _ := resource["methods"].(map[string]interface{})
	for _, mName := range sortedKeys(methods) {
		m, _ := methods[mName].(map[string]interface{})
		httpMethod := registry.GetStrFromMap(m, "httpMethod")
		desc := schemaTruncate(registry.GetStrFromMap(m, "description"), 70)
		fmt.Fprintf(w, "  %-7s %-22s %s\n", httpMethod, mName, desc)
	}
	fmt.Fprintf(w, "\n用法: feishu-cli schema %s.%s.<method>\n", serviceName, resName)
	return nil
}

func printMethodDetail(w io.Writer, spec map[string]interface{}, resName, methodName string, method map[string]interface{}) error {
	servicePath := registry.GetStrFromMap(spec, "servicePath")
	specName := registry.GetStrFromMap(spec, "name")
	methodPath := registry.GetStrFromMap(method, "path")
	fullPath := servicePath + "/" + methodPath
	httpMethod := registry.GetStrFromMap(method, "httpMethod")
	desc := registry.GetStrFromMap(method, "description")

	fmt.Fprintf(w, "%s.%s.%s\n\n", specName, resName, methodName)
	fmt.Fprintf(w, "  %s %s\n", httpMethod, fullPath)
	if desc != "" {
		fmt.Fprintf(w, "  %s\n", desc)
	}
	fmt.Fprintln(w)

	// Parameters (URL/query)
	params, _ := method["parameters"].(map[string]interface{})
	if len(params) > 0 {
		fmt.Fprintln(w, "Parameters:")
		for _, p := range sortedParamKeys(params) {
			pm, _ := params[p].(map[string]interface{})
			pType := registry.GetStrFromMap(pm, "type")
			if pType == "" {
				pType = "string"
			}
			location := registry.GetStrFromMap(pm, "location")
			required, _ := pm["required"].(bool)
			reqStr := "optional"
			if required {
				reqStr = "required"
			}
			fmt.Fprintf(w, "  - %s (%s, %s, %s)\n", p, pType, location, reqStr)
			if pdesc := schemaTruncate(registry.GetStrFromMap(pm, "description"), 100); pdesc != "" {
				fmt.Fprintf(w, "      %s\n", pdesc)
			}
			if ex := registry.GetStrFromMap(pm, "example"); ex != "" {
				fmt.Fprintf(w, "      e.g. %s\n", ex)
			}
			printFieldEnum(w, pm, "      ")
		}
		fmt.Fprintln(w)
	}

	// requestBody (for POST/PUT/PATCH/DELETE)
	if httpMethod == "POST" || httpMethod == "PUT" || httpMethod == "PATCH" || httpMethod == "DELETE" {
		requestBody, _ := method["requestBody"].(map[string]interface{})
		if len(requestBody) > 0 {
			fmt.Fprintln(w, "Request Body:")
			printNestedFields(w, requestBody, "  ", "")
			fmt.Fprintln(w)
		}
	}

	// responseBody
	responseBody, _ := method["responseBody"].(map[string]interface{})
	if len(responseBody) > 0 {
		fmt.Fprintln(w, "Response:")
		printNestedFields(w, responseBody, "  ", "")
		fmt.Fprintln(w)
	}

	// accessTokens / identities
	if tokens, ok := method["accessTokens"].([]interface{}); ok && len(tokens) > 0 {
		var idents []string
		for _, t := range tokens {
			if s, ok := t.(string); ok {
				switch s {
				case "user":
					idents = append(idents, "user")
				case "tenant":
					idents = append(idents, "tenant (bot)")
				}
			}
		}
		if len(idents) > 0 {
			fmt.Fprintf(w, "Identity: %s\n", strings.Join(idents, ", "))
		}
	}

	// scopes
	if scopes, ok := method["scopes"].([]interface{}); ok && len(scopes) > 0 {
		var ss []string
		for _, s := range scopes {
			if str, ok := s.(string); ok {
				ss = append(ss, str)
			}
		}
		fmt.Fprintf(w, "Scopes:   %s\n", strings.Join(ss, ", "))
	}

	// CLI example
	fmt.Fprintf(w, "CLI:      feishu-cli schema %s.%s.%s\n", specName, resName, methodName)

	// Docs
	if docURL := registry.GetStrFromMap(method, "docUrl"); docURL != "" {
		fmt.Fprintf(w, "Docs:     %s\n", docURL)
	}

	return nil
}

func printNestedFields(w io.Writer, fields map[string]interface{}, indent, prefix string) {
	for _, fieldName := range sortedFieldKeys(fields) {
		f, _ := fields[fieldName].(map[string]interface{})
		fullName := fieldName
		if prefix != "" {
			fullName = prefix + "." + fieldName
		}
		fType := registry.GetStrFromMap(f, "type")
		required, _ := f["required"].(bool)
		reqStr := "optional"
		if required {
			reqStr = "required"
		}
		fmt.Fprintf(w, "%s- %s (%s, %s)\n", indent, fullName, fType, reqStr)
		if desc := schemaTruncate(registry.GetStrFromMap(f, "description"), 100); desc != "" {
			fmt.Fprintf(w, "%s    %s\n", indent, desc)
		}
		if ex := registry.GetStrFromMap(f, "example"); ex != "" {
			fmt.Fprintf(w, "%s    e.g. %s\n", indent, ex)
		}
		printFieldEnum(w, f, indent+"    ")
		if props, ok := f["properties"].(map[string]interface{}); ok && len(props) > 0 {
			printNestedFields(w, props, indent+"  ", fullName)
		}
	}
}

// printFieldEnum 渲染字段的枚举取值：优先用 options（含 value + description），
// 回退 enum（纯值列表）。对应 meta_data.json 的 options/enum 字段（数据来自飞书
// 官方归一化端点，已含枚举描述），pretty 模式此前未渲染、白白丢弃。
// value/enum 成员既可能是字符串也可能是数字（如整数型 type 枚举），统一转字符串渲染。
func printFieldEnum(w io.Writer, f map[string]interface{}, indent string) {
	if opts, ok := f["options"].([]interface{}); ok && len(opts) > 0 {
		var parts []string
		for _, o := range opts {
			om, ok := o.(map[string]interface{})
			if !ok {
				continue
			}
			val := scalarToString(om["value"])
			if val == "" {
				continue
			}
			if desc := registry.GetStrFromMap(om, "description"); desc != "" {
				parts = append(parts, val+" ("+desc+")")
			} else {
				parts = append(parts, val)
			}
		}
		if len(parts) > 0 {
			fmt.Fprintf(w, "%senum: %s\n", indent, strings.Join(parts, ", "))
		}
		return
	}
	if enum, ok := f["enum"].([]interface{}); ok && len(enum) > 0 {
		var parts []string
		for _, e := range enum {
			if s := scalarToString(e); s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) > 0 {
			fmt.Fprintf(w, "%senum: %s\n", indent, strings.Join(parts, ", "))
		}
	}
}

// scalarToString 把一个 JSON 标量（字符串/数字/布尔）渲染成字符串。
// 整数型数字（如枚举值 0/1/2）去掉 JSON 解析引入的 .0 小数尾巴。非标量返回空串。
func scalarToString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case json.Number:
		return t.String()
	case float64:
		if t == math.Trunc(t) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'g', -1, 64)
	default:
		return ""
	}
}

// sortedKeys returns map keys in alphabetical order.
func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortedParamKeys returns parameter keys: required first, then alphabetical.
func sortedParamKeys(params map[string]interface{}) []string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		pi, _ := params[keys[i]].(map[string]interface{})
		pj, _ := params[keys[j]].(map[string]interface{})
		ri, _ := pi["required"].(bool)
		rj, _ := pj["required"].(bool)
		if ri != rj {
			return ri
		}
		return keys[i] < keys[j]
	})
	return keys
}

// sortedFieldKeys returns field keys: required first, then alphabetical.
func sortedFieldKeys(fields map[string]interface{}) []string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		fi, _ := fields[keys[i]].(map[string]interface{})
		fj, _ := fields[keys[j]].(map[string]interface{})
		ri, _ := fi["required"].(bool)
		rj, _ := fj["required"].(bool)
		if ri != rj {
			return ri
		}
		return keys[i] < keys[j]
	})
	return keys
}

// schemaTruncate returns s truncated to maxLen chars with "..." appended if cut.
// Works on runes to avoid breaking multibyte UTF-8 (中文 description 常见).
func schemaTruncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
