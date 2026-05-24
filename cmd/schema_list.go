package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/riba2534/feishu-cli/internal/registry"
	"github.com/spf13/cobra"
)

var (
	schemaListService string
	schemaListFormat  string
)

var schemaListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出所有 service 或某个 service 下的 resource.method",
	Long: `列出可用的 OpenAPI service / resource / method。

不传 --service 时列出所有顶层 service（同 feishu-cli schema 无参数）。
传 --service 时列出该 service 下的所有 resource.method（同 feishu-cli schema <service>）。

示例:
  feishu-cli schema list
  feishu-cli schema list --service im
  feishu-cli schema list --service drive --format json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSchemaList(os.Stdout, schemaListService, schemaListFormat)
	},
}

func init() {
	schemaCmd.AddCommand(schemaListCmd)
	schemaListCmd.Flags().StringVar(&schemaListService, "service", "", "可选：指定 service 名（如 im / drive / calendar）")
	schemaListCmd.Flags().StringVar(&schemaListFormat, "format", "pretty", "输出格式: pretty (默认) | json")
}

func runSchemaList(w io.Writer, service, format string) error {
	var err error
	format, err = normalizeSchemaFormat(format)
	if err != nil {
		return err
	}
	if service == "" {
		return printServices(w, format)
	}
	spec := registry.LoadFromMeta(service)
	if spec == nil {
		available := strings.Join(registry.ListFromMetaProjects(), ", ")
		return fmt.Errorf("未知 service: %s\n可用 service: %s", service, available)
	}
	if format == "json" {
		// Return flat list of {service, resource, method, httpMethod, description}
		var rows []map[string]interface{}
		resources, _ := spec["resources"].(map[string]interface{})
		for _, resName := range sortedKeys(resources) {
			resMap, _ := resources[resName].(map[string]interface{})
			methods, _ := resMap["methods"].(map[string]interface{})
			for _, mName := range sortedKeys(methods) {
				m, _ := methods[mName].(map[string]interface{})
				rows = append(rows, map[string]interface{}{
					"service":     service,
					"resource":    resName,
					"method":      mName,
					"path":        service + "." + resName + "." + mName,
					"httpMethod":  registry.GetStrFromMap(m, "httpMethod"),
					"description": registry.GetStrFromMap(m, "description"),
				})
			}
		}
		return writeJSON(w, rows)
	}
	return printResourceList(w, spec)
}
