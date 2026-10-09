package cmd

import "github.com/spf13/cobra"

var okrCmd = &cobra.Command{
	Use:   "okr",
	Short: "OKR 操作命令",
	Long: `OKR 操作命令，用于查询 OKR 周期、进展记录。

子命令组:
  cycle         OKR 周期相关（list / detail）
  progress      OKR 进展记录相关（list / get / create / update / delete）
  objective     OKR 目标写入（create / update，v2）
  key-result    OKR 关键结果写入（create / update，v2）
  comment       OKR 评论（list / create）
  upload-image  上传进展图片素材

身份（--as，所有子命令继承）:
  bot（默认）  App/Tenant Token，无需 auth login，适合 cron 无人值守；
              scope 需在飞书应用后台开通（缺时报 99991672 + 给出申请链接）
  user        User Token（需 auth login 时带对应 okr scope，缺 scope 时服务端报 99991679）
  auto        User 优先、Tenant 兜底

权限要求（bot 走 tenant scope，user 走同名 user scope）:
  cycle list            okr:okr.period:readonly（v2 用户周期；--tenant 走 v1 时 okr:okr:readonly 亦可）
  cycle detail          okr:okr.content:readonly（实测只接受该 scope，okr:okr:readonly 不生效）
  progress list / get   okr:okr.progress:readonly（实测只接受该 scope；User/Bot 均可）
  progress create/update  okr:okr 或 okr:okr.progress:writeonly
  progress delete       okr:okr 或 okr:okr.progress:delete
  upload-image          okr:okr 或 okr:okr.progress.file:upload
  objective/key-result  okr:okr.content:writeonly
  comment list/create   okr:okr.comment.readonly / okr:okr.comment.writeonly（create 必需 User Token）

示例:
  # 查询自己的 OKR 周期（用户周期 ID，可直接用于 cycle detail）
  feishu-cli okr cycle list

  # 查询周期下全部目标 + 关键结果
  feishu-cli okr cycle detail 7123456789012345678

  # 查询某个目标的所有进展记录
  feishu-cli okr progress list --objective-id 7123456789012345678

  # 为某个关键结果创建一条进展记录（纯文本）
  feishu-cli okr progress create \
    --key-result-id 7123456789012345678 \
    --content "本周完成核心模块联调"

  # 以用户身份操作（需登录时带 okr scope）
  feishu-cli okr progress create --as user --objective-id 7xxx --content "..."`,
}

var okrCycleCmd = &cobra.Command{
	Use:   "cycle",
	Short: "OKR 周期相关命令",
	Long: `OKR 周期相关命令。

子命令:
  list     获取用户的 OKR 周期列表（v2 用户周期）
  detail   周期详情：全部目标 + 关键结果

示例:
  feishu-cli okr cycle list
  feishu-cli okr cycle detail <cycle_id>`,
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

	// --as 身份选择：persistent flag，所有 okr 子命令继承。
	// 默认 bot（保持 App Token 既有行为）：OKR 的 user scope 通常未随默认登录域授予，
	// 显式 --as user / auto 才切用户身份。
	okrCmd.PersistentFlags().String("as", "bot",
		"身份: bot(App Token, 默认) | user(User Token) | auto(User 优先 Tenant 兜底)")
	okrCmd.PersistentFlags().String("user-access-token", "", "User Access Token（--as user/auto 时优先使用）")
}
