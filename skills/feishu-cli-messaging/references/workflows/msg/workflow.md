# 飞书消息发送

用 feishu-cli 发送、回复、编辑、转发、合并转发、加急消息，管理消息书签，下载消息中的资源。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

## 目录

- [适用范围与 chat 工作流的边界](#适用范围与-chat-工作流的边界)
- [身份与权限](#身份与权限)
- [消息类型选择](#消息类型选择)
- [内容输入](#内容输入)
- [发送命令](#发送命令)
- [回复与话题](#回复与话题)
- [编辑已发送的消息](#编辑已发送的消息)
- [转发、合并转发与加急](#转发合并转发与加急)
- [下载消息资源](#下载消息资源)
- [消息书签（msg flag）](#消息书签msg-flag)
- [执行流程](#执行流程)
- [限制与错误处理](#限制与错误处理)
- [参考文档](#参考文档)

## 适用范围与 chat 工作流的边界

CLI 路径不等于工作流归属：`feishu-cli msg` 下的子命令按动作类型分到两个工作流。

| 动作类型 | 子命令 | 归属工作流 |
| --- | --- | --- |
| 发送与改写 | `send` / `reply` / `edit` / `forward` / `merge-forward` / `urgent` / `flag` / `resource-download` | 本文档（`msg` 工作流） |
| 读取 | `history` / `list` / `get` / `mget` / `thread-messages` / `search-chats` / `read-users` / `pins` | [`chat` 工作流](../chat/workflow.md) |
| 互动与撤回 | `reaction` / `pin` / `unpin` / `delete` | [`chat` 工作流](../chat/workflow.md) |

构造卡片 JSON 走 [`card` 工作流](../card/workflow.md)；拉一段时间窗的群消息走 chat 工作流的
`scripts/fetch_chat_history.py`。

## 身份与权限

| 命令 | 身份 |
| --- | --- |
| `msg send` / `reply` / `forward` | 默认 Bot（App Token），不自动加载 `token.json`；仅显式 `--user-access-token` 或 `FEISHU_USER_ACCESS_TOKEN` 时以本人身份发送 |
| `msg edit` / `urgent` | 仅 Bot，只能操作本应用 Bot 发出的消息 |
| `msg merge-forward` | 仅 Bot；传入的 User Token 会被忽略并在 stderr 提示 |
| `msg resource-download` | User 优先、Bot 兜底：已登录用本人身份（能看到该消息即可），未登录用 Bot（Bot 需能看到该消息）；User Token 不可用时 stderr 告警后改用 Bot |
| `msg flag create/list/cancel` | 必须 User（`im:feed.flag:read` / `im:feed.flag:write`） |

Bot 发消息需要 `im:message:send_as_bot`（或 `im:message`），且 Bot 必须在目标群内。以本人身份发送需要
`im:message.send_as_user`，`auth login` 的批量申请会剔除这个 scope；只有用户明确要求"以我的名义发"时才使用。

## 消息类型选择

需要结构化层级或操作入口时用 `interactive`；短回复、原文转达用 `text`；Markdown 排版用 `post`。
用户已指定消息类型时沿用其要求。

```text
用户需求
├─ 有结构化字段、图表或真实操作入口的通知/报告/告警 → interactive（卡片）
├─ 发送图片/文件/音视频 → image/file/audio/media
├─ 分享群聊或用户名片 → share_chat/share_user
└─ 原文转达、简短回复或用户明确指定：
   ├─ 简短纯文本 → text
   └─ Markdown 或富文本 → post
```

| 类型 | 说明 | content 格式 | 请求体上限 |
|------|------|-------------|---------|
| text | 纯文本 | `{"text":"内容"}` | 150 KB |
| post | 富文本 | `{"zh_cn":{"title":"","content":[[...]]}}` | 30 KB |
| image | 图片 | `{"image_key":"img_xxx"}` | — |
| file | 文件 | `{"file_key":"file_v2_xxx"}` | — |
| audio | 语音 | `{"file_key":"file_v2_xxx"}` | — |
| media | 视频 | `{"file_key":"...","image_key":"..."}` | — |
| sticker | 表情包 | `{"file_key":"file_v2_xxx"}` | 仅转发 |
| interactive | 卡片 | Card JSON / template_id / card_id | 30 KB |
| share_chat | 群名片 | `{"chat_id":"oc_xxx"}` | — |
| share_user | 个人名片 | `{"user_id":"ou_xxx"}` | — |

`system` 系统分割线（仅 p2p）不在 `--msg-type` 白名单内，需要时用
`feishu-cli api POST /open-apis/im/v1/messages` 透传，见 `references/message_content.md`。

## 内容输入

### 输入参数与互斥

`msg send` 和 `msg reply` 共用以下内容参数。飞书消息的 `content` 是 **JSON 字符串**，快捷参数会替你包装。

| 输入方式 | 参数 | 说明 |
|---------|------|------|
| 纯文本 | `--text` / `-t` | 推断为 text |
| Markdown | `--markdown` | 包装成 post 的 `md` 段落，并做样式归一（见下） |
| 文件 | `--file` / `-f` | 本地路径（≤30 MB，自动上传）或 `file_xxx` |
| 图片 | `--image` | 本地路径（≤10 MB，JPEG/PNG/BMP/GIF/TIFF/WebP）或 `img_xxx` |
| 语音 | `--audio` | 本地 `.opus` / Ogg Opus `.ogg` 或 `file_xxx`；上传时自动解析时长 |
| 视频 | `--video` + `--video-cover` | 本地 MP4（或 `file_xxx`）+ 必填封面；上传时自动解析时长 |
| 内联 JSON | `--content` / `-c` | 需配合 `--msg-type` |
| JSON 文件 | `--content-file` | 复杂 post / 卡片 |
| 附件区 | `--attachment` | 把文件放进 post 底部附件区，见[附件区](#附件区--attachment) |

- `content/content-file/text/markdown/file/image/audio/video` 只能指定一个；`--video-cover` 只能且必须与 `--video` 同用。
- 快捷参数会推断消息类型，显式 `--msg-type` 与推断结果冲突时在上传和发送前报错。
- 远程 URL 不会被下载：`--image/--file/--audio/--video` 只接受本地路径或飞书资源 key。
- 本地路径优先于 key 前缀：当前目录下真的存在 `img_logo.png` 这类文件时，仍按本地文件上传。
- IM 的 `file_key` / `image_key` 与云盘 `file_token` 不是同一种 token：不要先 `file upload` / `media upload`
  再把返回值当作消息资源 key。

### 统一输入：@文件、stdin 与 @@ 转义

`--text` / `--markdown` / `--content` 三个参数支持（`msg edit` 同样适用）：

| 写法 | 含义 |
| --- | --- |
| `--markdown @report.md` | 读取文件内容（去掉 UTF-8 BOM，≤4 MB） |
| `--text -` | 从 stdin 读取（三个参数里只能有一个用 `-`） |
| `--text "@@abc"` | 发送字面量 `@abc` |
| `--text "@张三 你好"` | 文件不存在时按原文发送（`--text/--markdown` 兼容旧写法；像路径时 stderr 提示） |

`--content @file` 找不到文件直接报错（JSON 不可能以 `@` 开头）。长 Markdown 优先用 `@file`，可避开 shell 对
`!`、反引号的转义问题。`--content-file` 本身就是文件路径，不需要 `@`。

```bash
cat note.txt | feishu-cli msg send --receive-id-type chat_id --receive-id oc_xxx --text -
```

### 接收者类型

| --receive-id-type | 示例 |
|-------------------|------|
| email | user@example.com |
| open_id | ou_xxx |
| user_id | xxx |
| union_id | on_xxx |
| chat_id | oc_xxx |

`thread_id`（`omt_xxx`）不是合法接收者，见[回复与话题](#回复与话题)。

## 发送命令

### 基础格式、预览与输出

```bash
feishu-cli msg send \
  --receive-id-type <type> \
  --receive-id <id> \
  [--msg-type <msg_type>] \
  [--text ... | --markdown ... | --file ... | --image ... | --audio ... | \
   --video ... --video-cover ... | --content '<json>' | --content-file <file.json>] \
  [--attachment <file>] [--upload-images] [--idempotency-key <key>] [--dry-run] [-o json]
```

- `--dry-run`：只打印将要发送的请求（含身份），**不上传本地文件**（用 `img_dryrun_N` / `file_dryrun_N`
  占位）、不发送。`--markdown` 的样式归一和 @ 规范化在预览里就能看到。
- `-o json` 输出 `{message_id, chat_id, create_time}`；`msg reply -o json` 另含 `parent_message_id`，
  进入话题时还有 `thread_id`。文本模式只打印消息 ID 和会话 ID。

### 幂等键（--idempotency-key）

`--idempotency-key` 在 `msg send` 和 `msg reply` 中都映射到 API 的 `uuid`：发送接口对同一 key 去重；
回复接口保证同一 key 1 小时内至多成功回复一条。适用于重试、定时任务和可能重复触发的自动化。

```bash
# 相同 key 调用两次，第二次不会重复发送
feishu-cli msg send --receive-id-type email --receive-id user@example.com \
  --text "对账通知" --idempotency-key "bill-2026-07-22"
```

- 上限 50 字符（按字符计，中文不会被误拒），超限本地报错、不发请求。
- 同一逻辑消息用固定键，不同消息用不同键，否则会被当作重复而丢弃。
- 本地媒体在提交消息前上传，重试可能再次上传并得到新 key；幂等键保证的是**可见消息不重复**。

### text 与 @ 提及

```bash
feishu-cli msg send --receive-id-type email --receive-id user@example.com \
  --text "你好，这是一条测试消息"
```

- `@` 用户：`<at user_id="ou_xxx">Tom</at>`（必须是真实 open_id；@ 机器人同理，换成机器人 open_id）
- `@` 所有人：`<at user_id="all"></at>`
- `<at email="...">` 不会产生真正的 @ 提醒。按邮箱 @ 人先查 open_id：

```bash
feishu-cli user search --email user@example.com -o json
feishu-cli msg send --receive-id-type chat_id --receive-id oc_xxx \
  --text '<at user_id="ou_xxx">Alice</at> 你好'
```

**@ 标签规范化**：`--text`、`--markdown`，以及 msg_type 为 text/post 的 `--content` / `--content-file`，
发送前会把 AI 常写错的形式统一成 `<at user_id="...">`，例如 `<at id=ou_xxx>`、`<at open_id="ou_xxx"/>`、
`<at user_id=ou_xxx/>`。JSON 内容按字符串逐个处理，不会破坏转义；interactive 卡片 JSON 不处理。

text 类型不渲染加粗、链接等格式；需要排版用 `--markdown` 或 post。

### Markdown（--markdown）

`--markdown` 包装成 post 的单个 `md` 段落。发送前做样式归一（对齐官方 CLI）：

- 原文含 H1~H3 时降级标题：H1→H4、H2~H6→H5（不含 H1~H3 时标题保持不变）；
- 连续标题之间、表格前后补空行；代码块内容不改动；
- post 的 `md` 只能渲染 `img_xxx` 图片：外链和其他图片引用会被**移除**并在 stderr 提示；
  本地图片加 `--upload-images` 自动上传后保留。

```bash
feishu-cli msg send --receive-id-type chat_id --receive-id oc_xxx --markdown @weekly.md --dry-run
```

### post（富文本 JSON）

需要标题、多段落或混合 tag 时用 `--msg-type post --content-file`。推荐用 `md` tag 承载 Markdown，
一个 `md` 独占一个段落；`--content-file` 不做 `--markdown` 的样式归一。

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

feishu-cli msg send --receive-id-type email --receive-id user@example.com \
  --msg-type post --content-file /tmp/msg.json
```

post 的 tag：`text`（style 支持 bold/italic/underline/lineThrough）、`a`、`at`、`img`、`media`、`emotion`、
`hr`、`code_block`、`md`。完整结构见 `references/message_content.md`。

### 附件区（--attachment）

`--attachment <本地路径或 file_key>`（可重复或逗号分隔）把文件放进 post 底部的附件区（顶层 `files`）：

- 可与 `--markdown`、或 `--msg-type post` 的 `--content/--content-file` 同用；单独使用时发送只含附件的 post。
- 不能与 `--text`、`--file/--image/--audio/--video` 同用；`--content` 已含 `files` 时不能再加。
- 本地文件按普通附件上传（`.mp4/.opus` 也按普通文件，不解析时长）；文件名、大小由服务端回填。

```bash
feishu-cli msg send --receive-id-type email --receive-id user@example.com \
  --markdown @report.md --attachment ./data.csv
```

### 图片自动上传（--upload-images）

`msg send` / `msg reply` 的 `--upload-images` 扫描 post / interactive 内容（含 `--markdown`）里的本地图片，
上传到 IM 图床并替换成当前 App 可用的 key 后再发送。

| 维度 | 行为 |
| --- | --- |
| 生效范围 | `--markdown`，以及 msg_type 为 post / interactive 的 `--content` / `--content-file`；其他类型忽略 |
| 识别位置 | Markdown `![alt](path)`、post 的 `img.image_key`、Card V2 的 `img.img_key` 与 `img_combination.img_list[].img_key`；URL 和已有 key 不改写 |
| 路径基准 | `~/` 展开为主目录；`--content-file` 以文件所在目录为基准；`--markdown` 与 `--content`（含 `@file`）以**当前工作目录**为基准 |
| 失败处理 | 任一文件缺失、不是受支持图片或上传失败时立即报错，不调用发消息 API；修复后用同一幂等键重试 |
| 预览 | `--dry-run` 不上传，stderr 提示实际发送时才替换 |

```bash
feishu-cli msg send --receive-id-type email --receive-id user@example.com \
  --msg-type post --content-file /path/to/post.json --upload-images

feishu-cli msg send --receive-id-type chat_id --receive-id oc_xxx \
  --msg-type interactive --content-file /tmp/card.json --upload-images
```

不需要预先调 `feishu-cli media upload`：它返回的是文档素材 token，不是消息图片 key。

### image / file / audio / media

```bash
# 图片、文件：本地路径自动上传
feishu-cli msg send --receive-id-type chat_id --receive-id oc_xxx --image /path/to/screenshot.png
feishu-cli msg send --receive-id-type email --receive-id user@example.com --file /path/to/report.pdf

# 语音只接受 Opus；视频必须 MP4 + 封面
feishu-cli msg send --receive-id-type chat_id --receive-id oc_xxx --audio /path/to/voice.opus
feishu-cli msg reply om_xxx --video /path/to/demo.mp4 --video-cover /path/to/cover.png \
  --idempotency-key "demo-video-reply-001"
```

- 文件按扩展名推断上传类型（opus/mp4/pdf/doc/xls/ppt，其余 `stream`）。超过 30 MB 时压缩、拆分、改发云盘链接，
  或让用户从客户端发送。
- `.mp4` / `.opus` 通过 `--file` 发送时按普通 `stream` 附件上传，避免 230055（上传类型与 `msg_type=file`
  不匹配）；要呈现为视频或语音用 `--video` / `--audio`。
- MP3/WAV 需先转为 Opus，或用 `--file` 当附件发；非 MP4 视频先转码，或用 `--file`。

### interactive（卡片）

三种 content：

| 方式 | content | 说明 |
| --- | --- | --- |
| 完整 Card JSON 2.0 | `{"schema":"2.0","header":...,"body":...}` | 先按 [`card` 工作流](../card/workflow.md) 生成并用 `lint_card.py` 校验 |
| 模板 | `{"type":"template","data":{"template_id":"...","template_variable":{...}}}` | 引用卡片搭建工具的模板 |
| 卡片实体 | `{"type":"card","data":{"card_id":"..."}}` | 引用已创建的卡片 |

```bash
feishu-cli msg send --receive-id-type chat_id --receive-id oc_xxx \
  --msg-type interactive --content-file /tmp/card.json --upload-images --idempotency-key "card-001"
```

- `type=template` / `type=card` 是引用信封，不能交给只接受完整 Card JSON 2.0 的 linter；需核对真实
  template_id/card_id、模板变量和当前应用可用性，不要擅自重建已有卡片。
- 不要在本工作流新写 v1（`elements/action/note`）卡片；v1 结构只用于历史排障，见 `references/card_schema.md`。
- 回调按钮需要应用接收 `card.action.trigger`（可用 [`event` 工作流](../event/workflow.md) 消费）；没有回调处理时只用 URL 按钮。

## 回复与话题

```bash
feishu-cli msg reply om_xxx --text "收到" --idempotency-key "ack-001"
feishu-cli msg reply om_xxx --image /path/to/photo.png --idempotency-key "photo-001"
feishu-cli msg reply om_xxx --msg-type interactive --content-file card.json --upload-images
```

- 飞书 create message 不支持 `thread_id`。要在已有话题追加消息，回复话题内任一 `om_xxx`：目标已在话题中时，
  回复默认进入同一话题。
- 普通消息群里要开启新话题时加 `--reply-in-thread`。
- `msg reply` 只接受 `om_xxx`；传 `omt_xxx` 或 `msg send --thread-id` 会在上传和调用 API 前报错。
- 读取话题回复属于 [`chat` 工作流](../chat/workflow.md)（`msg thread-messages <omt_xxx>`）。

## 编辑已发送的消息

`msg edit <om_xxx>` 改写本应用 Bot 已发送的 text / post 消息（PUT），**只能用 Bot 身份**，不接受 User Token；
卡片消息不在范围内。

```bash
feishu-cli msg edit om_xxx --text "更正：会议改到 15:00"
feishu-cli msg edit om_xxx --markdown @notice.md --dry-run
feishu-cli msg edit om_xxx --markdown "**最终版**" --set-attachments ./final.pdf
```

- 内容参数 `--text` / `--markdown` / `--content`（配 `--msg-type text|post`）三选一，同样支持 `@file`、`-`、`@@`；
  `--markdown` 同样做样式归一，可配 `--upload-images`。
- 不能改变消息类型（text 只能改成 text，post 只能改成 post）；飞书对可编辑次数和时间窗口有限制。
- 编辑会替换整条消息内容。只改正文（不带附件参数）时，post 原有附件区会保留。
- `--set-attachments`（本地路径或 file_key，可重复）**覆盖**附件区，`--clear-attachments` 清空附件区，二者互斥。
  **只传附件参数、不传正文时，正文会被清空**；要保留正文必须同时传完整的 `--markdown` / `--content`。
- `-o json` 输出 `{message_id, chat_id, create_time, update_time}`。

## 转发、合并转发与加急

```bash
feishu-cli msg forward om_xxx --receive-id user@example.com --receive-id-type email
feishu-cli msg merge-forward --receive-id oc_xxx --receive-id-type chat_id --message-ids om_xxx,om_yyy
feishu-cli msg urgent om_xxx --user-id-type open_id --user-ids ou_xxx,ou_yyy
```

- `forward` 转发一条消息；默认 Bot 身份，Bot 需能看到原消息。
- `merge-forward` 把多条消息合并成一条转发；只用 Bot 身份，Bot 需能看到这些消息且在目标会话中。
  `--receive-id-type` 默认 `email`，发到群时显式写 `chat_id`。
- `urgent` 对 Bot 自己发出的消息加急，`--urgent-type app|sms|phone`（默认 app）；不支持批量消息 ID（`bm_xxx`）。
  短信、电话加急打扰强，执行前确认用户列表和方式。

## 下载消息资源

```bash
# 不传 -o：用服务端文件名保存到当前目录（拿不到时用 file_key + 按 MIME 推断的扩展名）
feishu-cli msg resource-download om_xxx file_xxx --type file

# 指定路径；大文件加长超时（默认 5m）
feishu-cli msg resource-download om_xxx img_xxx --type image -o /tmp/photo.png
feishu-cli msg resource-download om_xxx file_xxx --type file -o /tmp/large.zip --timeout 30m
```

- `--type image|file` 必填；`file_key` 从 `msg get <om_xxx> -o json` 的 `body.content` 里取
  （`image_key`、`file_key`，post 附件区在 `files[].file_key`）。
- 默认 User 优先：已登录时以本人身份下载，Bot 看不到的历史消息资源也能下；未登录时 Bot 需能看到该消息。
- 按服务端文件名命名时不覆盖已存在的同名文件（自动追加 `_1`、`_2`）；`-o` 给了无扩展名的路径时只补扩展名。
- 用户身份下载遇到大文件限制时会自动按 HTTP Range 分片下载并合并。

## 消息书签（msg flag）

把消息标记到用户的 Feed/书签（服务端称 message flag，`/open-apis/im/v1/flags`）。必须 User Token：
`list` 需 `im:feed.flag:read`，`create/cancel` 需 `im:feed.flag:write`。

```bash
feishu-cli msg flag create om_xxx                                       # 消息层书签（默认 default + message）
feishu-cli msg flag create om_xxx --flag-type feed                      # feed 层，自动按群模式选 thread / msg_thread
feishu-cli msg flag create om_xxx --item-type msg_thread --flag-type feed
feishu-cli msg flag list --page-size 50                                 # 始终输出 JSON，不接受 -o
feishu-cli msg flag cancel om_xxx                                       # 默认取消消息层，并尽量取消 feed 层
```

CLI 用字符串，底层映射为 OpenAPI 整数枚举（`list` 输出里是整数）：

| 字段 | CLI 字符串 | OpenAPI 整数 | 含义 |
| --- | --- | --- | --- |
| --item-type | default | 0 | 普通消息 |
| --item-type | thread | 4 | 话题群（topic） |
| --item-type | msg_thread | 11 | 普通群里的消息线程 |
| --flag-type | message | 2 | 消息层书签（默认） |
| --flag-type | feed | 1 | feed 层（侧边栏） |

- 只支持 `default + message`、`thread + feed`、`msg_thread + feed` 三种组合，其余服务端拒绝。
- 注意 `flag_type` 是 `1=feed / 2=message`，不要按顺序臆测成 `1=message`。
- `list` 不接受 `--flag-type/--item-type`，输出含 `flag_items` / `delete_flag_items` / `messages` / `has_more` /
  `page_token`，需要时在 `flag_items[*].flag_type` 上自行过滤。
- `cancel` 显式传 `--item-type` 和 `--flag-type` 时只取消指定层；自动判断 feed 层失败时跳过并打印 warning。

## 执行流程

1. **确定接收者**：使用用户明确指定、上下文已确认或已授权配置中的真实接收者；缺少时先补齐。
   `user@example.com` 只是示例，不是默认收件人。
2. **选择消息类型**：用户指定的类型优先；结构化或美观通知先按 [`card` 工作流](../card/workflow.md)
   构造并校验 JSON，再以 `interactive` 发送；短内容用 `text` / `post`。
3. **准备内容**：纯文本 `--text`；Markdown `--markdown @file`；卡片 `--content-file`；图片/附件/语音/视频
   用 `--image` / `--file` / `--audio` / `--video + --video-cover`；附件区用 `--attachment`。
4. **预览**：内容复杂或含本地素材时先 `--dry-run` 核对请求体。
5. **发送并检查结果**：退出码为 0 且返回非空 `message_id` 才算成功；不要仅凭上传成功判定。
   失败重试使用同一 `--idempotency-key`。

## 限制与错误处理

| 限制 | 说明 |
|------|------|
| 请求体大小 | text 150 KB；post 与卡片 30 KB |
| 本地上传 | 文件 ≤30 MB，图片 ≤10 MB |
| sticker | 只能转发收到的表情包，不能上传 |
| system 消息 | `--msg-type` 不支持，需 `feishu-cli api` 透传，仅 p2p 有效 |
| 频率限制 | 429 时 CLI 自动退避重试；仍失败时稍后再试 |

| 错误 | 原因 | 解决 |
|------|------|------|
| `content format of a post type is incorrect` | post JSON 结构错误 | 确认为 `{"zh_cn":{"title":"","content":[[...]]}}` |
| `invalid receive_id` | 接收者 ID 与类型不匹配 | 核对 `--receive-id-type` 与 `--receive-id` |
| `bot has no permission` | 应用缺发送权限 | 开通 `im:message:send_as_bot` 并发布版本 |
| `Bot/User can NOT be out of the chat` | Bot 不在目标群 | 用 `chat member list <chat_id> --member-types bot` 确认，请群管理员把 Bot 拉进群；不要擅自改用本人身份发送 |
| `user not found` | 用户不存在或应用无法查看该用户 | 检查邮箱或 ID |
| 230025 `message content reaches its limit` | 请求体超限（text 150 KB；post、卡片 30 KB；用 template_id 时模板数据也计入） | 精简内容或拆分多条 |
| 230055 | 上传类型与 msg_type 不匹配 | 普通附件用 `--file`，语音/视频用 `--audio` / `--video` |

## 参考文档

- `references/message_content.md`：各消息类型的 content JSON 结构
- `references/card_schema.md`：interactive 发送格式与 v1 历史卡片排障；新卡片构造见 [`card` 工作流](../card/workflow.md)
- 读消息详情、批量获取（`msg get/mget`，默认带 `card_texts`）见 [`chat` 工作流](../chat/workflow.md)
