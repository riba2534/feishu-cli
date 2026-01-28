# CLAUDE.md - 飞书 CLI 项目指南

## 项目概述

`feishu-cli` 是一个功能完整的飞书开放平台命令行工具，**核心功能是 Markdown ↔ 飞书文档双向转换**，支持文档操作、消息发送、权限管理、知识库操作、文件管理、评论管理等功能。

## 核心功能：Markdown 转换

### Mermaid 图表转画板

**推荐用户使用 Mermaid 画图**，导入时会自动转换为飞书画板：

```bash
feishu-cli doc import doc.md --title "技术文档" --verbose
```

支持的 Mermaid 类型（全部已验证 2026-01-27）：
- ✅ flowchart（流程图，支持 subgraph）
- ✅ sequenceDiagram（时序图）
- ✅ classDiagram（类图）
- ✅ stateDiagram-v2（状态图）
- ✅ erDiagram（ER 图）
- ✅ gantt（甘特图）
- ✅ pie（饼图）

**技术实现**：使用飞书画板 API `/nodes/plantuml` 端点，`syntax_type=2` 表示 Mermaid 语法。

### 大表格自动拆分

飞书 API 限制单个表格最多 9 行。超过 9 行的表格会**自动拆分**为多个表格，每个都保留表头：

- 10 行表格 → 拆分为 2 个表格（9行 + 2行）
- 20 行表格 → 拆分为 3 个表格（9行 + 9行 + 4行）

### 表格智能处理（2026-01-28 更新）

- **列宽自动计算**：根据内容自动计算列宽（中文 14px，英文 8px，最小 80px，最大 400px）
- **空行问题修复**：飞书 API 创建表格时会自动在单元格内创建空文本块，现已改为更新现有块而非创建新块

### Mermaid 导入优化（2026-01-28 更新）

- **服务端错误重试**：500 错误时自动重试 3 次，指数退避（2s, 4s, 6s）

### 已验证的大规模导入

- **10,000+ 行 Markdown** ✓
- **127 个 Mermaid 图表** → 全部成功转换为飞书画板 ✓
- **170+ 个表格**（含大表格拆分、列宽自动计算）→ 全部成功 ✓

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
│   ├── search_chats.go           # 搜索群聊
│   ├── get_message_history.go    # 获取会话历史
│   ├── user.go                   # 用户命令组
│   ├── get_user_info.go          # 获取用户信息
│   ├── board.go                  # 画板命令组
│   ├── get_board_image.go        # 下载画板图片
│   ├── import_diagram.go         # 导入图表到画板
│   ├── create_board_notes.go     # 创建画板节点
│   ├── add_callout.go            # 添加高亮块
│   ├── add_board.go              # 添加画板到文档
│   ├── batch_update_blocks.go    # 批量更新块
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
│   │   ├── search.go             # 搜索 API
│   │   ├── user.go               # 用户 API
│   │   ├── board.go              # 画板 API
│   │   └── sheets.go             # 电子表格 API
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
./feishu-cli doc blocks <doc_id> --all               # 获取所有块（自动分页）
./feishu-cli doc export <doc_id> -o output.md
./feishu-cli doc import input.md --title "导入的文档"
./feishu-cli doc add <doc_id> -c '[{"block_type":2,"text":{"elements":[{"text_run":{"content":"文本"}}]}}]'  # JSON 格式
./feishu-cli doc add <doc_id> README.md --content-type markdown  # Markdown 格式
./feishu-cli doc add-callout <doc_id> "提示内容" --callout-type info  # 添加高亮块
./feishu-cli doc add-board <doc_id>                  # 添加画板
./feishu-cli doc batch-update <doc_id> '[...]' --source-type content  # 批量更新
./feishu-cli doc delete <doc_id> --start 1 --end 3   # 删除块

# === 用户操作 ===
./feishu-cli user info <user_id>                     # 获取用户信息
./feishu-cli user info <user_id> --user-id-type user_id -o json

# === 画板操作 ===
./feishu-cli board image <whiteboard_id> output.png  # 下载画板图片
./feishu-cli board import <whiteboard_id> diagram.puml --syntax plantuml  # 导入图表
./feishu-cli board create-notes <whiteboard_id> nodes.json  # 创建画板节点

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
./feishu-cli msg search-chats                     # 搜索群聊
./feishu-cli msg search-chats --query "关键词" --page-size 20
./feishu-cli msg history --container-id <chat_id> --container-id-type chat  # 会话历史
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

# === 电子表格操作 ===
./feishu-cli sheet create --title "新表格"           # 创建电子表格
./feishu-cli sheet get <spreadsheet_token>           # 获取表格信息
./feishu-cli sheet list-sheets <spreadsheet_token>   # 列出工作表
./feishu-cli sheet read <token> "Sheet1!A1:C10"      # 读取单元格（V2 API）
./feishu-cli sheet write <token> "Sheet1!A1:B2" --data '[["姓名","年龄"],["张三",25]]'  # 写入数据（V2 API）
./feishu-cli sheet append <token> "Sheet1!A:B" --data '[["新行","数据"]]'  # 追加数据（V2 API）
./feishu-cli sheet add-sheet <token> --title "新工作表"   # 添加工作表
./feishu-cli sheet delete-sheet <token> <sheet_id>   # 删除工作表
./feishu-cli sheet add-rows <token> <sheet_id> -n 5  # 添加 5 行
./feishu-cli sheet add-cols <token> <sheet_id> -n 3  # 添加 3 列
./feishu-cli sheet delete-rows <token> <sheet_id> --start 0 --end 5  # 删除行
./feishu-cli sheet delete-cols <token> <sheet_id> --start 0 --end 3  # 删除列
./feishu-cli sheet merge <token> "Sheet1!A1:C3"      # 合并单元格
./feishu-cli sheet unmerge <token> "Sheet1!A1:C3"    # 取消合并
./feishu-cli sheet find <token> <sheet_id> "关键词"   # 查找内容
./feishu-cli sheet replace <token> <sheet_id> "旧值" "新值"  # 替换内容
./feishu-cli sheet style <token> "Sheet1!A1:C3" --bold --bg-color "#FF0000"  # 设置样式
./feishu-cli sheet filter create <token> <sheet_id> "A1:C10"  # 创建筛选
./feishu-cli sheet protect <token> <sheet_id> --dimension ROWS --start 0 --end 5  # 创建保护
./feishu-cli sheet image add <token> <sheet_id> --token img_xxx --range "A1:A1"  # 添加浮动图片

# === 电子表格操作（V3 API 新版单元格） ===
./feishu-cli sheet read-plain <token> <sheet_id> "sheet!A1:C10"  # 获取纯文本内容
./feishu-cli sheet read-rich <token> <sheet_id> "sheet!A1:C10"   # 获取富文本内容
./feishu-cli sheet write-rich <token> <sheet_id> --data-file data.json  # 写入富文本
./feishu-cli sheet insert <token> <sheet_id> "sheet!A1:B2" --data '[["a","b"]]' --simple  # 插入数据
./feishu-cli sheet append-rich <token> <sheet_id> "sheet!A1:B2" --data '[["a"]]' --simple  # 追加富文本
./feishu-cli sheet clear <token> <sheet_id> "sheet!A1:B3"  # 清除单元格内容
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

| block_type | 名称 | Markdown | 说明 |
|------------|------|----------|------|
| 1 | Page | 根节点 | 文档根节点 |
| 2 | Text | 段落 | 普通文本 |
| 3-11 | Heading1-9 | `#` ~ `######` | 标题 |
| 12 | Bullet | `- item` | 无序列表 |
| 13 | Ordered | `1. item` | 有序列表 |
| 14 | Code | ` ```lang ``` ` | 代码块 |
| 15 | Quote | `> text` | 引用 |
| 16 | Equation | `$$formula$$` | 公式 |
| 17 | Todo | `- [x]` / `- [ ]` | 待办事项 |
| 19 | Callout | `> [!NOTE]` | 高亮块 |
| 21 | Diagram | Mermaid | 图表（自动转画板） |
| 22 | Divider | `---` | 分隔线 |
| 27 | Image | `![](url)` | 图片 |
| 31 | Table | Markdown 表格 | 表格 |
| 42 | WikiCatalog | `[Wiki 目录]` | **知识库目录块**，表示有子节点 |
| 43 | Board | 画板 | 画板 |

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
- Callout 块只需设置 BackgroundColor（1-7 对应不同颜色），不能同时设置 EmojiId
- 画板 API 使用通用 HTTP 请求方式（client.Get/Post），非专用 SDK 方法
- 用户信息 API 需要 `contact:user.base:readonly` 权限
- 电子表格 V3 API 用于表格管理（创建/获取/工作表），V2 API 用于单元格读写
- 电子表格新版 V3 单元格 API 支持富文本读写（三维数组格式，包含类型信息）
- V3 单元格 API 元素类型：text、value、date_time、mention_user、mention_document、image、file、link、reminder、formula
- V3 单元格写入限制：单次最多 10 个范围、5000 个单元格、50000 字符/单元格
- 电子表格范围格式：`SheetID!A1:C10`，支持整列 `A:C` 和整行 `1:3`
- 电子表格单元格数据使用 JSON 二维数组：`[["A1","B1"],["A2","B2"]]`
- V3 富文本数据使用三维数组：`[[[[{"type":"text","text":{"text":"Hello"}}]]]]`
- **表格单元格**：飞书 API 创建表格时会自动在每个单元格内创建空的 Text 块，填充内容时应更新现有块而非创建新块（否则会出现空行）
- **表格列宽**：通过 `TableProperty.ColumnWidth` 设置，单位为像素，数组长度需与列数一致

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
| `/feishu-cli-sheet` | 电子表格操作（V2/V3 API、富文本、行列操作） | `/feishu-cli-sheet <token>` |
| `/feishu-cli-file` | 云空间文件管理（列出、创建、移动、复制、删除） | `/feishu-cli-file list [folder_token]` |
| `/feishu-cli-comment` | 文档评论操作（列出、添加评论） | `/feishu-cli-comment list <file_token>` |
| `/feishu-cli-media` | 素材管理（上传图片、下载素材） | `/feishu-cli-media upload <file>` |
| `/feishu-cli-msg` | 消息发送（text/post/image/interactive 等多种类型） | `/feishu-cli-msg <receive_id>` |
| `/feishu-cli-perm` | 权限管理（添加/更新协作者权限） | `/feishu-cli-perm <doc_token>` |
| `/feishu-cli-plantuml` | PlantUML 生成（飞书画板安全子集） | `/feishu-cli-plantuml <描述>` |
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
8. **消息发送**: 确定接收者 → 选择消息类型 → 构造内容 → 发送
9. **权限管理**: 收集文档信息 → 确定协作者 → 选择权限级别 → 授权
10. **PlantUML**: 分析需求 → 选择图类型 → 生成安全子集代码
11. **日历管理**: 列出日历 → 创建/查看/更新/删除日程
12. **任务管理**: 创建任务 → 查看/更新/完成/删除任务
13. **搜索功能**: 搜索消息/应用（需要 User Access Token）

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
| 群聊搜索 | `im:chat:readonly` | 搜索群聊 |
| 会话历史 | `im:message:readonly` | 获取历史消息 |
| 用户信息 | `contact:user.base:readonly` | 获取用户信息 |
| 画板操作 | `board:board` | 画板读写 |
| 电子表格 | `sheets:spreadsheet` | 电子表格读写 |
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

## API 限制与处理

| 限制 | 说明 | 处理方式 |
|------|------|----------|
| 表格行数 | 单个表格最多 9 行 | 自动拆分为多个表格 |
| 批量创建块 | 每次最多 50 个块 | 自动分批处理 |
| API 频率限制 | 请求过快会返回 429 | 自动重试 + 延迟 |
| Mermaid 间隔 | 画板创建需要间隔 | 每个图表间隔 2 秒 |
| sheet filter create | V3 API 需要完整的 col+condition 参数，仅 range 不足 | API 限制 |
| sheet protect | V2 API 返回 "invalid operation"，可能是权限或 API 格式问题 | 待修复 |
| sheet formatter | 简单小数格式如 "0.00" 无效，需使用 "#,##0.00"（带千位分隔符） | API 限制 |
| shell 转义 | zsh 中 `!` 会被自动转义为 `\!`，已在代码中处理 | 已处理 |

## 功能测试验证

以下功能已通过测试验证（2026-01-27）：

```
✅ doc create/get/blocks/blocks --all/export/import
✅ doc add (JSON/Markdown)
✅ doc add-callout (info/warning/error/success)
✅ doc add-board
✅ doc batch-update
✅ wiki get/export
✅ user info（需要 contact:user.base:readonly 权限）
✅ board image（下载画板图片）
✅ file list/mkdir/move/copy
✅ media upload/download
✅ comment list/add
✅ perm add
✅ msg send/get (text/post)
✅ msg search-chats
✅ msg history（需要 im:message:readonly 权限）
✅ task create/complete/delete
✅ sheet create/get/list-sheets（电子表格基础操作）
✅ sheet read/write/append（单元格读写，支持布尔值自动转换）
✅ sheet add-sheet/delete-sheet/copy-sheet（工作表管理）
✅ sheet add-rows/add-cols/delete-rows/delete-cols/insert-rows（行列操作）
✅ sheet merge/unmerge（合并单元格）
✅ sheet style（样式设置，hAlign/vAlign 使用整数值）
✅ sheet meta（元信息获取）
✅ sheet image list（浮动图片列表）
✅ sheet find/replace（查找替换，范围需要 sheetId! 前缀）

✅ Mermaid 图表导入（20个图表类型全部验证通过）

✅ sheet read-plain/read-rich（V3 API 纯文本/富文本读取，支持多范围批量获取）
✅ sheet write-rich（V3 API 富文本写入，支持文本样式）
✅ sheet insert（V3 API 插入数据，支持 --simple 简单模式）
✅ sheet append-rich（V3 API 追加富文本，支持 --simple 简单模式）
✅ sheet clear（V3 API 清除单元格内容，最多 10 个范围）

⚠️ sheet filter create（需要完整的 col+condition 参数）
⚠️ sheet protect（V2 API 返回 "invalid operation"）
⚠️ board import CLI（命令行单独导入，API 返回 404）
⚠️ board create-notes（API 格式问题）
```

### Mermaid 导入修复记录（2026-01-27）

**问题**：Mermaid 图表转画板显示空白

**原因**：
1. API 路径错误：使用 `/nodes` 而非 `/nodes/plantuml`
2. `diagram_type` 参数类型错误：使用字符串而非整数

**修复**（`internal/client/board.go`）：
- API 路径：`/open-apis/board/v1/whiteboards/{id}/nodes/plantuml`
- `diagram_type` 映射：0=auto, 1=mindmap, 2=sequence, 3=activity, 4=class, 5=er, 6=flowchart, 7=usecase, 8=component
- `syntax_type`：1=PlantUML, 2=Mermaid

### 表格导入修复记录（2026-01-28）

**问题 1**：表格列宽过窄，内容显示拥挤

**修复**（`internal/converter/markdown_to_block.go`）：
- 添加 `calculateColumnWidths` 函数，根据内容自动计算列宽
- 中文字符 14px，英文/数字 8px，加上 16px 内边距
- 最小 80px，最大 400px，总宽度不足时按比例扩展

**问题 2**：表格显示空行（只有一行数据但显示两行）

**原因**：飞书 API 创建表格时自动在每个单元格内创建空的 Text 块，`FillTableCells` 又创建了新的 Text 块导致重复

**修复**（`internal/client/docx.go`）：
- 先调用 `GetBlockChildren` 检查单元格是否已有子块
- 如果有，使用 `UpdateBlock` 更新现有块的内容
- 如果没有或更新失败，才创建新块

### Mermaid 导入重试机制（2026-01-28）

**问题**：部分 Mermaid 图表导入时服务端返回 500 错误

**修复**（`cmd/import_markdown.go`）：
- 添加重试机制，最多重试 3 次
- 指数退避：2 秒、4 秒、6 秒
- 仅对 500 服务端错误重试，其他错误直接返回
