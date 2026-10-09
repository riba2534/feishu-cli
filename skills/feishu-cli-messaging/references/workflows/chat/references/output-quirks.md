# 读消息相关命令的输出怪癖速查

> 这些差异不在 OpenAPI 文档里，来自 CLI 源码与实战。写解析脚本前先扫一眼，避免重复踩。

## 1. JSON key 大小写不统一

| 命令 | 顶层 key 风格 | 字段示例 |
|---|---|---|
| `msg history -o json` | snake_case | `items` / `has_more` / `page_token` / `sender_names` |
| `msg get -o json` / `msg mget` | snake_case | `message` 或 `messages` + `sender_names` |
| `msg thread-messages` | **PascalCase** | `Items` / `HasMore` / `PageToken`（没有 `sender_names`） |
| `msg search-chats -o json` | **PascalCase** | `Items` / `HasMore` / `PageToken`，服务端提示在小写 `notice` |
| `chat list -o json` / `chat member list` | snake_case | `items` / `has_more` / `page_token` |

写翻页循环时两套都要兼容：`d.get("items") or d.get("Items") or []`、
`d.get("page_token") or d.get("PageToken") or ""`。

## 2. 输出 flag 是否被接受

| 命令 | `-o json` | 默认输出 |
|---|---|---|
| `msg history` / `msg list` / `msg get` / `msg read-users` | ✅ 需显式传 | 文本摘要 |
| `msg search-chats` / `chat list` | ✅ 需显式传 | 文本摘要 |
| `msg thread-messages` / `msg mget` / `msg reaction list` / `msg pins` / `msg flag list` / `chat get` / `chat member list` | ❌ 传了报 `unknown shorthand flag: 'o'` | **始终 JSON** |
| `user info` | ✅ | 文本 |

脚本里**不要给始终输出 JSON 的命令传 `-o json`**。

## 3. 时间参数

| 命令 | `--start-time` / `--end-time` |
|---|---|
| `msg history` / `msg list` | **秒**（unix timestamp），服务端过滤 |
| `msg thread-messages` | **秒**（也兼容毫秒 / RFC3339 / YYYY-MM-DD），**客户端本地过滤当前页** |
| `msg pins` | **毫秒** |

历史文档曾写 `thread-messages` 用毫秒：实测服务端对 thread 容器**忽略**时间范围，传什么都不生效。
现在 CLI 不再把时间发给服务端，而是按消息 `create_time` 在本地过滤（结束时间含整秒边界），
过滤只作用于当前页；`has_more=true` 时需带 `--page-token` 继续翻页，stderr 会提示。

消息体里的 `create_time` 字段则全部是 **毫秒字符串**（无论哪条命令）。

## 4. `body.content` 不一定是 JSON

绝大多数消息 `body.content` 是 JSON 字符串（要 `json.loads` 一次），但**撤回消息**
直接是字面量字符串：

```json
{"body": {"content": "This message was recalled"}}
```

解析时务必 `try/except` 包住 `json.loads`，失败时把原字符串当结果显示，否则脚本会
在撤回消息处崩。

## 5. post 富文本有两种结构

OpenAPI 文档示例：

```json
{"zh_cn": {"title": "...", "content": [[{"tag": "text", "text": "..."}]]}}
```

IM 实际下发常用：

```json
{"title": "...", "content": [[{"tag": "text", "text": "..."}]]}
```

——直接平铺，没有 `zh_cn` 包装。两种都要兼容：

```python
block = c.get("zh_cn") or c.get("en_us") or (c if "content" in c else {})
```

## 6. system 消息的模板占位符

system 消息 `body.content` 形如：

```json
{
  "template": "{from_user} invited {to_chatters} to the group...",
  "from_user": ["张三"],
  "to_chatters": ["李四", "王五"],
  "divider_text": {}
}
```

把 `template` 里的 `{key}` 替换成同对象其他字段（list 用逗号 join，str 直接替换，
`divider_text` 跳过）。否则会看到一串带占位符的英文模板。

## 7. Bot 发送者的 ID

群里 Bot 发消息时 sender 是 app_id：

```json
{"sender": {"id": "cli_xxx", "id_type": "app_id", "sender_type": "app"}}
```

`sender_names` 按 sender.id 索引，Bot 的键就是 `cli_xxx`（服务端回填显示名），可以直接查。
`cli_xxx` 与同一 Bot 的 `ou_xxx` open_id 是两套 ID，`user info cli_xxx` 查不到；`sender_names` 里没有时
（例如解析 `thread-messages` 输出），再映射到调用方提供的 Bot 名字。

## 8. interactive 卡片 v2 schema 的解析路径

判断 schema：`c.get("schema") == "2.0"`。

```
c.header.title.content                  → 卡片大标题
c.header.subtitle.content               → 副标题
c.header.template                       → 颜色（blue/red/violet/...）
c.body.elements[]                       → 主体（递归处理）
```

`body.elements[]` 里要递归的 tag：

| tag | 提取路径 |
|---|---|
| `markdown` / `div` / `plain_text` | `el.content` 或 `el.text.content` |
| `column_set` | 遍历 `el.columns[].elements[]` |
| `form` | 递归 `el.elements[]` |
| `collapsible_panel` | 标题在 `el.header.title.content`，内容在 `el.elements[]` |
| `action` | 遍历 `el.actions[]`，提取 `text.content` 和 `url`/`multi_url.url` |
| `button` | `el.text.content` + `el.url`/`el.multi_url.url` |
| `img` | `el.alt.content` |
| `note` | `el.elements[]` 拼空格 |
| `hr` | 分割线 |

老版 v1 卡片走 `c.elements[]`（不在 `body` 里）；`type=template` / `type=card` 只能
打印 `template_id` / `card_id`（无 inline 内容）。

兜底：API 已抽取的 `body.card_texts`（数组）可以作为 fallback，但 v2 schema 完整解析
通常更完整。

## 9. 名字反解降级顺序

按可靠性 + 成本排序：

1. **`sender_names`**（`msg history` / `msg get` / `msg mget` 顶层）：服务端回填显示名，含 Bot 与跨租户外部用户，首选。
2. **`mentions[]`**（消息自带）：`{"id": "ou_xxx", "id_type": "open_id", "name": "张三", "key": "@_user_1"}`，
   `key` 是 text 消息里的 `@_user_N` 占位符；被 @ 的人只能从这里拿名字。
3. **`user info <ou_xxx> -o json`**：跨企业用户返回 `code=41050, msg=no user authority error`，静默跳过。
4. **bot app_id（`cli_xxx`）**：`sender_names` 缺失时映射成已知的 Bot 名字。

剩余实在反解不到的，保留原 ID 显示，不要伪造名字。

## 10. 话题群的判断

- `chat list -o json` 的 `items[].chat_mode` 为 `topic` 即话题群（`group` 为普通群）；`msg search-chats` 的
  `chat_mode` 原样透出搜索接口的枚举，写法不同。
- `chat get oc_xxx`（始终输出 JSON）也有 `chat_mode`，但外部群常返回 `232033`。
- 兜底：拉一页 history 后看消息字段——几乎所有非 system 消息都带 `thread_id` 的是话题群。

## 11. 话题群的"完整"含义

`msg history` 对群聊容器只取话题根消息（`only_thread_root_messages=true`），并**默认展开**每个话题的回复到
顶层 `thread_replies`（每话题 50 条、累计 500 条上限，`thread_has_more` 标记未拉完的话题）。只读 `items`
会漏掉全部回复；显式 `--expand-threads=false` 或某话题 `thread_has_more=true` 时，需要再用
`msg thread-messages <omt_xxx>` 翻页补齐。

## 12. 翻页的稳健写法

读时间窗内的"全部"消息：

```bash
feishu-cli msg history --container-id oc_xxx --container-id-type chat \
    --start-time <unix-sec> --end-time <unix-sec> \
    --sort-type ByCreateTimeAsc \
    --page-size 50 -o json
# 用返回的 page_token 循环；has_more=false 时停
```

为什么 **Asc**：从老到新翻页时，一旦命中 `has_more=false` 就肯定到当前结尾，逻辑清晰；
默认的 Desc + `start-time` 在某些版本里会先返回最新页，再往前翻反而绕。

## 13. Token 路径

`msg list/get/mget/thread-messages` 是"读类 · User 优先 + Bot 兜底"；`msg history` 群聊入口用 `--as`（默认 auto，
User Token 不可用时 stderr 告警后改用 Bot），私聊入口（`--user-id/--user-email`）必须 User。未登录时回落 Bot
要求 Bot 在群里；外部群 Bot 通常**不在群里**，读外部群先确认 User 授权：

```bash
feishu-cli auth check --scope "im:message:readonly im:message.group_msg:get_as_user"
feishu-cli auth login --domain chat --recommend
```

## 14. 错误码速查

| code / 报错 | 何处出现 | 处理 |
|---|---|---|
| 232033 `does NOT have the authority to manage external chats` | `chat get/member list` 等外部群操作 | 读 `external-chat.md`；判断话题群改用 §10 的其他方式 |
| 41050 `no user authority error` | `user info <跨企业 ou_xxx>` | 静默跳过，靠 `sender_names` / `mentions` |
| 99992354 `not a valid open_message_id` | `msg get/mget` 等用了不存在或不属于本租户的 message_id | 检查 message_id 来源 |
| 231007 `no permission to delete this reaction` | `msg reaction remove` 用了与添加时不同的身份 | 换回添加表情时的身份（`--as`） |
| 230026 / 230009 | `msg delete`：Bot 只能撤回自己的消息 / 超过企业设置的撤回时限 | 换有权限的身份，或放弃撤回 |
| `tenant token type not match user access token` | 已登录时执行 `msg read-users` | 接口只收 Bot：用 `feishu-cli api GET /open-apis/im/v1/messages/<om_xxx>/read_users --as bot` |
