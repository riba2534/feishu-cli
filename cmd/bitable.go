package cmd

import (
	"github.com/spf13/cobra"
)

var bitableCmd = &cobra.Command{
	Use:     "bitable",
	Aliases: []string{"base"},
	Short:   "多维表格（Base/Bitable）操作",
	Long: `多维表格操作命令组，底层调用飞书 base/v3 API（/open-apis/base/v3/bases/{base_token}/...）。

⚠️ 重要：本命令已从旧 bitable/v1 API 切换到 base/v3 API，支持的能力大幅增强：
  - 视图完整配置读写（filter/sort/group/visible-fields/timebar/card）
  - 记录 upsert + 修改历史 + 附件上传/下载/移除
  - 角色完整 CRUD + 协作者管理 + 高级权限
  - 工作流 CRUD + 启停
  - 仪表盘 / 表单 CRUD

命令分为以下子组：
  bitable <create|get|copy|update>      基础：创建/获取/复制/更新（重命名·高级权限）多维表格
  bitable table <list|get|create|...>   数据表 CRUD
  bitable field <list|get|create|...>   字段 CRUD + search-options
  bitable record <list|get|search|...>  记录 CRUD + upsert + batch + history + *-attachment
  bitable view <list|get|create|...>    视图 CRUD + rename
  bitable view-<filter|sort|group|visible-fields|timebar|card> <get|set>  视图配置
  bitable role <list|get|create|...>    角色 CRUD
  bitable role member <list|create|delete|batch-create|batch-delete>  角色协作者管理
  bitable advperm <enable|disable>      高级权限开关
  bitable data-query                    数据聚合查询
  bitable workflow <list|get|create|update|enable|disable>  工作流 CRUD + 启停
  bitable dashboard <list|get|create|...>  仪表盘 CRUD + copy + block
  bitable form <list|get|create|patch|...> 表单 CRUD + field + detail/submit
  bitable field extension <get|update|update-cells>  字段 AI 扩展（LLM 生成）
  bitable field button-rule <bind|get|unbind>        按钮字段绑定工作流
  bitable table <copy|copy-status>      复制单张数据表
  bitable template <categories|list|search>  模板中心（只读）
  bitable block <list|create|move|rename|delete>  顶层块（数据表/仪表盘/工作流/文件夹/文档）
  bitable resolve                       解析多维表格 / 知识库 / 分享 / BaseApp 链接

全部子命令支持 --dry-run（只打印请求，不需要凭据）；除 create/get/copy/record download-attachment 外支持 --format/--jq。
参数错误退出码 2；block delete、field extension update/update-cells 等高危写需 --yes（非交互缺失时退出码 10）。

身份选择 --as（底层 API 同时支持 User / Tenant 身份）：
  auto  默认。User 优先、Tenant 兜底——已登录用 User Token，未登录/过期自动回落 App Token
  bot   强制 App Token（无需 auth login，永不过期，适合 cron 无人值守）
  user  强制 User Token（缺失报错，先 feishu-cli auth login）
使用 --base-token 传入多维表格 token（从 URL 里的 /base/{token} 片段获取）。

示例:
  feishu-cli bitable create --name "项目管理"
  feishu-cli bitable table list --base-token bscnxxxx
  feishu-cli bitable record list --base-token bscnxxxx --table-id tblxxx
  feishu-cli bitable view view-sort-set --base-token bscnxxxx --table-id tblxxx --view-id viewxxx --config '[{"field_id":"fld1","desc":false}]'`,
}

func init() {
	rootCmd.AddCommand(bitableCmd)
	// --as 身份选择：persistent flag，所有 bitable 子命令继承。
	// base/v3 与 bitable/v1 API 本身同时支持 User / Tenant 身份，
	// 默认 auto（User 优先、Tenant 兜底）让未登录/cron 场景自动用 App Token。
	bitableCmd.PersistentFlags().String("as", "auto",
		"身份: bot(App Token) | user(User Token) | auto(User 优先 Tenant 兜底, 默认)")
}
