# CLAUDE.md - 飞书 CLI 项目指南

## 项目概述

`feishu-cli` 是一个功能完整的飞书开放平台命令行工具，支持文档操作、Markdown 双向转换、消息发送、权限管理、知识库操作、文件管理、评论管理等功能。

## 技术栈

| 组件 | 选型 | 说明 |
|------|------|------|
| CLI 框架 | github.com/spf13/cobra | 子命令、自动补全 |
| 飞书 SDK | github.com/larksuite/oapi-sdk-go/v3 | 官方 SDK |
| 配置管理 | github.com/spf13/viper | YAML/环境变量 |
| Markdown | github.com/yuin/goldmark | GFM 扩展支持 |

## 项目结构

```
feishu-cli/
├── cmd/                          # CLI 命令
│   ├── root.go                   # 根命令、全局配置
│   ├── doc.go                    # 文档命令组
│   ├── create_document.go        # 创建文档
│   ├── get_document.go           # 获取文档信息
│   ├── get_blocks.go             # 获取文档块
│   ├── add_content.go            # 添加内容
│   ├── update_block.go           # 更新块
│   ├── delete_blocks.go          # 删除块
│   ├── export_markdown.go        # 导出为 Markdown
│   ├── import_markdown.go        # 从 Markdown 导入
│   ├── wiki.go                   # 知识库命令组
│   ├── get_wiki_node.go          # 获取知识库节点
│   ├── list_wiki_spaces.go       # 列出知识空间
│   ├── list_wiki_nodes.go        # 列出空间节点
│   ├── export_wiki.go            # 导出知识库文档
│   ├── create_wiki_node.go       # 创建知识库节点
│   ├── update_wiki_node.go       # 更新知识库节点
│   ├── delete_wiki_node.go       # 删除知识库节点
│   ├── move_wiki_node.go         # 移动知识库节点
│   ├── file.go                   # 文件管理命令组
│   ├── list_files.go             # 列出文件
│   ├── create_folder.go          # 创建文件夹
│   ├── create_shortcut.go        # 创建快捷方式
│   ├── get_quota.go              # 获取配额信息
│   ├── move_file.go              # 移动文件
│   ├── copy_file.go              # 复制文件
│   ├── delete_file.go            # 删除文件
│   ├── media.go                  # 素材命令组
│   ├── upload_media.go           # 上传素材
│   ├── download_media.go         # 下载素材
│   ├── comment.go                # 评论命令组
│   ├── list_comments.go          # 列出评论
│   ├── add_comment.go            # 添加评论
│   ├── delete_comment.go         # 删除评论
│   ├── perm.go                   # 权限命令组
│   ├── add_permission.go         # 添加权限
│   ├── update_permission.go      # 更新权限
│   ├── msg.go                    # 消息命令组
│   ├── send_message.go           # 发送消息
│   ├── get_message.go            # 获取消息
│   ├── list_messages.go          # 列出消息
│   ├── delete_message.go         # 删除消息
│   ├── forward_message.go        # 转发消息
│   ├── read_users.go             # 获取消息已读用户
│   ├── calendar.go               # 日历命令组
│   ├── list_calendars.go         # 列出日历
│   ├── create_event.go           # 创建日程
│   ├── get_event.go              # 获取日程
│   ├── list_events.go            # 列出日程
│   ├── update_event.go           # 更新日程
│   ├── delete_event.go           # 删除日程
│   ├── task.go                   # 任务命令组
│   ├── create_task.go            # 创建任务
│   ├── get_task.go               # 获取任务
│   ├── list_tasks.go             # 列出任务
│   ├── update_task.go            # 更新任务
│   ├── delete_task.go            # 删除任务
│   ├── complete_task.go          # 完成任务
│   ├── search.go                 # 搜索命令组
│   ├── search_messages.go        # 搜索消息
│   ├── search_apps.go            # 搜索应用
│   ├── config.go                 # 配置命令组
│   └── init_config.go            # 初始化配置
├── internal/
│   ├── client/                   # 飞书 API 封装
│   │   ├── client.go             # 客户端初始化
│   │   ├── docx.go               # 文档 API
│   │   ├── wiki.go               # 知识库 API
│   │   ├── drive.go              # 文件/素材 API
│   │   ├── comment.go            # 评论 API
│   │   ├── permission.go         # 权限 API
│   │   ├── message.go            # 消息 API
│   │   ├── calendar.go           # 日历 API
│   │   ├── task.go               # 任务 API
│   │   └── search.go             # 搜索 API
│   ├── converter/                # Markdown 转换器
│   │   ├── block_to_markdown.go  # Block → Markdown
│   │   ├── markdown_to_block.go  # Markdown → Block
│   │   └── types.go              # 块类型定义
│   └── config/
│       └── config.go             # 配置管理
├── skills/                       # Claude Code 技能
│   ├── feishu-cli-read/          # 读取飞书文档
│   ├── feishu-cli-write/         # 写入飞书文档
│   ├── feishu-cli-create/        # 创建空白文档
│   ├── feishu-cli-export/        # 导出为 Markdown
│   ├── feishu-cli-import/        # 从 Markdown 导入
│   ├── feishu-cli-wiki/          # 知识库操作
│   ├── feishu-cli-file/          # 文件管理
│   ├── feishu-cli-comment/       # 评论管理
│   ├── feishu-cli-media/         # 素材管理
│   ├── feishu-cli-calendar/      # 日历管理
│   ├── feishu-cli-task/          # 任务管理
│   └── feishu-cli-search/        # 搜索功能
├── main.go
├── go.mod
├── Makefile
└── README.md
```

## 常用命令

```bash
# 构建
go build -o feishu-cli .
make build                        # 构建到 bin/feishu-cli

# 测试
go test ./...
go vet ./...

# 运行示例
./feishu-cli --help

# === 文档操作 ===
./feishu-cli doc create --title "测试"
./feishu-cli doc get <doc_id>
./feishu-cli doc blocks <doc_id>                     # 获取文档所有块
./feishu-cli doc export <doc_id> -o output.md
./feishu-cli doc import input.md --title "导入的文档"
./feishu-cli doc add <doc_id> -c '[{"block_type":2,"text":{"elements":[{"text_run":{"content":"文本"}}]}}]'  # 添加块（需要 JSON 格式）
./feishu-cli doc delete <doc_id> --start 1 --end 3   # 删除块

# === 知识库操作 ===
./feishu-cli wiki get <node_token>              # 获取知识库节点信息
./feishu-cli wiki export <node_token> -o doc.md # 导出知识库文档为 Markdown
./feishu-cli wiki spaces                        # 列出知识空间
./feishu-cli wiki nodes <space_id>              # 列出空间下的节点

# === 文件管理 ===
./feishu-cli file list                          # 列出根目录文件
./feishu-cli file list <folder_token>           # 列出指定文件夹
./feishu-cli file mkdir "新文件夹" --parent <folder_token>
./feishu-cli file move <file_token> --target <folder_token> --type docx
./feishu-cli file copy <file_token> --target <folder_token> --type docx
./feishu-cli file delete <file_token> --type docx

# === 素材管理 ===
./feishu-cli media upload image.png --parent-type docx_image --parent-node <doc_id>
./feishu-cli media download <file_token> --output image.png

# === 评论操作 ===
./feishu-cli comment list <file_token> --type docx
./feishu-cli comment add <file_token> --type docx --text "这是一条评论"

# === 权限管理 ===
./feishu-cli perm add <doc_id> --doc-type docx --member-type email --member-id user@example.com --perm full_access

# === 消息操作 ===
./feishu-cli msg send --receive-id-type email --receive-id user@example.com --text "Hello"  # 简单文本
./feishu-cli msg send --receive-id-type email --receive-id user@example.com --msg-type post --content-file msg.json  # 富文本
./feishu-cli msg get <message_id>                 # 获取消息详情
./feishu-cli msg list --container-id <chat_id>    # 列出会话消息
./feishu-cli msg delete <message_id>              # 删除消息
./feishu-cli msg forward <message_id> --receive-id <id> --receive-id-type email  # 转发消息
./feishu-cli msg read-users <message_id>          # 获取已读用户列表

# === 日历操作 ===
./feishu-cli calendar list                        # 列出日历
./feishu-cli calendar create-event --calendar-id <id> --summary "会议" --start "2024-01-01T10:00:00+08:00" --end "2024-01-01T11:00:00+08:00"
./feishu-cli calendar get-event <calendar_id> <event_id>
./feishu-cli calendar list-events <calendar_id>
./feishu-cli calendar update-event <calendar_id> <event_id> --summary "新标题"
./feishu-cli calendar delete-event <calendar_id> <event_id>

# === 任务操作 ===
./feishu-cli task create --summary "待办事项"     # 创建任务
./feishu-cli task get <task_id>                   # 获取任务详情
./feishu-cli task list                            # 列出任务
./feishu-cli task update <task_id> --summary "新标题"
./feishu-cli task delete <task_id>                # 删除任务
./feishu-cli task complete <task_id>              # 完成任务

# === 搜索操作（需要 User Access Token） ===
./feishu-cli search messages "关键词" --user-access-token <token>
./feishu-cli search apps "应用名"
```

## 配置方式

**优先级**: 环境变量 > 配置文件 > 默认值

```bash
# 环境变量
export FEISHU_APP_ID=cli_xxx
export FEISHU_APP_SECRET=xxx

# 配置文件 (~/.feishu-cli/config.yaml)
app_id: "cli_xxx"
app_secret: "xxx"
```

## 块类型映射

| block_type | 名称 | Markdown |
|------------|------|----------|
| 1 | Page | 根节点 |
| 2 | Text | 段落 |
| 3-11 | Heading1-9 | `#` ~ `######` |
| 12 | Bullet | `- item` |
| 13 | Ordered | `1. item` |
| 14 | Code | ` ```lang ``` ` |
| 15 | Quote | `> text` |
| 16 | Equation | `$$formula$$` |
| 17 | Todo | `- [x]` / `- [ ]` |
| 19 | Callout | `> [!NOTE]` |
| 21 | Diagram | Mermaid |
| 22 | Divider | `---` |
| 27 | Image | `![](url)` |
| 31 | Table | Markdown 表格 |
| 43 | Board | 画板 |

## 开发规范

1. **错误处理**: 使用中文错误信息，提供解决建议
2. **命令帮助**: 所有命令使用简体中文描述
3. **代码注释**: 关键逻辑使用中文注释
4. **提交信息**: 遵循 Conventional Commits 规范

## SDK 注意事项

- `larkdocx.Heading1-9`、`Bullet`、`Ordered`、`Code`、`Quote`、`Todo` 都使用 `*Text` 类型
- Todo 的完成状态在 `TextStyle.Done` 字段
- Code 的语言在 `TextStyle.Language` 字段（整数编码）
- Table.Cells 是 `[]string` 类型，非指针切片
- DeleteBlocks API 使用 StartIndex/EndIndex，非单独 block ID
- Wiki 知识库使用 `node_token`，普通文档使用 `document_id`，注意区分
- 文件操作需要指定 `--type` 参数（docx/sheet/folder/file 等）
- 评论 API 需要指定文件类型（docx/sheet/bitable）
- 素材上传需要指定 `--parent-type`（docx_image/docx_file 等）
- 日历 API 使用 CalendarEvent，时间格式为 RFC3339（如 `2024-01-01T10:00:00+08:00`）
- 任务 API 使用 Task V2 版本
- 搜索 API 需要 User Access Token，不能使用 App Access Token

## Claude Code 技能

本项目提供以下 Claude Code 技能，位于 `skills/` 目录：

| 技能 | 说明 | 用法 |
|------|------|------|
| `/feishu-cli-read` | 读取飞书文档/知识库并转换为 Markdown | `/feishu-cli-read <doc_id\|url>` |
| `/feishu-cli-write` | 创建或更新飞书文档 | `/feishu-cli-write "标题"` |
| `/feishu-cli-create` | 快速创建空白文档 | `/feishu-cli-create "标题"` |
| `/feishu-cli-export` | 导出文档为 Markdown | `/feishu-cli-export <doc_id> [path]` |
| `/feishu-cli-import` | 从 Markdown 导入创建文档 | `/feishu-cli-import <file.md>` |
| `/feishu-cli-wiki` | 知识库操作（获取节点、列出空间、导出文档） | `/feishu-cli-wiki get <node_token>` |
| `/feishu-cli-file` | 云空间文件管理（列出、创建、移动、复制、删除） | `/feishu-cli-file list [folder_token]` |
| `/feishu-cli-comment` | 文档评论操作（列出、添加评论） | `/feishu-cli-comment list <file_token>` |
| `/feishu-cli-media` | 素材管理（上传图片、下载素材） | `/feishu-cli-media upload <file>` |
| `/feishu-cli-calendar` | 日历和日程管理 | `/feishu-cli-calendar list` |
| `/feishu-cli-task` | 任务管理 | `/feishu-cli-task list` |
| `/feishu-cli-search` | 搜索功能（需要 User Access Token） | `/feishu-cli-search messages "关键词"` |

### 支持的 URL 格式

- 普通文档: `https://xxx.feishu.cn/docx/<document_id>`
- 知识库: `https://xxx.feishu.cn/wiki/<node_token>`
- 内部飞书: `https://xxx.larkoffice.com/wiki/<node_token>`
- Lark 国际版: `https://xxx.larksuite.com/wiki/<node_token>`

### 技能工作流程

1. **读取文档**: 飞书文档/知识库 → Markdown → 分析/展示
2. **写入文档**: 内容 → Markdown → 飞书文档
3. **双向转换**: 支持 Markdown 与飞书文档互转
4. **知识库操作**: 列出空间 → 获取节点 → 导出文档
5. **文件管理**: 列出文件 → 创建/移动/复制/删除
6. **评论管理**: 查看评论 → 添加/删除审查意见
7. **素材管理**: 上传图片 → 引用到文档 / 下载文档素材
8. **日历管理**: 列出日历 → 创建/查看/更新/删除日程
9. **任务管理**: 创建任务 → 查看/更新/完成/删除任务
10. **搜索功能**: 搜索消息/应用（需要 User Access Token）

## 配置凭证

```bash
# 使用环境变量（推荐）
export FEISHU_APP_ID=<your_app_id>
export FEISHU_APP_SECRET=<your_app_secret>

# 或使用配置文件 (~/.feishu-cli/config.yaml)
# 通过 feishu-cli config init 初始化
```

## 权限要求

不同功能需要不同的应用权限，请在飞书开放平台为应用开通相应权限：

| 功能模块 | 所需权限 | 说明 |
|---------|---------|------|
| 文档操作 | `docx:document` | 文档读写 |
| 知识库 | `wiki:wiki:readonly` | 知识库读取 |
| 云空间文件 | `drive:drive`, `drive:drive:readonly` | 文件管理 |
| 素材管理 | `drive:drive` | 上传下载 |
| 评论 | `drive:drive.comment:write` | 评论读写 |
| 权限管理 | `drive:permission:member:create` | 添加协作者 |
| 消息 | `im:message`, `im:message:send_as_bot` | 发送消息 |
| 日历 | `calendar:calendar:readonly`, `calendar:calendar` | 日历管理（需单独申请） |
| 任务 | `task:task:read`, `task:task:write` | 任务管理（需单独申请） |
| 搜索 | 需要 User Access Token | 用户授权 |

## 已知问题

| 问题 | 说明 | 状态 |
|------|------|------|
| 表格导出 | 导出 Markdown 时表格单元格内容可能丢失（块类型 32） | 待修复 |
| file quota | `file quota` 命令 SDK 未实现 | 不支持 |
| 删除确认 | `file delete` 需要交互输入 y/N 确认 | 设计如此 |
| wiki spaces | 列出知识空间可能返回空（取决于应用权限范围） | 权限相关 |

## 功能测试验证

以下功能已通过测试验证（2026-01-21）：

```
✅ doc create/get/blocks/export/import
✅ wiki get/export
✅ file list/mkdir/move/copy
✅ media upload/download
✅ comment list/add
✅ perm add
✅ msg send/get (text/post)
✅ task create/complete/delete
```
