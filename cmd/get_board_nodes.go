package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/spf13/cobra"
)

var getBoardNodesCmd = &cobra.Command{
	Use:   "nodes <whiteboard_id>",
	Short: "获取画板节点列表",
	Long: `获取指定画板的所有节点信息。

示例:
  feishu-cli board nodes <whiteboard_id>`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		whiteboardID := args[0]

		rawJSON, err := client.GetBoardNodes(whiteboardID)
		if err != nil {
			return err
		}

		// 格式化 JSON 输出
		var parsed any
		if err := json.Unmarshal(rawJSON, &parsed); err != nil {
			// 解析失败则直接输出原始内容
			fmt.Println(string(rawJSON))
			return nil
		}

		return printJSON(parsed)
	},
}

func init() {
	boardCmd.AddCommand(getBoardNodesCmd)
}
