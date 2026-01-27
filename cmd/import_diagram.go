package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var importDiagramCmd = &cobra.Command{
	Use:   "import <whiteboard_id> <source>",
	Short: "导入图表到画板",
	Long: `将 PlantUML 或 Mermaid 图表导入到飞书画板。

参数:
  <whiteboard_id>    画板 ID（必填）
  <source>           图表代码或文件路径（必填）
  --source-type      源类型：file/content，默认 file
  --syntax           图表语法：plantuml/mermaid，默认 plantuml
  --diagram-type     图表类型：auto/mindmap/sequence/activity/class/er/flowchart/usecase/component，默认 auto
  --style            样式类型：board/classic，默认 board
  --output, -o       输出格式 (json)

图表语法:
  plantuml    PlantUML 语法（默认）
  mermaid     Mermaid 语法

样式类型:
  board       画板风格（默认）
  classic     经典风格

示例:
  # 从文件导入 PlantUML 图表
  feishu-cli board import <whiteboard_id> diagram.puml

  # 导入 Mermaid 图表
  feishu-cli board import <whiteboard_id> diagram.mmd --syntax mermaid

  # 直接导入图表代码
  feishu-cli board import <whiteboard_id> "@startuml\nA -> B: hello\n@enduml" --source-type content

  # 使用经典样式
  feishu-cli board import <whiteboard_id> diagram.puml --style classic`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		whiteboardID := args[0]
		source := args[1]
		sourceType, _ := cmd.Flags().GetString("source-type")
		syntax, _ := cmd.Flags().GetString("syntax")
		diagramType, _ := cmd.Flags().GetString("diagram-type")
		style, _ := cmd.Flags().GetString("style")
		output, _ := cmd.Flags().GetString("output")

		opts := client.ImportDiagramOptions{
			SourceType:  sourceType,
			Syntax:      syntax,
			DiagramType: diagramType,
			Style:       style,
		}

		result, err := client.ImportDiagram(whiteboardID, source, opts)
		if err != nil {
			return err
		}

		if output == "json" {
			data, _ := json.MarshalIndent(map[string]interface{}{
				"whiteboard_id": whiteboardID,
				"ticket_id":     result.TicketID,
				"syntax":        syntax,
				"style":         style,
			}, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("图表导入成功！\n")
			fmt.Printf("  画板 ID: %s\n", whiteboardID)
			if result.TicketID != "" {
				fmt.Printf("  票据 ID: %s\n", result.TicketID)
			}
			fmt.Printf("  语法: %s\n", syntax)
			fmt.Printf("  样式: %s\n", style)
		}

		return nil
	},
}

func init() {
	boardCmd.AddCommand(importDiagramCmd)
	importDiagramCmd.Flags().String("source-type", "file", "源类型 (file/content)")
	importDiagramCmd.Flags().String("syntax", "plantuml", "图表语法 (plantuml/mermaid)")
	importDiagramCmd.Flags().String("diagram-type", "auto", "图表类型 (auto/mindmap/sequence/activity/class/er/flowchart/usecase/component)")
	importDiagramCmd.Flags().String("style", "board", "样式类型 (board/classic)")
	importDiagramCmd.Flags().StringP("output", "o", "", "输出格式 (json)")
}
