---
name: feishu-cli-search
description: >-
  搜索飞书云文档、消息和应用。当用户请求"搜索文档"、"搜索消息"、"搜索应用"、"找文档"、
  "search docs"、"查找飞书文档"、"有没有关于 xxx 的飞书文档"时使用。
  也适用于：用户想查找某个主题的飞书文档或 Wiki、按关键词检索消息记录、查找内部应用。
  搜索 API 必须使用 User Access Token，本技能包含完整的认证前置检查流程。
argument-hint: docs | messages | apps <query>
user-invocable: true
allowed-tools: Bash(feishu-cli search:*), Bash(feishu-cli auth:*), Read
---

# 飞书搜索

搜索飞书云文档、消息和应用。所有搜索命令**必须使用 User Access Token**。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

## 执行流程

每次执行搜索前，按以下流程操作：

### 1. 预检 scope（推荐 AI Agent 使用）

```bash
feishu-cli auth check --scope "search:docs:read"
# 或同时检查多个（含搜索应用）
feishu-cli auth check --scope "search:docs:read search:message search:app"
```

根据返回结果判断：
- `ok=true` → 直接执行搜索（步骤 3）
- `error=not_logged_in` 或 `error=token_expired` → 登录（步骤 2）
- `missing=[...]` 非空 → 先在飞书开放平台为应用开通缺失的 scope（见步骤 2），然后重新登录

### 2. 登录获取 Token（如需要）

**本项目使用 Device Flow（RFC 8628）授权**，无需任何 redirect URL 配置。AI Agent 推荐用后台阻塞模式：

```bash
# 后台启动（Claude Code 的 run_in_background=true）
feishu-cli auth login --scope "search:docs:read search:message" --json
```

首行 stdout 输出 `{"event":"device_authorization","verification_uri_complete":"...","user_code":"...","expires_in":240,...}`。将 `verification_uri_complete` 展示给用户，等用户在浏览器完成授权后后台进程自动退出，第二行 stdout 输出 `{"event":"authorization_complete",...}`。

如果 `auth check` 返回 `missing=[...]`，说明应用还没开通所需权限。**feishu-cli 不做权限申请自动化**——引导用户自己去飞书开放平台：

1. 打开飞书开放平台 → 你的应用 → 权限管理页面
2. 搜索并开通缺失的 scope（例如 `search:docs:read`、`search:message`），或复制 README 的完整 JSON 一次性导入
3. 等待 tenant 管理员审批（如果需要）
4. 审批通过后再执行 `feishu-cli auth login --scope "search:docs:read search:message" --json`

> 详细的 AI Agent 授权约定见 `feishu-cli-auth` 技能。

### 3. 执行搜索

登录后所有搜索命令自动从 `~/.feishu-cli/token.json` 读取 Token，无需手动传递。

---

## 搜索云文档

搜索当前用户有权访问的飞书云文档和 Wiki。**scope: `search:docs:read`**

```bash
feishu-cli search docs "关键词" [选项]
```

### 选项

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--docs-types` | string | 全部 | 文档类型过滤（逗号分隔，小写） |
| `--count` | int | 20 | 返回数量（0-50） |
| `--offset` | int | 0 | 偏移量（offset + count < 200） |
| `--owner-ids` | string | — | 文件所有者 Open ID（逗号分隔） |
| `--chat-ids` | string | — | 文件所在群 ID（逗号分隔） |
| `-o json` | string | — | JSON 格式输出 |

### 文档类型（小写）

| 类型 | 说明 | 类型 | 说明 |
|------|------|------|------|
| `doc` | 旧版文档 | `docx` | 新版文档 |
| `sheet` | 电子表格 | `slides` | 幻灯片 |
| `bitable` | 多维表格 | `mindnote` | 思维笔记 |
| `file` | 文件 | `wiki` | 知识库文档 |
| `shortcut` | 快捷方式 | | |

### 示例

```bash
# 基础搜索
feishu-cli search docs "产品需求"

# 只搜索新版文档和 Wiki
feishu-cli search docs "技术方案" --docs-types docx,wiki

# 搜索电子表格
feishu-cli search docs "数据报表" --docs-types sheet

# 分页获取更多
feishu-cli search docs "季度报告" --count 50

# 分页查询：获取第一页（20 条）
feishu-cli search docs "季度报告" --count 20 --offset 0
# 分页查询：获取第二页
feishu-cli search docs "季度报告" --count 20 --offset 20

# JSON 格式输出（适合程序解析）
feishu-cli search docs "产品需求" -o json
```

### v1 vs v2：何时用 `drive search`？

`search docs`（v1，本节）走 `/open-apis/suite/docs-api/search/object`，filter 简单（owner_ids / chat_ids / docs_types）。
**`drive search`**（v2，详见 `feishu-cli-drive` 技能 §9）走 `/open-apis/search/v2/doc_wiki/search`，支持更精细的扁平 filter：

- `--folder-tokens` 限定云盘文件夹（与 `--space-ids` 互斥）
- `--space-ids` 限定知识库 space
- `--creator-ids` / `--sharer-ids` 多人扇出
- `--only-title` / `--only-comment` 维度限定
- `--sort` 排序（edit_time / open_time / create_time / default）

两者都需要 `search:docs:read`，按需选择：粗筛用 v1，精筛用 v2。

### JSON 输出格式

```json
{
  "Total": 35367,
  "HasMore": true,
  "ResUnits": [
    {
      "DocsToken": "doc_token_xxx",
      "DocsType": "docx",
      "Title": "产品需求文档 - Q2",
      "OwnerID": "ou_xxx",
      "URL": "https://feishu.cn/docx/doc_token_xxx"
    }
  ]
}
```

`DocsToken` 可以直接用于 `feishu-cli doc get`、`doc export` 等文档操作命令。

---

## 搜索消息

搜索飞书消息记录。**scope: `search:message`**

```bash
feishu-cli search messages "关键词" [选项]
```

### 选项

| 参数 | 类型 | 说明 |
|------|------|------|
| `--chat-ids` | string | 限定群聊范围（逗号分隔） |
| `--from-ids` | string | 限定发送者 ID（逗号分隔） |
| `--at-chatter-ids` | string | 限定被@的用户 ID（逗号分隔） |
| `--message-type` | string | 消息类型：`file`/`image`/`media` |
| `--chat-type` | string | 会话类型：`group_chat`/`p2p_chat` |
| `--from-type` | string | 发送者类型：`bot`/`user` |
| `--start-time` | string | 起始时间（Unix 秒级时间戳） |
| `--end-time` | string | 结束时间（Unix 秒级时间戳） |
| `--page-size` | int | 每页数量（默认 20） |
| `--page-token` | string | 分页 token（上一页返回） |
| `--page-all` | bool | 自动翻页拉取全部（受 `--page-limit` 限制） |
| `--page-limit` | int | 自动翻页最大页数，`0`=不限（默认 0）；配合 `--page-all` 防失控 |
| `--enrich` | bool | 补全内容/发送者/群名/时间（opt-in，额外 API 调用） |
| `--card-content-type` | string | interactive 卡片富化格式：`user`（默认，提取 `card_texts`）/ `raw`（平台内部完整 cardDSL）/ `rendered`（OAPI 渲染版/降级版）。**仅在 `--enrich` 时生效**，非 enrich 路径忽略 |
| `--format` | string | 结构化输出：`json`/`pretty`/`table`/`ndjson`/`csv` |
| `--jq` | string | 用 jq 表达式过滤结构化输出 |
| `--user-id-type` | string | 用户 ID 类型：`open_id`（默认）/`union_id`/`user_id` |
| `-o json` | string | JSON 格式输出（等价 `--format json`） |

> **`--page-all` 截断无提示**：仅非翻页模式（`!--page-all` 且 `HasMore=true`）会打印"还有更多结果"。当 `--page-all` 因达到 `--page-limit` 提前停止（而非真正耗尽）时，CLI **不会**提示结果被截断——需要拉全量时把 `--page-limit` 设为 `0`，或结合 JSON 输出的 `HasMore` 自行判断。

> **默认 vs `--enrich`**：默认仅返回消息 ID（`-o json` 输出 `{MessageIDs,HasMore,PageToken}`），与历史行为一致、向后兼容。加 `--enrich` 才会多发 `BatchGetMessages` 等 API 补全内容/发送者/群名/时间，`-o json` 此时返回富化后的数组。

### 示例

```bash
# 搜索消息（默认返回消息 ID）
feishu-cli search messages "上线"

# 富化：补全内容/发送者/群名/时间
feishu-cli search messages "上线" --enrich
feishu-cli search messages "上线" --enrich --format table

# 富化 + 控制卡片消息格式（仅 --enrich 生效）
feishu-cli search messages "告警" --enrich --card-content-type raw

# 富化 + 自动翻页（最多 5 页，防失控）
feishu-cli search messages "项目" --enrich --page-all --page-limit 5 --format csv

# 搜索私聊消息（search-chats 无法搜到 p2p 会话，用此方式替代）
feishu-cli search messages "你好" --chat-type p2p_chat

# 搜索群聊中的文件消息
feishu-cli search messages "周报" --chat-type group_chat --message-type file

# 搜索机器人消息
feishu-cli search messages "告警" --from-type bot

# 限定时间范围
feishu-cli search messages "项目" --start-time 1704067200 --end-time 1704153600

# 限定特定群
feishu-cli search messages "会议" --chat-ids oc_xxx,oc_yyy
```

> **提示**：搜索群聊 API（`search-chats`）**无法搜到 p2p 私聊会话**。要查找私聊内容，使用 `search messages --chat-type p2p_chat`。

### JSON 输出格式

默认（无 `--enrich`）：

```json
{
  "MessageIDs": ["om_xxx", "om_yyy"],
  "PageToken": "ea9dcb2f...",
  "HasMore": true
}
```

返回的 `MessageIDs` 可用 `feishu-cli msg get <message_id>` 获取消息详情。

加 `--enrich` 时返回富化后的数组：

```json
[
  {
    "message_id": "om_xxx",
    "msg_type": "text",
    "chat_id": "oc_xxx",
    "chat_name": "项目群",
    "sender_id": "ou_xxx",
    "sender_name": "张三",
    "create_time": "1704067200000",
    "time": "2024-01-01 08:00:00",
    "text": "今天上线"
  }
]
```

---

## 搜索应用

搜索飞书应用。**scope: `search:app`**（飞书官方注册表名，可在飞书开放平台 → 应用 → 权限管理搜索开通；该 scope `recommend=false`，默认不会被 `auth login --recommend` 自动包含，需要 `auth login --scope "search:app"` 显式申请。若飞书侧已重命名，以 `feishu-cli auth check --scope "search:app"` 报错信息为准。）

```bash
feishu-cli search apps "关键词" [选项]
```

### 选项

| 参数 | 类型 | 说明 |
|------|------|------|
| `--page-size` | int | 每页数量（默认 20） |
| `--page-token` | string | 分页 token |
| `-o json` | string | JSON 格式输出 |

### 示例

```bash
feishu-cli search apps "审批"
feishu-cli search apps "OKR" --page-size 50
```

---

## 常见问题

| 问题 | 原因 | 解决 |
|------|------|------|
| "缺少 User Access Token" | 从未登录 | 执行两步式登录流程 |
| "User Access Token 已过期" | access + refresh token 都过期 | 重新登录 |
| 99991679 权限错误提到搜索应用 | 登录 scope 未包含 `search:app`，或飞书开放平台未开通该权限 | `feishu-cli auth login --scope "search:app"`；如仍报错，去飞书开放平台权限管理页搜索 `search:app` 并开通（必要时联系 tenant 管理员审批） |
| 99991679 权限错误提到 `search:docs:read` | 登录时未包含 `search:docs:read` scope | 重新登录，scope 加上 `search:docs:read` |
| 搜索结果为空 | 关键词不匹配或无权限文档 | 尝试更宽泛的关键词，或检查文档权限 |
| offset + count 超过 200 | 飞书 API 限制 | 最多翻到第 200 条结果 |

**完整的认证流程和 Token 管理请参考 `feishu-cli-auth` 技能。**

---

## 与其他技能的分工

| 场景 | 使用技能 |
|------|---------|
| 按关键词搜索文档/应用 | **feishu-cli-search**（本技能） |
| 按关键词搜索消息（含高级筛选） | **feishu-cli-search**（本技能） |
| 浏览群聊历史消息、搜索群聊列表 | feishu-cli-chat |
| Reaction/Pin/删除/获取消息详情 | feishu-cli-chat |
| 群聊信息管理、成员管理 | feishu-cli-chat |

搜索消息与浏览聊天记录的区别：搜索（`search messages`）用关键词跨会话检索，返回消息 ID 列表；浏览（`msg history`）获取指定会话的连续消息流。如果用户的意图是"找到包含某关键词的消息"用搜索，"看看某个群最近在聊什么"用浏览。
