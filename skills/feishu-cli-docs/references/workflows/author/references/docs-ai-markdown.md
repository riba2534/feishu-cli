<!-- 内容改编自 larksuite/cli（MIT License, Copyright (c) 2026 Lark Technologies Pte. Ltd.） -->
# docs_ai Markdown 写法参考

适用于 `doc create --content/--content-file`（默认 `--doc-format markdown`）、`doc content-update` 的 Markdown 内容，
以及 `doc read --engine docs_ai --doc-format markdown` 的输出。这是服务端 Markdown 方言，与 `doc import` 的本地转换器不同；
从零创作优先用 DocxXML（见 [`docx-xml.md`](docx-xml.md)），Markdown 适合短内容和文本级修改。

## 转义规则

字面文本含下列字符且不想触发 Markdown 语法时，用 `\` 转义（实测：`\$100`、`C:\\Users`、`\<b>`、`3 \* 5`、`foo\_bar`、
`\[非链接\]`、`a\~\~b\~\~c`、行首 `\#` 均按字面显示）。

| 符号 | 语法用途 | 写法 | 何时需要 |
|---|---|---|---|
| `\` | 转义符本身 | `\\` | 任意位置 |
| `` ` `` | 行内代码 | `` \` `` | 任意位置 |
| `*`、`_` | 斜体 / 加粗 | `\*`、`\_` | 任意位置 |
| `[`、`]` | 链接文本 | `\[`、`\]` | 任意位置 |
| `$` | 公式定界 | `\$` | 任意位置 |
| `~` | 删除线 `~~text~~` | `\~` | 任意位置 |
| `<` | XML 标签起始（`<b>`、`<u>`、`<img>` 会被当作标签生效） | `\<` | 任意位置；`a < b` 建议写 `a \< b` |
| `#`、`+`、`-`、`>` | 标题 / 列表 / 分隔线 / 引用 | `\#` 等 | 仅行首（去掉前导空白后） |
| `\|` | 表格分隔 | `\|` | 仅 GFM 表格单元格内 |

- 行内代码、代码块和 `$...$` 公式内部是字面量，不需要转义。
- `doc read --engine docs_ai --doc-format markdown` 的输出**已经转义**，修改后写回时保留这些 `\`，不要反转义；
  `content-update --mode str_replace --pattern` 匹配时也要用转义后的形式。
- Markdown 中写 `<u>`、`<b>` 等 XML 标签会生效（实测）；需要显示字面量时写 `\<u>`。下划线、高亮块、勾选框、画板、
  分栏、@人 / @文档、按钮、日期提醒、文字颜色等没有原生 Markdown 语法，用 DocxXML 标签表达。

## 传参

- 多行、含特殊字符或较长的内容写入文件，用 `--content-file`（`doc create`）或 `--markdown-file`（`content-update`）传入，
  CLI 原样读取；内联 `--content` / `--markdown` 中的字面 `\n` 会被转成换行。
- shell 内联时默认用单引号 `'...'`（`$`、反引号、`\` 原样保留）；多行内容用带引号的 heredoc（`<<'EOF'`）。

## 图片

- 网络图片 `![说明](https://example.com/photo.png)` 会被下载并插入（实测），说明文字成为图片标题；对应 XML 为 `<img href="..."/>`。
- 本地图片写 `![说明](@./images/photo.png)`（路径含空格时写 `![说明](<@./images/product shot.png>)`），由 CLI 上传并绑定，
  说明文字成为图片标题；附件用 XML 写法 `<source path="@./files/report.pdf"/>`。
- 不支持 Base64 Data URI 图片，先解码为本地文件再按本地图片写入。
