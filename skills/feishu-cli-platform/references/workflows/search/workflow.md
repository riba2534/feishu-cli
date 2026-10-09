# 飞书全局搜索

搜索飞书云文档、消息和应用（`search docs` / `search messages` / `search apps`）。业务域内的查询（审批、会议、
邮箱、任务等）不走这里；按文件夹/知识库精筛文档用 `feishu-cli-storage` 的 `drive search`。

| 命令 | 身份 | scope |
|---|---|---|
| `search docs` | 必须 User | `search:docs:read` |
| `search apps` | 必须 User | `search:app`（不在 `--recommend` 推荐集内，需 `--scope "search:app"` 显式申请） |
| `search messages` | `--as bot\|user\|auto`，默认 auto | `search:message` |

参数映射、文档类型与 JSON 输出格式见 `references/commands.md`；身份规则见 [身份选择](../auth/references/identity.md)。

## 执行流程

1. **选身份**：`search docs/apps` 必须 User；`search messages --as bot` 不依赖个人登录，确认应用已开通
   `search:message`（`auth scopes --scope "search:message" -o json` 的 `tenant_enabled`）后直接搜索；
   auto 在已配置 User 但刷新失败时直接报错，不会静默切 Bot。
2. **预检本地 User Token**（仅 User 路径）：

   ```bash
   feishu-cli auth check --scope "search:docs:read search:message"
   ```

   - `ok=true` → 直接搜索。
   - `error=not_logged_in` / `token_expired` → 按 auth 工作流的两步模式登录，如
     `feishu-cli auth login --domain search --recommend --no-wait --json`。
   - `missing` 非空 → 先 `feishu-cli auth scopes --scope "<缺失的>" -o json` 区分：`app_not_enabled` /
     `tenant_only` 需应用管理员在开放平台开通并发布；`user_not_granted` 直接 `auth login --scope "<缺失的>"` 补授。
3. **执行搜索**：本地 User 模式自动从当前 profile 读取 Token（含自动刷新）。

## 搜索云文档

```bash
feishu-cli search docs "关键词" [--docs-types docx,wiki] [--count 20] [--offset 0] [--owner-ids ou_xxx] [--chat-ids oc_xxx] [-o json]
```

- `--count` 0–50（默认 20），`--offset` 需满足 `offset + count < 200`，最多翻到第 200 条。
- `--docs-types` 用小写：`doc` `docx` `sheet` `slides` `bitable` `mindnote` `file` `wiki` `shortcut`。
- 结果的 `URL` 按配置品牌拼成 `https://www.feishu.cn/...`（Lark 为 `https://www.larksuite.com/...`），打开后由服务端重定向到租户域名。
- 后续操作必须看 `DocsType`，不能把所有 `DocsToken` 都交给 `doc` 命令：docx 走 doc，sheet 走 sheet，
  wiki 先按节点类型解析，bitable/file/slides 分别走对应命令。

```bash
feishu-cli search docs "技术方案" --docs-types docx,wiki
feishu-cli search docs "季度报告" --count 20 --offset 20 -o json
```

**`search docs` 与 `drive search`**：`search docs` 走 `/open-apis/suite/docs-api/search/object`，过滤只有
所有者、所在群和类型；`drive search`（`feishu-cli-storage` 的 drive 工作流第 9 节）走 `/open-apis/search/v2/doc_wiki/search`，
支持文件夹（`--folder-tokens`）、知识库（`--space-ids`）、创建者/分享者、仅标题/仅评论与排序。两者都需要
`search:docs:read`：粗筛用 `search docs`，按位置或维度精筛用 `drive search`。

## 搜索消息

走 `POST /open-apis/im/v1/messages/search`。query 可省略，仅靠过滤条件搜索。

```bash
feishu-cli search messages ["关键词"] [过滤条件] [--enrich] [--page-all --page-limit N] [--format json|pretty|table|ndjson|csv] [--jq '<expr>']
```

- **默认只返回消息 ID**：人类可读列表，`-o json` / `--format json` 为 `{MessageIDs, HasMore, PageToken}`（有服务端提示时多一个 `notice`）。
  拿 ID 后用 `feishu-cli msg get <message_id>` 或 `msg mget` 看详情。
- **`--enrich`** 额外调用 `GET /im/v1/messages/mget`（每批最多 50）等接口补全内容、发送者、群名和时间，
  JSON 变为对象数组。`--card-content-type user|raw|rendered` 只在 `--enrich` 时生效。
- **过滤条件**：`--chat-ids`、`--from-ids`、`--at-chatter-ids`（逗号分隔）；`--chat-type group_chat|p2p_chat`
  （也接受 `group`/`p2p`）；`--from-type` / `--exclude-from-type bot|user`；`--is-at-me`；
  `--message-type file|image|media|video|link`（`media` 映射为 `video`）；`--start-time` / `--end-time` 接受
  RFC3339、`YYYY-MM-DD` 或 Unix 秒。
- **分页**：`--page-size` 1–50（默认 20，越界报错）；`--page-all` 最多 40 页，`--page-limit` 1–40，
  `0` 在 `--page-all` 时等于 40（不是无限），负数发网前失败；空/重复游标也会停止。
- **服务端 notice**：如查询词超过 50 字被截断时，CLI 把提示写到 stderr（`[提示] 服务端提示: ...`），
  非 enrich 的 JSON 额外带 `notice` 字段。看到截断提示应缩短查询词。`msg search-chats` 同样处理。
- 非法 `--format` / `--jq` 在身份解析和发网前失败。

```bash
# 富化后表格输出
feishu-cli search messages "上线" --enrich --format table

# 私聊消息（msg search-chats 搜不到 p2p 会话，用这个替代）
feishu-cli search messages "你好" --chat-type p2p_chat

# 群聊里的文件消息，限定时间范围
feishu-cli search messages "周报" --chat-type group_chat --message-type file --start-time 2026-01-01 --end-time 2026-01-31

# 指定群 + 自动翻页（最多 5 页）+ CSV
feishu-cli search messages "项目" --chat-ids oc_xxx --enrich --page-all --page-limit 5 --format csv

# 应用身份搜索（无人值守）
feishu-cli search messages "告警" --from-type bot --as bot
```

## 搜索应用

```bash
feishu-cli search apps "关键词" [--page-size 20] [--page-token <token>] [--user-id-type open_id] [-o json]
```

`search:app` 不在 `auth login --recommend` 的推荐集内；需要时先 `auth scopes --scope "search:app"` 确认应用已开通，
再 `auth login --scope "search:app"` 显式申请。

## 常见问题

| 问题 | 原因 | 解决 |
|------|------|------|
| 缺少 User Access Token（退出码 3） | 从未登录 | 按 auth 工作流两步模式登录 |
| User Token 已过期 | access + refresh token 都失效 | 重新登录 |
| 99991679 提到 `search:app` / `search:docs:read`（退出码 3） | 用户未授权该 scope，或应用未开通（服务端对未开通的 user scope 也可能报 99991679） | `auth scopes --scope "<scope>"` 确认应用侧；`app_not_enabled` 先在开放平台开通，`user_not_granted` 执行 `auth login --scope "<scope>"` |
| 99991672 | 应用未开通 scope | 按 stderr 的开放平台链接开通并发布，重新登录修不好 |
| 搜索结果为空 | 关键词不匹配、无权限，或 `--as bot` 时应用看不到对应会话 | 换更宽泛的关键词，确认身份与可见范围 |
| `offset + count` 超过 200 | 接口限制 | 最多翻到第 200 条结果 |

## 与其他技能的分工

| 场景 | 使用技能 |
|------|---------|
| 按关键词搜索文档/应用/消息（含高级过滤） | **feishu-cli-platform**（本工作流） |
| 按文件夹、知识库精筛云文档（`drive search`） | feishu-cli-storage |
| 浏览群聊历史消息、搜索群聊列表（`msg search-chats`） | feishu-cli-messaging |
| Reaction/Pin/删除/获取消息详情、群成员管理 | feishu-cli-messaging |

搜索（`search messages`）用关键词跨会话检索，返回消息 ID 列表；浏览（`msg history`）获取指定会话的连续消息流。
用户的意图是"找到包含某关键词的消息"用搜索，"看看某个群最近在聊什么"用浏览。
