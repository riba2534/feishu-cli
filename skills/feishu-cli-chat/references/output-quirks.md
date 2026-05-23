# 读消息相关命令的输出怪癖速查

> 这些差异不在 OpenAPI 文档里，全部来自实战。改动 `msg history / thread-messages / get`
> 相关代码或写脚本前先扫一眼，避免重复踩。

## 1. JSON key 大小写不统一

| 命令 | 顶层 key 风格 | 字段示例 |
|---|---|---|
| `msg history -o json` | snake_case | `items` / `has_more` / `page_token` / `sender_names` |
| `msg thread-messages` | **PascalCase** | `Items` / `HasMore` / `PageToken` |
| `msg search-chats -o json` | **PascalCase** | `Items` / `HasMore` / `PageToken` |
| `msg get -o json` / `msg mget` | snake_case | 单条/批量消息详情 |

写翻页循环时务必两套都试一下。脚本里用 `d.get("items") or d.get("Items") or []`、
`d.get("page_token") or d.get("PageToken") or ""` 兼容。

## 2. 输出 flag 是否被接受

| 命令 | `-o json` | 默认输出 |
|---|---|---|
| `msg history` | ✅ 必须显式传 | 文本摘要 |
| `msg thread-messages` | ❌ **加了会报 `unknown shorthand flag: 'o'`** | **默认就是 JSON** |
| `msg search-chats` | ✅ | 文本摘要 |
| `msg get` / `msg mget` | ✅ | 文本摘要 |
| `chat get` | ❌ 也不接受 `--output` | 文本（无 JSON 模式） |
| `user info` | ✅ | 文本 |

`thread-messages` 的设计早于 `-o json` 通用化，源码里直接 `printJSON`，因此既不接受
flag 也不能切回文本。脚本里**不要给它传 `-o json`**。

## 3. 时间单位不统一

| 命令 | `--start-time` / `--end-time` 单位 |
|---|---|
| `msg history` | **秒**（unix timestamp） |
| `msg thread-messages` | **毫秒** |

写脚本时分别按 `int(time.time())` 和 `int(time.time() * 1000)` 两套传，不要复用。

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

## 7. Bot 的 sender.id 与 sender_names 不互通

群里 Bot 发消息时：

```json
{"sender": {"id": "cli_a94550d641e49cba", "id_type": "app_id", "sender_type": "app"}}
```

而 `sender_names` 用 **open_id** 索引，例如：

```json
{"ou_4eda5030f3cb3c8801d32de051431b94": "TikTok Shop Virtual AM"}
```

`cli_xxx` 是 app_id，与同一 Bot 的 `ou_xxx` open_id 是两套 ID。`user info cli_xxx`
查不到。脚本里直接把 `id_type=app_id` 的发送者映射到调用方提供的 bot 名字（通常就是
群里那个 bot 的固定显示名），不要试图反解。

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

## 9. 名字反解三级降级

按可靠性 + 成本排序：

1. **`mentions[]`** 字段（消息自带）：`{"id": "ou_xxx", "id_type": "open_id", "name": "张三"}`，
   且 `key` 字段是 text 消息里的 `@_user_N` 占位符 → 真名映射，**不受外部租户隔离限制**，
   首选。
2. **`sender_names`** 字段（history/thread-messages 顶层）：飞书后端反解的子集，**仅覆盖
   少数发送者**（实战中 26 个发送者只反解出 2~3 个），用作起点。
3. **`user info <ou_xxx> -o json`**：跨企业用户必返 `code=41050, msg=no user authority error`。
   外部群（`external=true`）几乎全部命中，静默跳过即可。
4. **bot app_id（`cli_xxx`）**：单独映射成你提前知道的 bot 名字。

剩余实在反解不到的，保留 `ou_xxx` 原样显示，不要伪造名字。

## 10. 话题群的判断

`chat get oc_xxx` 能拿 `chat_mode`，但**对外部群常返 `code=232033, msg=The operator or
invited bots does NOT have the authority to manage external chats.`**。

更可靠的判断方式：拉一页 history 后看消息字段——若 `items[].thread_id` 几乎所有非 system
消息都有，就是话题群（`chat_mode=thread`）；否则普通群。话题群每条主消息可对应 0~N 条
线程回复，需逐个 `msg thread-messages <tid>` 展开。

## 11. 话题群的"完整"含义

`msg history` 默认**不展开线程**，只返回每个 thread 的根消息（一条主消息 = 一个 thread）。
回复要单独 `msg thread-messages <tid>` 拉。如果只读 history 就报"群里 24 小时只有 25 条
消息"是错的——93 条线程回复全漏了。

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

## 13. Token 路径（实战观察）

`msg history` / `msg thread-messages` 在 CLAUDE.md 中归类为"读类 · User 优先 + Tenant 兜底"，
未登录时回落 App Token（要求 Bot 在群里）。外部群 Bot 通常**不在群里**，所以读外部群
**必须登录 User Token**：

```bash
feishu-cli auth check --scope "im:message:readonly im:message.group_msg:get_as_user"
feishu-cli auth login --domain chat --recommend
```

## 14. 错误码速查

| code | msg | 何处出现 | 处理 |
|---|---|---|---|
| 232033 | does NOT have the authority to manage external chats | `chat get` 外部群 | 改用消息字段判断话题群 |
| 41050 | no user authority error | `user info <跨企业 ou_xxx>` | 静默跳过，靠 mentions 兜底 |
| 99992354 | not a valid open_message_id | `msg get om_xxx` 用了不存在/不属于本租户的 id | 检查 message_id 来源 |
| 1062507 | 文件夹子节点 ≤ 1500 | 不在读消息路径，列出仅作参考 | — |
