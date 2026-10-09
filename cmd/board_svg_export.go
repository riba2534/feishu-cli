package cmd

import (
	"fmt"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/riba2534/feishu-cli/v2/internal/safefile"
	"github.com/spf13/cobra"
)

var boardSVGExportCmd = &cobra.Command{
	Use:   "svg-export <whiteboard_id>",
	Short: "导出画板为 SVG（服务端整板渲染视觉快照）",
	Long: `调用 POST /board/v1/whiteboards/{id}/export（export_type=svg），返回服务端整板渲染的 SVG。

与 board export-code 的区别：
  - export-code 仅提取 svg 节点的 svg_code 拼接，对 mermaid/plantuml/原生节点无效
  - svg-export 由服务端整板渲染，对任意画板有效，产出可二次编辑的完整 SVG

适用场景:
  - 导出任意画板为 SVG 做版本管理 / 离线预览
  - 配合 board import / svg_to_board.py 实现「导出 → 编辑 → 回写」闭环

示例:
  feishu-cli board svg-export <id>                          # 输出到 stdout
  feishu-cli board svg-export <id> --output-path board.svg
  feishu-cli board svg-export <id> --output-path board.svg --overwrite`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}
		whiteboardID := args[0]
		outputPath, _ := cmd.Flags().GetString("output-path")
		overwrite, _ := cmd.Flags().GetBool("overwrite")
		if outputPath != "" {
			// 输出路径在任何网络请求（含 token 刷新）之前校验，敏感目录直接拒绝
			if err := validateOutputPath(outputPath, ""); err != nil {
				return err
			}
			// 已存在且未 --overwrite 同样在请求之前拒绝（写入前还会再检查一次）
			if err := ensureBoardExportWritable(outputPath, overwrite); err != nil {
				return err
			}
		}
		userAccessToken := resolveOptionalUserTokenWithFallback(cmd)

		result, err := client.ExportWhiteboardSVG(whiteboardID, userAccessToken)
		if err != nil {
			return err
		}

		if outputPath == "" {
			fmt.Print(result.SVG)
			return nil
		}
		if err := ensureBoardExportWritable(outputPath, overwrite); err != nil {
			return err
		}
		if err := safefile.AtomicWriteFile(outputPath, []byte(result.SVG), 0o644); err != nil {
			return fmt.Errorf("写文件失败: %w", err)
		}
		fmt.Printf("画板 %s 已导出为 SVG → %s（%d 字节）\n", whiteboardID, outputPath, len(result.SVG))
		return nil
	},
}

func init() {
	boardCmd.AddCommand(boardSVGExportCmd)
	boardSVGExportCmd.Flags().String("output-path", "", "输出文件路径（不指定则打印到 stdout）")
	boardSVGExportCmd.Flags().Bool("overwrite", false, "覆盖已存在的输出文件")
	boardSVGExportCmd.Flags().String("user-access-token", "", "User Access Token")
}
