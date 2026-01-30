---
name: feishu-cli-msg
description: 向飞书用户或群聊发送消息。支持 text/post/image/file/audio/media/sticker/interactive/share_chat/share_user/system 等消息类型。当用户需要发送飞书消息、构造消息 JSON、发送卡片消息时使用。
argument-hint: <receive_id> [--msg-type <type>]
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书消息发送技能

通过 feishu-cli 发送飞书消息，支持多种消息类型。

## 消息类型

| 类型 | 说明 | 适用场景 |
|------|------|---------|
| text | 纯文本 | 简单通知、短消息 |
| post | 富文本 | Markdown、@用户、列表、代码块 |
| image | 图片 | 已上传的图片（需 image_key） |
| file | 文件 | 已上传的文件（需 file_key） |
| audio | 语音 | 已上传的语音（需 file_key） |
| media | 视频 | 已上传的视频（需 file_key + image_key） |
| sticker | 表情包 | 已上传的表情（需 file_key） |
| interactive | 卡片 | 卡片 JSON/template_id/card_id |
| share_chat | 群名片 | 分享群聊（需 chat_id） |
| share_user | 个人名片 | 分享用户（需 user_id） |
| system | 系统分割线 | 会话分割线（仅 p2p 有效） |

## 消息类型选择（未明确指定时）

- **text**：单段短文本、少量换行、无复杂排版（推荐简单消息使用）
- **post**：需要 Markdown 格式、多段落、@用户、链接、图片混排
- **image/file/audio/media/sticker**：用户提供已上传的 key
- **interactive**：用户提供卡片 JSON 或 template_id
- **share_chat/share_user**：用户要求分享名片
- **system**：需要系统分割线

## 命令格式

```bash
feishu-cli msg send \
  --receive-id-type <type> \
  --receive-id <id> \
  --msg-type <msg_type> \
  --text "<text>"               # 简单文本
  # 或
  --content-file <file.json>    # 复杂内容
```

## 接收者类型

| --receive-id-type | 说明 |
|-------------------|------|
| email | 邮箱地址 |
| open_id | Open ID |
| user_id | User ID |
| union_id | Union ID |
| chat_id | 群聊 ID |

## 示例

### text 类型（推荐简单消息）

```bash
feishu-cli msg send \
  --receive-id-type email \
  --receive-id user@example.com \
  --text "你好，这是一条测试消息"
```

### post 类型（富文本）

```bash
# 先创建内容文件
cat > /tmp/msg.json << 'EOF'
{
  "zh_cn": {
    "title": "通知",
    "content": [[{"tag": "md", "text": "**更新**\n- item1\n- item2"}]]
  }
}
EOF

# 发送消息
feishu-cli msg send \
  --receive-id-type email \
  --receive-id user@example.com \
  --msg-type post \
  --content-file /tmp/msg.json
```

### interactive 类型（卡片）

```bash
# 使用 template_id
cat > /tmp/card.json << 'EOF'
{
  "type": "template",
  "data": {
    "template_id": "your_template_id",
    "template_variable": {"key1": "value1"}
  }
}
EOF

feishu-cli msg send \
  --receive-id-type email \
  --receive-id user@example.com \
  --msg-type interactive \
  --content-file /tmp/card.json
```

### share_chat 类型（群名片）

```bash
cat > /tmp/share.json << 'EOF'
{"chat_id": "oc_xxx"}
EOF

feishu-cli msg send \
  --receive-id-type email \
  --receive-id user@example.com \
  --msg-type share_chat \
  --content-file /tmp/share.json
```

## 执行流程

1. **确定接收者**：询问或从上下文获取接收者 ID 和类型
2. **确定消息类型**：根据内容复杂度选择合适的消息类型
3. **构造消息内容**：
   - 简单文本：直接使用 `--text` 参数
   - 复杂内容：创建 JSON 文件后使用 `--content-file`
4. **发送消息**：执行命令并检查结果

## 权限要求

| 权限 | 说明 |
|------|------|
| `im:message` | 消息读写 |
| `im:message:send_as_bot` | 以机器人身份发送消息 |
| `im:chat:readonly` | 搜索群聊（用于 `search-chats`） |
| `im:message:readonly` | 获取历史消息（用于 `history`） |

## 错误处理

| 错误 | 原因 | 解决 |
|------|------|------|
| `content format of a post type is incorrect` | post 类型 JSON 格式错误 | 确保格式为 `{"zh_cn":{"title":"标题","content":[[...]]}}` |
| `invalid receive_id` | 接收者 ID 无效 | 检查 --receive-id-type 和 --receive-id 是否匹配 |
| `bot has no permission` | 机器人无权限 | 确认应用有 `im:message:send_as_bot` 权限 |
| `rate limit exceeded` | API 限流 | 等待几秒后重试 |
| `user not found` | 用户不存在 | 检查邮箱或 ID 是否正确 |

## 常见问题

### post 类型 content format 错误

**错误**：`content format of a post type is incorrect`

**解决**：
1. 改用 text 类型（如果是简单文本）
2. 确保格式为 `{"zh_cn":{"title":"标题","content":[[...]]}}`

### 参数格式错误

确保使用正确的参数名：
- `--receive-id-type`（不是 `--email`）
- `--receive-id`（接收者 ID）

