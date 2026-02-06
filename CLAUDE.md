# CLAUDE.md - 飞书 CLI 项目指南

## 项目概述

`feishu-cli` 是一个功能完整的飞书开放平台命令行工具，**核心功能是 Markdown ↔ 飞书文档双向转换**，支持文档操作、消息发送、权限管理、知识库操作、文件管理、评论管理等功能。

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
├── cmd/                          # CLI 命令（每个子命令一个文件）
│   ├── root.go                   # 根命令、全局配置
│   ├── doc.go                    # 文档命令组
│   ├── import_markdown.go        # Markdown 导入（三阶段并发管道）
│   ├── export_markdown.go        # 导出为 Markdown
│   ├── wiki.go                   # 知识库命令组
│   ├── msg.go                    # 消息命令组
│   ├── sheet_*.go                # 电子表格命令（V2/V3 API）
│   ├── calendar.go               # 日历命令组
│   ├── task.go                   # 任务命令组
│   └── utils.go                  # 公共工具（printJSON 等）
├── internal/
│   ├── client/                   # 飞书 API 封装
│   │   ├── client.go             # 客户端初始化、Context()
│   │   ├── helpers.go            # 工具函数（StringVal/BoolVal/IsRateLimitError 等）
│   │   ├── docx.go               # 文档 API（含 FillTableCells）
│   │   ├── board.go              # 画板 API（Mermaid/PlantUML 导入）
│   │   ├── sheets.go             # 电子表格 API
│   │   └── ...                   # wiki/drive/message/calendar/task 等
│   ├── converter/                # Markdown 转换器
│   │   ├── block_to_markdown.go  # Block → Markdown（导出）
│   │   ├── markdown_to_block.go  # Markdown → Block（导入）
│   │   ├── types.go              # 块类型定义、颜色映射、ConvertOptions
│   │   ├── *_test.go             # 单元测试 + roundtrip 测试
│   └── config/
│       └── config.go             # 配置管理
├── skills/                       # Claude Code 技能（每个技能一个目录）
├── main.go
├── go.mod
├── Makefile
└── README.md
```

## 开发指南

### 构建与测试

```bash
go build -o feishu-cli .          # 快速构建
make build                        # 构建到 bin/feishu-cli
make build-all                    # 多平台构建（发版用）
go test ./...                     # 运行所有测试
go vet ./...                      # 静态检查
```

### 开发规范

1. **错误处理**: 使用中文错误信息，提供解决建议
2. **命令帮助**: 所有命令使用简体中文描述
3. **代码注释**: 关键逻辑使用中文注释
4. **提交信息**: 遵循 Conventional Commits 规范
5. **指针解引用**: 使用 `helpers.go` 中的 `StringVal/BoolVal/IntVal` 等工具函数

### 配置方式

**优先级**: 环境变量 > 配置文件 > 默认值

```bash
# 环境变量（推荐）
export FEISHU_APP_ID=cli_xxx
export FEISHU_APP_SECRET=xxx

# 配置文件 (~/.feishu-cli/config.yaml)
# 通过 feishu-cli config init 初始化
app_id: "cli_xxx"
app_secret: "xxx"
```

## 核心功能

### Markdown ↔ 飞书文档双向转换

**导入**：`feishu-cli doc import doc.md --title "文档" --verbose`
**导出**：`feishu-cli doc export <doc_id> -o output.md`

支持的语法：标题、段落、列表（无限深度嵌套）、任务列表、代码块、引用（QuoteContainer）、Callout（6 种类型）、表格、分割线、图片（占位块，Open API 暂不支持插入实际图片）、链接、公式（块级/行内）、粗体/斜体/删除线/下划线/行内代码/高亮

### Mermaid / PlantUML 图表转画板

**推荐使用 Mermaid 画图**，导入时自动转换为飞书画板。同时支持 PlantUML（` ```plantuml ` 或 ` ```puml `）。

支持的 Mermaid 类型（8 种，全部已验证）：
- flowchart（流程图，支持 subgraph）
- sequenceDiagram（时序图）
- classDiagram（类图）
- stateDiagram-v2（状态图）
- erDiagram（ER 图）
- gantt（甘特图）
- pie（饼图）
- mindmap（思维导图）

PlantUML 支持：时序图、活动图、类图、用例图、组件图、ER 图、思维导图等全部类型。

### 嵌套列表

无序/有序列表支持**无限深度嵌套**，导入时自动保留缩进层级，导出时自动还原。支持无序与有序列表混合嵌套。

### 表格智能处理

- **大表格自动拆分**：飞书 API 限制单个表格最多 9 行，超出自动拆分并保留表头
- **列宽自动计算**：根据内容计算（中文 14px，英文 8px，最小 80px，最大 400px）
- **单元格多块支持**：单元格内可包含 bullet/heading/text 混合内容

### 图表导入容错

- 服务端错误自动重试（最多 10 次，1s 间隔）
- Parse error / Invalid request parameter 不重试，直接降级为代码块
- 失败回退：删除空画板块，在原位置插入代码块

### 三阶段并发管道（导入架构）

1. **阶段一（顺序）**：按文档顺序创建所有块，收集图表和表格任务
2. **阶段二（并发）**：图表 worker 池 + 表格 worker 池并发处理
3. **阶段三（逆序）**：处理失败图表，降级为代码块

CLI flags：`--diagram-workers`（默认 5）、`--table-workers`（默认 3）、`--diagram-retries`（默认 10）

## 常用命令

```bash
# === 文档操作 ===
feishu-cli doc create --title "测试"
feishu-cli doc get <doc_id>
feishu-cli doc blocks <doc_id> --all
feishu-cli doc export <doc_id> -o output.md
feishu-cli doc import input.md --title "文档" --diagram-workers 5 --table-workers 3 --verbose
feishu-cli doc add <doc_id> -c '<JSON>'                        # JSON 格式添加块
feishu-cli doc add <doc_id> README.md --content-type markdown  # Markdown 格式
feishu-cli doc add-callout <doc_id> "内容" --callout-type info
feishu-cli doc add-board <doc_id>
feishu-cli doc batch-update <doc_id> '[...]' --source-type content
feishu-cli doc delete <doc_id> --start 1 --end 3

# === 知识库 ===
feishu-cli wiki get <node_token>
feishu-cli wiki export <node_token> -o doc.md
feishu-cli wiki spaces
feishu-cli wiki nodes <space_id>

# === 消息 ===
feishu-cli msg send --receive-id-type email --receive-id user@example.com --text "Hello"
feishu-cli msg send --receive-id-type email --receive-id user@example.com --msg-type post --content-file msg.json
feishu-cli msg search-chats --query "关键词"
feishu-cli msg history --container-id <chat_id> --container-id-type chat
feishu-cli msg get <message_id>
feishu-cli msg forward <message_id> --receive-id <id> --receive-id-type email

# === 电子表格 ===
feishu-cli sheet create --title "新表格"
feishu-cli sheet read <token> "Sheet1!A1:C10"
feishu-cli sheet write <token> "Sheet1!A1:B2" --data '[["姓名","年龄"],["张三",25]]'
feishu-cli sheet read-rich <token> <sheet_id> "sheet!A1:C10"   # V3 富文本
feishu-cli sheet write-rich <token> <sheet_id> --data-file data.json

# === 其他 ===
feishu-cli user info <user_id>
feishu-cli board image <whiteboard_id> output.png
feishu-cli file list [folder_token]
feishu-cli media upload image.png --parent-type docx_image --parent-node <doc_id>
feishu-cli comment list <file_token> --type docx
feishu-cli perm add <doc_id> --doc-type docx --member-type email --member-id user@example.com --perm full_access
feishu-cli calendar list
feishu-cli calendar create-event --calendar-id <id> --summary "会议" --start "2024-01-01T10:00:00+08:00" --end "2024-01-01T11:00:00+08:00"
feishu-cli task create --summary "待办事项"
feishu-cli task complete <task_id>
feishu-cli search messages "关键词" --user-access-token <token>
```

## 块类型映射

| block_type | 名称 | Markdown | 说明 |
|------------|------|----------|------|
| 1 | Page | 根节点 | 文档根节点 |
| 2 | Text | 段落 | 普通文本 |
| 3-11 | Heading1-9 | `#` ~ `######` | 标题 |
| 12 | Bullet | `- item` | 无序列表（支持嵌套） |
| 13 | Ordered | `1. item` | 有序列表（支持嵌套） |
| 14 | Code | ` ```lang ``` ` | 代码块 |
| 15 | Quote | `> text` | 引用（旧版，导入使用 QuoteContainer） |
| 16 | Equation | `$$formula$$` | 公式（API 不支持创建，降级为内联 Equation） |
| 17 | Todo | `- [x]` / `- [ ]` | 待办事项 |
| 19 | Callout | `> [!NOTE]` | 高亮块（bgColor 2-7 对应 6 种类型） |
| 21 | Diagram | Mermaid/PlantUML | 图表（自动转画板） |
| 22 | Divider | `---` | 分隔线 |
| 23 | File | 附件 | 文件块 |
| 26 | Iframe | `<iframe>` | 内嵌网页 |
| 27 | Image | `![](url)` | 图片 |
| 28 | ISV | TextDrawing/Timeline | 第三方块（导出为 Mermaid 注释/占位符） |
| 31 | Table | Markdown 表格 | 表格 |
| 32 | TableCell | — | 表格单元格（内部类型） |
| 34 | QuoteContainer | `> text` | 引用容器（v1.4.0，替代 Quote 用于导入） |
| 40 | AddOns | — | 扩展块（导出时递归展开子块） |
| 42 | WikiCatalog | `[Wiki 目录]` | 知识库目录块 |
| 43 | Board | 画板 | 画板 |

## SDK 注意事项

### 通用

- `larkdocx.Heading1-9`、`Bullet`、`Ordered`、`Code`、`Quote`、`Todo` 都使用 `*Text` 类型
- Todo 完成状态在 `TextStyle.Done`，Code 语言在 `TextStyle.Language`（整数编码）
- Table.Cells 是 `[]string` 类型，非指针切片
- DeleteBlocks API 使用 StartIndex/EndIndex，非单独 block ID
- Wiki 知识库使用 `node_token`，普通文档使用 `document_id`，注意区分
- Callout 块只需设置 BackgroundColor（2-7 对应 6 种颜色：2=红/WARNING、3=橙/CAUTION、4=黄/TIP、5=绿/SUCCESS、6=蓝/NOTE、7=紫/IMPORTANT），不能同时设置 EmojiId

### 文档导入

- **嵌套列表**：通过 `BlockNode` 树结构实现，导入时递归调用 `CreateBlock(docID, parentBlockID, children, -1)` 创建父子关系
- **表格单元格**：飞书 API 创建表格时会自动在每个单元格内创建空的 Text 块，填充内容时应更新现有块而非创建新块
- **表格列宽**：通过 `TableProperty.ColumnWidth` 设置，单位像素，数组长度需与列数一致
- **画板 API**：路径 `/open-apis/board/v1/whiteboards/{id}/nodes/plantuml`，`syntax_type=1` PlantUML / `2` Mermaid
- **diagram_type 映射**：0=auto, 1=mindmap, 2=sequence, 3=activity, 4=class, 5=er, 6=flowchart, 7=state, 8=component

### 电子表格

- V3 API 用于表格管理（创建/获取/工作表），V2 API 用于单元格读写
- V3 单元格 API 支持富文本读写（三维数组格式），元素类型：text、value、date_time、mention_user、mention_document、image、file、link、reminder、formula
- V3 写入限制：单次最多 10 个范围、5000 个单元格、50000 字符/单元格
- 范围格式：`SheetID!A1:C10`，支持整列 `A:C` 和整行 `1:3`
- 数据格式：V2 二维数组 `[["A1","B1"]]`，V3 三维数组 `[[[[{"type":"text","text":{"text":"Hello"}}]]]]`

### 其他模块

- 素材上传需指定 `--parent-type`（docx_image/docx_file 等）
- 日历 API 时间格式 RFC3339（如 `2024-01-01T10:00:00+08:00`），任务 API 使用 V2 版本
- 搜索 API 需要 User Access Token，不能使用 App Access Token
- 画板 API 使用通用 HTTP 请求方式（client.Get/Post），非专用 SDK 方法
- 用户信息 API 需要 `contact:user.base:readonly` 权限

## API 限制与处理

| 限制 | 说明 | 处理方式 |
|------|------|----------|
| 表格行数 | 单个表格最多 9 行 | 自动拆分为多个表格 |
| 批量创建块 | 每次最多 50 个块 | 自动分批处理 |
| API 频率限制 | 请求过快返回 429 | 自动重试 + 指数退避 |
| 图表并发 | 并发导入 Mermaid/PlantUML | worker 池（默认 5 并发） |
| Mermaid 花括号 | `{text}` 被识别为菱形节点 | 自动降级为代码块 |
| Mermaid par 语法 | `par...and...end` 飞书不支持 | 用 `Note over X` 替代 |
| Mermaid 复杂度 | 10+ participant + 2+ alt + 30+ 长标签 | 重试后降级为代码块 |
| sheet filter | 需要完整 col+condition 参数 | API 限制 |
| sheet protect | V2 API 返回 "invalid operation" | 待修复 |
| 图片插入 | DocX Open API 不支持通过 API 插入图片 | 创建空 Image 占位块，导入报告显示跳过数 |
| shell 转义 | zsh 中 `!` 被转义为 `\!` | 已在代码中处理 |

## Claude Code 技能

本项目提供以下 Claude Code 技能，位于 `skills/` 目录：

| 技能 | 说明 | 用法 |
|------|------|------|
| `/feishu-cli-read` | 读取飞书文档/知识库并转换为 Markdown | `/feishu-cli-read <doc_id\|url>` |
| `/feishu-cli-write` | 创建或更新飞书文档 | `/feishu-cli-write "标题"` |
| `/feishu-cli-create` | 快速创建空白文档 | `/feishu-cli-create "标题"` |
| `/feishu-cli-export` | 导出文档为 Markdown | `/feishu-cli-export <doc_id> [path]` |
| `/feishu-cli-import` | 从 Markdown 导入创建文档 | `/feishu-cli-import <file.md>` |
| `/feishu-cli-wiki` | 知识库操作 | `/feishu-cli-wiki get <node_token>` |
| `/feishu-cli-sheet` | 电子表格操作（V2/V3 API） | `/feishu-cli-sheet <token>` |
| `/feishu-cli-file` | 云空间文件管理 | `/feishu-cli-file list [folder_token]` |
| `/feishu-cli-comment` | 文档评论操作 | `/feishu-cli-comment list <file_token>` |
| `/feishu-cli-media` | 素材管理 | `/feishu-cli-media upload <file>` |
| `/feishu-cli-msg` | 消息发送 | `/feishu-cli-msg <receive_id>` |
| `/feishu-cli-perm` | 权限管理 | `/feishu-cli-perm <doc_token>` |
| `/feishu-cli-board` | 画板操作（导入图表、下载图片） | `/feishu-cli-board <whiteboard_id>` |
| `/feishu-cli-plantuml` | PlantUML 生成（飞书画板安全子集） | `/feishu-cli-plantuml <描述>` |
| `/feishu-cli-calendar` | 日历和日程管理 | `/feishu-cli-calendar list` |
| `/feishu-cli-task` | 任务管理 | `/feishu-cli-task list` |
| `/feishu-cli-search` | 搜索功能（需 User Access Token） | `/feishu-cli-search messages "关键词"` |

### 支持的 URL 格式

- 普通文档: `https://xxx.feishu.cn/docx/<document_id>`
- 知识库: `https://xxx.feishu.cn/wiki/<node_token>`
- 内部飞书: `https://xxx.larkoffice.com/wiki/<node_token>`
- Lark 国际版: `https://xxx.larksuite.com/wiki/<node_token>`

## 权限要求

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
| 日历 | `calendar:calendar:readonly`, `calendar:calendar` | 需单独申请 |
| 任务 | `task:task:read`, `task:task:write` | 需单独申请 |
| 搜索 | 需要 User Access Token | 用户授权 |

## 发布 Release 规范

### 完整发版流程

```bash
# 1. 确保代码已提交且在 main 分支
git checkout main && git pull origin main

# 2. 打 tag（替换为实际版本号）
VERSION=v1.4.0
git tag $VERSION && git push origin $VERSION

# 3. 构建所有平台（自动注入版本号和构建时间）
make build-all

# 4. 打包为 tar.gz（必须遵循以下规范）
cd bin
for platform in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64; do
  dir="feishu-cli_${VERSION}_${platform}"
  mkdir -p "$dir"
  cp "feishu-cli-${platform}" "${dir}/feishu-cli"
  tar czf "${dir}.tar.gz" "$dir"
  rm -rf "$dir"
done
# Windows 特殊处理（平台名用下划线 windows_amd64）
dir="feishu-cli_${VERSION}_windows_amd64"
mkdir -p "$dir"
cp "feishu-cli-windows-amd64.exe" "${dir}/feishu-cli.exe"
tar czf "${dir}.tar.gz" "$dir"
rm -rf "$dir"
cd ..

# 5. 创建 GitHub Release
gh release create $VERSION \
  bin/feishu-cli_${VERSION}_linux-amd64.tar.gz \
  bin/feishu-cli_${VERSION}_linux-arm64.tar.gz \
  bin/feishu-cli_${VERSION}_darwin-amd64.tar.gz \
  bin/feishu-cli_${VERSION}_darwin-arm64.tar.gz \
  bin/feishu-cli_${VERSION}_windows_amd64.tar.gz \
  --title "$VERSION" --notes "Release notes" --latest
```

### 打包规范（必须严格遵守）

| 规则 | 说明 |
|------|------|
| **文件格式** | 必须 `.tar.gz`，不能上传裸二进制 |
| **命名格式** | `feishu-cli_{version}_{platform}.tar.gz` |
| **内部结构** | 包含同名目录，目录内二进制统一命名为 `feishu-cli`（Windows 为 `feishu-cli.exe`） |
| **平台名称** | `linux-amd64`, `linux-arm64`, `darwin-amd64`, `darwin-arm64`, `windows_amd64`（注意 Windows 用下划线） |

**tar.gz 内部结构示例**：
```
feishu-cli_v1.4.0_linux-amd64/
└── feishu-cli            # 统一二进制名（不带平台后缀）
```

**原因**：`install.sh` 一键安装脚本依赖此命名规范，`find "$tmpdir" -name "feishu-cli"` 按固定名查找二进制。不遵循规范会导致安装失败。

### 版本号规则

- **Major**：不兼容变更
- **Minor**：新功能
- **Patch**：Bug 修复

### 注意事项

- 必须使用 `make build-all` 构建，不要直接 `go build`，否则版本号不会注入
- 打包前先验证 tar.gz 内容：`tar tzf bin/feishu-cli_vX.Y.Z_linux-amd64.tar.gz`
- 发版后用 `curl -fsSL https://raw.githubusercontent.com/riba2534/feishu-cli/main/install.sh | bash` 验证安装

## 已知问题

| 问题 | 说明 | 状态 |
|------|------|------|
| 表格导出 | 导出时表格单元格内容可能丢失（块类型 32） | 待修复 |
| file quota | `file quota` 命令 SDK 未实现 | 不支持 |
| board import CLI | 命令行单独导入画板，API 返回 404 | API 限制 |
| board create-notes | API 格式问题 | API 限制 |

## 技能使用规范（Skills）

本项目提供 Claude Code 技能，位于 `skills/` 目录。使用技能时需遵守以下规范：

### 1. 操作完成后必须发送通知

**每次执行完飞书相关操作后，必须立即发送飞书消息通知用户**，告知操作结果。

飞书相关操作包括：
- 创建/导入飞书文档
- 更新飞书文档
- 导出飞书文档
- 修改文档权限
- 大文件上传完成

**通知内容必须包含**：
- 操作类型（创建/更新/导出/权限变更等）
- 文档链接或导出路径
- 操作结果摘要
- 大文档需包含：图表渲染统计（成功/失败数量）、文档规模（行数/段落数）

### 2. 文档创建后必须添加最高权限

**每次创建新飞书文档后，必须立即给指定用户授予 `full_access`（最高权限）**。

**执行命令**：
```bash
feishu-cli perm add <DOC_ID> --doc-type docx --member-type email --member-id user@example.com --perm full_access --notification
```

**full_access 权限包含**：
- 管理协作者（添加/移除成员、设置权限）
- 编辑文档内容（修改、删除、添加）
- 管理文档设置（复制、移动、删除文档）
- 查看历史版本
- 导出文档

### 3. 创建飞书文档前必须参考规范

**每次生成将要导入飞书的 Markdown 内容前，必须先参考 `feishu-cli-doc-guide` 技能中的规范**，确保内容兼容飞书。

**核心检查项**：
- Mermaid 图表：禁止花括号 `{}`（flowchart 标签）、禁止 `par...and...end`、方括号冒号加双引号、sequenceDiagram 参与者 ≤ 8
- PlantUML 图表：无行首缩进、无 `skinparam`、类图无可见性标记（`+ - # ~`）
- 表格：超过 9 行会自动拆分，无需手动处理
- 图片：Open API 不支持插入，仅创建占位块
- 公式：行内 `$...$`、块级 `$$...$$`（块级降级为行内）
- Callout：仅 6 种类型（NOTE/WARNING/TIP/CAUTION/IMPORTANT/SUCCESS）

**详细规范**：读取 `skills/feishu-cli-doc-guide/SKILL.md` 和 `references/mermaid-spec.md`

## 功能测试验证

```
✅ doc create/get/blocks/export/import（含嵌套列表、混合嵌套）
✅ doc add (JSON/Markdown) / add-callout / add-board / batch-update
✅ wiki get/export / user info / board image
✅ file list/mkdir/move/copy / media upload/download
✅ comment list/add / perm add
✅ msg send/get/search-chats/history/forward
✅ task create/complete/delete
✅ sheet create/get/list-sheets/read/write/append
✅ sheet add-sheet/delete-sheet/copy-sheet/add-rows/add-cols/delete-rows/delete-cols
✅ sheet merge/unmerge/style/meta/find/replace/image
✅ sheet read-plain/read-rich/write-rich/insert/append-rich/clear（V3 API）
✅ Mermaid 图表导入（8 种类型全部验证，88 个图表 93.2% 成功率）
✅ PlantUML 图表导入（时序图、活动图已验证）
✅ 大规模导入：10,000+ 行 / 127 个图表 / 170+ 个表格
```
