---
name: feishu-cli-search
description: >-
  搜索飞书云文档、消息和应用。当用户请求"搜索文档"、"搜索消息"、"搜索应用"、"找文档"、
  "找一下"、"search docs"、"查找飞书文档"、"有没有关于 xxx 的文档"时使用。
  也适用于：用户想查找某个主题的飞书文档或 Wiki、按关键词检索消息记录、查找内部应用。
  搜索 API 必须使用 User Access Token，本技能包含完整的认证前置检查流程。
user-invocable: true
allowed-tools: Bash
---

# 飞书搜索

搜索飞书云文档、消息和应用。所有搜索命令**必须使用 User Access Token**。

## 执行流程

每次执行搜索前，按以下流程操作：

### 1. 检查 Token 状态

```bash
feishu-cli auth status -o json
```

根据返回结果判断：
- `logged_in=false` → 需要登录（步骤 2）
- `access_token_valid=true` + scope 包含所需权限 → 直接搜索（步骤 3）
- `access_token_valid=false` + `refresh_token_valid=true` → 无需操作，下次搜索时自动刷新
- `access_token_valid=false` + `refresh_token_valid=false` → 需要重新登录（步骤 2）
- scope 缺少所需权限 → 需要重新登录并补充 scope（步骤 2）

### 2. 登录获取 Token（如需要）

使用两步式非交互登录，**始终使用最大 scope 范围**（覆盖搜索 + wiki + 日历 + 任务等全部功能）：

```bash
# 步骤 A：生成授权 URL（最大 scope）
feishu-cli auth login --print-url --scopes \
  "offline_access \
   search:docs:read search:message drive:drive.search:readonly \
   wiki:wiki:readonly \
   calendar:calendar:read calendar:calendar.event:read \
   calendar:calendar.event:create calendar:calendar.event:update \
   calendar:calendar.event:reply calendar:calendar.free_busy:read \
   task:task:read task:task:write \
   task:tasklist:read task:tasklist:write \
   im:message:readonly im:message.group_msg:get_as_user im:chat:read contact:user.base:readonly \
   drive:drive.metadata:readonly"
```

> **scope 命名说明**：飞书 OAuth scope 命名不完全统一，`search:docs:read` 带 `:read` 后缀，而 `search:message` 不带。这是飞书平台定义，非笔误。

将输出的 `auth_url` 展示给用户，请用户在浏览器中完成授权。授权后浏览器跳转到无法访问的页面，让用户复制地址栏完整 URL。

```bash
# 步骤 B：用回调 URL 换 Token
feishu-cli auth callback "<用户提供的回调URL>" --state "<步骤A输出的state>"
```

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
| `-o json` | string | JSON 格式输出 |

### 示例

```bash
# 搜索消息
feishu-cli search messages "上线"

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

```json
{
  "MessageIDs": ["om_xxx", "om_yyy"],
  "PageToken": "ea9dcb2f...",
  "HasMore": true
}
```

返回的 `MessageIDs` 可用 `feishu-cli msg get <message_id>` 获取消息详情。

---

## 搜索应用

搜索飞书应用。**注意：搜索应用的 scope 需在飞书开发者后台确认是否可用，部分应用可能未开通此权限。**

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
| 99991679 权限错误提到搜索应用 | 应用未开通搜索应用权限，或该 scope 在开发者后台不可用 | 在飞书开发者后台确认是否已开通对应权限 |
| 99991679 权限错误提到 `search:docs:read` | 登录时未包含 `search:docs:read` scope | 重新登录，scope 加上 `search:docs:read` |
| 搜索结果为空 | 关键词不匹配或无权限文档 | 尝试更宽泛的关键词，或检查文档权限 |
| offset + count 超过 200 | 飞书 API 限制 | 最多翻到第 200 条结果 |

**完整的认证流程和 Token 管理请参考 `feishu-cli-auth` 技能。**
