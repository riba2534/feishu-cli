package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
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

数据来源于内置 internal/registry/meta_data.json（编译期 embed），无需网络也无需 token。

路径格式: <service>.<resource>.<method>
  - service:  域名（im / docs / drive / bitable / calendar / vc / mail / ...）
  - resource: 资源（messages / events / records / ...，可含 .，如 chat.members）
  - method:   动作（create / get / list / update / delete / ...）

子命令:
  schema list                列出所有可用 service
  schema list --service im   列出 im 域下所有 resource.method
  schema <path>              查询某个具体 method（默认 pretty 输出）
  schema <path> --format json   JSON 格式输出（AI Agent 推荐）

示例:
  feishu-cli schema list
  feishu-cli schema list --service im
  feishu-cli schema im.messages.delete
  feishu-cli schema im.messages.delete --format json
  feishu-cli schema calendar.events
  feishu-cli schema drive.files

注意事项:
  - 不需要 token，纯本地查询
  - 内置 schema 覆盖 12 个 service（approval/attendance/calendar/drive/im/mail/minutes/sheets/slides/task/vc/wiki）
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
		if res, ok := resources[candidate]; ok {
			if resMap, ok := res.(map[string]interface{}); ok {
				return resMap, candidate, parts[i:]
			}
		}
	}
	return nil, "", nil
}

func printServices(w io.Writer, format string) error {
	services := registry.ListFromMetaProjects()
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
	for _, resName := range sortedKeys(resources) {
		resMap, _ := resources[resName].(map[string]interface{})
		methods, _ := resMap["methods"].(map[string]interface{})
		if len(methods) == 0 {
			continue
		}
		fmt.Fprintf(w, "  %s\n", resName)
		for _, methodName := range sortedKeys(methods) {
			m, _ := methods[methodName].(map[string]interface{})
			httpMethod := registry.GetStrFromMap(m, "httpMethod")
			desc := registry.GetStrFromMap(m, "description")
			desc = schemaTruncate(desc, 70)
			danger := ""
			if d, _ := m["danger"].(bool); d {
				danger = " [danger]"
			}
			fmt.Fprintf(w, "    %-7s %-22s %s%s\n", httpMethod, methodName, desc, danger)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "用法: feishu-cli schema %s.<resource>.<method>\n", name)
	return nil
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
		if props, ok := f["properties"].(map[string]interface{}); ok && len(props) > 0 {
			printNestedFields(w, props, indent+"  ", fullName)
		}
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
