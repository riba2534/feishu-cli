# 飞书文档阅读

从飞书云文档、知识库或电子表格读取内容（转为 Markdown/XML）后分析和展示，不主动把结果交付为本地文件（落盘交付走 `../export/workflow.md`）。

## 目录

- [身份与前置](#身份与前置)
- [选择读取方式](#选择读取方式)
- [大文档选择性读取（doc read）](#大文档选择性读取doc-read)
- [docs_ai 引擎：带 block id 读取](#docs_ai-引擎带-block-id-读取)
- [整篇读取与图片](#整篇读取与图片)
- [文档元信息与块结构](#文档元信息与块结构)
- [知识库与电子表格](#知识库与电子表格)
- [导出格式说明](#导出格式说明)
- [Wiki 目录节点](#wiki-目录节点)
- [错误处理](#错误处理)

## 身份与前置

- 读类命令（`doc read/export/get/blocks`、`wiki export/get/nodes`、`sheet export/read`）统一 **User 优先、Bot 兜底**：
  显式 `--user-access-token` → `FEISHU_USER_ACCESS_TOKEN` → `auth login` 保存的 token（过期自动刷新）→ config 中的
  `user_access_token` → App Token。读他人文档只需 `auth login` 一次，后续无需再传 token。
- User Token 损坏或刷新失败时会在 stderr 告警后改用 Bot（stdout 不受影响）；看到告警说明已不是用户身份，Bot 读不到的文档会报无权限。
- 所需 scope：普通文档 `docx:document:readonly`，知识库 `wiki:wiki:readonly`；展开 @用户需要 `contact:user.base:readonly`。

## 选择读取方式

| 场景 | 命令 |
|---|---|
| 普通大小的 docx，读完总结 | `doc export <doc> -o /tmp/x.md`，再用 Read 读取 |
| 大文档（几百块以上）或只关心某一节 | 先 `doc read <doc> --outline`，再 `--heading` / `--keyword` 取局部 |
| 要拿 block id 以便后续精确修改 | `doc read <doc> --with-ids [--heading "章节"]` |
| 知识库节点 | 底层是 docx：`doc read` / `doc export` 直接传 wiki URL；底层是 sheet 或需整节点导出：`wiki export` |
| 普通电子表格 | `sheet export <token_or_url> --format markdown -o /tmp/x.md` |
| 分析块类型、查原始 API 结构 | `doc blocks <document_id> --all` |

URL 判断：`/docx/<id>` 是 document_id；`/wiki/<token>` 是 node_token（不是 document_id）；`/sheets/<token>` 是 spreadsheet_token。
`doc read`、`doc export`（以及写入侧 `content-update`、`media-insert`、`doc history`）直接接受 docx token、`/docx/` URL 与
`/wiki/` URL：wiki 节点自动解析为底层 docx（stderr 提示"已将 wiki 节点 … 解析为 docx: …"），底层不是 docx 时报错。
URL 只按路径前缀识别，`?from=/wiki/...` 这类查询参数不改变解析结果。`doc get`、`doc blocks` 只接受裸 document_id。

## 大文档选择性读取（doc read）

**大文档不要整篇 export**——先看结构再取所需部分，节省上下文：

```bash
# 第一步：看标题大纲（层级缩进 + block_id）
feishu-cli doc read <document_id_or_url> --outline

# 第二步：只取目标章节（按标题子串匹配，输出到下一个同级/更高级标题前的 Markdown）
feishu-cli doc read <document_id_or_url> --heading "性能优化"

# 或直接按内容定位（正则，多词用 | 连接；--context 控制上下文行数，默认 3）
feishu-cli doc read <document_id_or_url> --keyword "QPS|限流" --context 5
```

- 本地引擎（默认）必须且只能选一种模式；三者都不传或同时传多个时以退出码 2 报错（读全文用 `doc export`，或改用 docs_ai 引擎）。
- `--heading` 命中多个标题时输出第一个并在 stderr 提示其余候选；代码块围栏内的 `#` 行不会被误判为标题。

## docs_ai 引擎：带 block id 读取

使用 `--with-ids`、`--scope`、`--detail`、`--doc-format`、`--start-block-id`、`--end-block-id`、`--context-before/after`、
`--max-depth`、`--revision-id`、`-o json` 任一项（或显式 `--engine docs_ai`）时改走服务端 docs_ai 读取；显式 `--engine local`
再用这些 flag 会以退出码 2 报错。docs_ai 引擎不传范围时读取全文。

```bash
# 带 block id 读取某一节（--heading 自动换成标题块 ID，scope=section），输出 XML
feishu-cli doc read <doc> --with-ids --heading "性能优化"

# 关键词定位：docs_ai 的 keyword 支持 a|b；上下文按"兄弟块数"计
feishu-cli doc read <doc> --with-ids --scope keyword --keyword "QPS|限流" --context-before 1 --context-after 1

# 按 block id 区间读取（-1 表示到文末）；服务端大纲（--max-depth 限制标题层级）
feishu-cli doc read <doc> --with-ids --start-block-id <A> --end-block-id -1
feishu-cli doc read <doc> --engine docs_ai --outline --max-depth 2

# 服务端 Markdown 全文；-o json 输出完整响应（含评论、引用表、revision_id）
feishu-cli doc read <doc> --engine docs_ai --doc-format markdown
feishu-cli doc read <doc> --with-ids -o json
```

- `--with-ids` 等价 `--detail with-ids`；`--detail with-ids|full` 只能输出 XML（默认 xml，配 `--doc-format markdown` 以退出码 2 报错），
  Markdown 无法携带 block id；`--detail full` 额外带样式属性与引用元数据。
- `--scope full|outline|range|keyword|section` 也可由 `--outline` / `--heading` / `--keyword` / `--start-block-id` 推断，显式值与推断冲突时报错。
- 文本级替换匹配不到时，用 `--engine docs_ai --doc-format xml`（或 markdown）查看服务端序列化原文。

典型闭环：`doc read --with-ids --heading "章节"` 拿到 block id → `doc content-update --mode replace_range --block-id <id>`
精确改写 → 再次 `doc read --with-ids` 验证（写操作后被改写的块会换新 ID，必须重新读取）。

## 整篇读取与图片

```bash
# 普通文档（含 /docx/ URL 与底层为 docx 的 /wiki/ URL）
feishu-cli doc export <document_id_or_url> --output /tmp/feishu_doc.md --download-images --assets-dir /tmp/feishu_assets

# 知识库节点（docx 或 sheet）
feishu-cli wiki export <node_token_or_url> --output /tmp/feishu_wiki.md --download-images --assets-dir /tmp/feishu_assets

# 普通电子表格（不指定 --sheet-id 时读取所有可见工作表）
feishu-cli sheet export <spreadsheet_token_or_url> --format markdown --output /tmp/feishu_sheet.md
```

1. 显式传 `/tmp` 下的输出路径，避免把中间文件留在项目目录（`doc export` 不传路径时打印到 stdout）。
2. 用 Read 工具读取导出的 Markdown，分析结构和内容。
3. 有图片时检查 `--assets-dir` 目录，用 Read 工具逐个查看图片并把内容整合到分析中。
4. 报告：标题、结构概要（标题层级）、内容摘要、图片内容描述；说明中间文件路径。

- `--download-images` 只属于 `doc export` / `wiki export`；`sheet export` 没有该 flag，电子表格内图片需用 sheet 图片命令或导出 XLSX 后处理。
- 内嵌电子表格块默认展开为 Markdown 表格；要保留 `<sheet .../>` 引用时加 `--expand-sheets=false`。
- 其他 `doc export` 参数（`--front-matter`、`--highlight`、`--expand-mentions`、`--engine docs_ai`）见 `../export/workflow.md`。

## 文档元信息与块结构

```bash
# 元信息（document_id、title、revision_id、链接）；只接受裸 document_id
feishu-cli doc get <document_id> -o json

# 块结构：默认第一页（500 块），--all 自动分页；--raw 输出 API 原始 JSON
feishu-cli doc blocks <document_id> --all -o json
feishu-cli doc blocks <document_id> --all --raw > /tmp/blocks_raw.json
```

`doc get` / `doc blocks` 传 URL 会失败：从 `/docx/<id>` 截取 ID；wiki 节点先 `feishu-cli wiki get <url> -o json` 取 `obj_token`，
或直接用接受 URL 的 `doc read` / `doc export`。

## 知识库与电子表格

知识库目录遍历与电子表格单元格读取分别由 `feishu-cli-storage`（wiki 工作流）和 `feishu-cli-data`（sheet 工作流）维护，
完整参数以对应工作流和 `--help` 为准。阅读场景的常用链路：

```bash
# 知识库：查节点（返回 space_id、obj_token、obj_type、has_child）→ 列子节点 → 导出目标节点
feishu-cli wiki get https://xxx.feishu.cn/wiki/<node_token>
feishu-cli wiki nodes <space_id> --parent <node_token> --page-all
feishu-cli wiki export <child_node_token> -o /tmp/child.md

# 电子表格：先列工作表拿 sheet_id，再按范围读单元格
feishu-cli sheet list-sheets <spreadsheet_token_or_url>
feishu-cli sheet read <spreadsheet_token_or_url> "<sheet_id>!A1:C10"
feishu-cli sheet read <spreadsheet_token_or_url> "A1:B20" --sheet-name Sheet1 --value-render Formula
```

## 导出格式说明

本地引擎导出的 Markdown 对飞书特有块的表示：

| 飞书块类型 | Markdown 表现 |
|---|---|
| Callout 高亮块 | `> [!NOTE]`、`> [!WARNING]` 等 6 种 GitHub-style alert（按背景色还原类型） |
| 块级/行内公式 | `$formula$`（LaTeX 原文） |
| 画板 (Board) | `--download-images` 时为图片引用；否则 `<whiteboard token="..." type="blank"/>` 占位 |
| 图片 | `--download-images` 时为本地图片引用；否则 `<image token="..." .../>` |
| 电子表格块 (Sheet) | 默认展开为 Markdown 表格；`--expand-sheets=false` 时为 `<sheet .../>` |
| 小组件 (AddOns) | 文本绘图组件输出 ```` ```mermaid ```` / ```` ```plantuml ```` 源码；其余为 `[小组件 ...]` 占位 |
| ISV 文本绘图 / 时间线 | 带注释的 ```` ```mermaid ```` 占位（Open API 不暴露源码） |
| QuoteContainer | `>` 引用语法 |
| 同步块 | 展开子块内容（跨文档引用读取源文档，失败时输出 `WARNING` 占位） |
| Iframe | `<iframe>` HTML 标签 |
| 无法表达的块 | `<!-- 不支持的块类型: 名称 (type=N) -->` 注释 |

使用 `--highlight` 时，带颜色的文本输出为 `<span style="color:...">`。表格合并单元格等已知限制见 `../export/workflow.md`。

## Wiki 目录节点

导出内容显示为下面这行时，说明该节点是知识库目录块，正文就是子节点列表：

```markdown
[Wiki 目录 - 使用 'wiki nodes <space_id> --parent <node_token>' 获取子节点列表]
```

```bash
feishu-cli wiki get <node_token>                               # 记录 space_id、has_child
feishu-cli wiki nodes <space_id> --parent <node_token> --page-all
feishu-cli wiki export <child_node_token> -o /tmp/child.md     # 逐个导出子节点
```

## 错误处理

| 错误 | 原因 | 处理 |
|---|---|---|
| `code=1770032, msg=forBidden` | 当前身份（常见为 Bot）无权读取该文档 | 确认 stderr 是否有 User Token 回退告警；`auth login` 后以 User 身份读取，或请文档所有者授权 |
| `code=1770002, msg=not found` | document_id 不存在或传错（如把 wiki node_token 当 document_id 给 `doc get/blocks`） | 核对 token；wiki 链接改用 `doc read/export` 或先 `wiki get` 取 `obj_token` |
| `code=99991679` / `99991672` 等 scope 错误（退出码 3） | User 未授权或应用未开通所需 scope | 按 feishu-cli-platform 的身份指引预检 scope 后补授 |
| `code=131006` | 当前身份无权读取该知识库节点 | Bot 需被加为知识空间成员或协作者；或 `auth login` 后用 User 身份 |
| `code=131012` | 知识库节点已删除或不存在 | 重新获取有效链接，不要重试同一 token |
| `code=131013` / `131016` | token 无效或被截断 | 检查 URL/token 是否完整 |
| `code=131014` | 文档不在知识库中 | 普通云文档直接用 `/docx/` 链接或 document_id |
| 内容为空或只有目录行 | 目录节点或空文档 | 见「Wiki 目录节点」 |

- 图片下载失败：检查 `--assets-dir` 是否可写、网络是否可达；图片可能已删除。
- 网络错误或限流：命令已内置限流重试；仍失败时加 `--debug` 查看请求详情，稍后重试。
- 认证失败：`feishu-cli auth status` 查看状态，过期按 feishu-cli-platform 指引重新登录。
