# 飞书权限管理

飞书云文档权限管理：添加/更新/删除/查看协作者、公开权限、分享密码、批量添加、权限检查、转移所有权、Bot 创建资源后
自动给当前用户授权（`permission_grant`），以及向所有者申请权限（`drive apply-permission`）和密级标签（`drive secure-label`）。
知识空间成员（不是文档协作者）用 `../wiki/workflow.md` 的 `wiki member`。

## 目录

- [身份与 scope](#身份与-scope)
- [Bot 创建资源后自动授权当前用户](#bot-创建资源后自动授权当前用户)
- [协作者](#协作者)
- [转移所有权](#转移所有权)
- [权限检查](#权限检查)
- [公开权限与分享密码](#公开权限与分享密码)
- [申请权限（drive apply-permission）](#申请权限drive-apply-permission)
- [密级标签（drive secure-label）](#密级标签drive-secure-label)
- [参数取值](#参数取值)
- [创建文档后标准授权流程](#创建文档后标准授权流程)
- [错误排障](#错误排障)

## 身份与 scope

权限 API 同时支持 **App（Bot）身份**与 **User 身份**，perm 全部子命令（含 `password`）共用以下规则；
`drive apply-permission`、`drive secure-label` 例外，必须 User Token（见对应小节）：

- 不传 `--as`：默认 **Bot 身份**（App ID / App Secret）；显式 `--user-access-token u-xxx` 时以该用户身份调用。
  **不读** `FEISHU_USER_ACCESS_TOKEN` 环境变量，避免环境变量静默切换身份。
- `--as user`：以 `auth login` 的用户身份调用（缺 User Token 报错）；`--as bot` 强制 App 身份；
  `--as auto`：已登录用 User、未配置回退 Bot，已配置但不可用时 fail-closed 报错。
- **用户自己的文档**（应用不是协作者）用 Bot 身份会返回 `1063004 User has no share permission` / `1063002 Permission denied`，
  CLI 会提示改用 `--as user`（实测同一文件 Bot 失败、`--as user` 成功）。Bot 自己创建的资源用默认身份即可。

| scope | 用途 |
|-------|------|
| `docs:permission.member:create` | 添加协作者（也用于 Bot 创建后的自动授权） |
| `docs:permission.member:retrieve` | 查看协作者列表 |
| `docs:permission.member:update` | 更新协作者权限 |
| `docs:permission.member:delete` | 删除协作者 |
| `docs:permission.member:transfer` | 转移所有权 |
| `docs:permission.member:auth` | 检查权限 |
| `docs:permission.setting:read` / `docs:permission.setting:readonly` | 读取公开权限设置 |
| `docs:permission.setting:write_only` | 更新公开权限、密码管理 |
| `docs:permission.member:apply` | 申请权限（`drive apply-permission`） |
| `docs:secure_label:readonly` / `docs:secure_label:write_only` | 查看 / 设置密级标签（`drive secure-label list` / `set`） |

## Bot 创建资源后自动授权当前用户

以下命令**实际以 Bot 身份执行**并新建资源时，CLI 会自动给当前 CLI 登录用户（`auth login` 的用户；open_id 取自本地缓存，
缓存失效时调一次 `/authen/v1/user_info`）授予 `full_access`（wiki 节点授予容器权限，不发通知），避免「Bot 建的资源用户自己打不开」：

`doc create`、`doc import`（新建文档时）、`doc import-file`、`slides create`、`sheet create`、`sheet import-md`、
`file mkdir`、`file upload`、`file copy`、`drive upload`（新建；`--file-token` 覆盖不触发）、`drive import`、
`drive task-result --scenario import`（导入完成时）、`wiki create`、`markdown create`、`bitable create/copy`。

JSON 输出 `permission_grant`（文本模式追加一行"当前用户权限"）：

| `status` | 含义 | 附加字段 |
|---|---|---|
| `granted` | 已授予 | `perm=full_access`、`user_open_id`、`member_type=openid`、`message` |
| `skipped` | 未登录 / User Token 不可用等，未授予 | `message`、`hint`（auth login 或 `perm add` 手动授权） |
| `failed` | 授权接口报错 | `lark_code`、`message`、`hint` |

- 以 User 身份执行（显式 `--user-access-token`、`FEISHU_USER_ACCESS_TOKEN`、`--as user`，或 auto 模式已登录）时资源本就属于用户，
  **不触发**，也不输出该字段；`--dry-run` 不触发。`drive import --as auto` 在已登录时走 User，同样不触发。
- 授权失败或跳过只在 stderr 告警，**不影响主操作的退出码**。
- 它只覆盖「当前 CLI 登录用户」；需要交付给其他人（如 `owner_email`）时仍按下文 `perm add` 授权。

## 协作者

```bash
# 添加（最常用）
feishu-cli perm add <TOKEN> --doc-type docx --member-type email --member-id user@example.com --perm edit --notification

# 用户自己的文档（应用不是协作者）：以当前登录用户身份操作
feishu-cli perm add <TOKEN> --doc-type docx --member-type email --member-id user@example.com --perm view --as user

# 知识库节点：仅当前页面（不含子页面）
feishu-cli perm add <WIKI_NODE_TOKEN> --doc-type wiki --member-type openid --member-id ou_xxx --perm view --perm-type single_page

# 按群 / 部门授权
feishu-cli perm add <TOKEN> --doc-type sheet --member-type openchat --member-id oc_xxx --perm edit
feishu-cli perm add <TOKEN> --doc-type sheet --member-type opendepartmentid --member-id od_xxx --perm view

# 更新、查看、删除
feishu-cli perm update <TOKEN> --doc-type docx --member-type email --member-id user@example.com --perm full_access
feishu-cli perm list <TOKEN> --doc-type docx --as user
feishu-cli perm delete <TOKEN> --doc-type docx --member-type email --member-id user@example.com

# 批量添加
feishu-cli perm batch-add <TOKEN> --doc-type sheet --members-file members.json --notification
```

- `--doc-type` 默认 `docx`，**所有子命令**都要让它与 TOKEN 的真实类型一致（sheet/bitable/file/wiki/folder/slides 等必须显式传）。
- `--notification` 默认关闭；传了才通知被授权者。
- `--perm-type` 只对知识库节点（`--doc-type wiki`）有效：`container` = 当前页面及子页面，`single_page` = 仅当前页面；
  不传时不下发，由服务端按默认（container）处理；非 wiki 文档传入会直接报用法错误（退出码 2，实测）。
- `perm list` 始终输出 JSON 数组（没有 `-o` 参数），每项含 `member_type`、`member_id`、`perm`、`perm_type`（按服务端返回）。
- `batch-add` 的 members.json 顶层为数组；知识库成员可额外带 `"perm_type": "container"|"single_page"`，非 wiki 文档带该字段会报错：

```json
[
  {"member_type": "email", "member_id": "user@example.com", "perm": "edit"},
  {"member_type": "openid", "member_id": "ou_xxx", "perm": "view"}
]
```

`perm add` 的输入检查清单见 `references/add_permission.md`。

## 转移所有权

```bash
feishu-cli perm transfer-owner <TOKEN> --doc-type docx --member-type email --member-id user@example.com --old-owner-perm view
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--notification` | true | 通知新所有者；不通知传 `--notification=false` |
| `--remove-old-owner` | false | 移除原所有者权限 |
| `--stay-put` | false | 文档保留在原位置 |
| `--old-owner-perm` | full_access | 原所有者保留权限（view/edit/full_access，仅 remove-old-owner=false 时生效） |

- `--member-type` 只支持 `email` / `openid` / `userid`（及 `open_id` / `user_id` 别名）。
- 只有当前所有者（或有权限的管理员）能转移：Bot 创建的资源用默认 Bot 身份；属于用户本人的文档用 `--as user`。
  这是不可逆的高风险操作，执行前确认目标与新所有者。

## 权限检查

```bash
feishu-cli perm auth <TOKEN> --action edit --doc-type docx            # 检查当前身份（默认 Bot）
feishu-cli perm auth <TOKEN> --action edit --doc-type docx --as user  # 检查登录用户本人
```

`--action` 取值：`view`、`edit`、`share`、`comment`、`export`；输出 `{"auth_result": true|false}`。
不传 `--as` 时检查的是应用（Bot）的权限，不是登录用户的权限。

## 公开权限与分享密码

```bash
feishu-cli perm public-get <TOKEN> --doc-type docx [--as user]

feishu-cli perm public-update <TOKEN> --doc-type docx --external-access --link-share-entity anyone_readable
feishu-cli perm public-update <TOKEN> --doc-type docx --link-share-entity tenant_readable --comment-entity anyone_can_view
feishu-cli perm public-update <TOKEN> --doc-type docx --external-access=false   # 关闭对外分享

feishu-cli perm password create <TOKEN> --doc-type docx
feishu-cli perm password update <TOKEN> --doc-type docx
feishu-cli perm password delete <TOKEN> --doc-type docx
```

- `public-get` 主读取走 v2（`GET /open-apis/drive/v2/permissions/:token/public`），比 v1 多出 `external_access_entity`
  （对外分享范围 open / closed / allow_share_partner_tenant）、`copy_entity`（谁可以复制）、`manage_collaborator_entity`
  （谁可以管理协作者）；同时补读 v1 保留 `external_access`、`invite_external`（v1 读取失败时 stderr 告警，`external_access`
  由 `external_access_entity` 推断）。其余字段：`security_entity`、`comment_entity`、`share_entity`、`link_share_entity`、`lock_switch`。
- `public-update` 只更新显式传入的字段。`--external-access` / `--invite-external` 是布尔开关，关闭必须写 `=false`。
  常用值：`--link-share-entity` tenant_readable / tenant_editable / anyone_readable / anyone_editable / closed；
  `--security-entity` anyone_can_view / anyone_can_edit / only_full_access；`--comment-entity` anyone_can_view / anyone_can_edit；
  `--share-entity` anyone / same_tenant / only_full_access。
- 分享密码只对**互联网公开链接**生效：先 `public-update --external-access --link-share-entity anyone_readable`，
  再 `password create`；链接仅组织内可见（如 tenant_readable）时 Bot 与 `--as user` 都返回 `1063002 Permission denied`（实测，
  此时不是身份问题）。`password create` 在 stdout 打印生成的密码；`password update` 刷新为新密码；`password delete` 删除密码。

## 申请权限（drive apply-permission）

向文档所有者**申请**查看/编辑权限（所有者收到审批卡片）。**必须 User Token**（Bot 身份会被拒绝；无 `--as`），需要
`docs:permission.member:apply`（或任一大权限：`drive:drive` / `docs:doc` / `docx:document` 等）。

```bash
feishu-cli drive apply-permission --token "https://xxx.feishu.cn/docx/doxcnxxx" --perm view --remark "申请理由"
feishu-cli drive apply-permission --token doxcnxxx --type docx --perm edit --remark "需要协作编辑"
feishu-cli drive apply-permission --token "https://xxx.feishu.cn/docx/doxcnxxx" --perm view --dry-run
```

- `--perm` 只有 `view` / `edit`（默认 view）；该端点未收录在飞书文档站，但服务端实测可用
  （调研方法见 [`embedded-api-discovery.md`](../../../../feishu-cli-platform/references/workflows/api/references/embedded-api-discovery.md)）。
  业务错误一律非零退出：`1063006` = 同一用户对同一文档每天最多申请 5 次；`1063007` = 该文档不接受权限申请。
- 碰到「没有权限查看此文档」时，先 `drive inspect --url <url>` 确认类型与 token（可选），再
  `drive apply-permission --token <url> --perm view --remark "<理由>"`；先 `--dry-run` 预览。理由会显示在所有者收到的审批卡片上。

## 密级标签（drive secure-label）

查看/设置云文档密级标签，**必须 User Token**，需要 `docs:secure_label:readonly` / `docs:secure_label:write_only`。

```bash
# 先查当前用户可用的标签 id（不要用显示名）
feishu-cli drive secure-label list --page-size 10 --lang zh
feishu-cli drive secure-label list --output json

# 把文档设置为指定密级（--label-id 用 list 返回的数字 id）
feishu-cli drive secure-label set doxcnxxx --type docx --label-id 7217780879644737539
```

- `list`：`--page-size` 1-10，`--lang` 支持 zh/en/ja，有更多时用 `--page-token` 续翻。
- `set`：`--type` 默认 docx，可选 doc/docx/sheet/file/bitable/mindnote/slides；`--label-id` 必须是数字 id（如 list 返回的
  id），不要传 `内部(D)` 这类显示名。
- **密级降级需审批**：命中 `1063013` 时需到文档界面完成降级审批，重试 API 不会绕过审批。

## 参数取值

- `--perm`：`view`（只读分享）/ `edit`（日常协作）/ `full_access`（可管理协作者与文档设置）。
- `--doc-type`：`docx`（默认）/ `doc` / `sheet` / `bitable` / `wiki` / `file` / `folder` / `mindnote` / `minutes` / `slides`。

| member-type | 别名（IM 风格） | 说明 | 示例 |
|----|-----------------|------|------|
| `email` | — | 飞书注册邮箱，**最常用** | user@example.com |
| `openid` | `open_id` | Open ID | ou_xxx |
| `userid` | `user_id` | 企业内部 User ID | 123456 |
| `unionid` | `union_id` | Union ID | on_xxx |
| `openchat` | `chat_id` | 按群聊授权，群内所有成员获得权限 | oc_xxx |
| `opendepartmentid` | — | 按部门授权，部门内所有成员获得权限 | od_xxx |
| `groupid` | — | 用户组 | gc_xxx |
| `wikispaceid` | — | 知识空间 | ws_xxx |

IM 风格别名会自动映射为标准值，两种写法等效。

**Token 与类型**：老 token 有前缀可参考（`doxcn`→docx、`doccn`→doc、`shtcn`→sheet、`bascn`→bitable、`wikcn`→wiki、
`fldcn`→folder）；新版 token 多数没有这类前缀，按 URL 路径判断，或 `feishu-cli drive inspect --url <token> -o json`
自动识别（返回 `type`；wiki 节点会展开为底层文档并附 `wiki_node`）。给知识库节点授权用 `/wiki/` URL 里的 node_token 配 `--doc-type wiki`。

## 创建文档后标准授权流程

```bash
# 1. 授予完全访问权限
feishu-cli perm add <TOKEN> --doc-type docx --member-type email --member-id user@example.com --perm full_access --notification

# 2. 仅生效配置 transfer_ownership 为 true 时转移所有权
feishu-cli perm transfer-owner <TOKEN> --doc-type docx --member-type email --member-id user@example.com --notification
```

授权邮箱与是否转移所有权使用 CLI 的生效配置，沿用本次相同的 `--profile` / `--config`，不要固定读取旧目录。读取命令和未配置
owner 时的处理见 [文档创建后的授权流程](../../../../feishu-cli-docs/references/workflows/write/workflow.md#新建文档)。示例邮箱必须替换为
用户指定或配置解析出的真实接收人。

## 错误排障

| 错误 | 原因 | 解决方法 |
|------|------|----------|
| `1063004 User has no share permission` | 当前身份对文档无管理协作者/分享权限；Bot 身份最常见（应用不是协作者） | 文档属于你本人或你有管理权限时加 `--as user`；否则联系所有者授予「可管理」，或把应用加为协作者 |
| `1063002 Permission denied` | 当前身份无权访问该文档 | 同上，Bot 身份时改用 `--as user` |
| `99991672` / `99991679` | 应用未开通 / 用户未授权对应 scope | 按错误提示在开放平台开通，或 `auth login --scope "..."` 补授权 |
| doc-type 与 token 不匹配 / token 无效 | `--doc-type` 与实际文档类型不一致 | 用 URL 路径或 `drive inspect` 确认类型后重试 |
| `member not found` | member-id 不存在或 member-type 不正确 | 确认邮箱/ID 正确；email 类型需要用户的飞书注册邮箱 |
| `password create` 返回 1063002 | 文档未开启互联网公开链接（与身份无关，实测） | 先 `public-update --external-access --link-share-entity anyone_readable`；仍失败再确认企业是否开通分享密码功能 |
| `transfer-owner` 无权限 | 只有文档所有者或管理员可转移 | 先用 `perm list` 确认当前身份；文档属于你本人时用 `--as user` |
| `--perm-type / perm_type 仅在文档类型为 wiki 时有效` | 对非 wiki 文档传了 perm_type | 去掉 `--perm-type`，或确认 `--doc-type wiki` |
