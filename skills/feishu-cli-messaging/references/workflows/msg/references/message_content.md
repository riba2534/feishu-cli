# 消息 content 结构体参考

## 目录

- [通用规则](#通用规则)
- [文本和富文本](#text文本消息)
- [图片与文件](#image图片消息)
- [音频、视频和表情](#audio语音消息)
- [群名片和个人名片](#share_chat群名片消息)
- [交互卡片](#interactive卡片消息)
- [系统消息](#system系统分割线消息)

## 通用规则

- `content` 字段是 **JSON 字符串**，不是 JSON 对象
- 换行使用 `\n`
- text 类型的 `@` 语法（`<at user_id="ou_xxx">`）与卡片 Markdown 的 `@` 语法（`<at id=ou_xxx>`）不同
- **大小限制**：text 类型请求体 150 KB，post/interactive 类型 30 KB（超限返回 230025）
- 用 CLI 发送时，`--text` / `--markdown` / `--image` / `--file` / `--audio` / `--video` 会替你构造 content；
  只有需要精确控制结构时才手写本页 JSON 并用 `--content` / `--content-file` 发送

---

## text（文本消息）

最简单的消息类型，适合短通知和简单文本。

```json
{"text": "你好，这是一条测试消息"}
```

### 支持的内联语法

| 语法 | 效果 | 示例 |
|------|------|------|
| `<at user_id="ou_xxx">名字</at>` | @指定用户 | `<at user_id="ou_123">张三</at>` |
| `<at user_id="all"></at>` | @所有人 | — |

**不支持的格式**（通过 Bot API 发送时不会渲染）：
- `**加粗**`、`<i>斜体</i>`、`<u>下划线</u>`、`<s>删除线</s>` → 均会被忽略或过滤
- `[文本](url)` 超链接 → 不会渲染为可点击链接

如需富文本排版（加粗、链接、列表等），请使用 `post` 类型。

### CLI 示例

```bash
# 简单纯文本
feishu-cli msg send --receive-id-type email --receive-id user@example.com \
  --text "部署完成，请验证"

# @用户
feishu-cli msg send --receive-id-type email --receive-id user@example.com \
  --text '<at user_id="all"></at> 会议已开始'
```

---

## post（富文本消息）

支持标题 + 多段落的复杂排版，适合通知、报告等场景。

```json
{
  "zh_cn": {
    "title": "标题（可选）",
    "content": [
      [{"tag": "md", "text": "Markdown 段落 1"}],
      [{"tag": "md", "text": "Markdown 段落 2"}]
    ]
  }
}
```

### 结构说明

- `zh_cn` / `en_us` / `ja_jp`：语言标识，至少提供一种
- `title`：可选标题，显示为粗体
- `content`：二维数组，外层是段落，内层是同一段落中的 node 列表

### tag 类型一览

| tag | 说明 | 主要属性 | 备注 |
|-----|------|---------|------|
| text | 文本 | text, style | style 支持 bold/italic/underline/lineThrough |
| a | 超链接 | text, href | — |
| at | @用户 | user_id, user_name | user_id="all" 为 @所有人 |
| img | 图片 | image_key, width, height | 需先上传获取 image_key |
| media | 视频 | file_key, image_key | 需先上传 |
| emotion | 表情 | emoji_type | 如 "SMILE", "THUMBSUP" |
| hr | 分割线 | — | 独占段落 |
| code_block | 代码块 | language, text | 独占段落 |
| md | Markdown | text | 独占段落，**推荐使用** |

### text tag 的 style 属性

```json
{
  "tag": "text",
  "text": "粗体下划线文本",
  "style": ["bold", "underline"]
}
```

可用值：`bold`（加粗）、`italic`（斜体）、`underline`（下划线）、`lineThrough`（删除线）

### 完整示例

**使用 md 标签（推荐）**：

```json
{
  "zh_cn": {
    "title": "周报 2024-W01",
    "content": [
      [{"tag": "md", "text": "## 本周完成\n- 功能 A 开发完毕\n- 修复 **3** 个 Bug"}],
      [{"tag": "md", "text": "## 下周计划\n1. 功能 B 设计\n2. 性能优化"}],
      [{"tag": "hr"}],
      [{"tag": "md", "text": "详情请查看 [项目看板](https://example.com)"}]
    ]
  }
}
```

**使用混合标签**：

```json
{
  "zh_cn": {
    "title": "审批通知",
    "content": [
      [
        {"tag": "text", "text": "请 "},
        {"tag": "at", "user_id": "ou_xxx", "user_name": "张三"},
        {"tag": "text", "text": " 审批以下申请："}
      ],
      [
        {"tag": "text", "text": "申请类型：", "style": ["bold"]},
        {"tag": "a", "text": "服务器扩容", "href": "https://example.com/approval/123"}
      ]
    ]
  }
}
```

### CLI 示例

```bash
# 创建 JSON 文件后发送
cat > /tmp/post.json << 'EOF'
{
  "zh_cn": {
    "title": "部署通知",
    "content": [
      [{"tag": "md", "text": "**服务**: api-gateway\n**版本**: v1.2.3\n**状态**: 部署成功"}]
    ]
  }
}
EOF

feishu-cli msg send \
  --receive-id-type email --receive-id user@example.com \
  --msg-type post --content-file /tmp/post.json
```

---

## image（图片消息）

```json
{"image_key": "img_v2_xxx"}
```

`image_key` 来自 IM 图片上传，不是 `media upload` 返回的文档素材 token。直接传本地路径，CLI 会先上传再发送。

### CLI 示例

```bash
# 本地图片：自动上传后发送
feishu-cli msg send \
  --receive-id-type email --receive-id user@example.com \
  --image ./screenshot.png

# 已有当前应用可用的 image_key
feishu-cli msg send \
  --receive-id-type email --receive-id user@example.com \
  --image img_v2_xxx
```

---

## file（文件消息）

CLI 用 `--file <本地路径或 file_key>`，本地文件（≤30 MB）自动上传。

```json
{"file_key": "file_v2_xxx"}
```

---

## audio（语音消息）

仅支持 Opus 编码。CLI 用 `--audio <.opus/.ogg 路径或 file_key>`，本地文件自动上传并解析时长。

```json
{"file_key": "file_v2_xxx"}
```

---

## media（视频消息）

CLI 用 `--video <MP4 路径或 file_key> --video-cover <图片路径或 image_key>`，二者缺一不可，本地文件自动上传。

```json
{
  "file_key": "file_v2_xxx",
  "image_key": "img_v2_xxx"
}
```

---

## sticker（表情包消息）

仅支持转发收到的表情包，不支持自行上传新表情。

```json
{"file_key": "file_v2_xxx"}
```

---

## share_chat（群名片消息）

分享一个群聊的名片。

```json
{"chat_id": "oc_xxx"}
```

### CLI 示例

```bash
feishu-cli msg send \
  --receive-id-type email --receive-id user@example.com \
  --msg-type share_chat --content '{"chat_id":"oc_xxx"}'
```

---

## share_user（个人名片消息）

分享一个用户的名片。

```json
{"user_id": "ou_xxx"}
```

### CLI 示例

```bash
feishu-cli msg send \
  --receive-id-type email --receive-id user@example.com \
  --msg-type share_user --content '{"user_id":"ou_xxx"}'
```

---

## interactive（卡片消息）

卡片消息是最复杂的消息类型，支持三种发送方式。发送格式与历史排障见 `card_schema.md`；新增卡片 JSON 的设计与模板见 [`card` 工作流](../../card/workflow.md)。

### 方式一：card_id（引用已创建的卡片）

```json
{
  "type": "card",
  "data": {
    "card_id": "7371713483664506900"
  }
}
```

### 方式二：template_id（使用卡片模板 + 变量）

```json
{
  "type": "template",
  "data": {
    "template_id": "AAqk1xxxxxx",
    "template_version_name": "1.0.0",
    "template_variable": {
      "key1": "value1",
      "key2": "value2"
    }
  }
}
```

### 方式三：完整 Card JSON（最灵活）

**v1 格式（历史兼容）**：

```json
{
  "header": {
    "template": "blue",
    "title": {"tag": "plain_text", "content": "卡片标题"}
  },
  "elements": [
    {"tag": "markdown", "content": "卡片内容"}
  ]
}
```

**v2 格式**（支持更多组件）：

```json
{
  "schema": "2.0",
  "header": {
    "template": "blue",
    "title": {"tag": "plain_text", "content": "卡片标题"}
  },
  "body": {
    "direction": "vertical",
    "elements": [
      {"tag": "markdown", "content": "卡片内容"}
    ]
  }
}
```

---

## system（系统分割线消息）

仅在 p2p（单聊）会话中有效，群聊中不生效。

```json
{
  "type": "divider",
  "params": {
    "divider_text": {
      "text": "新会话",
      "i18n_text": {
        "zh_CN": "新会话",
        "en_US": "New Session"
      }
    }
  },
  "options": {
    "need_rollup": true
  }
}
```

### CLI 示例

`feishu-cli msg send` 的 `--msg-type` 白名单不含 `system`，用 `feishu-cli api` 透传（content 仍是 JSON 字符串）：

```bash
feishu-cli api POST /open-apis/im/v1/messages --as bot \
  --params '{"receive_id_type":"open_id"}' \
  --data '{"receive_id":"ou_xxx","msg_type":"system","content":"{\"type\":\"divider\",\"params\":{\"divider_text\":{\"text\":\"新会话\"}},\"options\":{\"need_rollup\":true}}"}' \
  --dry-run
```

确认请求体无误后去掉 `--dry-run` 再执行。
