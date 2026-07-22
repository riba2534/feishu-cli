# 飞书消息发送技能

通过 feishu-cli 发送飞书消息、回复、转发、合并转发、加急和下载消息资源。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

## 目录

- [职责边界](#与-feishu-cli-messaging-技能的职责边界)
- [核心概念](#核心概念)
- [消息类型选择](#消息类型选择)
- [身份说明](#身份说明)
- [发送命令](#发送命令)
- [执行流程](#执行流程)
- [错误处理](#错误处理)
- [资源下载](#下载消息资源)
- [话题消息](#话题thread消息)

## 与 feishu-cli-messaging 技能的职责边界

> **重要：CLI 路径 ≠ SKILL 归属。** `feishu-cli msg` 下的子命令同时被两个 SKILL 分管，**按动作类型划分**，不按 CLI 路径划分。
>
> | 动作类型 | 子命令 | 归属 SKILL |
> | --- | --- | --- |
> | **发送类**（本 SKILL 覆盖） | `send` / `reply` / `forward` / `merge-forward` / `urgent` / `flag` / `resource-download` | **feishu-cli-messaging** |
> | **读取类**（请用 chat SKILL） | `history` / `list` / `get` / `mget` / `thread-messages` / `search-chats` / `read-users` / `pins` | **feishu-cli-messaging** |
> | **互动类**（请用 chat SKILL） | `reaction` / `pin` / `unpin` / `delete` | **feishu-cli-messaging** |
>
> 端到端拉一段时间窗的群消息（含话题展开、名字反解、卡片解析）直接跑：
> ```bash
> python3 skills/feishu-cli-messaging/references/workflows/chat/scripts/fetch_chat_history.py oc_xxx --since 24h
> ```

## 核心概念

### 消息架构

飞书消息 API 的 `content` 字段是一个 **JSON 字符串**（不是 JSON 对象）。CLI 提供三种输入方式：

| 输入方式 | 参数 | 适用场景 |
|---------|------|---------|
| 快捷文本 | `--text "内容"` | 纯文本消息，最简单 |
| 发送文件 | `--file <路径>` 或 `-f` | 本地文件自动上传并发送（限 30MB） |
| 发送图片 | `--image <路径>` | 本地图片自动上传并发送（限 10MB） |
| 内联 JSON | `--content '{"key":"val"}'` 或 `-c` | 简单 JSON，一行搞定 |
| JSON 文件 | `--content-file file.json` | 复杂消息（卡片、富文本等） |

**互斥**：以上 5 种输入方式**只能指定一个**，同时指定会报错。

### 接收者类型

| --receive-id-type | 说明 | 示例 |
|-------------------|------|------|
| email | 邮箱地址 | user@example.com |
| open_id | Open ID | ou_xxx |
| user_id | User ID | xxx |
| union_id | Union ID | on_xxx |
| chat_id | 群聊 ID | oc_xxx |

## 消息类型选择

### 决策树（Claude 未指定类型时自动选择）

**默认优先使用 `interactive`（卡片消息）**，样式美观、内容丰富、支持颜色/多列/按钮等。

```
用户需求
├─ 【默认】通知/报告/告警/任何有信息量的消息 → interactive（卡片）
├─ 发送已上传的图片/文件/音视频 → image/file/audio/media
├─ 分享群聊或用户名片 → share_chat/share_user
└─ 仅以下场景才用 text/post：
   ├─ 用户明确要求发纯文本 → text
   └─ 用户明确要求发富文本 → post
```

**为什么优先卡片**：text 不支持任何格式渲染，post 样式有限，卡片支持彩色 header、多列 fields、按钮、分割线、备注等，视觉效果远优于其他类型。

### 消息类型一览

| 类型 | 说明 | content 格式 | 大小限制 |
|------|------|-------------|---------|
| text | 纯文本 | `{"text":"内容"}` | 150 KB |
| post | 富文本 | `{"zh_cn":{"title":"","content":[[...]]}}` | 150 KB |
| image | 图片 | `{"image_key":"img_xxx"}` | — |
| file | 文件 | `{"file_key":"file_v2_xxx"}` | — |
| audio | 语音 | `{"file_key":"file_v2_xxx"}` | — |
| media | 视频 | `{"file_key":"...","image_key":"..."}` | — |
| sticker | 表情包 | `{"file_key":"file_v2_xxx"}` | 仅转发 |
| interactive | 卡片 | Card JSON / template_id / card_id | 30 KB |
| share_chat | 群名片 | `{"chat_id":"oc_xxx"}` | — |
| share_user | 个人名片 | `{"user_id":"ou_xxx"}` | — |

> `system` 系统分割线（仅 p2p）CLI `--msg-type` 白名单暂未收录（见 `cmd/send_message.go:261-266`），需要时用 `feishu-cli api POST /open-apis/im/v1/messages` 直接透传。

## 身份说明

本技能以**发送类命令**为主，默认使用 **App Token（Bot 身份）**，无需登录。
- **必需 User Token**：`msg flag` 收藏/书签子命令（`im:feed.flag:read/write`），见末尾"消息书签"章节。
- **优先 User Token + Tenant 兜底**：`msg resource-download` 已登录时自动用 User Token 下载（要求你能看到该消息），未登录则尝试 App Token（要求 Bot 能看到该消息）。

> 其他子命令（reaction/pin/delete/history/list/get/mget/thread-messages/search-chats）的 Token 策略见 **feishu-cli-messaging** 技能。

## 发送命令

### 基础格式

```bash
feishu-cli msg send \
  --receive-id-type <type> \
  --receive-id <id> \
  [--msg-type <msg_type>] \
  [--text "<text>" | --file <path> | --image <path> | --content '<json>' | --content-file <file.json>] \
  [--idempotency-key <key>]
```

### 幂等键（--idempotency-key，防重发）

`--idempotency-key` 映射到发消息 API 的 `uuid` 字段，服务端按此键去重：**相同键的重复请求返回首次发送的消息**（同一 `message_id`），只会真正发出一条。适用于重试、定时任务、可能重复触发的自动化场景。

```bash
# 相同 key 调用两次，第二次不会重复发送，返回与第一次相同的 message_id
feishu-cli msg send --receive-id-type email --receive-id user@example.com \
  --text "对账通知" --idempotency-key "bill-2026-07-22"
```

- 长度上限 **50 字符**（按 Unicode 字符 / rune 计数，非字节，中文键不会被误拒），超限本地直接报错，不发请求。
- 键由调用方自行保证唯一/可复现：同一逻辑消息用固定键（幂等），不同消息用不同键，否则会被误判为重复而丢弃。
- 不传该 flag 时行为不变（每次都新发）。

### file 类型（直发文件）

```bash
# 直接发送本地文件（自动上传，限 30MB）
feishu-cli msg send \
  --receive-id-type email \
  --receive-id user@example.com \
  --file /path/to/report.pdf
```

自动推断文件 MIME 类型（opus/mp4/pdf/doc/xls/ppt），未知类型使用 `stream`。IM file_key 与 Drive
file_token 不是同一种 token，不能先用 `file upload` 再把返回值当作 file_key。超过 30MB 时应压缩、
拆分、改发云盘链接，或让用户从飞书客户端发送。

### image 类型（直发图片）

```bash
# 直接发送本地图片（自动上传，限 10MB）
feishu-cli msg send \
  --receive-id-type chat_id \
  --receive-id oc_xxx \
  --image /path/to/screenshot.png
```

支持 JPEG、PNG、BMP、GIF、TIFF、WebP 格式。

### text 类型

```bash
# 最简形式（默认 msg-type 为 text）
feishu-cli msg send \
  --receive-id-type email \
  --receive-id user@example.com \
  --text "你好，这是一条测试消息"
```

text 类型支持的内联 @ 语法：
- `@` 用户：`<at user_id="ou_xxx">Tom</at>` —— `ou_xxx` 必须是真实 open_id
- `@` 所有人：`<at user_id="all"></at>`
- `@` 机器人：与 @ 用户语法相同，把 `ou_xxx` 换成机器人 open_id 即可

**只能用 open_id**：`<at email="...">` 在 text 消息里**不会触发 @ entity**（飞书 IM 客户端会把邮箱字符串自动渲染成超链接，看起来像 @ 但没有通知、不是真的提及）。需要 @ 邮箱用户，先查 open_id：

```bash
# 第一步：查 open_id
feishu-cli user search --email user@example.com -o json
# 第二步：用真实 open_id @ 人
feishu-cli msg send --receive-id-type chat_id --receive-id oc_xxx \
  --text '<at user_id="ou_xxx">Alice</at> 你好'
```

**容错（仅 `--text` 模式）**：`msg send` / `msg reply` 在 `--text` 模式下会自动修正 AI 易写错的 @ 标签格式，下列写法都会被规范化为标准 `<at user_id="...">`：
- `<at id=ou_xxx>`（缺引号 / 用 `id` 而非 `user_id`）
- `<at open_id="ou_xxx"/>`（自闭合 / 用 `open_id`）
- `<at user_id=ou_xxx/>`（自闭合无引号）

`--content` / `--content-file` 模式**不做隐式 normalize**（用户自己写的 JSON 自己负责，避免破坏结构）。

**注意**：text 类型**不支持**富文本样式（加粗、斜体、下划线、删除线、超链接等均不会渲染）。如需格式排版，请使用 `post` 类型。

### post 类型（富文本）

推荐使用 `md` 标签承载 Markdown，一个 `md` 标签独占一个段落：

```bash
cat > /tmp/msg.json << 'EOF'
{
  "zh_cn": {
    "title": "项目进展通知",
    "content": [
      [{"tag": "md", "text": "**本周进展**\n- 完成功能 A 开发\n- 修复 3 个 Bug\n- [查看详情](https://example.com)"}],
      [{"tag": "md", "text": "**下周计划**\n1. 功能 B 开发\n2. 性能优化"}]
    ]
  }
}
EOF

feishu-cli msg send \
  --receive-id-type email \
  --receive-id user@example.com \
  --msg-type post \
  --content-file /tmp/msg.json
```

post 支持的 tag 类型：

| tag | 说明 | 主要属性 |
|-----|------|---------|
| text | 文本 | text, style（bold/italic/underline/lineThrough） |
| a | 超链接 | text, href |
| at | @用户 | user_id, user_name |
| img | 图片 | image_key, width, height |
| media | 视频 | file_key, image_key |
| emotion | 表情 | emoji_type |
| hr | 分割线 | — |
| code_block | 代码块 | language, text |
| md | Markdown | text（独占段落，推荐使用） |

### 图片自动上传（--upload-images）

`msg send` 支持 `--upload-images`，扫描 **post / interactive** 消息中的 Markdown 图片语法 `![alt](path)`，把 `path` 指向的**本地图片**自动上传到飞书 IM 图床、替换为 `image_key`，再发送。

| 维度 | 行为 |
| --- | --- |
| 触发条件 | 仅当 `--msg-type post` 或 `--msg-type interactive` 时生效（其他类型即使传也忽略，见 `cmd/send_message.go:212`） |
| 适用语法 | 内容里的 `![alt](path)` 标记；URL（http/https）和已有 `image_key` 不会被改写 |
| 路径解析 | 相对路径：以 `--content-file` 所在目录为 basePath；用 `--content` 内联 JSON 时以**当前工作目录**为 basePath（见 `cmd/send_message.go:213-220`） |
| 失败回落 | 单张上传失败会写 warning 并保留原始图片引用，随后继续发送；因此可能部分成功，必须检查 stderr 和最终 message_id |
| 进度提示 | 上传 > 0 张时 stderr 打印 `已自动上传 N 张本地图片` |

```bash
# post 内嵌本地图：自动上传相对路径 ./diagrams/foo.png
feishu-cli msg send --receive-id-type email --receive-id user@example.com \
  --msg-type post --content-file /path/to/post.json --upload-images

# interactive 卡片内嵌本地图同理
feishu-cli msg send --receive-id-type chat_id --receive-id oc_xxx \
  --msg-type interactive --content-file /tmp/card.json --upload-images
```

> 不需要预先调 `feishu-cli media upload`：本标志已包装了"上传 + 替换 + 发送"的全流程。需要把图片当作独立 `image` 消息发送，请直接用 `--image <path>` 快捷方式。

### interactive 类型（卡片消息）

卡片消息有三种发送方式：

**方式一：完整 Card JSON（仅发送；复杂卡片先用 feishu-cli-messaging 生成）**

```bash
cat > /tmp/card.json << 'EOF'
{
  "schema": "2.0",
  "header": {
    "template": "blue",
    "title": {"tag": "plain_text", "content": "任务完成通知"}
  },
  "body": {
    "direction": "vertical",
    "elements": [
      {"tag": "markdown", "content": "**项目**: feishu-cli\n**状态**: 已完成\n**负责人**: <at id=all></at>"}
    ]
  }
}
EOF

feishu-cli msg send \
  --receive-id-type email \
  --receive-id user@example.com \
  --msg-type interactive \
  --content-file /tmp/card.json
```

**方式二：template_id**

```bash
cat > /tmp/card.json << 'EOF'
{
  "type": "template",
  "data": {
    "template_id": "AAqk1xxxxxx",
    "template_variable": {"name": "张三", "status": "已完成"}
  }
}
EOF

feishu-cli msg send \
  --receive-id-type email \
  --receive-id user@example.com \
  --msg-type interactive \
  --content-file /tmp/card.json
```

**方式三：card_id**

```bash
feishu-cli msg send \
  --receive-id-type email \
  --receive-id user@example.com \
  --msg-type interactive \
  --content '{"type":"card","data":{"card_id":"7371713483664506900"}}'
```

#### Interactive 卡片职责边界

本技能只负责发送 interactive 消息，不负责设计卡片 JSON。

- 结构化或美观卡片必须先使用 feishu-cli-messaging 生成 v2 JSON（schema=2.0）。
- 本技能发送：feishu-cli msg send --msg-type interactive --content-file <card.json>。
- 不要在本技能内新写 v1 elements/action/note 卡片模板；旧 v1 示例仅用于历史兼容排查。

## 执行流程

### 回复、转发、合并转发与加急

```bash
feishu-cli msg reply om_xxx --text "收到"
feishu-cli msg forward om_xxx --receive-id user@example.com --receive-id-type email
feishu-cli msg merge-forward --receive-id oc_xxx --receive-id-type chat_id --message-ids om_xxx,om_yyy
feishu-cli msg urgent om_xxx --user-id-type open_id --user-ids ou_xxx,ou_yyy
```

`forward` 转发一条原消息；`merge-forward` 把多条 message ID 合并成一条转发消息；`urgent` 针对已经
发送的 message ID 发应用内、短信或电话加急。加急前确认用户列表和方式，避免误打扰。

### 发送消息流程

1. **确定接收者**：默认 `user@example.com`（email），或从上下文获取
2. **选择消息类型**：
   - 用户明确指定类型 → 使用指定类型
   - 结构化或美观通知 → 先用 `feishu-cli-messaging` 构造 JSON，再用 `interactive` 发送
   - 用户明确要求纯文本/富文本，或内容很短 → 使用 `text` / `post`
3. **准备内容**：纯文本直接传 `--text`；卡片 JSON 使用 `--content-file`；文件/图片使用 `--file` / `--image`
4. **发送并检查结果**：执行命令，确认返回 message_id

## 权限要求

| 权限 | 说明 |
|------|------|
| `im:message` | 消息读写（发送/回复/转发） |
| `im:message:send_as_bot` | 以机器人身份发送消息 |

## 注意事项

| 限制 | 说明 |
|------|------|
| text 大小限制 | 单条最大 150 KB |
| 卡片/富文本大小限制 | 单条最大 30 KB |
| system 消息 | CLI `--msg-type` 暂不支持，仅 p2p 会话有效；需用 `feishu-cli api` 透传 |
| sticker 消息 | 仅支持转发收到的表情包，不支持自行上传 |
| 卡片按钮回调 | 按钮的交互回调需应用服务端支持，CLI 发送的按钮仅 url 跳转有效 |
| API 频率限制 | 请求过快返回 429，等待几秒后重试 |
| 删除消息 | 仅能删除机器人发送的消息 |

## 错误处理

| 错误 | 原因 | 解决 |
|------|------|------|
| `content format of a post type is incorrect` | post 类型 JSON 格式错误 | 确保格式为 `{"zh_cn":{"title":"","content":[[...]]}}` |
| `invalid receive_id` | 接收者 ID 无效 | 检查 --receive-id-type 和 --receive-id 是否匹配 |
| `bot has no permission` | 机器人无权限 | 确认应用有 `im:message:send_as_bot` 权限 |
| `rate limit exceeded` | API 限流 | 等待几秒后重试 |
| `user not found` | 用户不存在 | 检查邮箱或 ID 是否正确 |
| `card content too large` | 卡片 JSON 超过 30 KB | 精简卡片内容或拆分为多条消息 |
| `Bot/User can NOT be out of the chat` | Bot 不在目标群内 | 添加 `--user-access-token` 切换为 User 身份重试 |

## 批量获取消息

> 读消息详情和批量获取消息请使用 **feishu-cli-messaging** 技能。`msg get/list/history/mget` 默认请求 `user_card_content` 并额外提取 `card_texts`，该行为和排错说明维护在 chat skill 中，避免发送与读取职责混在一起。

## 下载消息资源

下载消息中的图片或文件附件。

```bash
# 下载消息中的图片
feishu-cli msg resource-download <message_id> <file_key> --type image -o /tmp/photo.png

# 下载消息中的文件
feishu-cli msg resource-download <message_id> <file_key> --type file -o /tmp/attachment.pdf

# Bot 不可见但当前用户可见的历史消息资源，可显式用 User Token
feishu-cli msg resource-download <message_id> <file_key> --type file --user-access-token u-xxx -o /tmp/attachment.pdf

# 下载大文件时指定超时时间
feishu-cli msg resource-download <message_id> <file_key> --type file --user-access-token u-xxx -o /tmp/large.bin --timeout 30m
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `<message_id>` | 消息 ID | 必填 |
| `<file_key>` | 资源的 file_key | 必填 |
| `--type` | 资源类型 `image`/`file` | 必填 |
| `-o, --output` | 输出文件路径 | — |
| `--user-access-token` | 使用用户身份下载用户可见、但 Bot 不可见的历史消息资源 | — |
| `--timeout` | 下载超时时间（Go duration 格式，如 `10m`、`30m`、`1h`） | `5m` |

> **file_key 来源**：通过 `msg get <message_id>` 获取消息详情，从 content 中提取 `image_key` 或 `file_key`。
> 使用用户身份直连下载时，如遇到飞书大文件限制，会自动使用 HTTP Range 分片下载并合并。

## 话题（Thread）消息

### 发送到已有话题

`msg send` 支持 `--thread-id`（等价于 `--receive-id-type thread_id --receive-id <thread_id>`）：

```bash
# 在已有话题内追加一条消息
feishu-cli msg send --thread-id omt_xxx --text "话题内继续聊"

# 卡片消息也支持
feishu-cli msg send --thread-id omt_xxx \
  --msg-type interactive \
  --content "$(cat card.json)"
```

> `--thread-id` 与 `--receive-id-type/--receive-id` **互斥**，只能指定一组。

### 回复时开启话题

`msg reply` 支持 `--reply-in-thread`（`reply_in_thread=true`）：

```bash
# 在非话题群聊中，以话题形式回复某条消息（会开启一个新话题）
feishu-cli msg reply om_xxx --text "这里开个话题" --reply-in-thread

# 若群聊已是话题模式，--reply-in-thread 会自动回复到消息所在话题
```

### 话题回复列表

获取话题回复属于读取消息，请使用 **feishu-cli-messaging** 技能。发送话题内消息仍使用本技能的 `msg send --thread-id`。

## 参考文档

- `references/message_content.md`：各消息类型的 content JSON 结构详解
- `references/card_schema.md`：interactive 发送格式与历史卡片排障；新增卡片构造优先用 `feishu-cli-messaging`

## 消息书签（msg flag，v1.23+ 新增）

服务端称 message flag，用于把消息加 Feed 标记，把消息推上用户 Feed/书签。
对应 HTTP API `POST/GET/PATCH /open-apis/im/v1/flags`。需 User Access Token：`list` 需要 `im:feed.flag:read`，`create/cancel` 需要 `im:feed.flag:write`。

### 命令速查

- `feishu-cli msg flag create <message_id>` — 创建消息层书签（默认 `--item-type default --flag-type message`）
- `feishu-cli msg flag create <message_id> --flag-type feed` — feed 层书签，自动读取 `chat_mode` 判断 `thread` / `msg_thread`
- `feishu-cli msg flag create <message_id> --item-type msg_thread --flag-type feed` — feed 层书签（显式指定普通群线程）
- `feishu-cli msg flag list [--page-size 50] [--page-token xxx]` — 列当前用户的书签
- `feishu-cli msg flag cancel <message_id> [--item-type ... --flag-type ...]` — 默认尽量取消消息层 + feed 层；显式传 item/flag 时只取消指定层

### 关键枚举（服务端真值，绝勿按 0/1/2 顺序臆造）

CLI flag 用字符串，底层映射到 OpenAPI 整数枚举：

| 字段 | CLI 字符串 | OpenAPI 整数 | 含义 |
| --- | --- | --- | --- |
| --item-type | default | 0 | 普通消息 |
| --item-type | thread | 4 | topic-style 话题群 |
| --item-type | msg_thread | 11 | 普通群消息线程 |
| --flag-type | message | 2 | 消息层书签（默认） |
| --flag-type | feed | 1 | feed 层（侧边栏书签） |

> ⚠️ **反向陷阱**：网上某些第三方教程把 `flag_type: 1=message / 2=feed` 写反了。本项目以**飞书 OpenAPI 官方真值**为准（`1=feed / 2=message`，见上表）。`list` 命令输出里也是这套真值，写代码处理 JSON 时认上面这张表。
>
> `list` 命令不接受 `--flag-type/--item-type` 作为入参（list 是全量返回），只能在输出 `flag_items[*].flag_type` 字段上过滤。要看自己有哪些书签直接 `feishu-cli msg flag list -o json`。

支持的组合（其余服务端拒绝）：
- `default + message`：消息层书签，最常见
- `thread + feed`：topic-style 话题群 feed 层
- `msg_thread + feed`：普通群消息线程 feed 层

源码引用：`internal/client/flag.go:23-28`（本仓库权威定义）。
