---
name: feishu-cli-mail
description: >-
  飞书邮箱（Mail）操作。查看邮件、发送邮件、回复、转发、管理草稿、批量获取、线程、过滤。
  当用户请求"发邮件"、"看邮件"、"查邮件"、"回复邮件"、"转发邮件"、"邮件草稿"、"收件箱"、
  "feishu mail"、"lark mail"、"未读邮件"时使用。
  所有命令需要 User Access Token（先 feishu-cli auth login）。
  首期限制：发送/草稿/回复支持纯文本或 HTML body；forward 当前仅支持纯文本 body。暂不支持附件和 CID 内联图片。
user-invocable: true
allowed-tools: Bash(feishu-cli mail:*), Bash(feishu-cli auth:*), Read
---

# 飞书邮箱（Mail）

查看、发送、回复、转发邮件，管理草稿，过滤收件箱。

> **首期限制**：`send / draft-create / draft-edit / reply / reply-all` 支持纯文本/HTML body；`forward` 当前仅支持纯文本 body。暂不支持附件和 CID 内联图片。

## 前置条件

- **认证**：所有 mail 命令都需要 **User Access Token**（执行 `feishu-cli auth login` 登录）
- **预检**：`feishu-cli auth check --scope "mail:user_mailbox:readonly"` 可验证 scope

## 命令速查

### 查询类命令（只读）

| 命令 | 用途 |
|---|---|
| `mail message` | 获取单封邮件（含 HTML 或纯文本 body） |
| `mail messages` | 批量获取多封邮件 |
| `mail thread` | 获取邮件线程（对话） |
| `mail triage` | 列出/过滤邮件（folder/label/query/unread-only） |

```bash
# 查未读收件箱
feishu-cli mail triage --folder INBOX --unread-only --page-size 20

# 列出可用文件夹和标签
feishu-cli mail triage --list-folders
feishu-cli mail triage --list-labels

# 搜索邮件
feishu-cli mail triage --query "周会"

# 获取单封
feishu-cli mail message --message-id msg_xxx
feishu-cli mail message --message-id msg_xxx --format plain_text_full

# 批量获取
feishu-cli mail messages --message-ids m1,m2,m3

# 获取线程
feishu-cli mail thread --thread-id thread_xxx
```

### 写入类命令

| 命令 | 用途 |
|---|---|
| `mail send` | 发送邮件（默认草稿，加 `--confirm-send` 立即发送） |
| `mail draft-create` | 创建草稿（不发送） |
| `mail draft-edit` | 编辑已有草稿（全量覆盖） |
| `mail reply` | 回复邮件（自动 Re: 前缀 + 引用块 + In-Reply-To） |
| `mail reply-all` | 全部回复（包含原邮件 To 和 CC 的所有人，**自动排除自己**） |
| `mail forward` | 转发邮件（自动 Fwd: 前缀 + 原文；正文 text-only） |

```bash
# 发邮件（默认保存为草稿，安全兜底）
feishu-cli mail send --to user@example.com --subject "测试" --body "hi"

# 直接发送
feishu-cli mail send --to user@example.com --subject "测试" --body "hi" --confirm-send

# 多人、抄送、HTML
feishu-cli mail send --to a@x.com,b@x.com --cc c@x.com \
  --subject "会议纪要" --body "<h2>议程</h2><p>1. ...</p>" --html --confirm-send

# 创建草稿
feishu-cli mail draft-create --to user@example.com --subject "草稿" --body "初稿"

# 编辑草稿
feishu-cli mail draft-edit --draft-id xxx --to user@example.com --subject "修订" --body "新内容"

# 回复
feishu-cli mail reply --message-id msg_xxx --body "收到，周三开会"
feishu-cli mail reply --message-id msg_xxx --body "同意" --confirm-send

# 全部回复
feishu-cli mail reply-all --message-id msg_xxx --body "+1"

# 转发
feishu-cli mail forward --message-id msg_xxx --to new@example.com --body "请关注此邮件"
```

## 典型工作流

### 处理未读邮件

```bash
# 1. 查未读
feishu-cli mail triage --folder INBOX --unread-only -o json > unread.json

# 2. 逐封处理（拿 message_id → 看内容 → 回复）
feishu-cli mail message --message-id <id> -o json
feishu-cli mail reply --message-id <id> --body "已阅" --confirm-send
```

### 发送 HTML 邮件

```bash
feishu-cli mail send \
  --to team@example.com \
  --subject "周报" \
  --body "$(cat weekly-report.html)" \
  --html \
  --confirm-send
```

### 草稿审阅工作流

```bash
# 1. 创建草稿
DRAFT_ID=$(feishu-cli mail draft-create --to user@example.com --subject "合同" --body "初稿" -o json | jq -r .draft_id)

# 2. 审阅后修改
feishu-cli mail draft-edit --draft-id $DRAFT_ID --to user@example.com --subject "合同 v2" --body "修订后内容"

# 3. 通过 mail send 重建为真发送（draft-edit 只更新不发送）
# 或在飞书 Web 上手动发送
```

## 权限要求

| 命令 | 必需 scope |
|---|---|
| `mail message/messages/thread/triage` | `mail:user_mailbox:readonly`、`mail:user_mailbox.message:readonly`、`mail:user_mailbox.message.body:read`、`mail:user_mailbox.message.address:read`、`mail:user_mailbox.message.subject:read` |
| `mail send/draft-create/draft-edit/reply/reply-all/forward` | 上述只读权限 + `mail:user_mailbox.message:send`、`mail:user_mailbox.message:modify` |

## 注意事项

- **默认草稿**：`mail send` 默认只保存草稿（安全兜底）。必须显式加 `--confirm-send` 才会真正发送邮件。
- **HTML 自动检测**：`send / draft-create / draft-edit / reply / reply-all` 如果 `--body` 含以下任一标签会自动按 HTML 发送：`<html>` / `<body>` / `<div>` / `<p>` / `<br>` / `<b>` / `<i>` / `<a ` / `<table>` / `<h1>` / `<h2>` / `<h3>`。可用 `--plain-text` 或 `--html` 强制指定。`forward` 当前没有 `--html`/`--plain-text` 参数。
- **引用块**：`reply/reply-all` 会自动把原邮件 body 作为 `> ` 引用块附加到回复正文后。
- **发件人识别**：不传 `--from` 时，从 mailbox profile（`GET /profile`）自动读取 `primary_email_address` 和 `name`。
- **EML 格式**：所有发送命令底层都构造 RFC 5322 格式 EML，经过 base64 URL-safe 编码后提交给 `/drafts` API。
- **Mailbox 定位**：`--mailbox` 默认 `me`（当前登录用户），也可以传具体邮箱地址（前提是当前 Token 有权限）。
- **subject 去重**：`reply` 自动避免 `Re: Re:` 重复；`forward` 自动避免 `Fwd: Fwd:` 重复。
- **In-Reply-To / References**：`reply/reply-all` 自动从原邮件的 `smtp_message_id` / `references` 继承，确保邮件客户端正确展示对话线程。
- **附件 / CID 图片暂不支持**：首期 EML builder 是简化版。如需附件，请在 feishu Web 手动处理或等后续迭代。
- **批量 messages 上限**：取决于飞书 API 端；本命令不做数量校验，但通常建议 ≤50 条。

## 高级能力（v1.23+ mail-advanced）

### CID 内联图片自动扫描

`mail send --inline-images-auto-scan` 自动扫描 HTML body 中
`<img src="本地路径">` → 上传 drive (parent_type=email) → 重写为 `cid:xxx` →
multipart/related 拼装。

- 路径安全：拒 `..` 路径遍历；限 cwd / home 子树内
- 多媒体合规：RFC 2046 multipart/related CRLF 严格（每 part body 末尾 `\r\n` + 边界前 `\r\n` 隔离）
- 跳过已有 scheme：`cid:` / `http(s):` / `data:` / `//cdn` 等不重复上传
- 仅 HTML body 生效；纯文本下静默跳过
- 需要 `auth login` 缓存里有当前用户 `open_id`（drive upload 的 `parent_node` 必填）

```bash
# 自动扫描内嵌图，HTML body 中 <img src="./figs/chart.png"> 会被改写为 cid:xxx
feishu-cli mail send --to user@example.com --subject "周报" \
  --body "$(cat report.html)" --html --inline-images-auto-scan --confirm-send
```

### 邮件模板 CRUD

```bash
# 创建模板（body 支持 @file 读盘）
feishu-cli mail template create --name "周报模板" \
  --subject "本周进度" --body @template.html

# 列出全部模板（接口不分页，一次性返回 id+name）
feishu-cli mail template list
feishu-cli mail template list -o json
```

⚠️ **mail scope 字节租户未开放**：`mail:user.email.template` 在字节租户暂未开放，
UAT 调用可能返回 401 / scope 校验失败。建议先预检：

```bash
feishu-cli auth check --scope "mail:user_mailbox:readonly mail:user_mailbox.message:modify mail:user.email.template"
```

功能代码已完整，等飞书侧开放 scope 后即可使用；其他租户（如个人 lark）通常可用。

### 未做（暂未 MVP）

receipt send/decline / watch (WebSocket) / share-to-chat / template update / template delete
