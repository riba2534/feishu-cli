---
name: feishu-cli-chat
description: >-
  飞书会话浏览、消息互动与群聊管理。看聊天记录（单聊/群聊/话题群）、搜群、获取消息详情、
  Reaction 表情回应、Pin 置顶/取消置顶、删除消息，以及群聊信息管理（获取/更新/解散/成员）。
  当用户请求"看群消息记录 / 拉群聊天记录 / 导出聊天记录 / 获取群历史 / 看话题回复 /
  搜群 / 看私聊记录 / dump 飞书消息"时使用本技能；包括话题群（chat_mode=thread）的整线程展开。
  读类（msg history/list/get/mget/thread-messages）登录后默认 User Token、未登录回落 Bot；
  互动类（reaction/pin/search-chats/chat get/update/delete/member）必需 User Token；
  msg delete 默认 App Token 用于 Bot 自撤回，可显式 User Token 给管理员撤回场景；
  chat create/link 默认 Bot 身份创建群/获取群链接。
argument-hint: <chat_id|群名|用户名>
user-invocable: true
allowed-tools: Bash(feishu-cli auth:*), Bash(feishu-cli msg:*), Bash(feishu-cli chat:*), Bash(feishu-cli search:*), Bash(feishu-cli user:*), Bash(python3:*), Read, Write
---

# 飞书会话浏览与管理

本技能处理"读聊天记录 / 管群 / 消息互动"。发送新消息走 `feishu-cli-msg`；构造卡片走 `feishu-cli-card`。

## 选哪条路径

| 场景 | 走哪 |
|---|---|
| 看一段时间窗内的群消息（含话题回复、名字反解、卡片解析） | **`scripts/fetch_chat_history.py`**（一条命令搞定） |
| 看一页群聊最新消息 | `msg history` 单次调用 |
| 看私聊记录 | `msg history --user-email` 或 `--user-id` |
| 找群 | `msg search-chats --query` |
| 看单条消息 / 合并转发 | `msg get` / `msg mget` |
| 看一个话题的全部回复 | `msg thread-messages <thread_id>` |
| 按关键词搜消息 | `search messages`（属于 `feishu-cli-search`） |
| 群成员管理、改群名 | `chat update` / `chat member ...` |
| Reaction / Pin / 删除 | `msg reaction` / `msg pin` / `msg delete` |

## 认证

按命令类型区分：

- **读类**（`msg history/list/get/mget/thread-messages/resource-download`）：登录后自动从 `~/.feishu-cli/token.json` 加载 User Token，未登录回落 App Token（要求 Bot 在群里）。**外部群 Bot 通常不在群里**，读外部群必须先登录：
  ```bash
  feishu-cli auth check --scope "im:message:readonly im:message.group_msg:get_as_user"
  feishu-cli auth login --domain chat --recommend
  ```
- **必需 User Token**（`reaction add/remove/list`、`pin/unpin/pins`、`search-chats`、`chat get/update/delete`、`chat member list/add/remove`）：未登录直接报错。
- **写类 · 默认 Bot 身份**（`chat create`、`chat link`）：默认 App Token，无需登录；显式 `--user-access-token` 时切到 User（以本人身份创建）。
- **`msg delete`**：默认 App Token，用于 Bot 撤回自己 24 小时内发送的消息；传 `--user-access-token` 或环境变量时可走管理员撤回场景。

## 端到端：拉一段时间窗的完整聊天记录

最常见的需求——"把群 X 最近 24 小时的全部消息拉出来"——同时涉及翻页、话题展开、
名字反解、撤回消息、富文本和卡片渲染。这些步骤每一步都有不止一个坑（详见
`references/output-quirks.md`），单条 CLI 解不完，所以封装成脚本：

```bash
# 默认最近 24 小时
python3 skills/feishu-cli-chat/scripts/fetch_chat_history.py oc_xxxxxxxx --since 24h

# 自定义时间窗 / 输出目录 / Bot 显示名
python3 skills/feishu-cli-chat/scripts/fetch_chat_history.py oc_xxxxxxxx \
    --start 2026-05-20T00:00:00 --end 2026-05-22T00:00:00 \
    --output-dir /tmp/my_chat \
    --bot-name "TikTok Shop Virtual AM"

# 不展开线程（更快，但话题群会丢回复）
python3 skills/feishu-cli-chat/scripts/fetch_chat_history.py oc_xxx --since 24h --no-thread
```

输出 4 个文件到 `--output-dir`（默认 `/tmp/lark_chat/`）：

| 文件 | 内容 |
|---|---|
| `history.json` | 主消息原始 JSON + 飞书侧返回的部分 sender_names |
| `threads.json` | 每个 thread_id → 完整回复列表 |
| `names.json` | 合并后的 open_id / app_id → 名字映射 |
| `timeline.txt` | **可读时间线**：主消息升序 + 缩进 4 空格的线程回复（`└─` 标识） |

脚本默认行为：

1. `msg history` 用 `ByCreateTimeAsc + start-time + end-time`，按 page_token 翻页到 `has_more=false`；
2. 收集所有非空 `thread_id`，对每个 tid 调 `msg thread-messages` 展开（不传 `-o json`，因为它默认就是 JSON 且加了反报错）；
3. **名字反解三级降级**：`mentions[].name` → `sender_names` API 字段 → `user info` 调用（**外部用户会 41050，静默跳过**）；
4. **Bot app_id**（`cli_xxx`）单独映射成 `--bot-name` 提供的名字；
5. 渲染时同时处理：撤回消息（content 为字符串非 JSON）、post 双结构、system 模板占位符、interactive v2 卡片递归。

如果脚本不能用，看下面的"手工拉群消息"小节，知道每步在做什么再退化到 jq + bash。

## 单次调用：常用读命令

```bash
# 群聊一页历史（不展开线程）
feishu-cli msg history --container-id oc_xxx --container-id-type chat --page-size 50 -o json

# 私聊：邮箱或 open_id 自动反查 P2P chat_id
feishu-cli msg history --user-email user@example.com --page-size 50 -o json
feishu-cli msg history --user-id ou_xxx --page-size 50 -o json

# 时间窗内全部消息（升序 + 翻页）
feishu-cli msg history --container-id oc_xxx --container-id-type chat \
    --start-time $(date -v-24H +%s) --end-time $(date +%s) \
    --sort-type ByCreateTimeAsc --page-size 50 -o json

# 单条 / 批量消息详情
feishu-cli msg get <message_id> -o json
feishu-cli msg mget --message-ids <id1,id2>

# 话题回复（注意：thread-messages 不接受 -o json，默认就是 JSON 输出）
feishu-cli msg thread-messages <thread_id> --page-size 50 --sort ByCreateTimeAsc
```

## 搜索与定位

```bash
feishu-cli msg search-chats --query "项目群" -o json   # 搜群
feishu-cli search messages "关键词" --chat-type p2p_chat -o json
feishu-cli search messages "关键词" --chat-ids oc_xxx -o json
```

搜消息属于 `feishu-cli-search`，本技能在阅读任务里顺带调用。

## 消息互动

```bash
feishu-cli msg reaction add <message_id> --emoji-type THUMBSUP
feishu-cli msg reaction remove <message_id> --reaction-id <reaction_id>
feishu-cli msg reaction list <message_id>

feishu-cli msg pin <message_id>
feishu-cli msg unpin <message_id>
feishu-cli msg pins --chat-id <chat_id>

feishu-cli msg delete <message_id>                                  # Bot 自撤回
feishu-cli msg delete <message_id> --user-access-token u-xxx        # 管理员撤回
```

## 群聊管理

```bash
feishu-cli chat get oc_xxx                          # 注意：外部群可能 232033
feishu-cli chat update oc_xxx --name "新群名"
feishu-cli chat member list oc_xxx -o json
feishu-cli chat member add oc_xxx --id-list ou_xxx,ou_yyy
feishu-cli chat member remove oc_xxx --id-list ou_xxx
feishu-cli chat create --name "项目群" --user-ids ou_xxx,ou_yyy
feishu-cli chat delete oc_xxx
```

## 踩坑速查（重要）

写代码前过一眼，否则容易卡 30 分钟。详细版见 `references/output-quirks.md`。

| 坑 | 一句话规避 |
|---|---|
| `thread-messages` 加 `-o json` 报错 | **不要传**；它默认就是 JSON 输出 |
| `thread-messages` 返回 PascalCase | 用 `d.get("items") or d.get("Items")` 兼容两套 key |
| `history` 时间秒、`thread-messages` 时间毫秒 | `start_sec` vs `start_sec * 1000`，分别传 |
| 撤回消息 `body.content` 是字面字符串 | `try: json.loads(...)` 包住，失败时直接当字符串显示 |
| post content 两种结构 | 兼容 `{zh_cn:{title,content}}` 和扁平 `{title,content}` |
| system 消息 `template` 含 `{from_user}` 占位符 | 用同对象其他字段填充（list 逗号 join） |
| Bot 发送者 `cli_xxx` ≠ `sender_names` 中的 `ou_xxx` | 单独映射成已知的 bot 名字 |
| 外部群 `chat get` 232033 | 用消息字段是否带 `thread_id` 判断话题群 |
| 跨企业 `user info` 41050 | 静默跳过，靠 `mentions[].name` 兜底 |
| `msg history` 默认不展开线程 | 话题群必须再调 `thread-messages` 展开，否则丢回复 |

## 名字反解策略

外部群里大多数发送者飞书后端不返回名字。按可靠性 + 成本排序：

1. **`mentions[]`**（消息自带）：`{"id":"ou_xxx","id_type":"open_id","name":"张三","key":"@_user_1"}`，
   还能用 `key` 替换 text 消息里的 `@_user_N` 占位符。**不受外部租户隔离限制**，首选。
2. **`sender_names`**（history/thread-messages 顶层）：飞书反解的子集，覆盖率低，作为起点。
3. **`user info <ou_xxx>`**：跨企业用户必返 `41050 no user authority error`，外部群几乎全部命中，静默跳过。
4. **Bot `cli_xxx`**：单独映射成你提前知道的 bot 名字（脚本的 `--bot-name`）。

## 输出处理

1. JSON 落到临时文件再分析，避免长消息刷屏；
2. 文本内容在 `body.content` 里，按 `msg_type` 解析 JSON 字符串（撤回消息除外，见上）；
3. `mentions` 字段同时含 open_id → name 映射 + `@_user_N` key → name 映射，比 `sender_names` 更全，优先用。

## 卡片消息（interactive）

`msg history` 默认带 `--card-content-type user`，会得到 v2 schema JSON + 抽取后的 `card_texts`。
脚本里已经覆盖 v2 schema 的递归（`column_set` / `form` / `collapsible_panel` / `action` /
`button` / `img` / `note`），手工解析见 `references/output-quirks.md` 的 §8。

需要"原版 cardDSL"或"OAPI 渲染版"时切换：

```bash
feishu-cli msg history --container-id oc_xxx --card-content-type raw      -o json   # 平台内部 cardDSL
feishu-cli msg history --container-id oc_xxx --card-content-type rendered -o json   # OAPI 渲染版/降级版
```

## 参考

- `scripts/fetch_chat_history.py` — 端到端拉群消息的可执行脚本
- `references/output-quirks.md` — JSON key / 时间单位 / 错误码 / 卡片解析等所有 API 怪癖
