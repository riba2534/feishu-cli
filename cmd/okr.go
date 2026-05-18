package cmd

import "github.com/spf13/cobra"

var okrCmd = &cobra.Command{
	Use:   "okr",
	Short: "OKR 操作命令",
	Long: `OKR 操作命令，用于查询 OKR 周期、进展记录。

子命令组:
  cycle      OKR 周期相关（list）
  progress   OKR 进展记录相关（list / create）

权限要求（User Token）:
  cycle list           okr:okr:readonly 或 okr:okr.period:readonly
  progress list        okr:okr:readonly 或 okr:okr.progress:readonly
  progress create      okr:okr 或 okr:okr.progress:writeonly

示例:
  # 查询当前租户的所有 OKR 周期（租户级全局列表）
  feishu-cli okr cycle list

  # 查询某个目标的所有进展记录
  feishu-cli okr progress list --objective-id 7123456789012345678

  # 为某个关键结果创建一条进展记录（纯文本）
  feishu-cli okr progress create \
    --key-result-id 7123456789012345678 \
    --content "本周完成核心模块联调"

提示:
  - OKR API 默认走 User Token，命令会自动读取 ~/.feishu-cli/token.json
  - 未登录或 token 过期时请先执行 feishu-cli auth login`,
}

var okrCycleCmd = &cobra.Command{
	Use:   "cycle",
	Short: "OKR 周期相关命令",
	Long: `OKR 周期相关命令。

子命令:
  list   获取当前租户的 OKR 周期列表

示例:
  feishu-cli okr cycle list`,
}

var okrProgressCmd = &cobra.Command{
	Use:   "progress",
	Short: "OKR 进展记录相关命令",
	Long: `OKR 进展记录相关命令。

子命令:
  list     获取某个目标 / 关键结果下的进展记录列表
  create   为某个目标 / 关键结果创建进展记录

示例:
  feishu-cli okr progress list --objective-id 7xxx
  feishu-cli okr progress create --key-result-id 7xxx --content "本周完成 X"`,
}

func init() {
	rootCmd.AddCommand(okrCmd)
	okrCmd.AddCommand(okrCycleCmd)
	okrCmd.AddCommand(okrProgressCmd)
}
