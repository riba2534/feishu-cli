# feishu-cli

飞书开放平台命令行工具，支持文档、知识库、消息、日历、任务等全功能操作，并提供 Markdown 双向转换。

## 项目定位

本项目提供两种使用方式：

1. **命令行工具** - 直接在终端使用 `feishu-cli` 命令操作飞书
2. **AI 技能集** - 为 [Claude Code](https://claude.ai/claude-code) 等 AI 编程助手提供飞书操作能力

### AI 技能 (skills/)

`skills/` 目录包含为 AI 编程助手设计的技能文件，让 AI 能够直接操作飞书：

```
skills/
├── feishu-cli-read/      # 读取飞书文档/知识库
├── feishu-cli-write/     # 写入/更新文档
├── feishu-cli-create/    # 创建空白文档
├── feishu-cli-export/    # 导出为 Markdown
├── feishu-cli-import/    # 从 Markdown 导入
├── feishu-cli-wiki/      # 知识库操作
├── feishu-cli-file/      # 云空间文件管理
├── feishu-cli-media/     # 素材上传/下载
├── feishu-cli-comment/   # 文档评论
├── feishu-cli-calendar/  # 日历日程管理
├── feishu-cli-task/      # 任务管理
└── feishu-cli-search/    # 搜索消息/应用
```

**使用方法**：将 `skills/` 目录复制到你的项目中，AI 助手即可通过 `/feishu-cli-xxx` 命令操作飞书。

## 功能特性

| 模块 | 功能 |
|------|------|
| 文档 | 创建、获取、编辑、删除文档及块内容 |
| 知识库 | 获取、创建、更新、移动、删除节点 |
| Markdown | 飞书文档 ↔ Markdown 双向转换 |
| 文件 | 列出、创建、移动、复制、删除云空间文件 |
| 素材 | 上传和下载图片、文件 |
| 权限 | 添加/更新协作者权限 |
| 评论 | 列出、添加文档评论 |
| 消息 | 发送、获取、删除、转发消息 |
| 日历 | 创建、查询、更新、删除日程 |
| 任务 | 创建、查询、更新、删除、完成任务 |
| 搜索 | 搜索消息和应用 |

支持 40+ 种飞书文档块类型的完整转换。

## 安装

### 使用 go install（推荐）

```bash
go install github.com/riba2534/feishu-cli@main
```

### 从源码编译

```bash
git clone https://github.com/riba2534/feishu-cli.git
cd feishu-cli
go build -o feishu-cli .
```

### 下载预编译版本

从 [Releases](https://github.com/riba2534/feishu-cli/releases) 页面下载对应平台的可执行文件。

## 配置

### 1. 获取应用凭证

1. 访问 [飞书开放平台](https://open.feishu.cn/app) 创建应用
2. 获取 App ID 和 App Secret
3. 配置应用权限（文档、消息等）

### 2. 设置凭证

**方式一：环境变量（推荐）**

```bash
export FEISHU_APP_ID="cli_xxx"
export FEISHU_APP_SECRET="xxx"
```

**方式二：配置文件**

```bash
feishu-cli config init
# 编辑 ~/.feishu-cli/config.yaml
```

```yaml
app_id: "cli_xxx"
app_secret: "xxx"
```

**优先级**: 环境变量 > 配置文件 > 默认值

## 快速开始

```bash
# 创建文档
feishu-cli doc create --title "我的文档"

# 导出文档为 Markdown
feishu-cli doc export <document_id> -o doc.md

# 从 Markdown 创建文档
feishu-cli doc import doc.md --title "导入的文档"

# 发送消息
feishu-cli msg send --receive-id-type email --receive-id user@example.com --text "Hello"
```

## 命令概览

```
feishu-cli [命令] [子命令] [选项]

命令:
  doc       文档操作（创建、获取、编辑、导入导出、高亮块、画板）
  wiki      知识库操作（节点增删改查）
  user      用户操作（获取用户信息）
  board     画板操作（下载图片、导入图表、创建节点）
  file      文件管理（列出、创建、移动、复制、删除）
  media     素材操作（上传、下载）
  comment   评论操作（列出、添加）
  perm      权限操作（添加、更新）
  msg       消息操作（发送、获取、删除、转发、搜索群聊、历史消息）
  calendar  日历操作（日程增删改查）
  task      任务操作（增删改查、完成）
  search    搜索操作（消息、应用）
  config    配置管理
```

## 文档操作

### 创建文档

```bash
feishu-cli doc create --title "我的文档"
feishu-cli doc create --title "我的文档" --folder <folder_token>
```

### 获取文档

```bash
feishu-cli doc get <document_id>
feishu-cli doc blocks <document_id>
```

### 编辑文档

```bash
# 添加内容（JSON 格式）
feishu-cli doc add <document_id> --content '[{"block_type":2,"text":{"elements":[{"text_run":{"content":"Hello"}}]}}]'

# 添加内容（Markdown 格式）
feishu-cli doc add <document_id> README.md --content-type markdown
feishu-cli doc add <document_id> --content "# 标题\n正文内容" --content-type markdown

# 获取所有块（自动分页）
feishu-cli doc blocks <document_id> --all

# 批量更新块
feishu-cli doc batch-update <document_id> '[{"block_id":"xxx","update_text_elements":{"elements":[...]}}]' --source-type content

# 更新块
feishu-cli doc update <document_id> <block_id> --content '{"update_text_elements":{...}}'

# 删除块
feishu-cli doc delete <document_id> <parent_block_id> --start 1 --end 3
```

### 高亮块和画板

```bash
# 添加高亮块（Callout）
feishu-cli doc add-callout <document_id> "提示内容" --callout-type info
feishu-cli doc add-callout <document_id> "警告内容" --callout-type warning
feishu-cli doc add-callout <document_id> "错误内容" --callout-type error
feishu-cli doc add-callout <document_id> "成功内容" --callout-type success

# 添加画板到文档
feishu-cli doc add-board <document_id>
```

## Markdown 转换

### 导出为 Markdown

```bash
feishu-cli doc export <document_id>
feishu-cli doc export <document_id> -o doc.md --download-images
```

### 从 Markdown 导入

```bash
feishu-cli doc import doc.md --title "新文档"
feishu-cli doc import doc.md --document-id <document_id> --upload-images
```

## 知识库操作

```bash
# 列出知识空间
feishu-cli wiki spaces

# 获取节点
feishu-cli wiki get <node_token>
feishu-cli wiki get https://xxx.feishu.cn/wiki/<node_token>

# 导出为 Markdown
feishu-cli wiki export <node_token> -o doc.md

# 创建/更新/删除/移动节点
feishu-cli wiki create --space-id <space_id> --title "新节点"
feishu-cli wiki update <node_token> --title "新标题"
feishu-cli wiki delete <node_token>
feishu-cli wiki move <node_token> --target-space <target_space_id>
```

## 用户操作

```bash
# 获取用户信息
feishu-cli user info <user_id>
feishu-cli user info <user_id> --user-id-type user_id
feishu-cli user info <user_id> -o json
```

## 画板操作

```bash
# 下载画板图片
feishu-cli board image <whiteboard_id> output.png

# 导入图表到画板
feishu-cli board import <whiteboard_id> diagram.puml --syntax plantuml
feishu-cli board import <whiteboard_id> diagram.mmd --syntax mermaid

# 创建画板节点
feishu-cli board create-notes <whiteboard_id> nodes.json
```

## 文件管理

```bash
feishu-cli file list [folder_token]
feishu-cli file mkdir "新文件夹" --parent <folder_token>
feishu-cli file move <file_token> --target <folder_token> --type docx
feishu-cli file copy <file_token> --target <folder_token> --type docx
feishu-cli file delete <file_token> --type docx
```

## 消息操作

```bash
# 发送文本消息
feishu-cli msg send --receive-id-type email --receive-id user@example.com --text "Hello"

# 发送到群组
feishu-cli msg send --receive-id-type chat_id --receive-id oc_xxx --text "群消息"

# 搜索群聊
feishu-cli msg search-chats
feishu-cli msg search-chats --query "关键词" --page-size 20

# 获取会话历史消息
feishu-cli msg history --container-id <chat_id> --container-id-type chat

# 其他操作
feishu-cli msg get <message_id>
feishu-cli msg list --container-id <chat_id>
feishu-cli msg delete <message_id>
feishu-cli msg forward <message_id> --receive-id <id> --receive-id-type email
```

## 日历操作

```bash
feishu-cli calendar list
feishu-cli calendar create-event --calendar-id <id> --summary "会议" --start-time "2024-01-15 14:00:00" --end-time "2024-01-15 15:00:00"
feishu-cli calendar list-events <calendar_id>
feishu-cli calendar update-event --calendar-id <id> --event-id <id> --summary "新标题"
feishu-cli calendar delete-event <calendar_id> <event_id>
```

## 任务操作

```bash
feishu-cli task create --summary "待办事项"
feishu-cli task list
feishu-cli task get <task_id>
feishu-cli task update <task_id> --summary "新标题"
feishu-cli task complete <task_id>
feishu-cli task delete <task_id>
```

## 权限管理

```bash
feishu-cli perm add <document_id> \
  --doc-type docx \
  --member-type email \
  --member-id user@example.com \
  --perm edit
```

权限级别: `view` | `edit` | `full_access`

## 素材操作

```bash
feishu-cli media upload image.png --parent-type doc_image --parent-node <document_id>
feishu-cli media download <file_token> --output image.png
```

## 搜索操作

```bash
# 搜索消息（需要 User Access Token）
feishu-cli search messages "关键词"

# 搜索应用
feishu-cli search apps "应用名"
```

## 块类型支持

| 类型 | 名称 | Markdown |
|------|------|----------|
| 2 | Text | 段落 |
| 3-11 | Heading1-9 | `#` ~ `######` |
| 12 | Bullet | `- item` |
| 13 | Ordered | `1. item` |
| 14 | Code | ` ```lang ``` ` |
| 15 | Quote | `> text` |
| 16 | Equation | `$$formula$$` |
| 17 | Todo | `- [ ]` / `- [x]` |
| 19 | Callout | `> [!NOTE]` |
| 21 | Diagram | Mermaid |
| 22 | Divider | `---` |
| 27 | Image | `![](url)` |
| 31 | Table | Markdown 表格 |
| 43 | Board | 画板 |

还支持 Bitable、Sheet、File、Grid、ISV 等 40+ 种块类型。

## 完整工作流示例

```bash
# 1. 设置凭证
export FEISHU_APP_ID="cli_xxx"
export FEISHU_APP_SECRET="xxx"

# 2. 创建文档并获取 ID
DOC_ID=$(feishu-cli doc create --title "API文档" --output json | jq -r '.document_id')

# 3. 导入 Markdown 内容
feishu-cli doc import README.md --document-id $DOC_ID --upload-images

# 4. 添加协作者
feishu-cli perm add $DOC_ID --doc-type docx --member-type email --member-id colleague@example.com --perm edit

# 5. 发送通知
feishu-cli msg send --receive-id-type email --receive-id colleague@example.com --text "文档已创建"
```

## 开发

```bash
go mod tidy
go test ./...
go vet ./...
go build -o feishu-cli .
```

## 许可证

MIT License
