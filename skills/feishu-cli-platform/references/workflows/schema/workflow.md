# 飞书 OpenAPI 浏览 + 调用技能

两个命令的组合拳：
- **`feishu-cli schema`** —— 本地查询 OpenAPI 方法的 path/动词/参数/scope/文档链接（纯本地，无需 Token / 无需网络）
- **`feishu-cli api`** —— 直接调用任意飞书 OpenAPI 端点（v1.29+，自动鉴权 + 错误码翻译，覆盖未封装的 2500+ 端点）

典型工作流：`schema list 发现` → `schema get 看参数 + scope` → `feishu-cli auth check/login` 补 scope → `feishu-cli api` 调用。

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

- `schema` 无参数 → 列出所有 service
- `schema <service>` → 列出该 service 下所有 resource.method
- `schema <service>.<resource>` → 列出 resource 下的所有 method
- `schema <service>.<resource>.<method>` → method 详情（含 path / 参数 / scope / docUrl）

### 数据来源

编译期 `embed` 的 `internal/registry/meta_data.json`（约 690KB），与 `auth check` 等模块共用同一份元数据。当前覆盖 **12 个 service**：approval / attendance / calendar / drive / im / mail / minutes / sheets / slides / task / vc / wiki。

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

# Step 2: scope 不够时补
feishu-cli auth check --scope "im:chat"
feishu-cli auth login --scope "im:chat"          # 缺则补

# Step 3: 直接调（无需写代码 / 无需 curl）
feishu-cli api POST /open-apis/im/v1/chats \
  --data '{"name":"测试群","description":"by feishu-cli api"}' \
  --as bot
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

### 内置错误码翻译（v1.29+）

`feishu-cli api` 收到飞书业务错误时会自动打印中文解决方案，覆盖：

- **99991661/99991663/99991668/99991672/99991679/99991677** → Token 失效或权限不足
- **99991400** → 限流
- **230001/230002/230020** → scope 不足，提示 `auth check --scope`
- **232033** → 外部群权限不足（提示切对外共享 App）
- **232011** → Bot/用户不在群
- **232006** → chat_id 无效
- **232025** → App 未启用 Bot 能力

### URL 智能处理

- 只自动剥 `open.feishu.cn` / `open.larksuite.com` / `open.larkoffice.com` 这三类 OpenAPI host；
  不支持任意租户 host
- 自动补 `/open-apis/` 前缀
- 自动拆 URL 里内嵌的 `?query=string` 到 query 参数
- 不要传 `#fragment`；`?query#fragment` 中的 fragment 当前会落入 query 值

```bash
# 下面三种等价：
feishu-cli api GET /open-apis/authen/v1/user_info --as user
feishu-cli api GET /authen/v1/user_info --as user
feishu-cli api GET https://open.feishu.cn/open-apis/authen/v1/user_info --as user

# 租户文档 URL 和 fragment 不要直接传；先提取 path 并移除 fragment
feishu-cli api GET '/open-apis/authen/v1/user_info?foo=bar' --as user
```

### 与 `feishu-cli-shared` skill 的关系

如要让其他工具（curl/Python）复用 token 而不通过本命令，用 `feishu-cli auth token --as user/bot` 导出 token 字符串（见 `feishu-cli-platform` skill）。

---

## 踩坑

1. **路径过深会报错**：`schema im.messages.delete.foo` → `路径过深: ...（多余片段: foo）`。多写一层不会被静默吞掉。
2. **路径不存在分级提示**：未知 service / resource / method 都会列出该层的所有可用候选名，便于纠正。
3. **resource 含点号用最长前缀匹配**：`im.chat.members.create` 会匹配 resource = `chat.members`、method = `create`，不必担心拆错。
4. **只读不调 API**：本命令永远不发起 HTTP 请求，不消耗任何配额，没有 token 过期顾虑。要真正调用见下方"何时转其他技能"。
5. **覆盖范围 = 12 service**：当前覆盖 approval / attendance / calendar / drive / im / mail / minutes / sheets / slides / task / vc / wiki；**未含 docx / bitable(base) / contact / authen / board / okr / tenant / application 等业务域**；如果 `schema list` 里没列出，说明本地元数据未收录，请去飞书 OpenAPI Explorer 查在线最新版（或本项目对应的专用 `feishu-cli <模块>` 命令通常都已封装好）。
6. **JSON 输出不转义 HTML**：`<` / `>` / `&` 保留原样，便于直接吞进 jq / yq 管道。

---

## 何时转其他技能

| 需求 | 该用什么 |
|------|---------|
| 调 API 而且本项目有对应业务命令 | `feishu-cli <模块>` 对应命令（msg / doc / bitable / drive…）—— 体验更好 |
| 调 API 但没对应业务命令 / 想直接走 OpenAPI | **本 skill 介绍的 `feishu-cli api <method> <path>`**（v1.29+）|
| 给 curl/Python 拿 token | `feishu-cli auth token --as user/bot`（详见 `feishu-cli-platform` skill） |
| 查在线最新 schema、本地没收录 | 飞书 [OpenAPI Explorer](https://open.feishu.cn/api-explorer) |
| 调"埋藏 API"（飞书文档站未收录） | 见 `skills/feishu-cli-platform/references/workflows/api/references/embedded-api-discovery.md`，已知 6 个埋藏 API |
| 申请 scope / 登录拿 User Token | `/feishu-cli-platform`（`auth check --scope` 预检、`auth login --domain --recommend` 按业务域申请） |
| 发消息/文档/卡片等具体业务 | `/feishu-cli-messaging` / `/feishu-cli-docs` / `/feishu-cli-messaging` 等专用技能 |
