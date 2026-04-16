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

**Token 使用策略**：
- **默认使用 App Token**（租户身份）：wiki、msg、calendar、task 等 55+ 个命令默认通过 `resolveOptionalUserToken` 使用 App Token，不会自动加载 User Token
- **显式使用 User Token**：通过 `--user-access-token` 参数或 `FEISHU_USER_ACCESS_TOKEN` 环境变量覆盖为用户身份
- **必须 User Token**：搜索命令（`search docs/messages/apps`）通过 `resolveRequiredUserToken` 走优先级链：
  1. `--user-access-token` 参数
  2. `FEISHU_USER_ACCESS_TOKEN` 环境变量
  3. `~/.feishu-cli/token.json`（过期自动用 refresh_token 刷新）
  4. `config.yaml` 中的 `user_access_token`
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

- **自动拆分**：飞书 API 限制单表格最多 9 行 × 9 列，超出自动拆分并保留表头/首列
- **列宽自动计算**：中文 14px，英文 8px，最小 80px，最大 400px
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
feishu-cli doc content-update <doc_id> --mode <mode> --markdown "..."
#   mode: append / overwrite / replace_range / delete_range / insert_after

# 多维表格（统一用 --base-token，底层 base/v3 API）
feishu-cli bitable {create|get|copy} ...
feishu-cli bitable {table|field|record|view|role} <action> ...
feishu-cli bitable view view-{filter|sort|group|visible-fields|timebar|card}-{get|set} ...
feishu-cli bitable advperm {enable|disable} --base-token ...
feishu-cli bitable {data-query|workflow list} ...

# 邮件（User Token 必需，默认存草稿，--confirm-send 才发送）
feishu-cli mail {triage|send|draft-create|reply|forward|message|thread} ...

# 云盘增强
feishu-cli drive {upload|download|export|import|move|add-comment|task-result} ...

# 视频会议与妙记（全部需 User Token）
feishu-cli vc {search|notes|recording} ...
feishu-cli minutes {get|download} --minute-tokens ...

# 其他模块：doc / msg / sheet / calendar / task / tasklist / chat / wiki / file / perm / board / search / user / dept / comment / media
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
| 表格行数/列数 | 单表最多 9×9 | 自动拆分，保留表头/首列 |
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

位于 `skills/` 目录，15 个：

| 技能 | 说明 |
|------|------|
| `/feishu-cli-read` | 读取文档/知识库 → Markdown |
| `/feishu-cli-write` | 创建/写入文档（含素材插入、快速空白文档） |
| `/feishu-cli-import` | 从 Markdown 导入创建文档 |
| `/feishu-cli-export` | 导出 Markdown/PDF/Word，下载素材 |
| `/feishu-cli-perm` | 权限管理 |
| `/feishu-cli-msg` | 消息全功能（发送/回复/转发/批量/资源/话题） |
| `/feishu-cli-card` | 构造美观 V2.0 interactive 卡片（7 场景模板 + 30+ 组件） |
| `/feishu-cli-chat` | 会话浏览与群聊管理 |
| `/feishu-cli-toolkit` | 综合工具箱（表格/日历/任务/清单/文件/素材/评论/知识库/通讯录） |
| `/feishu-cli-board` | 画板（精排绘图/Mermaid/截图/节点） |
| `/feishu-cli-bitable` | 多维表格全功能 |
| `/feishu-cli-vc` | 视频会议与妙记 |
| `/feishu-cli-auth` | OAuth 认证、Token、scope 配置 |
| `/feishu-cli-search` | 搜索文档/消息/应用 |
| `feishu-cli-doc-guide` | 飞书文档创建规范（内部参考） |

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
| 平台名称 | `linux-amd64`、`linux-arm64`、`darwin-amd64`、`darwin-arm64`、`windows_amd64`（Windows 用下划线） |

**原因**：`install.sh` 一键安装脚本依赖此命名规范，`find "$tmpdir" -name "feishu-cli"` 按固定名查找。

### 流程要点

1. 在 main 分支打 tag 并 push：`VERSION=vX.Y.Z; git tag $VERSION && git push origin $VERSION`
2. `make build-all` 构建所有平台（**不要直接 `go build`**，否则版本号不会注入）
3. 按规范打包成 tar.gz（参考现有 release 资产结构）
4. `gh release create $VERSION <所有 .tar.gz> --title "$VERSION" --notes "..." --latest`
5. 验证：`curl -fsSL https://raw.githubusercontent.com/riba2534/feishu-cli/main/install.sh | bash`

### 版本号规则

Major：不兼容变更；Minor：新功能；Patch：Bug 修复。

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

生成将导入飞书的 Markdown 前，必须参考 `skills/feishu-cli-doc-guide/SKILL.md`。核心检查项：

- **Mermaid**：禁止花括号 `{}`（flowchart 标签）、禁止 `par...and...end`、方括号冒号加双引号、sequenceDiagram 参与者 ≤ 8
- **PlantUML**：无行首缩进、无 `skinparam`、类图无可见性标记（`+ - # ~`）
- **表格**：超 9 行自动拆分
- **图片**：默认 `--upload-images` 上传，关闭时创建占位块
- **公式**：行内 `$...$`、块级 `$$...$$`（块级降级为行内）
- **Callout**：仅 6 种（NOTE/WARNING/TIP/CAUTION/IMPORTANT/SUCCESS）
