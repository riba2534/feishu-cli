# 飞书 OpenAPI 浏览 + 调用技能

两个命令的组合拳：
- **`feishu-cli schema`** —— 查询 OpenAPI 方法的 path/动词/参数/scope/文档链接（无需 Token；catalog overlay 为无凭证 public meta）
- **`feishu-cli api`** —— 直接调用任意飞书 OpenAPI 端点（v1.29+，自动鉴权 + 错误码翻译，覆盖未封装的 2500+ 端点）

典型工作流：`schema list 发现` → `schema <service.resource.method>` 看参数与身份 → 按执行身份预检 → `feishu-cli api` 调用。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

---

## 核心概念

### 路径格式：`<service>.<resource>.<method>`

| 段 | 含义 | 示例 |
|----|------|------|
| service | 业务域 | im / docs / drive / bitable / calendar / vc / mail / wiki / approval / sheets / slides / task / attendance / minutes |
| resource | 资源（可含 `.`，按最长前缀匹配） | messages / events / records / chat.members |
| method | 动作 | create / get / list / update / delete / patch |

按路径深度自动分发：

- `schema` 无参数 → 列出所有 service（pretty 含 Catalog 头）
- `schema status` → catalog 来源 / 版本 / service+method 数
- `schema <service>` → 列出该 service 下所有 resource.method（含嵌套 resources）
- `schema <service>.<resource>` → 列出 resource 下的所有 method
- `schema <service>.<resource>.<method>` → method 详情（含 path / 参数 / scope / docUrl）

### 数据来源

编译期 embed 的 `internal/registry/meta_data.json` 是离线 baseline（约 690KB，12 个 service / 152 个 method）。
运行时默认从官方 public `api_definition?protocol=meta` 拉 overlay（10MB 上限、24h TTL、原子 cache、无凭证）；
失败回退 embedded。`FEISHU_CLI_REMOTE_META=off` 可关闭。

**overlay 实际覆盖范围**：远端提供 **15 个 service / 250 个 method**（比 embedded 多约 98 个方法）。
首次运行会同步拉取（约 190ms，预算 2s，可用 `FEISHU_CLI_META_FIRST_SYNC_MS` 调整，`0` 表示只走后台刷新），
之后命中本地 cache（`~/.feishu-cli/cache/remote_meta.json`）。
判断 overlay 是否生效看 `schema status` 的 `source`：`runtime`（本次刚拉取）/ `cache`（命中缓存）/
`embedded`（未生效，检查网络或 `FEISHU_CLI_REMOTE_META`）。

`feishu-cli schema status --format json` 报告 `source`（embedded/cache/runtime）、版本、
service/method 数；`doctor --only catalog` 与 `auth status -o json` 的 `catalog` 字段同样可读。

---

## 命令速查

### 1. 列出所有 service

```bash
feishu-cli schema
# 等价：
feishu-cli schema list
```

输出表格：`name | version | resources 数 | title`。

### 2. 列出某个 service 的所有方法

```bash
feishu-cli schema im                       # 列出 im 域全部 resource.method
feishu-cli schema list --service im        # 等价（推荐用 list，语义更清楚）
feishu-cli schema list --service im --format json
```

`pretty` 输出按 resource 分组、列 `HTTP verb + method 名 + description`；`json` 输出扁平 `{service, resource, method, path, httpMethod, description}` 列表，方便 AI Agent 二次处理。

### 3. 列出 resource 下的方法

```bash
feishu-cli schema im.messages              # messages 资源下所有 method
feishu-cli schema im.chat.members          # 含点号的 resource（最长前缀匹配）
```

### 4. 查具体 method 详情

```bash
feishu-cli schema im.messages.delete
feishu-cli schema im.messages.delete --format json
```

`pretty` 输出包含：

- HTTP verb + 完整 URL path（`/open-apis/im/v1/messages/{message_id}`）
- 方法描述
- Parameters（含 `path` / `query` / `required` 标记 + 类型 + 描述 + example）
- Request Body（POST/PUT/PATCH/DELETE 才显示，含嵌套字段）
- Response Body
- Identity：`tenant (bot)` / `user`（指明支持哪种 Token）
- Scopes：调用所需权限点
- Docs：飞书开放平台官方文档链接

---

## 关键 flag

| flag | 作用 | 适用 |
|------|------|------|
| `--format pretty`（默认） | 人类可读，表格 + 缩进字段树 | 终端阅读 |
| `--format json` | 原始 JSON（不转义 HTML） | AI Agent 解析、脚本拼装 |
| `--service <name>` | 仅 `schema list` 子命令，过滤 service | 等价 `schema <service>` |

---

## 常见用例

**1. 找飞书有没有某个 API**

```bash
feishu-cli schema list --service drive | grep -i comment
```

**2. 拼调用前查参数**

```bash
feishu-cli schema im.chats.create
# 然后用 feishu-cli api 或 feishu-cli chat create 调用
```

**3. AI Agent 拿 JSON 推断调用**

```bash
feishu-cli schema sheets.spreadsheets.create --format json
```

**4. 确认某方法的 scope 要求**

```bash
feishu-cli schema vc.meeting.get
# 看 Identity / Scopes 行即可
```

---

## 🔥 黄金搭档：schema 查 → api 调（v1.29+）

`feishu-cli api <method> <path>` 是 v1.29+ 的通用 OpenAPI 透传命令，配合 schema 形成完整闭环。

### 完整工作流

```bash
# Step 1: schema 查 path + scope + token 类型
feishu-cli schema im.chats.create
# 输出: POST /open-apis/im/v1/chats
#       Identity: tenant (bot)
#       Scopes:   im:chat

# Step 2: 本例选择 Bot，确认应用已开通 im:chat；User 登录不能替代应用授权
feishu-cli doctor --only bot_identity

# Step 3: 本地预览；用户已授权创建且参数核对后去掉 --dry-run 执行
feishu-cli api POST /open-apis/im/v1/chats \
  --data '{"name":"测试群","description":"by feishu-cli api"}' \
  --as bot --dry-run
```

### `feishu-cli api` 关键 flag

| Flag | 用途 |
|---|---|
| `--params '<json>'` | Query 参数 |
| `--data '<json>'` | Body（POST/PUT/PATCH） |
| `--data-file <path>` | Body 从文件读（`-` 表示 stdin） |
| `--as bot\|user\|auto` | 强制身份；auto = user 优先回退 bot |
| `--dry-run` | 仅打印请求，不实际调 |
| `--format json\|pretty\|table\|ndjson\|csv` | 响应渲染格式（指定后走内置渲染，覆盖默认 pretty） |
| `--jq '<expr>'` | 内置 gojq 过滤响应（无需外部 jq） |
| `--raw` | 原样输出（默认 pretty JSON） |
| `--include-headers` | stderr 打印响应头 |
| `--output <file>` / `-o` | 写入文件而非 stdout（`-o` 二进制下载与 `--format/--jq` 互斥，见 `feishu-cli-platform` skill） |
| `--timeout <seconds>` | 自定义超时（默认 30s） |
| `--page-all` / `--page-limit` | 仅识别 `data.has_more` + `page_token`/`next_page_token`；空/重复 cursor 停止 |

### 内置错误码翻译（v1.29+）

`feishu-cli api`（以及其他命令）收到飞书业务错误时会在 stderr 附加 `log_id`、所需 scope、字段校验与中文解决方案，覆盖：

- **99991672** → 应用未开通 scope：重新登录修不好，给出开放平台开通链接（`https://open.feishu.cn/app/<app_id>/auth?q=<scope>`）
- **99991679 / 99991676** → 用户未授权该 scope：提示 `feishu-cli auth login --scope "<缺失 scope>"`
- **99991668** → msg 含 `not support` 时提示改用 `--as bot`；否则视为 User Token 无效，提示重新登录
- **99991661/99991663/99991677/20037 等** → Token 缺失、无效、过期或 refresh_token 失效
- **99991400** → 限流
- **230001** → 请求参数无效；**230002** → Bot/用户不在群；**230020** → 触发频率限制
- **232033** → 外部群权限不足（提示切对外共享 App）
- **232011** → Bot/用户不在群
- **232006** → chat_id 无效
- **232025** → App 未启用 Bot 能力

### URL 智能处理

- 只自动剥 `open.feishu.cn` / `open.larksuite.com` / `open.larkoffice.com` 这三类 OpenAPI host；
  不支持任意租户 host
- 自动补 `/open-apis/` 前缀
- 自动拆 URL 里内嵌的 `?query=string` 到 query 参数
- fragment 先于 query 剥离，`?a=1#frag?b=2` 只会留下 `a=1`
- 完整 URL 只接受官方 OpenAPI host 的 **https**；短 path 仍自动补 `/open-apis/`

```bash
# 下面三种等价：
feishu-cli api GET /open-apis/authen/v1/user_info --as user
feishu-cli api GET /authen/v1/user_info --as user
feishu-cli api GET https://open.feishu.cn/open-apis/authen/v1/user_info --as user

# 租户文档 URL 和 fragment 不要直接传；先提取 path 并移除 fragment
feishu-cli api GET '/open-apis/authen/v1/user_info?foo=bar' --as user
```

### 与 `feishu-cli-platform` 的 auth 工作流的关系

如要让其他工具（curl/Python）复用 token 而不通过本命令，用 `feishu-cli auth token --as user/bot` 导出 token 字符串（见 `feishu-cli-platform` skill）。

---

## 踩坑

1. **路径过深会报错**：`schema im.messages.delete.foo` → `路径过深: ...（多余片段: foo）`。多写一层不会被静默吞掉。
2. **路径不存在分级提示**：未知 service / resource / method 都会列出该层的所有可用候选名，便于纠正。
3. **resource 含点号用最长前缀匹配**：`im.chat.members.create` 会匹配 resource = `chat.members`、method = `create`，不必担心拆错。
4. **查询不需要 token**：schema 查询走本地/缓存 catalog。overlay 是无凭证的 public meta 请求，失败不影响命令。
5. **覆盖范围**：embedded baseline 是 12 个 service / 152 method；overlay 生效后为 15 个 service / 250 method（`schema status` 的 `source` 为 `runtime` 或 `cache`）。overlay 仍未收录的域请去飞书 OpenAPI Explorer 查，或直接用专用 `feishu-cli <模块>` 命令 / `feishu-cli api` 透传。
6. **JSON 输出不转义 HTML**：`<` / `>` / `&` 保留原样，便于直接吞进 jq / yq 管道。

---

## 何时转其他技能

| 需求 | 该用什么 |
|------|---------|
| 调 API 而且本项目有对应业务命令 | `feishu-cli <模块>` 对应命令（msg / doc / bitable / drive…）—— 体验更好 |
| 调 API 但没对应业务命令 / 想直接走 OpenAPI | **本 skill 介绍的 `feishu-cli api <method> <path>`**（v1.29+）|
| 给 curl/Python 拿 token | `feishu-cli auth token --as user/bot`（详见 `feishu-cli-platform` skill） |
| 查在线最新 schema、本地没收录 | 飞书 [OpenAPI Explorer](https://open.feishu.cn/api-explorer) |
| 调"埋藏 API"（飞书文档站未收录） | 见 `../api/references/embedded-api-discovery.md`（相对本 workflow 目录），已知 6 个埋藏 API |
| 申请 scope / 登录拿 User Token | `/feishu-cli-platform`（`auth check --scope` 预检、`auth login --domain --recommend` 按业务域申请） |
| 发消息/文档/卡片等具体业务 | `/feishu-cli-messaging` / `/feishu-cli-docs` / `/feishu-cli-messaging` 等专用技能 |
