# 身份选择与预检

本文件是 Skill 中身份规则的维护入口；具体命令是否接受某个 flag 以当前二进制 `--help` 为准。
先看用户要求的身份、目标资源与所选 App，再读取本次任务所需的工作流。

## 预检的含义

- `auth check --scope` 只检查选中 profile 的本地 User Token 的 scope 和有效期；不验证 Bot、资源权限或显式传入的 User Token。
- Bot：确认应用权限已开通（`auth scopes --scope "..." -o json` 的 `tenant_enabled`），`doctor --only bot_identity` 可验证凭证能否换取 Tenant Token；实际资源访问仍以 API 返回为准。
- 区分"应用没开通"与"用户没授权"：`auth scopes --scope` 的 `diagnosis`（`app_not_enabled` / `tenant_only` 需在开放平台开通，`user_not_granted` 需 `auth login --scope`）。
- 本地登录的 User：按本次命令所需 scope 运行 `auth check`；缺失时按提示补授。
- 显式 `--user-access-token` / `FEISHU_USER_ACCESS_TOKEN`：不把另一个本地 token 的预检结果当作它的授权证明。
- `--dry-run` 成功仅表示本地校验与请求构造完成，不代表已访问飞书或写入成功。

## 解析模式

| 模式 | 典型命令 | 行为 |
|---|---|---|
| 读类 User 优先，可回退 Bot | doc read、msg list/get/mget/thread-messages、chat list、task get/list、calendar get/list/freebusy、file 读、wiki 读、board 读、sheet 全家桶、drive pull/push/status | 显式 User flag → User 环境变量 → profile token.json → config 的 User Token → Bot；损坏/刷新失败会在 stderr 告警后回退，不应误以为仍是原用户。sheet、drive pull/push/status、file list 另有 `--as`，不传保持此默认 |
| 写类默认 Bot | doc create/import/add/content-update、msg send/reply/delete、comment reply、slides 写命令等 | 默认不加载 token.json；仅显式 User flag 或 User 环境变量切到用户。日历/任务写命令例外（默认 auto，见下表） |
| 必须 User | search docs/apps、approval 全部、task my/search/related、calendar rsvp/event-reply、vc note transcript、mail 写/规则/线程/签名/模板、drive add-comment/search/secure-label、file quota、okr comment create、apps get/list/release | 无可用 User Token 时报错（exit 3），不回退 Bot。drive upload/download 默认同样要求 User，可 `--as bot` |
| 仅 Bot | vc bot meeting-join/meeting-leave、msg merge-forward | 传 --user-access-token 会报用法错误（exit 2），接口只接受应用身份 |
| 固定 App | chat create/link、user info/list、dept | 不支持用 User Token 替代 App 身份，按帮助查看参数 |

`drive pull --delete-local` / `drive push --delete-remote` 是读类回退的例外：
已配置 User 但不可用时直接报错；只有从未配置 User 的纯 Bot 场景才能使用 Bot。
`msg history` 和 `chat member` 有各自的身份分支，不能根据组名前缀推导所有子命令身份。

## 支持 --as 的入口

| 命令 | 默认 | 特别要求 |
|---|---|---|
| bitable 全部 | auto | Bot 定时任务显式 --as bot |
| okr 全部 | bot | cycle list 默认查 v2 用户周期（两种身份都支持）；`--tenant` 查旧租户周期仅支持 Tenant |
| perm 全部 | bot | 操作用户自己的文档用 `--as user` 或显式 `--user-access-token`（不读环境变量）；Bot 遇 1063004/1063002 会提示改用 User |
| calendar create-event/update-event/delete-event/attendee/event-transfer、task/tasklist 写命令 | auto | 日程与任务是个人资源；操作应用自己的日历/任务时显式 --as bot |
| msg reaction/pin、chat get/update/delete/member | auto | 外部群成员管理推荐 --as bot（需对外共享能力 App 且 Bot 已入群） |
| wiki delete-space/node-copy | auto | 写身份：已配置 User 但不可用时直接失败 |
| drive update-title/version-history/version-get | auto | — |
| vc search/detail/recording/notes、vc note detail、vc meeting list-active、minutes search/get/download/apply-permission | user | 端点接受 Bot，Bot 需应用开通对应 scope（否则 99991672） |
| markdown create/fetch/overwrite/patch/diff | auto | 使用 Drive 上传/下载 scope，不是 docx scope |
| drive import/export/export-download/move/task-result | auto | 任务轮询和下载应沿用创建任务时的身份 |
| search messages、msg search-chats | auto | current IM 端点支持两种身份；search docs/apps 仍必须 User |
| calendar agenda、calendar event-search | auto | primary 与当前身份对应，不能混用两种身份的日历 ID |
| attendance user-task | auto | 本人自查需要 User，Bot 查询显式指定目标用户 |
| mail triage/message/messages/thread | auto | Bot 不支持 mailbox=me，必须显式指定有权访问的邮箱 |
| vc bot meeting-events | auto | meeting_id 来自用户发现则选 user，来自机器人入会则选 bot |
| api | auto | --as bot 强制 Bot；仅 auto/user 路径解析 User Token |

`auto` 的共同规则：优先 User；从未配置时才回退 Bot；已配置但 token 文件损坏、
绑定不匹配或刷新失败时直接失败。显式 `bot` 强制应用身份，`user` 缺 Token 时失败。
`user search` 的邮箱/手机号主查询使用 App 身份，可用 User Token 只用于补全搜索结果；`--query` 名称搜索必须 User，不能把通讯录组视为同一种身份。

不同命令是否提供 dry-run 以帮助为准，不给只读命令臆造 --dry-run。

## 配置来源

目录/User Token 与 App 凭证是两条独立解析链：

1. 目录/User Token：--profile → FEISHU_PROFILE → active-profile 指针 → 旧布局/可用 profile 回退。
2. App 凭证：--bot-app-id/--bot-app-secret → FEISHU_APP_ID/FEISHU_APP_SECRET → 生效 config.yaml。

`--config` 只切换 YAML，不切换 token.json 候选目录。读取 owner 等配置时沿用本次参数执行
`config get owner_email`，不要直接固定读取旧布局文件。单次操作不运行 profile use；
已授权的 App 选择不需要重复确认。`profile list --json` 显示的是配置来源，不是目标命令实际采用的身份。
