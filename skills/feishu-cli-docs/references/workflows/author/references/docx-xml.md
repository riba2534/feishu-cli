<!-- 内容改编自 larksuite/cli（MIT License, Copyright (c) 2026 Lark Technologies Pte. Ltd.） -->
# 飞书 DocxXML 写作规范

`doc create --doc-format xml`、`doc content-update --doc-format xml` 与 `doc script --command parse` 使用这套 XML。
语法采用类 HTML 标签，渲染为纵向块级文档流：顶层块按文档顺序纵向排列，块内支持富文本和子块嵌套。
默认正文宽度约 820 px，宽版模式约 1020 px。

以下示例需替换示例值。属性必须写成 `name="value"`，禁止省略引号。标注"实测"的写法已用 `doc create` / `content-update`
写入测试文档并回读确认；未标注的写法来自官方规范，使用前先在测试文档上验证。

## 常用标签

- `p, h1-h9, blockquote, hr, img, b, em, u, del, br, span` 语义不变（实测）。普通文档只用 `h1-h6`，`h7-h9` 仅在确需更深层级时使用。
- `<a type="url-preview" href="URL">链接标题</a>`：链接（实测，回读时只保留 `href`）。
- `<latex>E = mc^2</latex>`：行内公式，也用于上标、下标（实测）。
- `<ol><li>第一项<ul><li>子项</li></ul></li><li>第二项</li></ol>`：子列表放在 `<li>` 内；新增列表项必须放在 `<ul>` 或 `<ol>` 内（实测）。
- `<pre lang="go" caption="示例"><code>fmt.Println(&quot;hello&quot;)</code></pre>`：代码必须放在 `<code>` 内，禁止直接放在 `<pre>` 下；
  `caption` 可省略；代码中的换行原样保留（实测）。
- `<checkbox done="true|false">待办</checkbox>`（实测）。
- `p, h1-h9, li, checkbox, title` 支持可选 `align`：`left | center | right`，如 `<p align="center">居中正文</p>`（实测 checkbox）。

### 图片与附件

`doc create --doc-format xml` 与 `doc content-update --doc-format xml` 都支持以下写法，三种来源任选其一：

| 写法 | 说明 |
|---|---|
| `<img path="@./photo.png" width="600" caption="说明"/>` | 本地图片：CLI 先上传素材再绑定到图片块 |
| `<img href="https://example.com/photo.png" caption="说明"/>` | 公开 HTTP(S) 网络图片：由飞书服务端下载，URL 须公网可达（实测，回读为 `<img src>`） |
| `<img src="FILE_TOKEN"/>` | 复制已有图片（token 来自 `doc read --engine docs_ai --detail full` 回读的 `<img src>`，实测） |
| `<source path="@./report.pdf" name="报告.pdf"/>` | 本地附件；`<source token="FILE_TOKEN"/>` 复制已有附件（实测，复制时 `name` 不生效，沿用原文件名） |

- 可选属性：`width`、`height`、`caption`、`name`；`path` / `href` / `src` 只能选一个，不能混用。
- `path` / `<source path>` 的本地文件由 CLI 上传并绑定；`href` 不经过 CLI 下载，内网、需要登录或临时签名的图片服务端取不到，
  先下载到草稿工作区再用 `path`。远程图片建议为 BMP、GIF、JPEG、PNG、TIFF 或 WebP 且不超过 20MiB（parse 预检按此检查）。
- `<source>` 可独立成块（实测回读为 `<figure view-type="Preview">`）、写成 `<figure view-type="Card|Preview"><source .../></figure>`
  指定卡片 / 预览视图（实测 `Card`），或放进 `<p>` 作为行内附件——行内附件只能放在段落末尾，其后的文字会被丢弃（实测）。
- `path` 中 cwd 内的文件用 `@./相对路径`，其他目录用 `@绝对路径`（敏感目录会被拒绝）；相对路径先查 cwd，
  仅文件不存在时再查源 XML 文件所在目录。`doc script --command parse` 在有 Presentation Decision 时会预检这些约束。

## 标题与列表编号

- 完整文档以唯一的 `<title>` 开头。用 `doc create --content-file` 写入时，内容里已有 `<title>` 就**不要再传 `--title`**：
  两者同时出现时服务端保留第一个并返回 warning `degrade_code=1017`（实测）。
- 正文标题用 `<h1>` 至 `<h9>`，层级连续、不跳级（`<h1>` 后不能直接用 `<h3>`）。需要自动编号时设置 `seq="auto"`，
  系统按层级生成 `1`、`1.1` 这类阿拉伯数字编号（实测）。
- 有序列表起始编号写在 **`<li>`** 上：`<ol><li seq="3">从 3 开始</li><li>第二项</li></ol>`（实测显示 3、4）。
  `seq="auto"` 为默认值。**不要把 `seq` 写在 `<ol>` 上**：服务端报 `degrade_code=5002` 并忽略该属性（实测）。

## 表格

- `<table><thead><tr><th><p>表头</p></th></tr></thead><tbody><tr><td><p>内容</p></td></tr></tbody></table>`（实测）。
- `<colgroup><col width="160"/><col span="2" width="260"/></colgroup>` 紧跟 `<table>` 定义列宽；`width` 为像素列宽，可选 `span` 表示连续作用的列数（实测）。
- `<th>` / `<td>` 支持 `background-color`、`vertical-align`（`top | middle | bottom`）、`colspan`、`rowspan`（实测）；
  被合并的单元格不再写出。表头优先用 `light-gray` 或 `medium-gray`，彩色单元格只表达状态或分类。

## 扩展标签

- `<cite type="doc" doc-id="DOC_TOKEN"/>`：@文档，渲染为文档标题（实测）。
- `<cite type="user" user-id="ou_xxx"/>`：@人，渲染为用户头像；必须传真实 `open_id`，不得用纯文本名字冒充 @人。
  **写入后会通知被 @ 的人**，测试时不要 @ 真实用户。
- `<cite type="citation"><a href="URL" url-type="5">标题</a></cite>`：参考文献容器，只含多个 `<a>`（实测）。
  `url-type`：`5`（网页，须在 `<a></a>` 中写标题）、`1`（Docx）、`6`（妙记）、`12`（多维表格）、`13`（电子表格），后四种可留空。
- `<whiteboard>`：画板，`type` 与 `token` 二选一：
  - `type="mermaid" | "plantuml" | "svg"` 时可用 `path` 从文件导入：`<whiteboard type="svg" path="@./diagram.svg"/>`
    （扩展名分别为 `.svg`、`.mmd` / `.mermaid`、`.puml` / `.plantuml` / `.pu` / `.uml`）；
  - 也可在标签内直接写源码，源码中的换行原样保留（三种均实测）；`path` 与标签内源码不能同时出现；`type="blank"` 新建空白画板（实测）：

    ```xml
    <whiteboard type="mermaid">flowchart LR
      A[草稿] --> B{parse 通过?}
      B -->|是| C[doc create]</whiteboard>
    ```

  - `token="WHITEBOARD_TOKEN"` 复制已有画板（实测；官方写法 `src=` 在本项目所连服务端报 `degrade_code=5004`）。
  - 复杂图表、需要精确布局或后续编辑的画板，按 `feishu-cli-visual` 的 board 工作流制作。
- `<grid><column width-ratio="0.5"><p>左栏</p></column><column width-ratio="0.5"><p>右栏</p></column></grid>`：各列 `width-ratio` 之和为 1（实测）。
- `<callout emoji="💡" background-color="light-blue" border-color="blue"><p>高亮块内容</p></callout>`（实测）：
  子块只支持 `p`、`ol`、`ul`、`checkbox` 与行内标签；禁止 `table`、`img`、`pre`、`hr`、`grid`、`whiteboard` 等块级标签或资源块。
  可选 `text-color`。emoji 以服务端支持为准：实测 `💡`、`🔥`、`📝` 保留，`⚠️` / `⚠` 被替换为默认的 `💡`。
- 其他扩展标签 `html5-block`、`bookmark`、`button`、`time`、`sheet`、`task`、`chat_card`、`sub-page-list`、`okr`
  见 [`docx-xml-extended-blocks.md`](docx-xml-extended-blocks.md)。

## 颜色

颜色表达语义，并在全文保持一致；默认中性色排版，不为装饰着色。

- **合法值**：色相为 `red, orange, yellow, green, blue, purple, gray`；`text-color`、`border-color` 用基础色相；
  `<span>`、`<th>`、`<td>`、`<button>` 背景支持基础色相、`light-{色相}`、`medium-gray`；高亮块背景支持 `gray`、`light-{色相}`、`medium-{色相}`（实测 `medium-red`）。
- **高亮块**：默认用 `light-*` 背景和默认文字色；强提醒才用 `medium-*`，彩色文字只强调短语。
- **表格**：表头优先 `light-gray` 或 `medium-gray`；彩色单元格只表达状态或分类，避免整表铺色。
- 回读（`--detail full`）时颜色会显示为 `rgb(...)` 值，写入时仍用上面的名称。

## 转义规则

禁止转义标签本身；只转义标签内部的文本内容。

- 文本转义：`<` → `&lt;`，`>` → `&gt;`，`&` → `&amp;`，段内换行写 `<br/>`（`<pre><code>` 内的换行原样保留）。
- 错误：`&lt;p&gt;内容&lt;/p&gt;`
- 正确：`<p>A &amp; B 的对比：1 &lt; 2</p>`
- 属性值里的 `&`（如 URL 查询串）建议写成 `&amp;`；实测裸 `&` 也会被服务端规范化为 `&amp;`，`doc script parse` 同样容错。
