---
name: feishu-cli-schema
description: >-
  飞书 OpenAPI 方法本地浏览。schema <service.resource.method> 查路径/参数/scope（无需联网）；
  schema list 列出所有可用 service。复用 feishu-cli 内置 OpenAPI 元数据（embed 746KB）。
  当用户请求"飞书有没有 XX API"、"X API 的参数是什么"、"X 方法需要什么 scope"、
  "OpenAPI 方法浏览"、"看 SDK 怎么调用"时使用。
  不适用：调用 API（请用 lark-cli api）、查在线最新 schema（请用 OpenAPI Explorer）。
argument-hint: <service.resource.method>
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书 OpenAPI 方法浏览技能

通过 `feishu-cli schema` 在本地查询飞书开放平台 OpenAPI 方法的 HTTP path / 动词 / 参数 / 请求体 / 响应体 / scope / 文档链接。**纯本地**、无需 Token、无需网络。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

> **要真正调用 API？** 本技能只查 schema，不发请求。拿到 path + scope 后用 `lark-cli api` 或对应 `feishu-cli <模块>` 命令调用。

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

编译期 `embed` 的 `internal/registry/meta_data.json`（约 746KB），与 `auth check` 等模块共用同一份元数据。当前覆盖 **12 个 service**：approval / attendance / calendar / drive / im / mail / minutes / sheets / slides / task / vc / wiki。

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
feishu-cli schema im.messages.create
# 然后用 lark-cli api 或 feishu-cli msg send 调用
```

**3. AI Agent 拿 JSON 推断调用**

```bash
feishu-cli schema bitable.records.create --format json
```

**4. 确认某方法的 scope 要求**

```bash
feishu-cli schema vc.notes.get
# 看 Identity / Scopes 行即可
```

---

## 踩坑

1. **路径过深会报错**：`schema im.messages.delete.foo` → `路径过深: ...（多余片段: foo）`。多写一层不会被静默吞掉。
2. **路径不存在分级提示**：未知 service / resource / method 都会列出该层的所有可用候选名，便于纠正。
3. **resource 含点号用最长前缀匹配**：`im.chat.members.create` 会匹配 resource = `chat.members`、method = `create`，不必担心拆错。
4. **只读不调 API**：本命令永远不发起 HTTP 请求，不消耗任何配额，没有 token 过期顾虑。要真正调用见下方"何时转其他技能"。
5. **覆盖范围 = 12 service**：当前未含 docx / contact / approval 等的较新 endpoint；如果 `schema list` 里没列出，说明本地元数据未收录，请去飞书 OpenAPI Explorer 查在线最新版。
6. **JSON 输出不转义 HTML**：`<` / `>` / `&` 保留原样，便于直接吞进 jq / yq 管道。

---

## 何时转其他技能

| 需求 | 该用什么 |
|------|---------|
| 真的要调 API | `lark-cli api` 通用调用 / `feishu-cli <模块>` 对应命令（msg / doc / bitable / drive…） |
| 查在线最新 schema、本地没收录 | 飞书 [OpenAPI Explorer](https://open.feishu.cn/api-explorer) |
| 申请 scope / 登录拿 User Token | `/feishu-cli-auth`（`auth check --scope` 预检、`auth login --domain --recommend` 按业务域申请） |
| 发消息/文档/卡片等具体业务 | `/feishu-cli-msg` / `/feishu-cli-read` / `/feishu-cli-card` 等专用技能 |
