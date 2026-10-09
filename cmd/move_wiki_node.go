package cmd

import (
	"fmt"
	"strings"

	"github.com/riba2534/feishu-cli/v2/internal/client"
	"github.com/riba2534/feishu-cli/v2/internal/clierr"
	"github.com/riba2534/feishu-cli/v2/internal/config"
	"github.com/spf13/cobra"
)

var moveWikiNodeCmd = &cobra.Command{
	Use:   "move <node_token>",
	Short: "移动知识库节点",
	Long: `移动知识库节点到指定位置，支持跨知识空间移动。

如果节点有子节点，会携带子节点一起移动。

参数:
  node_token       节点 Token（必填，也接受挂载在知识库中的文档 obj_token）
  --target-space   目标知识空间 ID（未传 --target-parent 时必填；传了 --target-parent 时可省略，自动取父节点所在空间）
  --target-parent  目标父节点 Token（可选，不指定则移动到根目录；也接受文档 obj_token）

校验:
  传了 --target-parent 时，会先通过 node_by_token 解析父节点；若 --target-space 与父节点实际所在空间不一致，
  直接报错拒绝移动（不会把节点移到意料之外的空间）。

权限要求:
  - 节点编辑权限
  - 原父节点容器编辑权限
  - 目标父节点容器编辑权限

示例:
  # 移动到同一空间的根目录
  feishu-cli wiki move wikcnXXXXXX --target-space 7012345678901234567

  # 移动到同一空间的指定父节点下
  feishu-cli wiki move wikcnXXXXXX --target-space 7012345678901234567 --target-parent wikcnYYYYYY

  # 跨空间移动
  feishu-cli wiki move wikcnXXXXXX --target-space 7098765432109876543

  # 只给目标父节点，空间自动取父节点所在空间
  feishu-cli wiki move wikcnXXXXXX --target-parent wikcnYYYYYY

  # JSON 格式输出
  feishu-cli wiki move wikcnXXXXXX --target-space 7012345678901234567 --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.Validate(); err != nil {
			return err
		}

		nodeToken, err := extractWikiLookupToken(args[0])
		if err != nil {
			return err
		}
		targetSpace, _ := cmd.Flags().GetString("target-space")
		targetParent, _ := cmd.Flags().GetString("target-parent")
		output, _ := cmd.Flags().GetString("output")

		targetSpace = strings.TrimSpace(targetSpace)
		targetParent = strings.TrimSpace(targetParent)
		if targetSpace == "" && targetParent == "" {
			return clierr.Usagef("--target-space 与 --target-parent 至少传一个（只传 --target-parent 时自动取父节点所在空间）")
		}

		// 先通过 node_by_token 获取节点信息（当前 space_id 与真实 node_token）。
		// 输入可能是挂载在知识库中的文档 obj_token，移动接口必须使用响应里的 node_token。
		token := resolveOptionalUserToken(cmd)
		node, err := client.ResolveWikiNode(nodeToken, token)
		if err != nil {
			return fmt.Errorf("获取节点信息失败: %w", err)
		}
		if strings.TrimSpace(node.SpaceID) == "" || strings.TrimSpace(node.NodeToken) == "" {
			return fmt.Errorf("node_by_token 未返回节点的 space_id / node_token，已中止移动")
		}

		targetSpace, targetParent, err = resolveWikiMoveTarget(targetSpace, targetParent, token)
		if err != nil {
			return err
		}

		result, err := client.MoveWikiNode(node.SpaceID, node.NodeToken, targetSpace, targetParent, token)
		if err != nil {
			return err
		}

		if output == "json" {
			if err := printJSON(result); err != nil {
				return err
			}
		} else {
			fmt.Printf("知识库节点移动成功！\n")
			fmt.Printf("  节点 Token:     %s\n", result.NodeToken)
			fmt.Printf("  目标空间 ID:    %s\n", targetSpace)
			if targetParent != "" {
				fmt.Printf("  目标父节点:     %s\n", targetParent)
			} else {
				fmt.Printf("  目标位置:       根目录\n")
			}
		}

		return nil
	},
}

// wikiMyLibrarySpaceID 是"个人文档库"的空间别名，仅对 User 身份有意义。
const wikiMyLibrarySpaceID = "my_library"

// resolveWikiMoveTarget 在发起移动前核对目标位置（对齐官方 +move 的 resolveWikiNodeMoveSpaces）：
//   - 传了 --target-parent：经 node_by_token 解析父节点，换算为真实 node_token；
//     --target-space 为空时取父节点所在空间，不为空时必须与父节点所在空间一致，否则拒绝；
//   - --target-space 为 my_library 别名时，先解析出真实 space_id 再比较。
func resolveWikiMoveTarget(targetSpace, targetParent, token string) (string, string, error) {
	if targetParent == "" {
		return targetSpace, "", nil
	}
	parent, err := client.ResolveWikiNode(targetParent, token)
	if err != nil {
		return "", "", fmt.Errorf("获取目标父节点信息失败: %w", err)
	}
	parentSpace := strings.TrimSpace(parent.SpaceID)
	parentNode := strings.TrimSpace(parent.NodeToken)
	if parentSpace == "" || parentNode == "" {
		return "", "", fmt.Errorf("node_by_token 未返回目标父节点的 space_id / node_token，已中止移动")
	}
	if targetSpace == "" {
		return parentSpace, parentNode, nil
	}
	effective := targetSpace
	if targetSpace == wikiMyLibrarySpaceID {
		space, err := client.GetWikiSpace(wikiMyLibrarySpaceID, token)
		if err != nil {
			return "", "", fmt.Errorf("解析 --target-space my_library 失败（该别名仅支持 User 身份）: %w", err)
		}
		effective = strings.TrimSpace(space.SpaceID)
	}
	if effective != parentSpace {
		return "", "", clierr.Usagef("--target-space %q 与目标父节点 %s 实际所在空间 %q 不一致，已拒绝移动；请核对，或省略 --target-space 由父节点自动推断", targetSpace, targetParent, parentSpace)
	}
	return parentSpace, parentNode, nil
}

func init() {
	wikiCmd.AddCommand(moveWikiNodeCmd)
	moveWikiNodeCmd.Flags().String("target-space", "", "目标知识空间 ID（未传 --target-parent 时必填）")
	moveWikiNodeCmd.Flags().String("target-parent", "", "目标父节点 Token（可选）")
	moveWikiNodeCmd.Flags().StringP("output", "o", "", "输出格式（json）")
	moveWikiNodeCmd.Flags().String("user-access-token", "", "User Access Token（可选，用于访问个人知识库）")
}
