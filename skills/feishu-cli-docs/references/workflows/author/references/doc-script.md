<!-- 内容改编自 larksuite/cli（MIT License, Copyright (c) 2026 Lark Technologies Pte. Ltd.） -->
# `doc script` 命令参考

| `--command` | 用途 |
|---|---|
| `init-draft` | 校验 Presentation Decision，创建独占草稿工作区并保存决策基线，预留尚不存在的 XML 路径 |
| `parse` | 解析本地 DocxXML 或在线文档，返回画像，并按决策检查字数、展示块与资源 |

输出默认是 JSON（`--format json|pretty|table|ndjson|csv`，`--jq` 裁剪）；字段直接位于顶层，没有 `data` 包装。
`--dry-run` 只输出执行计划：不写文件、不调用 OpenAPI。

## init-draft

```bash
feishu-cli doc script --command init-draft --presentation-decision '{}'
feishu-cli doc script --command init-draft --presentation-decision "@./decision.json"
```

- 只接受 `--presentation-decision`（必填）；同时传 `--content` / `--doc` 以退出码 2 拒绝。决策支持内联 JSON、`"@文件路径"`、`-`（stdin）。
- CLI 在**当前目录**创建 `draft_<8 位十六进制>_folder/` 与其中的 `.presentation-decision.json`，**不创建** `draft.xml`；
  当前目录位于敏感目录（`~/.ssh`、`/etc` 等）时拒绝。不要自行创建工作区或决策文件，以返回值为准。
- 返回 `cwd`（绝对路径）、`workspace`（相对路径）、`draft_path`（相对路径，已含工作区前缀）和操作提示 `tip`。
  后续命令在 `cwd` 下执行，草稿直接写到 `<cwd>/<draft_path>`；不要另建 XML、复用其他任务的路径或修改 `.presentation-decision.json`。
- 内联 JSON 被 shell 吞掉引号时：外层多余的单引号（Windows 命令行垫片）会自动去掉；PowerShell 5.x 去掉键 / 值引号但能按
  schema 无歧义重建时自动恢复并保存规范 JSON；有歧义时报错并提示改用 `"@./decision.json"`。文件和 stdin 输入始终严格解析。

### Presentation Decision 字段

决策是单个 JSON 对象，无约束时传 `{}`。

| 字段 | 规则 |
|---|---|
| `audience`、`reader_task`、`genre_contract`、`adapter`、`presentation_mode`、`visual_plan.reason`、`blocks[].purpose` | 描述信息，可省略、空字符串或 `null`，不参与判定；`purpose` 会出现在诊断文案里 |
| `word_count` | 仅在用户提出字数要求时填写 `{"min": N, "max": N}`，未限制的一侧写 `null`；至少一侧为正整数且 `min <= max`。无要求时**省略整个字段**（写 `null` 报错） |
| `visual_plan.blocks[]` | `{"type": "...", "min_count": N}`；`type` 与 `min_count` 都有效时才参与数量检查，同一 `type` 不能重复启用 |
| `type` 取值 | 展示块：`img`、`whiteboard`、`html5-block`、`table`、`grid`、`callout`、`pre`、`sheet`、`bitable`、`mindnote`、`figure`、`chat_card`、`okr`、`poll`、`agenda`、`bookmark` 等；`list` 按 `<ul>` 与 `<ol>` 合计。`p`、`h1` 等普通文本块不能作为约束 |

显式填写的字段必须合法：`visual_plan` 为对象、`blocks` 为数组、`min_count` 为正整数；未知字段（如旧版的 `hard_rules`）、
空字符串类型、零 / 负数 / 小数数量都以退出码 2 拒绝。

## parse

```bash
feishu-cli doc script --command parse --content "@./draft_1a2b3c4d_folder/draft.xml"
feishu-cli doc script --command parse --doc "https://xxx.feishu.cn/docx/doxcnxxx" --as user
feishu-cli doc script --command parse --content "@./doc.xml" --presentation-decision '{"word_count":{"min":800,"max":null}}'
```

| 参数 | 说明 |
|---|---|
| `--content` | 本地 XML：字面内容、`"@文件路径"` 或 `-`（stdin）；`@@` 开头表示字面 `@`。只接受 XML，Markdown 以退出码 2 拒绝 |
| `--doc` | 在线 docx URL / token 或 `/wiki/` URL（先解析为底层 docx），经 docs_ai fetch 取 XML；与 `--content` 互斥 |
| `--presentation-decision` | 可选；`--content` 与它最多一个读 stdin |
| `--as bot\|user\|auto` | `--doc` 使用的身份，默认 `auto`（User 优先，未登录回退 Bot）；需要 `docx:document:readonly` |

- 用 `--content "@./<draft_path>"` 时自动加载同目录的 `.presentation-decision.json`；显式 `--presentation-decision` 优先。
  已保存的决策被改坏时以退出码 2 报错（不要重试），重新执行 `init-draft`。
- 输出：`assessment.status`（`passed` / `failed`）、`profile`（`word_count`、`char_count`、`block_count`、`blocks[]` 的
  `type` / `count` / `ratio`）和按需出现的 `diagnostics[]`。**检查未通过时退出码仍为 0**，判断看 `assessment.status`。
- `word_count` 按飞书写作口径计数：汉字、中文标点逐字计，英文单词、数字、URL 各计 1；列表序号、勾选框也计入。
- `passed` 只表示已启用的检查通过，不代表未声明的要求已满足；parse 也不是 schema 校验器，能否写入以 `doc create` 的结果为准。
- 解析带容错：未闭合标签、缺引号属性、旧式 `<block_id="...">` 等常见错误会在内存中修复后统计（不改动文件）；
  DOCTYPE / ENTITY 声明、非法 UTF-8、XML 1.0 禁止字符、嵌套超过 1024 层仍以退出码 2 拒绝。

### 诊断码

| `code` | 含义 | 处理 |
|---|---|---|
| `word_count_out_of_range` | 字数不在决策区间内（闭区间） | 按 `expected.min/max` 与 `actual` 增删内容 |
| `required_block_missing` | 某类展示块少于 `min_count` | 补足 `expected.type`，`suggested` 带用途说明 |
| `resource_preflight_failed` | 本地资源或标签不合法（只报第一个问题） | 修复 `msg` 指出的资源后重新解析 |
| `remote_image_source_disallowed` | 远程图片 URL 指向本地 / 内网或不允许访问 | 下载到工作区，改为 `<img path="@..."/>` |
| `remote_image_unavailable` | 远程图片不可访问（HTTP 非 2xx、网络错误） | 换可公开访问的 URL，或下载到本地 |
| `remote_image_format_unsupported` | 响应不是 BMP/GIF/JPEG/PNG/TIFF/WebP | 转换格式后改用本地路径 |
| `remote_image_too_large` | 超过 20MiB | 压缩后改用本地路径 |
| `remote_image_preflight_failed` | 其他探测失败 | 按 `msg` 处理 |

同一原因失败的远程图片合并为一条诊断，`image_indices[]` 列出图片序号（`<img>` 与 `<source>` 按出现顺序共用序号）。

### 资源预检

只在加载到 Presentation Decision 时执行：

- `<img path>`、`<source path>`：以 `@` 开头、文件存在、非空、可读；图片须能解出尺寸。路径不能越出当前目录（`..`）或位于敏感目录。
- `<whiteboard type="svg|mermaid|plantuml" path="@...">`、`<html5-block path="@x.html">`：扩展名匹配且文件可读
  （写法见 [`docx-xml.md`](docx-xml.md) 与 [`docx-xml-extended-blocks.md`](docx-xml-extended-blocks.md)）。
- 相对路径先查当前目录，仅文件不存在且 `--content` 来自 `@文件` 时，再查该 XML 文件所在目录；内联内容和 stdin 不回退。
- `<img href>`：先校验是不带用户名密码的绝对 HTTP(S) URL，再发一次 `Range: bytes=0-0` 的 GET 探测（会联网，不缓存图片），
  拒绝 localhost、内网与保留地址，并逐跳校验重定向。
