# CLAUDE.md - 飞书 CLI 项目指南

## 项目概述

`feishu-cli` 是一个功能完整的飞书开放平台命令行工具，**核心功能是 Markdown ↔ 飞书文档双向转换**，支持文档操作、消息发送、权限管理、审批查询、知识库操作、文件管理、评论管理等功能。

## 技术栈

| 组件 | 选型 | 说明 |
|------|------|------|
| CLI 框架 | github.com/spf13/cobra | 子命令、自动补全 |
| 飞书 SDK | github.com/larksuite/oapi-sdk-go/v3 | 官方 SDK |
| 配置管理 | github.com/spf13/viper | YAML/环境变量 |
| Markdown | github.com/yuin/goldmark | GFM 扩展支持 |

> 项目结构：`cmd/`（CLI 命令）、`internal/auth`（OAuth）、`internal/client`（API 封装）、`internal/converter`（Markdown 转换器）、`skills/`（Claude Code 技能）。按需 `ls` 查看详情。

## 开发指南

### 构建与测试

```bash
go build -o feishu-cli .          # 快速构建
make build                        # 构建到 bin/feishu-cli
make build-all                    # 多平台构建（发版用，自动注入版本号）
go test ./...                     # 运行所有测试
go vet ./...                      # 静态检查
```

### 质量标准（任何改动提交前必须满足）

1. **实物验证铁律：任何改动，都必须使用实际编译的二进制产物进行验证。**
   先 `go build -o feishu-cli .`（或 `make build`）编译当前代码，再用编译出的二进制
   实跑受影响的命令确认行为符合预期（读操作真实调用；写操作优先 `--dry-run`，必要时
   在测试文档/画板上真实执行）。禁止只凭 `go vet`、单元测试或静态读代码就宣告完成；
   文档/技能中对 CLI 行为的描述，同样以新编译二进制的实跑结果为准。
2. **基线全绿**：`gofmt -l cmd internal` 无输出、`go test ./...`、`go vet ./...` 通过。
3. **结论附证据**：报告"已完成/已修复"必须附带验证命令及其输出（或退出码/截图），
   不做未验证的声明。
4. **可执行文档必须实跑**：README / skills 里给出的命令、脚本、示例，改动后逐条实跑；
   涉及可视化配色的改动，从仓库根运行
   `node skills/feishu-cli-visual/references/workflows/dataviz/scripts/validate_palette.js` 复验定稿色板、
   `node skills/feishu-cli-visual/references/workflows/dataviz/scripts/check_docs.js` 核查文档一致性，全绿才算完成。
5. **Skill 结构校验**：修改 `skills/`、CLI 命令或源码中的 Skill 路径后运行 `make check-skills`；
   该目标会先从当前源码重新构建 `bin/feishu-cli`，再检查 Skill 结构、引用和命令唯一归属。
6. **隐私扫描**：提交前按下方"开发规范"第 6 条检查敏感信息。

发版有更严格的完整清单，见「发布 Release 规范」。

### 开发规范

1. **错误处理**：使用中文错误信息，提供解决建议
2. **命令帮助**：所有命令使用简体中文描述
3. **代码注释**：关键逻辑使用中文注释
4. **提交信息**：遵循 Conventional Commits 规范
5. **指针解引用**：使用 `internal/client/helpers.go` 中的 `StringVal/BoolVal/IntVal` 等工具函数
6. **隐私安全（开源项目，必须遵守）**：
   - 代码、文档、技能文件中**禁止出现任何真实的个人邮箱、密码、Token、密钥**
   - 示例邮箱统一使用 `user@example.com`，示例 Token 使用 `cli_xxx`、`u-xxx` 等占位符
   - 新增或修改文件前，检查是否包含 `@bytedance.com`、`@lark.com` 等内部邮箱域名
   - URL 域名使用通用的 `feishu.cn`，**禁止带企业前缀**（如 `bytedance.feishu.cn`）
   - `.env`、`config.yaml` 等含敏感信息的文件已在 `.gitignore` 中排除，禁止提交

### 配置方式

**优先级**：环境变量 > 配置文件 > 默认值

```bash
# 环境变量（推荐）
export FEISHU_APP_ID=cli_xxx
export FEISHU_APP_SECRET=xxx

# 配置文件 (~/.feishu-cli/config.yaml)，通过 feishu-cli config init 初始化
```

## 核心功能

### OAuth 认证（Device Flow）

通过 **OAuth 2.0 Device Flow（RFC 8628）** 获取 User Access Token，用于搜索、审批任务查询等需要用户授权的功能。**无需配置重定向 URL 白名单**（v1.18+ 已删除 Authorization Code Flow）。

**Token 使用策略**（按命令分四类，对应 `cmd/utils.go` 四个 helper）：
- **读类 · User 优先 + Tenant 兜底**（`resolveOptionalUserTokenWithFallback`，约 85 个命令）：`msg history/list/get/mget/thread-messages/resource-download`、`task get/list/subtask list/comment list/tasklist get/list/tasks`、`calendar get/list/primary/agenda/freebusy/suggestion/room-find/event get/list/search/attendee list`、`file meta/stats/list/version list/get/download`、`board image/nodes/export-code/lint`、`user read`、`wiki get/nodes/spaces/export/member list`、`drive pull/push/status`、`vc bot meeting-events`（端点拒收 Tenant Token，User 优先）、**sheet 全家桶**（所有 sheet 子命令含写）等。优先级链：`--user-access-token` → `FEISHU_USER_ACCESS_TOKEN` → `~/.feishu-cli/token.json`（过期自动刷新）→ `config.yaml` 的 `user_access_token` → App Token 兜底。
- **写类 · 默认 Bot 身份**（`resolveOptionalUserToken`）：所有 `add/create/update/delete/move/copy/import/upload/send/reply/forward/merge-forward` 类命令、`comment reply`、`doc content-update / table 写`、`msg delete`（Bot 自撤回）等。**不会自动加载 token.json**，仅当显式传 `--user-access-token` 或 `FEISHU_USER_ACCESS_TOKEN` 时切到 User Token。`vc bot meeting-join/leave` 同属默认 Bot 身份，但用更严格的 `resolveFlagUserToken`：**只认 `--user-access-token` flag，连 `FEISHU_USER_ACCESS_TOKEN` 环境变量都不读**。
- **必须 User Token**（`resolveRequiredUserToken` / `requireUserToken`）：`search docs/messages/apps`、`approval task query/approve/reject/transfer`、`approval instance get/cancel/cc`、`task my`（`my_tasks`）、`msg pin/reaction/search-chats/flag`、`chat get/update/delete/member`、`vc search/notes/recording`、`minutes/mail` 全部、`drive upload/download/export/import/move/add-comment/task-result/search`、`calendar rsvp`、`markdown create/fetch/overwrite/diff` 等。失败直接报错。
- **身份可选 · `--as` 显式切换**（`resolveIdentityToken`）：`bitable` 全家桶（所有子命令含读写）。命令组 persistent flag `--as bot|user|auto`，默认 `auto`（User 优先、Tenant 兜底，未登录自动用 App Token）；`--as bot` 强制 App Token（cron/无人值守，永不过期，无需 `auth login`）；`--as user` 强制 User Token（缺失报错）。底层 `base/v3` 与 `bitable/v1` API 本身同时支持 User/Tenant 身份，故 CLI 不再硬性强制 User Token。
- **审批任务查询**：`approval task query` 会调用 `/authen/v1/user_info` 推断 `open_id`，缓存到 `~/.feishu-cli/user_profile.json`，`auth logout` 时自动清理

**登录命令四种模式**：
- `auth login --scope "..."`：显式请求 scope，阻塞轮询
- `auth login --domain <name> --recommend`：按业务域申请推荐 scope（推荐）
- `auth login --json`：JSON 事件流输出（AI Agent 推荐，配合 `run_in_background`）
- `auth login --no-wait --json` + `auth login --device-code <code> --json`：两步模式

**scope 策略**：登录时显式声明 scope，不再依赖后台全量兜底。`offline_access` 和 `auth:user.id:read` 自动注入。AI Agent 执行业务前应先 `auth check --scope "REQ_SCOPES"` 预检。

**审批输出模式**：不传 `--output` 输出文本摘要；`--output json` CLI 归一化 JSON；`--output raw-json` 原始响应。

### Markdown ↔ 飞书文档双向转换

**导入**：`feishu-cli doc import doc.md --title "文档" --verbose`
**导出**：`feishu-cli doc export <doc_id> -o output.md`

支持的语法：标题、段落、列表（无限深度嵌套）、任务列表、代码块、引用（QuoteContainer）、Callout（6 种类型）、表格、分割线、图片（默认 `--upload-images` 上传）、链接、公式、粗体/斜体/删除线/下划线/行内代码/高亮

### Mermaid / PlantUML 图表转画板

**推荐 Mermaid**，导入时自动转画板。支持 8 种 Mermaid 类型：flowchart（含 subgraph）、sequenceDiagram、classDiagram、stateDiagram-v2、erDiagram、gantt、pie、mindmap。PlantUML 支持时序图、活动图、类图、用例图、组件图、ER 图、思维导图等全部类型。

### 表格智能处理

- **大表格保持单表连贯**：飞书 `create_block` 限制单表最多 9 行 × 9 列。行数超限时创建 9 行初始表，剩余行通过 `insert_table_row` API 追加到同一个 block，视觉上为一张连贯的表；列数超限仍按列组拆分（保留首列做标识）。⚠ 单文档写入受飞书 3 QPS 节流（v1.29+ 内置 docWriteLimiter，避免触发 99991400），追加 1 行 ≈ 333ms（100 行 ≈ 33s，单文档串行）；追加行数 ≥ 5 时 verbose 模式会打印进度
- **批量填充加速（v1.29+）**：单元格填充改用 `batch_update` API，single-group cell 每批 ≤30 个一次写入；典型 4×6×8 表场景从 ~70s 降到 ~3s（25-30x，issue #159）
- **列宽自定义**（v1.29+，issue #156）：默认按内容启发式（中文 14px / 英文 8px，最小 80 / 最大 400）。可通过两种方式覆盖：
  - 紧邻表格上方注释 `<!-- feishu-colwidth: 80,200,*,30% -->`：单位 px / 百分比 / `*`(走 auto)
  - CLI flag `--table-column-width=auto|fixed|N1,N2,...`（覆盖整篇文档；注释优先级高于 flag）
- **单元格多块支持**：单元格内可混合 bullet/heading/text

### 图表导入容错

- 服务端错误自动重试（最多 10 次，1s 间隔）
- Parse error / Invalid request parameter 不重试，直接降级为代码块
- 失败回退：删除空画板块，在原位置插入代码块

### 三阶段并发管道（导入架构）

1. **阶段一（顺序）**：按文档顺序创建所有块，收集图表和表格任务
2. **阶段二（并发）**：图表 worker 池 + 表格 worker 池并发处理
3. **阶段三（逆序）**：处理失败图表，降级为代码块

**CLI flags**：`--diagram-workers`（默认 5）、`--table-workers`（默认 3）、`--diagram-retries`（默认 10）、`--upload-images`（默认开启）、`--image-workers`（默认 2，API 限制 5 QPS）

### msg history 自动展开线程（v1.27.1+）

`msg history` 对话题群（chat_mode=thread）默认自动展开每条根消息的线程回复：

- 默认 `--expand-threads=true`，单话题最多 50 回复，所有话题累计上限 500
- JSON 输出顶层新增 `thread_replies` / `thread_has_more` / `thread_replies_card_texts`
- **发送者名字解析（v1.36+）**：所有读消息请求带 `with_sender_name=true`，服务端直接回填显示名（含 **Bot** 与**外部租户用户**，无需通讯录权限，实测内部群解析率 ~100%）；mentions 与 contact basic_batch 仍作兜底（`internal/client/sender_names.go` 进程级注册表 + `ResolveSenderNames` 三步解析）
- 关闭：`--expand-threads=false`；调规模：`--threads-per-page` / `--threads-total-limit`

## 命令速查

完整命令清单见 [README.md](README.md) 和对应 skill 文档。关键入口：

```bash
# 认证
feishu-cli auth login --domain <name> --recommend       # 按业务域登录
feishu-cli auth check --scope "REQ_SCOPES"              # 预检 scope
feishu-cli auth status                                   # 查看授权状态

# 文档导入/导出（核心功能）
feishu-cli doc import input.md --title "..." --upload-images --verbose
feishu-cli doc export <doc_id> -o output.md
feishu-cli doc read <doc_id> {--outline | --heading "标题" | --keyword "正则" [--context N]}  # 大文档选择性读取
feishu-cli doc content-update <doc_id> --mode <mode> --markdown "..."
#   mode: append / overwrite / replace_range / delete_range / insert_after
feishu-cli doc htmlbox {create|update|get|delete} <doc_id> [block_id] --html-file x.html  # 妙笔BOX HTML 小组件（文档里跑动画/ECharts/可交互图表，唯一能"动"的载体）

# 多维表格（统一用 --base-token，底层 base/v3 + 部分 bitable/v1 API）
feishu-cli bitable {create|get|copy|update} ...              # update=重命名/高级权限
feishu-cli bitable {table|field|record|view|role} <action> ...
feishu-cli bitable record batch-get --record-ids ...
feishu-cli bitable record list --filter-json '{"logic":"and","conditions":[["状态","==",["Doing"]]]}' --sort-json '[{"field":"分数","desc":true}]'  # 结构化过滤（tuple DSL，无需关键词；语法见 feishu-cli-data skill）
feishu-cli bitable view view-{filter|sort|group|visible-fields|timebar|card}-{get|set} ...
feishu-cli bitable dashboard {list|copy} ...
feishu-cli bitable form {get|patch} ... ; feishu-cli bitable form field {list|patch} ...
feishu-cli bitable role member {list|create|delete|batch-create|batch-delete} ...
feishu-cli bitable workflow {list|enable|disable} ...
feishu-cli bitable advperm {enable|disable} --base-token ...
feishu-cli bitable data-query ...
# 新增命令统一支持 --format json|pretty|table|ndjson|csv + --jq；写命令支持 --dry-run

# 搜索消息（默认返回消息 ID；--enrich 才补全内容/发送者/群名/时间；支持 --format/--jq/--page-all）
feishu-cli search messages "<query>" [--format table] [--jq ...] [--enrich]

# 通用 API 透传（api 命令支持 --jq/--format json|pretty|table|ndjson|csv）
feishu-cli api GET <path> --jq '.data.items[].name' --format table

# 邮件（User Token 必需，默认存草稿，--confirm-send 才发送）
feishu-cli mail {triage|send|draft-create|reply|forward|message|thread} ...

# 云盘增强
feishu-cli drive {upload|download|export|import|move|add-comment|task-result} ...

# 视频会议与妙记
feishu-cli vc {search|notes|recording} ...                       # 需 User Token
feishu-cli vc note {detail|transcript} <note_id> ...             # 智能纪要详情/统一逐字稿（需 User Token）
feishu-cli vc bot {meeting-join|meeting-leave} ...               # 默认 Bot/Tenant 身份
feishu-cli vc bot meeting-events --meeting-id ...                # 需 User Token（端点拒收 Tenant）
feishu-cli minutes {get|download} --minute-tokens ...           # 需 User Token

# 画板（v1.25+ 新增能力 ⭐）
feishu-cli board svg-import <id> drawing.svg                    # SVG 单节点装饰（< 2KB 小元素）
feishu-cli board clone <src> <dst> --batch-size 10              # 克隆画板（含 connector ID 重映射）
feishu-cli board upload-image <id> photo.png                    # 图片转 image 节点
feishu-cli board lint <id>                                       # 几何质检 + 综合评分
feishu-cli board export-code <id> --merge --output-path design.svg   # 反向导出 SVG
feishu-cli board import <id> diagram.mmd --syntax mermaid --engine local  # Mermaid 本地引擎（whiteboard-cli 翻译，节点可编辑）
feishu-cli board update <id> nodes.json --overwrite --snapshot old.json    # 覆盖+快照
# AI 自由作图首选：5 步管道（生成 SVG → whiteboard-cli 翻译 → 修 z_index → 修剪 viewBox → 分批上传）
python3 skills/feishu-cli-visual/references/workflows/board/scripts/svg_to_board.py drawing.svg <whiteboard_id>

# 电子表格类型保真 round-trip（读侧 table-get 与写侧 table-put 形状对称）
feishu-cli sheet table-get <token> <sheet_id> [--range A1:D50] > t.json   # dtype 自动推断
feishu-cli sheet table-put <token> <sheet_id> --sheets-file t.json

# OKR（--as bot 默认；cycle detail / progress get·update·delete / upload-image 均已就绪）
feishu-cli okr cycle {list|detail} ... ; feishu-cli okr progress {list|get|create|update|delete} ...

# 其他模块：doc / msg / sheet / calendar / task / tasklist / chat / wiki / file / perm / search / user / dept / comment / media
```

完整子命令列表：`feishu-cli --help` 或 `feishu-cli <cmd> --help`。

## SDK 注意事项

### 通用

- `larkdocx.Heading1-9`、`Bullet`、`Ordered`、`Code`、`Quote`、`Todo` 都使用 `*Text` 类型
- Todo 完成状态在 `TextStyle.Done`，Code 语言在 `TextStyle.Language`（整数编码）
- Table.Cells 是 `[]string` 类型，非指针切片
- DeleteBlocks API 使用 StartIndex/EndIndex，非单独 block ID
- Wiki 知识库使用 `node_token`，普通文档使用 `document_id`，注意区分
- **User Token vs App Token**：搜索 API 必须用 User Token，通过 `auth.ResolveUserAccessToken()` 解析，支持自动刷新
- Callout 块只需设置 BackgroundColor（2-7 对应 6 色：2=红/WARNING、3=橙/CAUTION、4=黄/TIP、5=绿/SUCCESS、6=蓝/NOTE、7=紫/IMPORTANT），不能同时设置 EmojiId

### 文档导入

- **嵌套列表**：通过 `BlockNode` 树结构实现，递归调用 `CreateBlock(docID, parentBlockID, children, -1)`
- **表格单元格**：飞书创建表格时会自动创建空 Text 块，填充时应更新现有块而非创建新块
- **表格列宽**：`TableProperty.ColumnWidth` 单位像素，长度需与列数一致
- **画板 API**：`/open-apis/board/v1/whiteboards/{id}/nodes/plantuml`，`syntax_type=1` PlantUML / `2` Mermaid
- **diagram_type 映射**：0=auto, 1=mindmap, 2=sequence, 3=activity, 4=class, 5=er, 6=flowchart, 7=state, 8=component
- **画板图片节点**：上传用 `parent_type=whiteboard` + `parent_node=画板ID`；节点格式 `{"image":{"token":"xxx"}}`（嵌套）；每节点独立 token；API 不支持裁切/遮罩
- **画板克隆 GET→POST**：必须移除 `id`、`locked`、`children`、`parent_id`；`composite_shape` 保留完整子结构；批量建议每批 10 个、间隔 3s

### 电子表格

- V3 API 用于表格管理，V2 API 用于单元格读写
- V3 单元格 API 支持富文本（三维数组），元素类型：text、value、date_time、mention_user、mention_document、image、file、link、reminder、formula
- V3 写入限制：单次最多 10 个范围、5000 个单元格、50000 字符/单元格
- 范围格式：`SheetID!A1:C10`，支持整列 `A:C` 和整行 `1:3`

### 其他模块

- 素材上传需指定 `--parent-type`（docx_image/docx_file 等）
- 日历 API 时间格式 RFC3339（`2024-01-01T10:00:00+08:00`），任务 API 使用 V2 版本
- 搜索文档 API 支持类型：doc, docx, sheet, slides, bitable, mindnote, file, wiki, shortcut
- 画板 API 使用通用 HTTP 请求（client.Get/Post），非专用 SDK 方法
- 用户信息 API 需 `contact:user.base:readonly` 权限

## 块类型速记

常用：1=Page、2=Text、3-11=Heading1-9、12=Bullet、13=Ordered、14=Code、17=Todo、19=Callout、21=Diagram、22=Divider、27=Image、31=Table、34=QuoteContainer、40=AddOns、42=WikiCatalog、43=Board。

完整映射见 `internal/converter/types.go`。

## API 限制与处理

| 限制 | 说明 | 处理方式 |
|------|------|----------|
| 表格行数/列数 | create 单表最多 9×9 | 行超限：9 行初始表 + `insert_table_row` 追加（单 block 连贯）；列超限：按列组拆分保留首列 |
| 文件夹子节点 | 直接子节点 ≤ 1500 | 超出报错 1062507 |
| 文档块总数 | 有上限 | 超出报错 1770004 |
| 批量创建块 | 每次最多 50 | 自动分批 |
| API 频率限制 | 429 | 自动重试 + 指数退避 |
| 图表并发 | worker 池 | 默认 5 并发 |
| Mermaid 花括号 | `{text}` 识别为菱形 | 自动降级为代码块 |
| Mermaid par 语法 | 飞书不支持 | 用 `Note over X` 替代 |
| Mermaid 复杂度 | 10+ participant + 2+ alt + 30+ 长标签 | 重试后降级 |
| sheet filter | 需完整 col+condition | API 限制 |
| sheet protect | V2 返回 "invalid operation" | 待修复 |
| 图片插入 | 素材上传 + Image 块引用 | 失败时创建占位块 |
| shell 转义 | zsh 中 `!` 转义为 `\!` | 已在代码中处理 |

## Claude Code 技能

位于 `skills/` 目录，当前 9 个领域 Skill。每个 Skill 在 `references/workflows/` 下按需加载子工作流；README 的“AI 技能集成”章节是对外安装清单。

### 新建与拆分原则

Agent 为本项目增加能力时，默认扩展现有领域 Skill，不新建顶层 Skill。顶层 Skill 会常驻
Agent 的路由上下文；按功能点持续拆分会增加触发冲突和维护成本，因此应把当前 9 个领域
作为稳定边界，并控制总量。

1. 新建前先判断能力能否归入现有领域；可以复用时，在对应 Skill 的
   `references/workflows/<workflow>/` 中增加子工作流，并按需添加 references、scripts 或 assets。
2. 不要为单个命令、接口或相邻功能单独创建顶层 Skill。优先补充现有工作流，或在同一领域内
   新建按需加载的子工作流。
3. 只有当新能力无法合理归入任何现有领域、触发语义独立且长期维护边界清晰时，才考虑新增
   顶层 Skill。提交说明必须解释为什么不能复用现有领域，并同步更新 `skills/manifest.yaml`、
   README、评测用例和 `make check-skills` 覆盖。
4. 每次新增顶层 Skill 前都要复查现有 Skill 是否可以合并；不要让 Skill 数量随功能数量无限增长。
5. 修改顶层 Skill 的 `description` 时，必须同步维护 `skills/trigger-evals.json`。每个领域保留
   8 个正例；用 `python3 scripts/build_trigger_eval_set.py <skill-name>` 生成 8 正例 + 8 个
   同属 feishu-cli 的近邻负例，再按 `skill-creator` 流程实跑。触发评测必须使用
   `--num-workers 1`：并发 worker 会同时注册多个同描述临时 Skill，造成名称碰撞和错误漏报。

| 技能 | 说明 |
|------|------|
| 平台基础 | `feishu-cli-platform`：auth/config/profile/doctor/api/schema/search/user/dept |
| 文档核心 | `feishu-cli-docs`：read/write/import/export/markdown |
| 云盘与权限 | `feishu-cli-storage`：drive/file/media/wiki/comment/perm |
| 消息协作 | `feishu-cli-messaging`：msg/chat/card/event |
| 数据与表格 | `feishu-cli-data`：sheet/bitable |
| 画板与展示 | `feishu-cli-visual`：dataviz/board/slides/htmlbox/apps |
| 工作管理 | `feishu-cli-work`：calendar/task/tasklist/approval/attendance/okr |
| 邮箱 | `feishu-cli-mail` |
| 会议与妙记 | `feishu-cli-meetings`：vc/minutes |

### 支持的 URL 格式

- 文档：`https://xxx.feishu.cn/docx/<document_id>`
- 多维表格：`https://xxx.feishu.cn/base/<app_token>`
- 知识库：`https://xxx.feishu.cn/wiki/<node_token>`（含 `larkoffice.com` / `larksuite.com`）

## 权限要求

**推荐做法**：

```bash
feishu-cli config create-app --save   # 一键创建飞书应用（Device Flow 自注册）
# 在飞书开放平台应用权限管理页面粘贴 README 的 JSON 一次性开通全部 scope
feishu-cli auth login                 # OAuth 用户授权
```

完整权限清单（tenant + user 共 400+ scope）见 [README.md 权限要求](README.md#权限要求)。关键 scope 速查：

- **doc**：`docx:document:*`、`docs:document.media:*`
- **wiki**：`wiki:wiki:readonly`、`wiki:node:*`、`wiki:space:*`、`wiki:member:*`
- **drive**：`drive:drive`、`drive:drive.metadata:readonly`、`drive:file:*`（注意：没有 `drive:drive:readonly`）
- **消息**：`im:message`、`im:message:send_as_bot`；加急 `im:message.urgent*`；群聊 `im:chat:*`、`im:chat.members:*`；历史 `im:message:readonly`
- **sheet/bitable**：`sheets:spreadsheet:*`、`base:{app,table,record,field,view}:*`
- **calendar/task**：`calendar:calendar*:*`、`task:{task,tasklist,comment}:*`
- **审批**：`approval:approval:readonly`；任务查询 `approval:task`（User Token）
- **搜索（必需 User Token）**：`search:docs:read`、`search:message`
- **群消息（User 身份）**：`im:message.group_msg:get_as_user`、`im:message.p2p_msg:get_as_user`
- **vc/minutes（User 身份）**：`vc:meeting.search:read`、`vc:note:read`、`vc:record:readonly`、`minutes:minutes*:*`
- **mail（User 身份）**：`mail:user_mailbox:readonly`、`mail:user_mailbox.message.body:read`
- **Refresh Token**：`offline_access`（Device Flow 自动注入）

## 发布 Release 规范

### 打包规范（严格遵守）

| 规则 | 说明 |
|------|------|
| 文件格式 | 必须 `.tar.gz`，不能上传裸二进制 |
| 命名格式 | `feishu-cli_{version}_{platform}.tar.gz` |
| 内部结构 | 同名目录，二进制统一命名为 `feishu-cli`（Windows 为 `feishu-cli.exe`） |
| 平台名称 | `linux-amd64`、`linux-arm64`、`darwin-amd64`、`darwin-arm64`、`windows-amd64` |
| 校验文件 | 打完所有 tar.gz 后在同目录生成 `checksums.txt`：`sha256sum *.tar.gz > checksums.txt`（macOS 用 `shasum -a 256 *.tar.gz > checksums.txt`），随 release 一起上传 |

**原因**：`install.sh` 一键安装脚本依赖此命名规范，`find "$tmpdir" -name "feishu-cli"` 按固定名查找；
并在下载后按 `checksums.txt` 校验 tar.gz 的 sha256（缺失该文件时告警但继续，兼容旧 release）。

### 流程要点

1. 发版前验证：`gofmt -l cmd internal`、`go test ./...`、`go vet ./...`、`make check-skills`、敏感信息扫描
2. `VERSION=vX.Y.Z; make build-all VERSION="$VERSION"` 构建所有平台（**不要在打 tag 前直接跑无 `VERSION` 的 `make build-all`**，否则 `git describe` 会注入上一版 tag 的开发版本号）
3. 按规范打包成 tar.gz（参考现有 release 资产结构），再生成 `checksums.txt`：`sha256sum *.tar.gz > checksums.txt`
4. 本地验证安装包结构和 `install.sh` 资产命名
5. 在 main 分支打 tag 并 push：`VERSION=vX.Y.Z; git tag $VERSION && git push origin $VERSION`
6. `gh release create $VERSION <所有 .tar.gz> checksums.txt --title "$VERSION" --notes "..." --latest`
7. 验证：`curl -fsSL https://raw.githubusercontent.com/riba2534/feishu-cli/main/install.sh | bash`

### 版本号规则

Major：不兼容变更；Minor：新功能；Patch：Bug 修复。

## 通用 API 透传命令（兜底）

如果某个飞书 OpenAPI 端点本项目没封装专门命令，**不要**手写 curl 或绕路，**用 `feishu-cli api` 透传**：

```bash
feishu-cli api <METHOD> </open-apis/...> [--params '<JSON>'] [--data '<JSON>'] [--as bot|user|auto] [--dry-run]
```

自动复用本地 token（含自动刷新）、自动错误码翻译（含 232033/99991679/scope 不足等中文提示）、支持 URL 内嵌 query 拆解、`--dry-run` 预览请求。

先用 `feishu-cli schema <service>.<resource>.<method>` 查参数和 path，再用 `feishu-cli api` 调即可。常见场景：调本地 152 个本地元数据未覆盖的 API（飞书开放平台 2500+ 端点 ≈ 94% 用 `api` 命令）。

## 外部群操作（重要）

飞书**外部群**（`external=true`）的「群信息/成员/配置」类 API 默认 232033 拒绝。
碰到 232033 错误，**不要**当作"飞书完全不支持"——它只是要求换 App：

1. App 开启「对外共享能力」（飞书开放平台 → 应用 → 凭证与基础信息）
2. 该 App 的 Bot 已加入此群

切换方式（不写盘）：
```bash
FEISHU_APP_ID=cli_对外共享App FEISHU_APP_SECRET=xxx feishu-cli <命令> --as bot
```

`feishu-cli chat member list/add/remove` 已支持 `--as bot|user|auto`，外部群推荐用 `--as bot`。
完整路径与排错见 `skills/feishu-cli-messaging/references/workflows/chat/references/external-chat.md`。

> 不受外部群限制的：`msg history`、`msg search-chats`、`msg send/reply` 等"消息侧"和"列表侧"API 都可以正常调。受限的是"群内信息/成员/配置"侧。

## 已知问题

| 问题 | 说明 | 状态 |
|------|------|------|
| 表格导出 | 表格单元格内容可能丢失（块类型 32） | 待修复 |
| `file quota` | SDK 未实现 | 不支持 |
| board import CLI | 命令行单独导入画板返回 404 | API 限制 |

## 技能使用规范（Skills）

`~/.claude/rules/lark-config.md` 已覆盖飞书操作的全局规则（通知、权限、创建前规范）。本项目特殊项：

### owner_email 配置

创建新文档后，权限授予的邮箱按以下优先级解析：
1. 环境变量 `FEISHU_OWNER_EMAIL`
2. 配置文件 `~/.feishu-cli/config.yaml` 的 `owner_email`
3. 未配置则提示用户设置

若 `transfer_ownership: true`（或 `FEISHU_TRANSFER_OWNERSHIP=true`），还需执行 `perm transfer-owner` 转移所有权。

### 飞书 Markdown 兼容检查

生成将导入飞书的 Markdown 前，必须参考 `skills/feishu-cli-docs/references/workflows/import/references/doc-guide.md`。核心检查项：

- **Mermaid**：普通标签禁止字面花括号 `{}`；`A{判断}` 条件菱形合法。禁止 `par...and...end`、方括号冒号加双引号、sequenceDiagram 参与者 ≤ 8
- **SVG**：使用恰好三个反引号的 `svg` fence，导入时转换为画板节点
- **PlantUML**：无行首缩进、无 `skinparam`、类图无可见性标记（`+ - # ~`）
- **表格**：超 9 行自动走"9 行初始表 + `insert_table_row` 追加"策略保持单 block 连贯；列 > 9 按列组拆分
- **图片**：默认 `--upload-images` 上传，关闭时创建占位块
- **公式**：行内 `$...$`、块级 `$$...$$`（块级降级为行内）
- **Callout**：仅 6 种（NOTE/WARNING/TIP/CAUTION/IMPORTANT/SUCCESS）
