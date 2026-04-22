# Changelog

所有重要的项目变更都会记录在此文件。

版本格式：[MAJOR.MINOR.PATCH](https://semver.org/lang/zh-CN/)

## 未发布

### 新增 — `comment reply add`：为已有评论添加回复

新增命令 `feishu-cli comment reply add <file_token> <comment_id> --text "..."`，补齐评论回复
生命周期的最后一块拼图（此前只有 list / delete）。

**背景**：飞书 Open SDK v3.5.3 的 `fileCommentReply` 只暴露 `List`/`Delete`/`Update`，没有
`Create` 方法，而 Open API 本身是支持的（`POST /drive/v1/files/:token/comments/:comment_id/replies`）。
此 PR 不依赖 SDK 升级，用通用 HTTP client（`client.Post`）直接调用 API 实现。

**同时改进**：

- `comment reply add` / `delete` / `list` 全部加上 `--user-access-token` 参数支持，并走
  `resolveOptionalUserTokenWithFallback` 自动读取登录态，和 msg/chat/doc export 等模块保持一致
- **重要修复**：`comment reply delete` 在 App Token（Bot 身份）下调用飞书侧会返回 `1069303
  forbidden`——飞书只允许回复作者本人删除。现在命令默认优先使用 User Token（如果已登录），
  行为才符合用户预期。命令帮助中也显式说明了这个权限模型
- `comment reply add` 默认也走 User Token fallback，回复会以用户身份发布（而非显示为 Bot），
  且该回复能被后续 `reply delete` 正常删除

**权限要求**：`docs:document.comment:create`（User Token）

**使用示例**：

```bash
feishu-cli auth login                       # 确保有 User Token
feishu-cli comment reply add <file_token> <comment_id> --text "已处理"
feishu-cli comment reply delete <file_token> <comment_id> <reply_id>  # 自动用 User Token
```

**代码影响范围**：

- `internal/client/comment.go`：新增 `CreateCommentReply`（HTTP client 直调），
  `ListCommentReplies` / `DeleteCommentReply` 签名增加 `userAccessToken` 参数
- `cmd/comment_reply.go`：新增 `addReplyCmd`，三个子命令统一加 `--user-access-token` flag
- `cmd/comment.go`：Long help 中补充 reply add 示例

---

### Breaking Changes — 移除 `config add-scopes` 命令

`feishu-cli config add-scopes` 子命令及其 `--domain` / `--scopes` / `--print-only` flag 全部删除。

**删除理由**：

1. **命令几乎不可用** — 硬编码的 `scopeDomains` 字典里多数 scope 名已过时（`docx:document` / `sheets:spreadsheet` / `bitable:app` / `im:chat:readonly` / `drive:export:readonly` / `vc:room:readonly` 等都是飞书不支持的粗粒度名称），生成的申请链接里多数 scope 会被后台拒绝
2. **权限开通不适合自动化** — 飞书开放平台的权限申请通常需要 tenant 管理员审批，scope 选择也是业务决策而非技术"默认值"。CLI 自动化只会造成"看起来装好了但后台还没批"的幻觉
3. **有更简单的替代** — 飞书开放平台的应用权限管理页面支持"导入权限 JSON"入口，复制 [README 权限要求](../README.md#权限要求) 章节里的完整权限清单一次性粘贴即可开通 400+ 个 scope

**迁移指引**：

旧：
```bash
feishu-cli config add-scopes --domain all
```

新：
1. 打开飞书开放平台 → 你的应用 → 权限管理页面
2. 复制 README 的完整权限 JSON（tenant + user 两套 400+ scope）
3. 粘贴到"导入权限"入口，一键开通全部
4. 等待 tenant 管理员审批（如果需要）

**代码影响范围**：
- 删除 `cmd/config_add_scopes.go` 整个文件
- `cmd/auth_check.go` 的 `suggestion` 文案改为引导用户去开放平台开通（不再推荐 `config add-scopes`）
- README / CLAUDE.md / AGENTS.md / 6 个 skill 的 `config add-scopes` 引用全部更新为"去开放平台开通"
- 保留 `config create-app --save`（Device Flow 创建应用）不变

---

### Breaking Changes — 多维表格（bitable）切换到 `base/v3` API

**旧实现**：`bitable` 模块全部调用 `/open-apis/bitable/v1/apps/{app_token}/...` 老 API，覆盖 ~30 个基础 CRUD 命令。
**新实现**：全面切换到 `/open-apis/base/v3/bases/{base_token}/...` 新 API，覆盖 48 个命令，支持深度能力（视图完整配置读写、记录 upsert、修改历史、角色 CRUD、高级权限、数据聚合、工作流查询）。

#### 命令名迁移表

| 旧命令 | 新命令 |
|---|---|
| `bitable tables <app>` | `bitable table list --base-token <t>` |
| `bitable create-table <app>` | `bitable table create --base-token <t> --name x` |
| `bitable rename-table <app> <tbl>` | `bitable table update --base-token <t> --table-id <tbl> --name x` |
| `bitable delete-table <app> <tbl>` | `bitable table delete --base-token <t> --table-id <tbl>` |
| `bitable fields <app> <tbl>` | `bitable field list --base-token <t> --table-id <tbl>` |
| `bitable create-field` | `bitable field create` |
| `bitable update-field` | `bitable field update`（method 改为 `PUT`） |
| `bitable delete-field` | `bitable field delete` |
| `bitable records <app> <tbl>` | `bitable record list --base-token <t> --table-id <tbl>` |
| `bitable get-record` | `bitable record get` |
| `bitable add-record` | `bitable record upsert --base-token <t> --table-id <tbl> --config '...'` |
| `bitable add-records --data-file` | `bitable record batch-create --config-file ...` |
| `bitable update-record` | `bitable record upsert --record-id ...`（根据是否传 id 自动 PATCH/POST） |
| `bitable delete-records` | `bitable record delete --record-id ...` |
| `bitable views` | `bitable view list` |
| `bitable create-view` | `bitable view create` |
| `bitable delete-view` | `bitable view delete` |
| `bitable view-filter get/set` | `bitable view view-filter-get / view-filter-set` |
| `bitable dashboard list`（v1） | **暂不支持**（v3 dashboard CRUD 留待下次迭代） |
| `bitable form list` | **暂不支持** |
| `bitable role list` | `bitable role list`（新增 get/create/update/delete） |
| `bitable workflow list/enable` | `bitable workflow list`（改为 POST /workflows/list） |
| `bitable advperm enable/disable` | 同名但底层改为 `PUT .../advperm/enable?enable=true/false` |
| `bitable data-query` | 同名但路径从 table 级改为 base 级：`POST .../bases/{t}/data/query` |

#### 新增能力

- **视图配置完整写入**：`view-sort-set` / `view-group-set` / `view-visible-fields-set` / `view-timebar-set` / `view-card-set`（老 v1 只能写 filter）
- **记录修改历史**：`bitable record history-list --record-id xxx`
- **角色 CRUD**：`bitable role create/update/delete`（老 v1 只有 list）
- **字段选项搜索**：`bitable field search-options`
- **Base create 支持时区**：`--time-zone Asia/Shanghai`

#### Flag 变化
- **删除 `--app-token` 别名**：只保留 `--base-token`（与 base/v3 API 命名一致，不再做兼容别名）
- `bitable create` 的 `--description` 被删除（base/v3 不支持），新增 `--time-zone`
- `bitable data-query` 的 `--table-id` 被删除（v3 端点挂在 base 下）

#### 删除的文件
- `internal/client/bitable.go` / `bitable_test.go`（v1 实现）
- `cmd/bitable_create.go` / `bitable_get.go` / `bitable_copy.go` / `bitable_advperm.go` / `bitable_dashboard.go` / `bitable_data_query.go` / `bitable_form.go` / `bitable_record_upload_attachment.go` / `bitable_role.go` / `bitable_view_config.go` / `bitable_workflow.go`

#### 新增的文件
- `internal/client/base.go`（`BaseV3Call` + `BaseV3Path` helper + `X-App-Id` header 自动注入）
- `cmd/bitable_base.go` / `bitable_misc.go`（所有 base/v3 命令的注册）
- `cmd/bitable_table.go` / `bitable_field.go` / `bitable_record.go` / `bitable_view.go` 全部重写

---

### Breaking Changes — VC（视频会议）改造升级

- **`vc search`**：底层 API 从 `GET /meeting_list` 切换到 `POST /meetings/search`。
  - 新增 flag：`--query` / `--organizer-ids` / `--participant-ids` / `--room-ids`
  - 删除 flag：`--meeting-no` / `--meeting-status`
  - 必须指定至少一个过滤条件
- **`vc notes`**：
  - flag 从 `--meeting-id` / `--minute-token`（单数）改为 `--meeting-ids` / `--minute-tokens`（复数，支持 CSV 批量最多 50）
  - 新增第三路径 `--calendar-event-ids`：从日历事件自动反查会议 / 妙记
  - 新增开关 `--with-artifacts`（获取 AI 产物）/ `--download-transcript --output-dir`（下载逐字稿）
- **所有 vc / minutes 命令默认 User Access Token**，未登录时统一报错提示 `feishu-cli auth login`

#### 新增命令
- `vc recording --meeting-ids/-calendar-event-ids`：查询会议录制并自动提取 `minute_token`
- `minutes download --minute-tokens x,y,z --output ./dir`：批量下载妙记音视频媒体（SSRF 防护 / 重定向校验 / Content-Disposition 解析 / 文件名去重 / 5 req/s 速率限制 / `--url-only` 预览链接）
- `minutes get <token> --with-artifacts`：新增 AI 产物合并输出

---

### Added — `drive` 云盘命令组（8 个命令）

新增独立的 `drive` 子命令组，与现有 `file` / `media` / `doc media-*` 命令并存，提供增强能力：

| 命令 | 相比老命令的增强 |
|---|---|
| `drive upload` | 大文件自动分块（>20MB 走 `upload_prepare/part/finish` 三步式，每片独立重试 3 次；支持 User Token） |
| `drive download` | 流式下载 + 路径校验 + `--overwrite` / `--timeout` |
| `drive export` | 新增 **markdown 快捷路径**：docx → markdown 走 `/docs/v1/content` 直接拉取，不跑异步 export task；支持 sheet / bitable 按 `--sub-id` 导出 CSV；有界轮询（10×5s）+ 超时返回 resume 命令 |
| `drive export-download` | 通过 `file_token` 直接下载已完成的导出任务产物，配合 `drive export` 超时后接力完成 |
| `drive import` | **切换到 `/medias/upload_*` 端点 + `parent_type=ccm_import_open` + `extra` 字段**（不再在用户云盘留下中间文件）；格式特定大小限制（docx 20MB / sheet 20MB / bitable 100MB）；有界轮询 + resume |
| `drive move` | 文件夹移动自动轮询 `task_check`（30×2s），文件移动同步返回 |
| `drive add-comment` | 支持**富文本 `reply_elements`**（text / mention_user / link）+ `--block-id` 局部评论（docx）+ **wiki URL 自动解析**成 docx token |
| `drive task-result` | 通用异步任务查询（`--scenario import/export/task_check`），配合 drive export / import / move 的超时 resume |

**保留不动**：`file list / delete / mkdir / copy / shortcut / quota / meta / stats / version` + `media upload / download` + `doc media-download / media-insert` + `comment list / resolve / delete / reply`

---

### Added — `mail` 飞书邮箱模块（10 个命令，从零新建）

**全新命令组**。首期不支持附件和 CID 内联图片，仅支持纯文本和 HTML body。所有命令默认 User Access Token。

| 命令 | 功能 |
|---|---|
| `mail message --message-id x` | 获取单封邮件（`--format full/plain_text_full/raw`） |
| `mail messages --message-ids a,b,c` | 批量获取多封邮件 |
| `mail thread --thread-id x` | 获取邮件线程 |
| `mail triage` | 列出 / 搜索邮件（`--folder INBOX --label x --query xxx --unread-only --list-folders --list-labels`），`--query` 走专用 `POST /search` 端点 |
| `mail send` | 发送邮件（**默认保存为草稿**，加 `--confirm-send` 立即发送，安全兜底） |
| `mail draft-create` | 仅创建草稿 |
| `mail draft-edit --draft-id x` | 编辑已有草稿（全量覆盖） |
| `mail reply --message-id x --body "..."` | 回复邮件（自动 `Re: ` 前缀 + 引用块 + `In-Reply-To` / `References` header 继承） |
| `mail reply-all` | 全部回复（包含 To 和 CC，自动排除自己） |
| `mail forward --message-id x --to y` | 转发（自动 `Fwd: ` 前缀 + 原文正文引用） |

**关键技术点**：
- RFC 5322 EML 构建 + base64 URL-safe 编码，`POST /drafts` body `{"raw":"..."}`
- HTML 自动检测（`<html>/<div>/<b>/<br>` 等标签），可用 `--plain-text` / `--html` 强制
- 发件人地址默认从 `/user_mailboxes/{mailbox}/profile` 读取
- 地址格式支持 `"Name <email>"` 和 `"email"`
- Subject 去重：`reply` 自动避免 `Re: Re:`，`forward` 自动避免 `Fwd: Fwd:`

---

### Fixed

- **`mail reply` 引用块缺日期占位符**：之前的 quote header 模板第一个 `%s` 传空字符串，会输出 `"在 ，xxx 写道:"`，已修正为 `"{email} 写道:"`
- **分片上传 fd 泄漏**：`uploadFileMultipart` 之前每片每次重试都 `os.Open + Seek`，现改为外层打开一次 + `io.NewSectionReader`，大文件不稳定网络下重试时节省 N×syscall
- **`mail reply` 重复 `GetMailboxProfile` 调用**：之前在 `runMailReply` 里调用 2 次（一次取 selfEmail 一次取 from/fromName），现合并为 1 次，省 1 个 API RTT
- **`drive import` 上传端点错误**：之前走 `/files/upload_all` 会在用户云盘留下中间文件，现改为官方的 `/medias/upload_all` + `parent_type=ccm_import_open` + `extra`
- **`mail triage --query` 静默失效**：之前把 query 当 list 端点的查询参数，飞书会忽略；现改走专用的 `POST /search` 端点

### Refactor（内部代码清理，用户感知较小）

- 新增 `requireUserToken(cmd, cmdName)` helper，统一所有新命令的 "需要 User Access Token" 错误信息格式
- 删除重复的 `GetWikiNodeByToken`（58 行），改用已有的 `GetWikiNode`
- 删除 `internal/client/mail.go` 的 `joinPath`，用 `strings.Join`
- `dedupStrings` 从 `vc_recording.go` 移到 `vc_common.go`
- `runBaseV3WithJSON` 重构，抽出 `runBaseV3WithBody` 让命令层直接传已构造的 body
- `bitable view create/rename` 去掉 `cmd.Flags().Set("config", ...)` + `MarkHidden` 的 hack 模式
- 删除 `runBaseV3Simple` / `addBaseTokenFlag` / `exactlyOneNonEmpty` 三处死参数/死变量
- 所有文件统一 `gofmt`

---

## [v1.18.0] - 未发布

### Breaking Changes — OAuth 认证全面切换到 Device Flow

彻底删除 Authorization Code Flow，只保留 Device Flow（RFC 8628）。本地桌面、SSH 远程、容器、CI 全环境统一使用同一条命令，**无需任何重定向 URL 白名单配置**。

#### 删除的 flag

`auth login` 命令删除以下 flag：

- `--manual` — SSH 远程手动粘贴回调模式（Device Flow 下 SSH 和本地一视同仁）
- `--no-manual` — 强制本地回调模式（本地回调 HTTP server 已移除）
- `--port` — 本地回调端口（不再需要）
- `--print-url` — 非交互两步式第一步（改用 `--no-wait` + `--device-code`）
- `--method` — 授权方式选择（Device Flow 是唯一方式）
- `--scopes` — 请求 OAuth scope（飞书 token v2 端点实际忽略此参数，返回应用预配置的全部 scope）

#### 删除的子命令

- `auth callback <url> --state <state>` — Authorization Code Flow 换 token 专用，整体删除

#### 删除的代码

- `internal/auth/oauth.go`：`Login` / `loginLocal` / `loginManual` / `buildAuthURL` / `GenerateAuthURL` / `ParseCallbackURL` / `ExchangeToken` 等函数
- `internal/auth/browser.go`：`isLocalEnvironment()` 函数（曾在 macOS 上无条件返回 true 导致 SSH 远程 Darwin bug）
- `cmd/auth_callback.go`：整个文件

#### 修复的 bug

- **Issue #95**：飞书错误码 20029（重定向 URL 有误）。根因是 Authorization Code Flow 需要用户在飞书开放平台配置 `http://127.0.0.1:9768/callback` 白名单，Device Flow 直接绕过此要求
- **Darwin SSH bug**：`isLocalEnvironment()` 在 macOS 上无条件返回 `true`，SSH 到 Mac 服务器时错误走本地回调模式会 2 分钟超时失败。已通过删除该函数消除

### 新增 — `auth login` 的 JSON 事件流模式

- **`auth login --json`**：阻塞轮询 + JSON 事件流输出到 stdout。AI Agent 推荐配合 Claude Code 的 `run_in_background=true` 使用
  - 首次输出：`{"event":"device_authorization","verification_uri":"...","verification_uri_complete":"...","user_code":"...","device_code":"...","expires_in":240,"interval":5}`
  - 成功输出：`{"event":"authorization_success","expires_at":"...","refresh_expires_at":"...","scope":"..."}`

- **`auth login --no-wait --json`**：两步模式第一步。只请求 `device_code` 并立即输出 JSON，不启动轮询。适合 AI Agent 希望把"请求"和"轮询"拆到两次独立 Bash 调用的场景

- **`auth login --device-code <code> --json`**：两步模式第二步。用已有的 `device_code` 继续轮询直到授权完成

### 新增 — `auth check` 子命令

预检当前 Token 是否包含指定 scope，专为 AI Agent 在执行业务命令前做前置判断而设计：

```bash
feishu-cli auth check --scope "search:docs:read"
feishu-cli auth check --scope "search:docs:read im:message:readonly"
```

输出 JSON：

```json
{
  "ok": true,
  "granted": ["search:docs:read"],
  "missing": null
}
```

或失败情况：

```json
{
  "ok": false,
  "error": "not_logged_in",
  "missing": ["search:docs:read"],
  "suggestion": "feishu-cli auth login"
}
```

退出码 0 = 满足，非 0 = 缺少或未登录，AI Agent 可直接分支。

### 不变

- **Token 存储格式**：`~/.feishu-cli/token.json` 仍是明文 JSON，数据结构完全兼容。升级后**不需要重新登录**
- **Token 自动刷新**：`ResolveUserAccessToken()` 路径和 `RefreshAccessToken()` 逻辑保持不动，access_token 过期时用 refresh_token 自动刷新
- **`config create-app`** 命令完全不变（它本来就用 Device Flow）
- **`auth status`** / **`auth logout`** 行为不变
- **所有业务命令**（doc/msg/search/wiki/task/calendar/...）行为不变

### 迁移指引

#### 人类用户

无需任何迁移。一条命令通吃所有场景：

```bash
feishu-cli auth login
```

本地桌面会自动开浏览器，SSH 远程需要手动复制 stderr 里的链接在本机浏览器打开，一模一样的命令。

#### AI Agent / 脚本用户

旧的两步式：
```bash
feishu-cli auth login --print-url --scopes "..."
feishu-cli auth callback "<回调URL>" --state "<state>"
```

迁移为以下**任一**方案：

**方案 A（推荐）**：阻塞 + 后台运行：
```bash
# run_in_background=true
feishu-cli auth login --json
# 读 stdout 第一行拿 verification_uri_complete，展示给用户
# 等后台进程退出，读第二行 stdout 拿 authorization_success
```

**方案 B**：两步模式：
```bash
# 第一步
feishu-cli auth login --no-wait --json  # → device_code JSON
# 把链接展示给用户等待授权
# 第二步
feishu-cli auth login --device-code <code> --json  # → authorization_success
```

#### CI / 无头脚本

**Authorization Code Flow 本来就无法无头完成**（需要浏览器授权），Device Flow 同样需要人类介入一次。如果 CI 需要 User Token，应该预先在本地通过 `auth login` 拿到 token.json 然后把它作为 secret 部署到 CI 环境，**不需要任何迁移**。

### 详细对比

| 方面 | v1.17.0 及以前 | v1.18.0 |
|---|---|---|
| OAuth Flow | Authorization Code Flow（默认）+ Device Flow（`--method device`） | 仅 Device Flow |
| 子命令 | `login` / `callback` / `status` / `logout` | `login` / `check` / `status` / `logout` |
| `auth login` 的 flag | `--port` / `--manual` / `--no-manual` / `--print-url` / `--scopes` / `--method` | `--json` / `--no-wait` / `--device-code` |
| 重定向 URL 白名单 | 必须（Authorization Code Flow 前置条件） | 不需要 |
| SSH 远程支持 | 要么手动粘贴（`--manual`）要么非交互两步（`--print-url`） | 一条命令通吃 |
| AI Agent 非交互方案 | `--print-url` + `auth callback` | `--json` + `run_in_background` 或 `--no-wait` / `--device-code` |
| `offline_access` 注入 | 用户手动通过 `--scopes` 传 | CLI 强制注入，用户无需操心 |
| scope 预检 | 手动解析 `auth status` JSON 的 scope 字段 | `auth check --scope "..."` |

---

更早的版本请参考 [GitHub Releases](https://github.com/riba2534/feishu-cli/releases)。
