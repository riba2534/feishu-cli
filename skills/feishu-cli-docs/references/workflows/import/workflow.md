# Markdown 导入

从本地 Markdown 文件创建飞书云文档，或把 Markdown 追加导入到已有文档。Mermaid/PlantUML/SVG 代码块转飞书画板，
大表格保持单 block（行 > 9 追加行；列 > 9 拆分并保留首列）。

> **Owner 规则**：创建后按 [新建文档授权流程](../write/workflow.md#新建文档) 处理用户指定接收人或当前生效的 owner 配置，沿用本次 profile/config；只在 `transfer_ownership=true` 时转移所有权。不要把示例邮箱当成真实接收人。

## 目录

- [适用范围与边界](#适用范围与边界)
- [前置条件与身份](#前置条件与身份)
- [执行流程](#执行流程)
- [参数要点](#参数要点)
- [支持的 Markdown 语法](#支持的-markdown-语法)
- [HTML 扩展标签](#html-扩展标签)
- [输出与结果判定](#输出与结果判定)
- [图表转换与降级](#图表转换与降级)
- [常见问题](#常见问题)

## 适用范围与边界

- 只处理 Markdown 源文本。Word/Excel 等二进制文件导入用 `feishu-cli doc import-file` 或 `feishu-cli-storage` 的 `drive import`。
- 把 Markdown 里的表格变成飞书电子表格时不要走 `doc import`，改用 `feishu-cli sheet import-md report.md --title "报表"`。
- `--document-id` 只会把内容**追加**到已有文档末尾，不会替换；修改已有内容用 `doc content-update`（见 `../write/workflow.md`）。
- 生成 Markdown 前先按 `references/doc-guide.md` 检查语法；Mermaid 细节见 `references/mermaid-spec.md`。

## 前置条件与身份

- 默认 **Bot 身份**，只需 App 凭证，无需 `auth login`。新建文档时 CLI 会自动给当前 CLI 登录用户授予 `full_access`
  （输出 `permission_grant`）；追加到已有文档或以 User 身份执行时不触发。
- `--document-id` 接受 document_id、`/docx/` URL 与 `/wiki/` URL（wiki 自动解析为底层 docx）；Bot 必须对该文档有编辑权限，
  要以本人身份写入用户自己的文档时显式传 `--user-access-token "$(feishu-cli auth token --as user)"`。
- Markdown 必须是合法 UTF-8（非法字节直接拒绝）；合法 UTF-8 中的 `U+FFFD` 替换字符不会被拦截，导入前自查（见 `../write/workflow.md`）。

## 执行流程

### 创建新文档

```bash
feishu-cli doc import ./document.md --title "文档标题" -o json
```

1. 确认文件存在、编码正确，按 doc-guide 检查图表与表格语法。
2. 执行导入；`--title` 缺省时用文件名（去扩展名）。
3. 按 owner 规则授权（仅 owner 已配置时；`transfer_ownership=true` 才转移所有权）。
4. 检查退出码与 `failures`（见「输出与结果判定」），在当前会话返回文档链接与导入统计。只有用户明确要求飞书通知，
   或已有适用的通知授权时，再按指定接收人发送消息。

### 追加导入到已有文档

```bash
feishu-cli doc import ./document.md --document-id <document_id_or_url>
```

转换后的块追加到文档末尾；交付与通知规则同上。

## 参数要点

| 参数 | 说明 | 默认值 |
|---|---|---|
| `--title, -t` | 新文档标题 | 文件名（去扩展名） |
| `--document-id, -d` | 追加导入到已有文档（document_id 或 `/docx/`、`/wiki/` URL） | 新建文档 |
| `--folder, -f` | 新文档的目标文件夹 token | 云空间根目录 |
| `--upload-images` | 上传本地和网络图片；`--upload-images=false` 时图片变为 `[Image: 路径]` 文本占位 | 开启 |
| `--table-column-width` | 列宽策略 `auto` / `fixed` / `N1,N2,...`（`*` 走 auto）；完整规则见 doc-guide 表格章节 | auto |
| `--diagram-workers` / `--table-workers` / `--image-workers` | 图表 / 表格 / 图片并发数（图片受 API 5 QPS 限制） | 5 / 3 / 2 |
| `--diagram-retries` | 图表服务端错误（5xx、限流）的最大重试次数 | 10 |
| `--verbose, -v` | 打印每个图表/表格/图片的进度 | 否 |
| `--output, -o` | `json` 输出统计与失败明细 | 文本 |

## 支持的 Markdown 语法

- 标题（`#` ~ `######`）、段落、分割线、链接
- 无序/有序列表（多级嵌套、有序/无序混合嵌套）、任务列表（`- [ ]` / `- [x]`，可嵌套）；列表项首段是该项正文，
  其后的段落、代码块、引用等（缩进到列表正文列）作为该列表项的子块保留，不会并入首段
- 代码块（带语言标识）
- **Mermaid / PlantUML / SVG 代码块** → 飞书画板（见「图表转换与降级」）
- 引用块（转为 QuoteContainer，内部可含多段落与列表；`>` 空行会生成一个空段落）。飞书不允许引用块嵌套（服务端 1770030），
  嵌套引用 `> >`（含引用内的 Callout）会被**扁平化**进外层引用：内容完整保留，只是不再有缩进层级
- **Callout**：`> [!NOTE]` 等 6 种类型，内部可含段落、列表等子块
- **图片**：默认上传本地（相对 Markdown 文件所在目录）和网络图片；表格单元格内的图片会真正嵌入为单元格内图片；
  与文字混排的行内图片统一转为 `[图片: 说明]` 占位（http(s) 为可点击链接）
- **表格**：行 > 9 用 `insert_table_row` 追加保持单 block；列 > 9 按列组拆分（每组 ≤ 9 列，首列在每组保留）；
  紧邻表格上方的 `<!-- feishu-colwidth: ... -->` 注释可控制列宽；单元格内的粗体/行内代码/链接保留，`<br>` 拆成多段，
  `$...$` 与正文一样转为行内公式
- 粗体、斜体、删除线、行内代码、下划线（`<u>文本</u>`）
- 行内公式 `$...$`（一段可多个）；块级 `$$...$$` 或独立行 `$...$` 导入为只含公式的文本块（飞书无独立块级公式块）

### Callout 背景色映射

```markdown
> [!NOTE]
> 这是一个提示信息。

> [!WARNING]
> 这是一个警告信息。
```

| 类型 | 背景色（飞书 Callout 枚举） |
|---|---|
| NOTE / INFO / 未知类型 | 浅蓝（5） |
| WARNING | 浅红（1） |
| CAUTION | 浅橙（2） |
| TIP | 浅黄（3） |
| SUCCESS | 浅绿（4） |
| IMPORTANT | 浅紫（6） |

（枚举：1 浅红、2 浅橙、3 浅黄、4 浅绿、5 浅蓝、6 浅紫、7 中灰；导入后经 docs_ai 回读实测颜色一致。）

## HTML 扩展标签

导出端生成的扩展标签在导入时会被识别，也可手写用于精确控制块类型。块级标签必须**开标签独占一行、内容另起一行**，
写在同一行（`<callout type="NOTE">内容</callout>`）时只会得到一段普通文字。

| 标签 | 导入结果（除标注外均为 2026-10 实测） |
|---|---|
| `<mention-user id="ou_xxx"/>`、`<mention-doc token="xxx" type="docx">标题</mention-doc>` | 行内 @用户 / @文档（未实测，按源码） |
| 多行 `<callout type="NOTE" color="7">`…`</callout>` | 高亮块；`color` 为背景色枚举原值（1-14），优先于 `type`；内容按纯文本处理 |
| `<whiteboard type="blank"/>`（不带 token） | 新建空白画板 |
| `<sheet rows="5" cols="5"/>` | 新建空电子表格块 |
| `<bitable view="table"/>` | 新建空多维表格块 |
| `<image token="xxx" width height align/>`（及 `![](feishu://media/xxx)`） | 素材复用：建空图片块 → 下载原素材 → 重新上传绑定，保留原显示宽高/对齐；表格单元格里的行内 `<image token/>` 同样嵌入为单元格图片 |
| `<file token="xxx" name="a.pdf"/>`、`<video src="feishu://media/xxx" data-name="a.mp4">` | 附件/视频复用：建空附件块 → 下载原素材 → 以原文件名重新上传 |
| `<video src="./demo.mp4" data-name="demo.mp4"></video>` | 上传本地视频为附件块（文件名取 `data-name`） |
| `<whiteboard token="xxx" type="blank"/>` | 复制源画板：由 Mermaid/PlantUML 生成的画板按节点里保存的源码重新导入（可编辑），其余逐节点复制；都失败时插入源画板图片（`whiteboard_as_image` 计数，不算失败） |
| `<sheet token="xxx" id="..."/>`、`<bitable token="xxx"/>` | 无法把已有表格挂进新文档（服务端 1770001）：降级为指向原表格的链接文本，计入 `failures`（kind=`sheet` / `bitable`） |
| `<grid cols="2">` + `<column>` | 分栏：列由服务端按列数自动生成，各列内容写入对应列；列内可含多段落、列表、表格、图片；列数按 `<column>` 实际数量取 2-5，超过 5 列的内容并入最后一列 |

素材复用需要当前身份能读到源文档的素材：下载失败（素材已删除、Bot 不是源文档协作者等）时该块降级为
`[图片未能复用: feishu://media/…]` / `[附件未能复用: …]` 占位文本并计入 `failures`，其余内容照常导入。

**导出再导入（roundtrip）**，两条路径均已实测（2026-10，覆盖全部块类型的测试文档）：

- 不带 `--download-images`：`doc export -o a.md` → `doc import a.md`。图片/附件/视频按 token 复用素材，画板按源码重建或复制节点，
  仍是可编辑画板；两轮导出除 token 值外逐行一致。要求导入身份（默认 Bot）能读源文档素材，读不到时按上文降级为占位文本。
- 带 `--download-images`：`doc export --download-images -o out/a.md` → `doc import out/a.md`。`-o` 写文件时资源引用路径相对
  Markdown 文件所在目录（`--assets-dir` 可以是任意相对/绝对路径），原地导入即可找到资源；画板以图片形式导入（其余内容一致）。

要写回已有文档用 `doc content-update`（会把 `<image token>` / `<file token>` 转换为 docs_ai 写法，画板占位则拒绝写回，
见 `../write/workflow.md`）。

## 输出与结果判定

文本模式（stderr 打印进度，stdout 打印汇总）：

```text
导入完成!
  文档ID: <document_id>
  添加块数: 25
  表格: 4/4 成功
  图表: 3/3 成功 (Mermaid: 2, PlantUML: 1)
  总耗时: 11.9s
  链接: https://www.feishu.cn/docx/<document_id>   # Lark 品牌为 www.larksuite.com
  当前用户权限: 已授予 full_access（ou_xxx）
```

`-o json` 输出 `document_id`、`url`、`blocks`、`blocks_failed`、`image_*` / `video_*` / `file_*` / `whiteboard_*` /
`table_*` / `diagram_*` / `cell_image_*` 统计、`diagram_fallback`、`permission_grant`、`partial_failure` 与 `failures`。

- **部分失败以退出码 1 结束**：图片/视频/附件/表格/单元格图片写入失败、素材复用或画板复制失败、图表导入失败且降级为代码块也失败、
  嵌套子块创建失败、带 token 的 `<sheet>`/`<bitable>` 降级为链接时，命令仍输出文档链接与统计，文本模式在 stderr 列出失败明细，
  JSON 输出 `partial_failure: true` 与 `failures`（每项含 `kind`/`index`/`source`/`error`；kind 取值 `blocks`/`image`/`video`/`file`/
  `whiteboard`/`sheet`/`bitable`/`table`/`cell_image`/`diagram`/`nested_blocks`）。按 `failures` 用 `doc media-insert` 或
  `doc content-update` 补齐，**不要重新导入整篇**（会产生重复文档）。
- 图表降级为代码块、画板降级为图片属于设计内降级，不计入 `failures`、退出码仍为 0；看 `diagram_fallback` / `whiteboard_as_image` 判断。
- 建块被服务端拒绝的单个块（如参数越界）会被隔离跳过，计入 `failures`（kind=`blocks`，`index` 为段落序号），其余内容照常写入；
  只有权限不足、文档不存在、网络重试耗尽等致命错误才停止后续建块。无论哪种情况都会输出结果（`-o json` 同样输出 JSON）并以退出码 1 结束。
- 写入均为本次新建的块，建块与画板写入的重试会复用幂等 token 或回读确认，不会因 5xx 重放产生重复块/重复图。

## 图表转换与降级

导入分三阶段：顺序创建所有块（收集图表、表格、图片任务）→ 并发导入图表、填充表格、上传图片 → 对失败图表删除空画板、在原位置插入代码块。

- 代码块标识：` ```mermaid `、` ```plantuml ` / ` ```puml `、` ```svg `（围栏必须恰好三个反引号）。
- Mermaid 当前可渲染：flowchart / graph、sequenceDiagram、classDiagram、stateDiagram / stateDiagram-v2、erDiagram、gantt、pie、
  mindmap、timeline、quadrantChart、xychart（服务端报错信息列出的支持集合，2026-10 实测）；`journey`、`gitGraph` 等不支持 → 降级为代码块。
- SVG 代码块整体作为一个 svg 节点写入画板（不拆成可编辑节点）。
- 只有服务端错误（5xx）和限流会重试（`--diagram-retries`，默认 10，指数退避）；语法错误、不支持的图表类型等 4xx（如 `code=2890002`）
  不重试，直接降级为代码块。
- 语法限制与复杂度建议见 `references/doc-guide.md` 与 `references/mermaid-spec.md`。

## 常见问题

| 现象 | 原因 | 解决方式 |
|---|---|---|
| 认证失败 / Token 无效 | App 凭证错误，或显式传入的 User Token 失效 | 检查 App 凭证；User 身份按 feishu-cli-platform 指引重新登录 |
| 图表降级为代码块 | 图表类型不受支持、语法错误或服务端持续报错 | 按 doc-guide / mermaid-spec 调整后，用 `content-update` 替换该代码块，或改用 `board import` 单独导入 |
| 超长表格导入耗时显著 | 行 > 9 时逐行追加到同一 block，受单文档写入限流（约 3 次/秒） | 正常行为；200+ 行的数据改用 Sheet |
| 表格被拆分为多个 block | 列 > 9 时按列组拆分 | 正常行为，首列在每组保留以便对照 |
| 图片上传失败 | 网络不通、图片 URL 不可访问或本地路径不存在 | 命令以退出码 1 结束并在 `failures` 中给出明细；修正后用 `doc media-insert` 补图 |
| `<callout>` / `<grid>` 变成普通文字 | 块级标签与内容写在同一行 | 开标签独占一行、内容另起一行 |
| 出现 `[图片未能复用: feishu://media/…]` 占位 | 导入身份读不到源文档素材（素材已删除或无权限） | 改用 `doc export --download-images` 落地资源后再导入，或用有源文档权限的身份 `--user-access-token` 导入 |
| 文档创建成功但用户无法编辑 | 未按 owner 配置授权（Bot 自动授权只覆盖当前 CLI 登录用户） | 按 [新建文档授权流程](../write/workflow.md#新建文档) 读取 `owner_email` 后 `perm add`；仅 `transfer_ownership=true` 时转移所有权 |
