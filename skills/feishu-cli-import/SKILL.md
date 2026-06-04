---
name: feishu-cli-import
description: >-
  从 Markdown 文件导入创建飞书文档。支持嵌套列表、Mermaid/PlantUML 图表自动转画板、
  大表格智能处理（超过 9 行用 insert_table_row API 追加保持单 block，超过 9 列拆分保留首列）、公式、Callout 高亮块。
  内置飞书侧渲染规范（`references/doc-guide.md` + `references/mermaid-spec.md`），
  覆盖图表花括号/par 等语法限制、表格 9 行/列阈值、Callout 6 种颜色映射等飞书兼容性规则。
  当用户请求"导入 Markdown"、"从 md 创建文档"、"从 md 文件创建文档"、"把 Markdown 转换到飞书"、
  "上传 Markdown"、"Markdown 转飞书"、"md 导入"、"批量导入"，或排查"飞书 Mermaid 渲染失败"、
  "Callout 颜色不对"、"飞书表格被拆"、"飞书文档导入兼容性"等问题时使用。
  注意：仅支持 Markdown 源文件导入为飞书文档。Markdown 表格创建电子表格请用 feishu-cli-toolkit 的 `sheet import-md`；
  DOCX/XLSX 导入为云文档请使用 feishu-cli-drive 的 drive import。
argument-hint: <markdown_file> [--title "标题"] [--verbose]
user-invocable: true
allowed-tools: Bash(feishu-cli doc:*), Bash(feishu-cli perm:*), Bash(feishu-cli sheet:*), Read
---

# Markdown 导入技能

从本地 Markdown 文件创建飞书云文档，或把 Markdown 追加导入到已有文档。**支持 Mermaid/PlantUML 图表转飞书画板、大表格智能处理（行 > 9 单 block API 追加；列 > 9 拆分保留首列）**。

> **Owner 规则**：创建新文档后如需交付给用户，先解析 `FEISHU_OWNER_EMAIL` 或配置文件 `owner_email`。只在解析到 owner 时授予 `full_access`；只在 `transfer_ownership=true` 时转移所有权。不要把示例邮箱当成真实接收人。

## 核心特性

1. **三阶段并发管道**：顺序创建块 → 并发处理图表/表格 → 失败回退
2. **Mermaid/PlantUML → 飞书画板**：`mermaid`/`plantuml`/`puml` 代码块自动转换为飞书画板
3. **图表故障容错**：语法错误自动降级为代码块展示，服务端错误自动重试（默认最多 10 次，可用 `--diagram-retries` 调整）
4. **大表格智能处理**：行 > 9 时创建 9 行初始表 + `insert_table_row` API 追加到同一 block（视觉连贯，每行约 1 次 API 往返；verbose 模式 ≥ 5 行打印进度）；列 > 9 按列组拆分保留首列作为标识。**单元格内容填充已走 `batch_update` 批量加速（v1.29+，#159）**：阶段二预热 `cellMap` 后按批（single-group cell 每批 ≤ 30 个）一次性写入，失败再降级为 per-cell；典型 4×6×8 表从 ~70s 降到 ~3s（25-30x）
5. **表格列宽**：默认按内容启发式（中文 14px / 英文 8px，最小 80px，最大 400px）。可通过紧邻表格上方注释 `<!-- feishu-colwidth: 80,200,*,30% -->`（单位 px / 百分比 / `*` 走 auto）或 CLI flag `--table-column-width=auto|fixed|N1,N2,...` 全局覆盖；注释优先级高于 flag
6. **API 限流自动重试**：画板创建和图表导入遇到 HTTP 429 时自动重试，读取服务端 `x-ogw-ratelimit-reset` 响应头精确计算退避时间，采用指数退避策略，默认最多重试 10 次
7. **并发控制**：图表和表格分别使用独立的 worker 池（默认图表 5、表格 3 并发）

## 核心概念

**Markdown 作为中间态**：本地文档与飞书云文档之间通过 Markdown 格式进行转换。

> **边界澄清**：本 skill 仅支持 Markdown 源文本导入。若需导入 Word/Excel 等二进制格式，请用 `feishu-cli doc import-file` 或 `feishu-cli-drive` skill 的 `drive import`。

如果用户目标是“把 Markdown 里的表格变成飞书电子表格”，不要走 `doc import`，改用：

```bash
feishu-cli sheet import-md report.md --title "报表"
```

## 前置条件

- **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式
- 已配置 App Token（`FEISHU_APP_ID` + `FEISHU_APP_SECRET`），无需 `auth login`
- Markdown 文件使用 UTF-8 编码（CLI 会用 `utf8.Valid` 拒收非法 UTF-8 字节序列；合法 UTF-8 中包含的 `U+FFFD` 替换字符不会被拦截，建议先在编辑器里搜一遍）

## 使用方法

```bash
# 创建新文档
feishu-cli doc import ./document.md --title "文档标题"

# 追加导入到已有文档（不会原地替换）
feishu-cli doc import ./document.md --document-id <existing_doc_id>

# 上传本地图片
feishu-cli doc import ./document.md --title "带图文档" --upload-images
```

## 执行流程

### 创建新文档

1. **验证文件**
   - 检查 Markdown 文件是否存在
   - 预览文件内容
   - **编码验证**：CLI 内置 `utf8.Valid` 校验，遇到非法 UTF-8 字节直接拒绝导入；合法 UTF-8 里残留的 `U+FFFD` 替换字符不会被拦截，建议导入前先用编辑器全局搜一遍 `�`

2. **执行导入**
   ```bash
   feishu-cli doc import <file.md> --title "<title>" [--upload-images]
   ```

3. **添加权限**（仅 owner 已配置时）
   ```bash
   feishu-cli perm add <document_id> --doc-type docx --member-type email --member-id <owner_email> --perm full_access --notification
   ```

4. **转移文档所有权**（仅 `transfer_ownership=true` 时）
   ```bash
   feishu-cli perm transfer-owner <document_id> --doc-type docx --member-type email --member-id <owner_email> --notification
   ```

5. **发送通知**
   发送飞书消息通知用户文档已创建

### 追加导入到已有文档

`--document-id` 不会替换已有内容，只会把 Markdown 转换后的块追加到文档末尾。修改/覆盖已有内容请用 `feishu-cli doc content-update`（详见 `feishu-cli-write` skill）。

1. **执行追加导入**
   ```bash
   feishu-cli doc import <file.md> --document-id <doc_id> [--upload-images]
   ```

2. **通知用户**

## 参数说明

| 参数 | 说明 | 默认值 |
|------|------|--------|
| markdown_file | Markdown 文件路径 | 必需 |
| --title | 新文档标题 | 文件名 |
| --document-id | 追加导入到已有文档 | 创建新文档 |
| --upload-images | 上传本地和网络图片到飞书 | 是（默认开启） |
| --image-workers | 图片并发上传数 | 2（API 限制 5 QPS） |
| --folder, -f | 新文档的目标文件夹 Token | 根目录 |
| --diagram-workers | 图表 (Mermaid/PlantUML) 并发导入数 | 5 |
| --table-workers | 表格并发填充数 | 3 |
| --table-column-width | 列宽策略：`auto` / `fixed` / `N1,N2,...`（像素列表，`*` 走 auto） | auto |
| --diagram-retries | 图表最大重试次数 | 10 |
| --verbose | 显示详细进度信息 | 否 |

## 支持的 Markdown 语法

- 标题（# ~ ######）
- 段落文本
- 无序/有序列表（支持无限深度嵌套、混合嵌套）
- 任务列表（- [ ] / - [x]）
- 代码块（带语言标识）
- **Mermaid/PlantUML 图表** → 自动转换为飞书画板
- **引用块**（支持嵌套引用，自动转换为 QuoteContainer）
- **Callout 高亮块**（`> [!NOTE]`、`> [!WARNING]` 等 6 种类型）
- 分割线
- **图片**（默认通过 `--upload-images` 自动上传本地和网络图片；无此参数时创建占位块；内联图片转为链接或文本占位符）
- **表格**（行 > 9 用 `insert_table_row` API 追加保持单 block；列 > 9 按列组拆分保留首列）
- 粗体、斜体、删除线、行内代码、**下划线**（`<u>文本</u>`）
- 链接
- **行内公式**（`$E = mc^2$`，支持一段中多个公式）
- **块级公式**（`$$formula$$` 或独立行 `$formula$`）

### 图表示例（推荐使用 Mermaid）

````markdown
```mermaid
flowchart TD
    A[开始] --> B{判断}
    B -->|是| C[处理]
    B -->|否| D[结束]
```
````

````markdown
```plantuml
@startuml
Alice -> Bob: Hello
Bob --> Alice: Hi
@enduml
```
````

支持的 Mermaid 图表类型（全部已验证）：
- ✅ flowchart（流程图，支持 subgraph 嵌套）
- ✅ sequenceDiagram（时序图）
- ✅ classDiagram（类图）
- ✅ stateDiagram-v2（状态图）
- ✅ erDiagram（ER 图）
- ✅ gantt（甘特图）
- ✅ pie（饼图）
- ✅ mindmap（思维导图）

### Callout 高亮块示例

````markdown
> [!NOTE]
> 这是一个提示信息。

> [!WARNING]
> 这是一个警告信息。

> [!TIP]
> 这是一个技巧提示。

> [!CAUTION]
> 这是一个警示。

> [!IMPORTANT]
> 这是一个重要信息。

> [!SUCCESS]
> 这是一个成功信息。
````

Callout 内部支持子块（段落、列表等），自动创建为 Callout 的子块。

背景色映射：

| 类型 | 背景色 |
|------|--------|
| NOTE/INFO | 蓝色 (6) |
| WARNING | 红色 (2) |
| TIP | 黄色 (4) |
| CAUTION | 橙色 (3) |
| IMPORTANT | 紫色 (7) |
| SUCCESS | 绿色 (5) |

### 公式示例

````markdown
行内公式：爱因斯坦质能方程 $E = mc^2$ 是最著名的公式。

块级公式（独立行）：
$\int_{0}^{\infty} e^{-x^2} dx = \frac{\sqrt{\pi}}{2}$
````

- 行内公式支持一段内多个 `$...$` 公式
- 块级公式在飞书中创建为 Text 块内的 Equation 元素
- 公式内容保持 LaTeX 原文

### 下划线示例

```markdown
这段文本包含 <u>下划线</u> 样式。
```

## 输出格式

```
已导入文档！
  文档 ID: <document_id>
  文档链接: https://feishu.cn/docx/<document_id>
  导入块数: 25
```

## 示例

```bash
# 创建新文档
feishu-cli doc import ./meeting-notes.md --title "会议纪要"

# 追加导入到现有文档
feishu-cli doc import ./updated-spec.md --document-id <document_id>

# 带图片导入（自动上传本地和网络图片）
feishu-cli doc import ./blog-post.md --title "博客文章" --upload-images

# 批量导入（一次循环多个 Markdown）
for f in *.md; do feishu-cli doc import "$f" --title "${f%.md}" --upload-images; done
```

## 已验证功能

上述"支持的 Markdown 语法"中列出的所有语法均已通过测试验证，全部正常工作。表格/图片/视频的具体处理细节见上方"核心特性"和"支持的 Markdown 语法"小节，下面只列大规模测试结果。

### 大规模测试结果

已验证可成功导入的大型文档：
- **10,000+ 行 Markdown** ✓
- **127 个 Mermaid 图表** → 全部成功转换为飞书画板 ✓
- **170+ 个表格**（含 17 行 × 5 列单 block 连贯追加、9 列以上列拆分、列宽自动计算）→ 全部成功 ✓
- **8 种图表类型** → flowchart/sequenceDiagram/classDiagram/stateDiagram/erDiagram/gantt/pie/mindmap 全部成功 ✓
- **88 个 Mermaid 图表逐个测试** → 82/88 成功，6 个失败（3 个服务端瞬时错误 + 2 个花括号语法 + 1 个提取异常）

### 三阶段并发管道架构

1. **阶段一（顺序）**：创建所有文档块，收集图表（Mermaid/PlantUML）和表格任务
2. **阶段二（并发）**：使用 worker 池并发处理图表导入和表格填充。表格单元格填充先预热 `cellMap`，再走 `batch_update` API 批量写入（single-group cell 每批 ≤ 30 个），仅在批量失败时降级为 per-cell；行 > 9 的 `insert_table_row` 追加仍逐行串行（受单文档 3 QPS 节流）
3. **阶段三（逆序）**：处理失败的图表 → 删除空画板块，插入代码块作为降级展示

### Mermaid 已知限制

| 限制 | 说明 | 处理方式 |
|------|------|----------|
| `{}` 花括号 | Mermaid 解析器将 `{text}` 识别为菱形节点 | 自动降级为代码块 |
| `par...and...end` | 飞书解析器完全不支持 par 并行语法 | 用 `Note over X: 并行执行` 替代 |
| 渲染复杂度组合超限 | 单一因素不会触发，但 10+ participant + 2+ alt 块 + 30+ 长消息标签组合时服务端返回 500 | 重试后降级为代码块 |
| 服务端瞬时错误 | 偶发 HTTP 500（并发压力导致） | 自动重试（默认最多 10 次，指数退避） |
| Parse error 不重试 | 语法错误直接降级 | 自动降级为代码块 |

**渲染复杂度安全阈值**（二分法实测）：
- participant ≤8 或 alt ≤1 或消息标签简短 → 安全
- 10 participant + 2 alt + 30 条长消息标签 → 超限
- 建议：sequenceDiagram 保持 participant ≤8、alt ≤1、消息标签简短

### 技术说明

图表通过飞书画板 API 导入：
- API 端点：`/open-apis/board/v1/whiteboards/{id}/nodes/plantuml`
- `syntax_type=1` 表示 PlantUML 语法，`syntax_type=2` 表示 Mermaid 语法
- `diagram_type` 使用整数（0=auto, 6=flowchart 等）
- 重试策略：指数退避 + 读取 `x-ogw-ratelimit-reset` 响应头精确退避，默认最多 10 次；Parse error 和 Invalid request parameter 不重试
- 失败回退：删除空画板块，在原位置插入代码块
- 支持的代码块标识：` ```mermaid `、` ```plantuml `、` ```puml `

### HTML 标签扩展语法

除标准 Markdown 语法外，导入时还识别以下 HTML 标签形式的扩展语法。这些标签由导出端自动生成，支持 roundtrip（导出→导入不丢失信息）。

| 标签 | 说明 | 示例 |
|------|------|------|
| `<mention-user id="ou_xxx"/>` | @用户 | 创建 MentionUser 元素 |
| `<mention-doc token="xxx" type="docx">标题</mention-doc>` | @文档 | 创建 MentionDoc 元素 |
| `<grid cols="2"><column>...</column><column>...</column></grid>` | 分栏布局 | 创建 Grid Block + GridColumn 子块 |
| `<callout type="NOTE">内容</callout>` | 高亮块（HTML 标签形式） | 与 `> [!NOTE]` 等效 |
| `<whiteboard type="blank"/>` | 空白画板 | 创建 Board Block |
| `<sheet rows="5" cols="5"/>` | 电子表格 | 创建 Sheet Block |
| `<bitable view="table"/>` | 多维表格 | 创建 Bitable Block |
| `<image token="xxx" width="800" align="center" caption="说明"/>` | 带属性图片 | 创建 Image Block，保留尺寸/对齐 |
| `<file token="xxx" name="report.pdf" view-type="1"/>` | 文件块 | 创建 File Block |
| `<video src="./demo.mp4" data-name="demo.mp4" data-view-type="1"></video>` | 视频块（v1.22+） | 创建 File Block (type=23)，识别 mp4/mov/avi/mkv 等扩展名作为视频；`src` 为本地路径或上传后的 token；单文件 ≤ 20MB 直传 |

这些标签主要用于 roundtrip 场景（导出后重新导入），也可手动编写用于精确控制飞书块类型。

**视频导入并发**：与图片共用 worker 池（默认 2 并发，受 API 5 QPS 限制），导入统计含 `video_total/success/failed/skipped`。verbose 模式打印每个视频的上传进度。

## 常见问题

| 现象 | 原因 | 解决方式 |
|------|------|----------|
| 认证失败 / Token 过期 | 未登录或 Token 已失效 | 执行 `feishu-cli auth login` 重新认证（Device Flow，自动注入 offline_access） |
| 图表降级为代码块 | Mermaid/PlantUML 语法不兼容飞书渲染引擎 | 参考 `references/doc-guide.md` 调整语法（禁花括号、禁 par 等） |
| 超长表格导入耗时显著 | 单元格内容填充已走 `batch_update` 批量加速（v1.29+，~25-30x），主要耗时来自行 > 9 时 `insert_table_row` API **逐行串行追加**到同一 block（受单文档 3 QPS 节流，每行约 1 次 API 往返） | 属于正常行为；verbose 模式每 5 行打印进度。行数极多（200+）时建议改用电子表格（Sheet）承载 |
| 表格被拆分为多个 block | 列 > 9 时 CLI 按列组拆分（每组 ≤ 9 列），首列作为标识在所有组中保留 | 属于正常行为，避免拆分后行无法识别 |
| 图片上传失败 | 网络不通或图片 URL 不可访问 | 检查网络连通性；失败的图片会自动创建占位块，不影响整体导入 |
| 文档创建成功但无法编辑 | 未按 owner 配置添加权限 | 设置 `FEISHU_OWNER_EMAIL` 后执行 `perm add`；仅当 `transfer_ownership=true` 时再执行 `perm transfer-owner` |
