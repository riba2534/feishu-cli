---
name: feishu-cli-msg
description: >-
  飞书消息发送。发送消息（text/post/interactive 卡片等 11 种类型）、回复消息、
  转发/合并转发、消息加急、批量获取消息、下载消息资源（图片/文件）、获取话题回复列表。
  使用 App Token（Bot 身份），无需登录。
  当用户请求"发消息"、"回复"、"转发"、"合并转发"、"消息加急"、"发通知"、"发卡片"、
  "给某人发飞书消息"、"通知某人"、"批量获取消息"、"下载消息资源"、
  "下载消息图片"、"下载消息文件"、"话题回复"、"thread 消息"时使用，
  即使没有明确说"发送"，只要意图是把信息传达给某人，都应使用此技能。
  注意：Reaction/Pin/删除/获取消息详情/消息历史/搜索群聊/群聊管理
  请使用 feishu-cli-chat 技能（需 User Token）。
argument-hint: <receive_id> [--msg-type <type>]
user-invocable: true
allowed-tools: Bash, Read, Write
---

# 飞书消息发送与互动技能

通过 feishu-cli 发送飞书消息、回复、转发、Reaction、Pin 等互动操作。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

> **查看聊天记录？** 请使用 **feishu-cli-chat** 技能（msg history/list/get/search-chats/群管理）。本技能专注于消息的发送与互动操作。

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
├─ 会话分割线（仅 p2p） → system
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
| system | 系统分割线 | `{"type":"divider",...}` | 仅 p2p |

## 身份说明

本技能所有命令使用 **App Token（Bot 身份）**，无需登录。

> **Reaction/Pin/删除/获取消息/搜索群聊？** 这些操作需要 User Token，已移至 **feishu-cli-chat** 技能（需先 `auth login`）。

## 发送命令

### 基础格式

```bash
feishu-cli msg send \
  --receive-id-type <type> \
  --receive-id <id> \
  [--msg-type <msg_type>] \
  [--text "<text>" | --file <path> | --image <path> | --content '<json>' | --content-file <file.json>]
```

### file 类型（直发文件）

```bash
# 直接发送本地文件（自动上传，限 30MB）
feishu-cli msg send \
  --receive-id-type email \
  --receive-id user@example.com \
  --file /path/to/report.pdf
```

自动推断文件 MIME 类型（opus/mp4/pdf/doc/xls/ppt），未知类型使用 `stream`。超过 30MB 的文件请先用 `file upload` 上传到云空间，再用 `--msg-type file --content '{"file_key":"..."}'` 发送。

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

text 类型支持的内联语法：
- `@` 用户：`<at user_id="ou_xxx">Tom</at>`
- `@` 所有人：`<at user_id="all"></at>`

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

### interactive 类型（卡片消息）

卡片消息有三种发送方式：

**方式一：完整 Card JSON（推荐，最灵活）**

```bash
cat > /tmp/card.json << 'EOF'
{
  "header": {
    "template": "blue",
    "title": {"tag": "plain_text", "content": "任务完成通知"}
  },
  "elements": [
    {"tag": "markdown", "content": "**项目**: feishu-cli\n**状态**: 已完成\n**负责人**: <at id=all></at>"},
    {"tag": "hr"},
    {"tag": "note", "elements": [{"tag": "plain_text", "content": "由 CI/CD 自动发送"}]}
  ]
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

#### Card JSON 结构（v1 vs v2）

**v1 格式（推荐，兼容性好）**：

```json
{
  "header": {"template": "blue", "title": {"tag": "plain_text", "content": "标题"}},
  "elements": [...]
}
```

**v2 格式（更多组件）**：

```json
{
  "schema": "2.0",
  "header": {"template": "blue", "title": {"tag": "plain_text", "content": "标题"}},
  "body": {"direction": "vertical", "elements": [...]}
}
```

v2 额外支持：table（表格）、chart（图表）、form_container（表单）、column_set（多列布局）等高级组件。对于简单通知，v1 足够；需要复杂布局时用 v2。

#### header 颜色模板

| 颜色值 | 色系 | 推荐场景 |
|--------|------|---------|
| blue | 蓝色 | 通用通知、信息 |
| wathet | 浅蓝 | 轻量提示 |
| turquoise | 青色 | 进行中状态 |
| green | 绿色 | 成功、完成 |
| yellow | 黄色 | 注意、提醒 |
| orange | 橙色 | 警告 |
| red | 红色 | 错误、紧急 |
| carmine | 深红 | 严重告警 |
| violet | 紫罗兰 | 特殊标记 |
| purple | 紫色 | 自定义分类 |
| indigo | 靛蓝 | 深色主题 |
| grey | 灰色 | 已处理、归档 |

**语义化推荐**：绿=成功 / 蓝=通知 / 橙=警告 / 红=错误 / 灰=已处理

#### 常用组件速查

**内容组件**：

| 组件 | tag | 说明 |
|------|-----|------|
| Markdown | `markdown` | 支持 lark_md 语法，最常用 |
| 分割线 | `hr` | 水平分割线 |
| 备注 | `note` | 底部灰色小字备注 |
| 图片 | `img` | 图片展示 |

**布局组件**：

| 组件 | tag | 说明 |
|------|-----|------|
| 文本+附加 | `div` | 文本块，可含 fields（多列）和 extra（右侧附加） |
| 多列布局 | `column_set`（v2） | 横向分栏布局 |

**交互组件**：

| 组件 | tag | 说明 |
|------|-----|------|
| 按钮 | `button` | default/primary/danger 三种类型 |
| 下拉选择 | `select_static` | 静态下拉菜单 |
| 日期选择 | `date_picker` | 日期选择器 |
| 折叠菜单 | `overflow` | 更多操作菜单 |

#### 卡片 Markdown 语法（lark_md）

卡片内 `markdown` 组件使用 `lark_md` 语法，与标准 Markdown 有差异：

```markdown
# 支持的语法
**加粗** *斜体* ~~删除线~~ [链接](url) `行内代码`
![图片](img_v2_xxx)

# 特有语法
<font color='green'>绿色文字</font>
<font color='red'>红色文字</font>
<font color='grey'>灰色文字</font>
<at id=ou_xxx></at>
<at id=all></at>
```

**注意**：lark_md 的 `<font color>` 仅支持 green/red/grey 三种颜色。

### 常用卡片模板

#### 模板 1：简单通知卡片

```json
{
  "header": {
    "template": "blue",
    "title": {"tag": "plain_text", "content": "通知标题"}
  },
  "elements": [
    {"tag": "markdown", "content": "通知内容，支持 **加粗** 和 [链接](https://example.com)"},
    {"tag": "note", "elements": [{"tag": "plain_text", "content": "来自自动化工具"}]}
  ]
}
```

#### 模板 2：告警卡片（多列 + 按钮）

```json
{
  "header": {
    "template": "red",
    "title": {"tag": "plain_text", "content": "告警通知"}
  },
  "elements": [
    {
      "tag": "div",
      "fields": [
        {"is_short": true, "text": {"tag": "lark_md", "content": "**服务**\napi-gateway"}},
        {"is_short": true, "text": {"tag": "lark_md", "content": "**级别**\n<font color='red'>P0</font>"}},
        {"is_short": true, "text": {"tag": "lark_md", "content": "**时间**\n2024-01-01 10:00"}},
        {"is_short": true, "text": {"tag": "lark_md", "content": "**影响**\n<font color='red'>用户无法登录</font>"}}
      ]
    },
    {"tag": "hr"},
    {
      "tag": "action",
      "actions": [
        {"tag": "button", "text": {"tag": "plain_text", "content": "查看详情"}, "type": "primary", "url": "https://example.com/alert/123"},
        {"tag": "button", "text": {"tag": "plain_text", "content": "忽略"}, "type": "default"}
      ]
    }
  ]
}
```

#### 模板 3：进度报告卡片

```json
{
  "header": {
    "template": "green",
    "title": {"tag": "plain_text", "content": "构建报告"}
  },
  "elements": [
    {"tag": "markdown", "content": "**项目**: feishu-cli\n**分支**: main\n**提交**: abc1234"},
    {"tag": "hr"},
    {"tag": "markdown", "content": "<font color='green'>Tests: 42/42 passed</font>\n<font color='green'>Build: Success</font>\n<font color='grey'>Duration: 3m 25s</font>"},
    {"tag": "hr"},
    {
      "tag": "action",
      "actions": [
        {"tag": "button", "text": {"tag": "plain_text", "content": "查看日志"}, "type": "default", "url": "https://ci.example.com/build/123"}
      ]
    },
    {"tag": "note", "elements": [{"tag": "plain_text", "content": "CI/CD Pipeline #123"}]}
  ]
}
```

#### 模板 4：文档操作通知

```json
{
  "header": {
    "template": "turquoise",
    "title": {"tag": "plain_text", "content": "文档操作通知"}
  },
  "elements": [
    {
      "tag": "div",
      "fields": [
        {"is_short": true, "text": {"tag": "lark_md", "content": "**操作类型**\n创建文档"}},
        {"is_short": true, "text": {"tag": "lark_md", "content": "**状态**\n<font color='green'>成功</font>"}}
      ]
    },
    {"tag": "markdown", "content": "**文档标题**: 周报 2024-W01\n**文档链接**: [点击查看](https://xxx.feishu.cn/docx/abc123)"},
    {"tag": "note", "elements": [{"tag": "plain_text", "content": "由 feishu-cli 自动创建"}]}
  ]
}
```

#### 模板 5：审批确认卡片（多按钮）

```json
{
  "header": {
    "template": "orange",
    "title": {"tag": "plain_text", "content": "审批请求"}
  },
  "elements": [
    {"tag": "markdown", "content": "**申请人**: 张三\n**申请类型**: 服务器扩容\n**说明**: 线上流量增长，需要增加 2 台服务器"},
    {"tag": "hr"},
    {
      "tag": "action",
      "actions": [
        {"tag": "button", "text": {"tag": "plain_text", "content": "批准"}, "type": "primary"},
        {"tag": "button", "text": {"tag": "plain_text", "content": "拒绝"}, "type": "danger"},
        {"tag": "button", "text": {"tag": "plain_text", "content": "查看详情"}, "type": "default", "url": "https://example.com/approval/456"}
      ]
    }
  ]
}
```

## 回复消息

回复指定消息，支持与 `msg send` 相同的消息类型和输入方式。

```bash
# 文本回复
feishu-cli msg reply <message_id> --text "收到，我来处理"

# 卡片回复
feishu-cli msg reply <message_id> --msg-type interactive --content-file /tmp/card.json

# 富文本回复
feishu-cli msg reply <message_id> --msg-type post --content-file /tmp/post.json
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--msg-type` | 消息类型 | `text` |
| `--text` / `--content` / `--content-file` | 消息内容（三选一） | 必填 |

## 消息加急

对指定用户发送消息加急通知，支持应用内加急、电话加急和短信加急三种方式。

```bash
# 应用内加急（默认）
feishu-cli msg urgent <message_id> \
  --user-ids ou_xxx,ou_yyy \
  --user-id-type open_id

# 电话加急
feishu-cli msg urgent <message_id> \
  --urgent-type phone \
  --user-ids u_xxx,u_yyy \
  --user-id-type user_id

# 短信加急
feishu-cli msg urgent <message_id> \
  --urgent-type sms \
  --user-ids on_xxx,on_yyy \
  --user-id-type union_id
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `<message_id>` | 消息 ID | 必填 |
| `--urgent-type` | 加急类型：`app`/`phone`/`sms` | `app` |
| `--user-ids` | 目标用户 ID 列表（逗号分隔） | 必填 |
| `--user-id-type` | 用户 ID 类型：`open_id`/`user_id`/`union_id` | `open_id` |

### 加急类型说明

| 类型 | 说明 | 权限 |
|------|------|------|
| `app` | 应用内加急通知 | `im:message.urgent` |
| `phone` | 电话加急（需审批） | `im:message.urgent:phone` |
| `sms` | 短信加急（需审批） | `im:message.urgent:sms` |

**注意**：
- 电话和短信加急需要单独申请权限并通过审批
- 加急通知会向指定用户发送强提醒
- 建议仅在紧急情况下使用

## 合并转发

将多条消息合并转发给指定接收者。

```bash
feishu-cli msg merge-forward \
  --receive-id user@example.com \
  --receive-id-type email \
  --message-ids om_xxx,om_yyy,om_zzz
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--receive-id` | 接收者 ID | 必填 |
| `--receive-id-type` | 接收者类型 | `email` |
| `--message-ids` | 消息 ID 列表（逗号分隔） | 必填 |

## Reaction / Pin / 删除消息

> 这些命令需要 **User Token**，已移至 **feishu-cli-chat** 技能。包括：
> - `msg reaction add/remove/list` — 表情回应
> - `msg pin/unpin` — 置顶/取消置顶
> - `msg pins` — 查看群内置顶消息
> - `msg delete` — 删除消息（仅 Bot 自己发的）
> - `msg get` — 获取消息详情
>
> 请使用 feishu-cli-chat 技能操作以上功能。

## 其他消息命令

### 转发消息

```bash
feishu-cli msg forward <message_id> \
  --receive-id user@example.com \
  --receive-id-type email
```

## 执行流程

### 发送消息流程

1. **确定接收者**：默认 `user@example.com`（email），或从上下文获取
2. **选择消息类型**：
   - 用户明确指定类型 → 使用指定类型
   - **默认使用 `interactive`（卡片消息）** → 根据内容语义选择 header 颜色和合适的组件布局
   - 仅在用户明确要求纯文本/富文本时 → 使用 `text` / `post`
3. **构造卡片内容**：
   - 根据消息语义选择 header 颜色（绿=成功、红=错误、橙=警告、蓝=通知、灰=归档）
   - 使用 `markdown` 组件承载主要内容
   - 有多个键值对时使用 `div` + `fields` 多列布局
   - 需要操作链接时添加 `action` + `button`
   - 底部添加 `note` 备注来源
   - 将 JSON 写入临时文件后用 `--content-file` 发送
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
| system 消息 | 仅 p2p 会话有效，群聊无效 |
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

一次获取多条消息的详细信息。

```bash
feishu-cli msg mget --message-ids om_xxx,om_yyy,om_zzz
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--message-ids` | 消息 ID 列表（逗号分隔） | 必填 |

## 下载消息资源

下载消息中的图片或文件附件。

```bash
# 下载消息中的图片
feishu-cli msg resource-download <message_id> <file_key> --type image -o /tmp/photo.png

# 下载消息中的文件
feishu-cli msg resource-download <message_id> <file_key> --type file -o /tmp/attachment.pdf

# 下载大文件时指定超时时间
feishu-cli msg resource-download <message_id> <file_key> --type image -o /tmp/photo.png --timeout 10m
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `<message_id>` | 消息 ID | 必填 |
| `<file_key>` | 资源的 file_key | 必填 |
| `--type` | 资源类型 `image`/`file` | 必填 |
| `-o, --output` | 输出文件路径 | — |
| `--timeout` | 下载超时时间（Go duration 格式，如 `10m`、`30m`、`1h`） | `5m` |

> **file_key 来源**：通过 `msg get <message_id>` 获取消息详情，从 content 中提取 `image_key` 或 `file_key`。

## 话题回复列表

获取话题（Thread）中的所有回复消息。

```bash
# 获取话题回复
feishu-cli msg thread-messages <thread_id>

# 指定排序和分页
feishu-cli msg thread-messages <thread_id> \
  --sort ByCreateTimeAsc \
  --page-size 20

# 指定时间范围
feishu-cli msg thread-messages <thread_id> \
  --start-time 1704067200000 \
  --end-time 1704153600000
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `<thread_id>` | 话题 ID | 必填 |
| `--sort` | 排序方式 `ByCreateTimeAsc`/`ByCreateTimeDesc` | — |
| `--page-size` | 每页数量 | 50 |
| `--start-time` | 起始时间（毫秒时间戳） | — |
| `--end-time` | 结束时间（毫秒时间戳） | — |

## 参考文档

- `references/message_content.md`：各消息类型的 content JSON 结构详解
- `references/card_schema.md`：卡片消息完整构造指南（组件、布局、模板）
