# 身份选择与预检

本文件是全部 feishu-cli 技能身份规则的维护入口；具体命令是否接受某个 flag、`--as` 默认值是什么，
以当前二进制 `feishu-cli <cmd> --help` 为准。先看用户要求的身份、目标资源与所选 App，再读取本次任务所需的工作流。

## 预检的含义

- `auth check --scope` 只检查选中 profile 的本地 User Token（token.json）的 scope 与有效期；不验证 Bot、
  资源权限或显式传入的 User Token。缺失或未登录时退出码 3。
- 应用侧是否开通：`auth scopes --scope "..." -o json`（以应用身份查询开放平台配置）。逐项看 `checks[].diagnosis`：
  `app_not_enabled`（User/Tenant 都没开）或 `tenant_only`（只对 Bot 开）需在开放平台开通并发布；
  `user_not_granted` 需 `auth login --scope` 补授；`not_logged_in` 需登录；`ok` 表示两侧都满足。
  Bot 路径看 `checks[].tenant_enabled`。
- Bot 凭证：`doctor --only bot_identity` 验证 App 凭证能否换取 Tenant Token；能否访问具体资源仍以 API 返回为准。
- 显式 `--user-access-token` / `FEISHU_USER_ACCESS_TOKEN`：不把本地 token 的预检结果当作它的授权证明。
- `--dry-run` 成功只表示本地校验与请求构造完成，不代表已访问飞书或线上有权限。

## 解析模式（命令没有 `--as`，或不传 `--as` 时）

| 模式 | 典型命令 | 行为 |
|---|---|---|
| 读类：User 优先，可回退 Bot | doc read/export、msg list/get/mget/thread-messages/read-users/resource-download、chat list、task get/list、calendar get/list/get-event/list-events/freebusy、file meta/stats/download、wiki 读、board 读、slides get；以及不传 `--as` 的 sheet、drive pull/push/status、file list | 显式 flag → `FEISHU_USER_ACCESS_TOKEN` → profile token.json → config 的 `user_access_token` → Bot。已配置 User 但损坏、未绑定 App 或刷新失败时，stderr 告警后改用 Bot；此时结果是 Bot 视角（可能为空或无权限），不要当作本人的结果 |
| 写类：默认 Bot | doc create/import/add/content-update、msg send/reply/forward/delete、comment reply、slides 写命令、file/wiki 写命令等 | 不读 token.json；仅显式 `--user-access-token` 或 `FEISHU_USER_ACCESS_TOKEN` 时切到用户。日历/任务写命令例外（auto，见下表） |
| 必须 User | search docs/apps、approval 全部、apps 全部、task my/search/related、calendar rsvp/event-reply、msg flag、vc note transcript、mail 写/规则/签名/模板/thread-modify/thread-trash、drive add-comment/search/secure-label/apply-permission、file quota、wiki space-create、user search `--query`、user search-bot、okr comment create | 无可用 User Token 时报错（退出码 3），不回退 Bot。drive upload/download 不传 `--as` 时同样要求 User，可显式 `--as bot` |
| 仅 Bot | vc bot meeting-join/meeting-leave | 不读任何 User Token；显式传 `--user-access-token` 报用法错误（退出码 2） |
| 仅 Bot（忽略 User Token） | msg merge-forward | 传入的 User Token（flag 或环境变量）被忽略并在 stderr 提示，仍以 Bot 执行 |
| 固定 App | chat create/link、msg edit/urgent、user list、dept get/children | 没有 User Token 参数，只能用应用身份 |

`drive pull --delete-local` / `drive push --delete-remote` 是读类回退的例外：已配置 User 但不可用时直接报错
（Bot 视角的远端更少，会误删）；只有从未配置 User 的纯 Bot 场景才以 Bot 执行。
不能根据命令组前缀推导所有子命令的身份（如 `msg history` 的单聊入口与群聊入口不同、`okr comment create` 忽略 `--as`）。

## 支持 --as 的入口

| 命令 | 默认 | 特别要求 |
|---|---|---|
| bitable 全部 | auto | 定时/无人值守任务显式 `--as bot` |
| okr 全部 | bot | `cycle list` 默认查 v2 用户周期（两种身份都支持），`--tenant` 查租户周期仅 Bot；部分端点只收 Bot（User 报 99991668，见各命令 `--help` 的权限说明）；`comment create` 忽略 `--as`，必须 User |
| perm 全部 | 不传 = Bot | 操作用户自己的文档用 `--as user` 或显式 `--user-access-token`；不传 `--as` 时不读 `FEISHU_USER_ACCESS_TOKEN`。Bot 遇 1063002/1063004 会提示改用 User |
| calendar create-event/update-event/delete-event/attendee add·remove/event-share/event-transfer、task/tasklist 写命令（含 set-ancestor） | auto | 日程与任务是个人资源；操作应用自己的日历/任务时显式 `--as bot` |
| calendar agenda、calendar event-search | auto | primary 日历与当前身份对应，不能混用两种身份的日历 ID |
| msg reaction add/list/remove、msg pin/unpin/pins、chat get/update/delete | auto | reaction remove 只能删除同一身份添加的表情 |
| chat member list/add/remove、msg history（`--container-id`）、wiki space-list | auto（告警回退） | 已配置 User 但不可用时 stderr 告警后改用 Bot（不 fail-closed）。外部群推荐 `--as bot`（需对外共享能力 App 且 Bot 已入群）。`msg history --user-id/--user-email`（单聊）始终需要 User |
| wiki delete、wiki delete-space、wiki node-copy | auto | 已配置 User 但不可用时直接失败 |
| drive update-title/version-history/version-get | auto | — |
| drive import/export/export-download/move/task-result | auto | 任务轮询和下载沿用创建任务时的身份 |
| markdown create/fetch/overwrite/patch/diff | auto | 使用 Drive 上传/下载 scope，不是 docx scope |
| search messages、msg search-chats | auto | current IM 端点支持两种身份；search docs/apps 仍必须 User |
| attendance user-task query | auto | 不传 `--user-ids` 查本人需要 User；Bot 查询需显式指定目标用户 |
| mail triage/message/messages/thread | auto | Bot 不支持 `--mailbox me`，必须显式指定有权访问的邮箱 |
| vc search/detail/recording/notes、vc note detail、vc meeting list-active、minutes search/get/download/apply-permission | user | 端点也接受 Bot，Bot 需应用开通对应 scope（否则 99991672） |
| vc bot meeting-events | auto | meeting_id 来自用户发现选 user，来自机器人入会选 bot |
| user info | bot | `--as user` 走 basic_batch，只返回姓名类字段，适合应用通讯录范围外的同事 |
| sheet 全部、drive pull/push/status、file list | 不传 = 读类回退 | 显式传 `--as` 后按下方共同规则 |
| drive upload/download | 不传 = 必须 User | 可显式 `--as bot` 或 `--as auto` |
| api | auto | `--as bot` 强制 Bot；仅 auto/user 路径解析 User Token |
| auth token | auto | 导出 token 给外部工具；`--as bot` 不能与 `--user-access-token` 同用 |

`auto` 的共同规则：优先 User；从未配置任何 User Token 时才回退 Bot；已配置但 token 文件损坏、
绑定的 App 不匹配或刷新失败时直接失败（上表标注"告警回退"的命令除外）。显式 `bot` 强制应用身份，
`user` 缺 Token 时失败。

`user search --email/--mobile` 的主查询使用 App 身份，已登录时额外用 User Token 补全姓名；`--query`
关键词搜索与 `user search-bot` 必须 User。不能把通讯录命令视为同一种身份。

不同命令是否提供 `--dry-run` 以帮助为准，不给只读命令臆造 `--dry-run`。

## 配置来源

目录/User Token 与 App 凭证是两条独立解析链：

1. 目录/User Token：`--profile` → `FEISHU_PROFILE` → active-profile 指针 → 旧布局/可用 profile 回退。
2. App 凭证：`--bot-app-id`/`--bot-app-secret` → `FEISHU_APP_ID`/`FEISHU_APP_SECRET` → 生效 config.yaml。

`--config` 只切换 YAML，不切换 token.json 候选目录。读取 owner 等配置时沿用本次参数执行
`config get owner_email`，不要直接固定读取旧布局文件。单次操作不运行 `profile use`；
已授权的 App 选择不需要重复确认。`profile list --json` 显示的是配置来源，不是目标命令实际采用的身份。
