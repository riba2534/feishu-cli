# 飞书文档写入

本工作流负责创建和编辑飞书 docx。Markdown 文件导入建新文档见 `../import/workflow.md`；只读/导出走 `../read/workflow.md` / `../export/workflow.md`。
从零创作一篇文档（选体裁、写 DocxXML 草稿、`doc script` 初始化与预检）先走 `../author/workflow.md`，DocxXML 写法见
`../author/references/docx-xml.md`。

## 目录

- [身份与文档参数](#身份与文档参数)
- [新建文档](#新建文档)（含 docs_ai 带内容建文档）
- [用 Markdown 创建文档](#用-markdown-创建文档)
- [编辑已有文档](#编辑已有文档)（content-update 文本级 / 块级 / 方言转换 / 结果判定）
- [历史版本与回滚](#历史版本与回滚doc-history)
- [Markdown 图片](#markdown-图片)
- [低层块操作](#低层块操作)
- [表格](#表格)
- [扩展语法](#扩展语法)
- [验证](#验证)

## 身份与文档参数

- 写命令（`doc create/import/add/add-board/add-callout/update/delete/batch-update/content-update/media-insert/table`、
  `doc history revert`）**默认 Bot 身份**，不自动加载 `auth login` 的 token.json。Bot 必须对目标文档有编辑权限；
  编辑用户自己的文档而 Bot 不是协作者时（报 forbidden / 1770032 等），显式以本人身份写：
  `--user-access-token "$(feishu-cli auth token --as user)"`（或设置 `FEISHU_USER_ACCESS_TOKEN`）。
- `doc history list`、`doc history revert-status` 属于读类：User 优先、Bot 兜底。
- 文档参数：`doc content-update`、`doc media-insert`、`doc history *`、`doc add/add-board/add-callout/update/delete/batch-update/table`
  与 `doc import --document-id` 都接受 docx token、`/docx/` URL 和 `/wiki/` URL（wiki 自动解析为底层 docx，底层不是 docx 时报错）。
  `doc htmlbox` 暂只接受裸 document_id。

## 新建文档

```bash
feishu-cli doc create --title "文档标题" --output json
```

以 Bot 身份创建（默认）时，CLI 会自动给当前 CLI 登录用户授予 `full_access`，结果见 JSON 的
`permission_grant.status`（`granted` / `skipped` / `failed`，详见 feishu-cli-storage 的 perm 工作流）；
以 User 身份创建时不触发。这只覆盖当前登录用户本人，不替代下面的 owner 交付流程。

用户已指定接收人时按该目标授权；否则，需要按 owner 配置交付时：

1. 用 CLI 读取生效配置；以下命令与创建、授权命令沿用本次相同的 `--profile` / `--config`（若已指定）：
   ```bash
   feishu-cli config get owner_email
   feishu-cli config get transfer_ownership
   ```
   CLI 会合并环境变量与所选配置文件，不要另行读取旧布局的 `~/.feishu-cli/config.yaml`。
2. 解析到 owner 后授予 `full_access`：
   ```bash
   feishu-cli perm add <document_id> --doc-type docx --member-type email --member-id <owner_email> --perm full_access --notification
   ```
3. 仅当生效的 `transfer_ownership` 为 `true` 时转移所有权：
   ```bash
   feishu-cli perm transfer-owner <document_id> --doc-type docx --member-type email --member-id <owner_email> --notification
   ```
4. 未配置 owner 时，不使用占位邮箱；在当前会话返回文档链接，说明尚未按 owner 配置授权，不要求用户为已明确指定的接收人再设置环境变量。

### 服务端带内容建文档（docs_ai）

```bash
# 内容默认按 Markdown 解析；--title 以 <title> 前置，优先于内容中的一级标题
feishu-cli doc create --title "周报" --content-file /tmp/weekly.md -o json
feishu-cli doc create --content '<title>XML 文档</title><p>正文</p>' --doc-format xml
# 位置：--folder / --parent-token（文件夹或知识库节点 token）/ --parent-position my_library 三选一
```

- 带 `--content/--content-file/--doc-format/--parent-token/--parent-position` 任一项即走 docs_ai 服务端建文档；
  不带时是本地空文档创建（`--title` 必填）。`--content` 中的字面 `\n` 会转成换行，`--content-file` 原样读取。
- 大内容时服务端转为异步任务：CLI 自动轮询（最长约 10 分钟，只重试查询、不重放创建请求）；`expired` /
  `execution_interrupted` 报超时并提示分批。客户端超时或网关错误时文档**可能已在后台建出**，先在云空间确认再决定是否
  重试，避免重复文档。**大文档推荐先建少量内容，再 `content-update --mode append` 分批追加**。
- Markdown 中 `doc export` 的本地方言按 content-update 同样规则转换；**不支持本地图片/附件**（以退出码 2 拒绝，改用 `doc import`）。
- Bot 身份创建后同样自动授予当前登录用户 `full_access`（JSON 输出 `permission_grant`）。

## 用 Markdown 创建文档

```bash
feishu-cli doc import /tmp/doc.md --title "标题"
```

写入临时 Markdown 后先做编码检查：

```bash
python3 -c "d=open('/tmp/doc.md','rb').read(); assert b'\xef\xbf\xbd' not in d; d.decode('utf-8')"
```

`doc import` 会拒绝非法 UTF-8；合法编码中的 U+FFFD 替换字符不会被 CLI 拦截。上面的生成阶段检查可发现它，避免把乱码写入云文档。
导入前语法检查见 `../import/references/doc-guide.md`。

## 编辑已有文档

不要把 `doc import --document-id` 当成更新命令；它会把 Markdown 转成新块追加到文档末尾。已有文档编辑用 `doc content-update`
（官方 docs_ai 单操作原子更新，无先删后写破坏窗口，支持 `--revision-id` 乐观锁；没有 `--dry-run`）。

| 用户意图 | 推荐写法 | 粒度 |
|---|---|---|
| 在末尾新增内容 | `--mode append` | 块 |
| 改几个字/改一个术语 | `--mode replace_all --selection-with-ellipsis "旧文本"`（全部命中），或 `--mode str_replace --pattern "旧"`（唯一命中） | **文本级**：只换文字，段落其余内容保留 |
| 删除一段文字 | `--mode delete_range --selection-with-ellipsis "（草稿）"` 或 `--mode str_replace --pattern "（草稿）" --markdown ""` | 文本级 |
| 替换某个章节 | `--mode replace_range --selection-by-title "## 章节"` | 块（标题到下一个同级/更高级标题） |
| 精确替换/删除某些块 | `--mode replace_range` / `delete_range` + `--block-id A[,B]` 或 `--start-block-id A --end-block-id B` | 块 |
| 在某处前/后插入 | `--mode insert_before` / `insert_after` + 标题/文本/`--block-id` | 块 |
| 调整顺序 / 复制块 | `--mode block_move_after` / `block_copy_insert_after` + `--block-id 锚点 --src-block-ids A,B` | 块 |
| 完全重写 | `--mode overwrite`（会丢评论；Markdown 首个 `# 标题` 会成为文档标题） | 全文 |

官方指令名 `block_replace` / `block_delete` / `block_insert_after` 可直接作为 `--mode` 使用（分别等价 replace_range / delete_range / insert_after）。
block id 用 `feishu-cli doc read <doc> --with-ids [--heading "章节"]` 获取。替换、Markdown 文本替换后被改写的块会换新 ID
（`block_move_after` 移动的块保留原 ID），再次按 ID 操作前重新读取。

```bash
# 文本级全文替换（只改这几个字，所在段落其余文字、下划线、颜色都保留）
feishu-cli doc content-update <doc> --mode replace_all \
  --selection-with-ellipsis "旧文本" --markdown "新文本"

# 按标题替换章节（块级）
feishu-cli doc content-update <doc> --mode replace_range \
  --selection-by-title "## 旧章节" --markdown-file /tmp/new-section.md

# 先拿 block id，再按块精确改写 / 删除区间 / 移动
feishu-cli doc read <doc> --with-ids --heading "旧章节"
feishu-cli doc content-update <doc> --mode replace_range --block-id <block_id> --markdown "新段落"
feishu-cli doc content-update <doc> --mode delete_range --start-block-id <A> --end-block-id <B>
feishu-cli doc content-update <doc> --mode block_move_after --block-id <锚点> --src-block-ids <A>,<B>

# 在章节后插入；在文档开头插入；追加到末尾；XML 写入
feishu-cli doc content-update <doc> --mode insert_after --selection-by-title "## 目标章节" --markdown "## 新增章节\n\n内容"
feishu-cli doc content-update <doc> --mode insert_after --block-id 0 --markdown "置顶说明"
feishu-cli doc content-update <doc> --mode append --markdown-file /tmp/append.md
feishu-cli doc content-update <doc> --mode append --doc-format xml --content '<p>XML <b>段落</b></p>'
```

`--markdown` / `--content` 内联内容中的字面 `\n` 会转成换行；`--markdown-file` / `--content-file` 原样读取（LaTeX 反斜杠不变）。

### 文本级替换（纯文本选择器、str_replace）

- `--selection-with-ellipsis "纯文本"`（不含 `...`）在 replace_all / replace_range / delete_range 下是**文本级**：
  replace_all 替换全部命中；replace_range / delete_range 与 `str_replace --pattern` 要求全文唯一命中，命中多处以退出码 2 拒绝。
- replace_all 多处命中时，CLI 为每处命中构造全文唯一的上下文窗口逐个替换；**任一处无法构造唯一窗口**（如同一行重复出现、
  紧贴 Markdown 标记）时在写入前整体拒绝（退出码 1，文档未修改），改用 `--block-id` 整块改写这些段落。
- 匹配按服务端序列化逐字进行：纯文字替换自动走 XML 序列化，保留块 ID 与下划线/颜色等样式；匹配文本或替换内容含 Markdown
  语法（如 `**新**`）时按 Markdown 序列化，所在段落的下划线/文字颜色会丢失且块 ID 会变（stderr 有提示），含 `_ * [ ]`
  时需写转义形式。跨样式的文字（如"锚点 **B2**"中的"锚点 B2"）匹配不到（退出码 1），带上样式标记或改用 `--block-id`。
  拿不准时先 `feishu-cli doc read <doc> --engine docs_ai --doc-format xml` 查看序列化原文。
- 只出现在文档标题里的文本会被拒绝（退出码 2）：文本级替换不改标题。

### 块级定位

- 定位方式（只能用一种）：`--selection-by-title`、`"开头...结尾"`、`--block-id`、`--start-block-id/--end-block-id`。
  标题与文本选择器只匹配文档**顶层块**（callout、表格等容器内部的标题/段落定位不到，改用 `--block-id`）。
- `--selection-by-title "## 章节"` 按标题文本**子串**匹配且只看该级别；不带 `#` 时匹配任意级别。若同时命中**父标题与其子标题**，
  命令拒绝执行（退出码 1）——改用带级别的选择器（`"## 部署检查"`）。
- `"开头...结尾"` 从含"开头"的块到其后**最近**一个含"结尾"的块。
- replace_range / delete_range / insert_* 命中多处时以退出码 2 拒绝，不会静默取第一处；块级 replace_all 处理全部命中
  （倒序逐个原子替换，中途失败非零退出并报告已完成数）。
- `--block-id` 在 replace_range / delete_range 可逗号分隔多个块；insert_* 与 block_move/copy 只接受单个锚点。
  `--block-id 0` / `-1` 表示文档开头 / 末尾（insert_after 可用；insert_before 必须给真实块 ID）。
  `--start-block-id 0`、`--end-block-id -1` 表示从文档开头 / 到文档末尾。

### 本地导出方言与结果判定

- **本地导出方言自动转换**：`doc export` 产出的 `> [!NOTE]` 等高亮块、`<callout type>`、`<image token>`、`<file token>`、
  `<mention-user>`、`<mention-doc>`、多行 `<grid cols>`、`<span style>` 颜色，发送前自动转换为 docs_ai 写法（stderr 提示转换统计）。
  无法无损写回的占位会**在任何网络请求前以退出码 2 拒绝并列出行号**：`<whiteboard token=… type="blank"/>`（画板）、
  `<bitable>`、`<sheet token>`、未下载的视频、展开的内嵌电子表格、`<!-- 不支持的块类型 -->`、未展开的同步块。
  处理方式：只改写不含这些结构的章节（`--block-id` 精确定位），或删除对应行明确放弃该内容；
  画板可改写为 ```` ```mermaid ```` 代码块或 `<whiteboard type="mermaid">…</whiteboard>` 重新生成。
  需要"导出 → 修改 → 写回"时优先用 `doc export --engine docs_ai` 导出（同一方言，见 `../export/workflow.md`）。
- **结果判定**：服务端 `result=partial_success` 或 `failed` 时命令以退出码 1 结束，错误信息含 warnings 与 log_id；
  `-o json` 仍输出完整响应（实测：跨文档按 token 克隆画板返回 `partial_success` + `degrade_code=2105`）。
  `result=success` 但带 warnings 时在 stderr 打印 warnings 与 log_id。
- **本地资源**：`content-update` 自动上传内容中的本地图片/附件（见下方「Markdown 图片」），只支持 append / overwrite /
  insert_* / 块级 replace_range；文本级替换（str_replace、纯文本选择器）带本地资源时以退出码 2 拒绝。
- 用户说"修改/替换/更新某段"时用 replace_range / replace_all / str_replace，不要 append 导致重复。

`--table-column-width`：**`content-update` 不支持自定义列宽**，传非 `auto` 值或内容中含 `<!-- feishu-colwidth: ... -->` 注释都会以退出码 2 拒绝；需要控制列宽请改用 `feishu-cli doc import`。`doc add` 仍支持该 flag，取值与注释的完整规则以 `../import/references/doc-guide.md` 表格章节为权威。

## 历史版本与回滚（doc history）

```bash
# 列出历史版本（每页 1-20；has_more 时 stderr 提示 page_token；--page-all 自动翻页，最多 50 页）
feishu-cli doc history list <doc> [--page-size 20] [--page-token TOKEN] [--page-all] [-o json]

# 回滚到 history_version_id（写操作：非交互环境不加 --yes 时以退出码 10 退出且不执行；--dry-run 只打印请求）
feishu-cli doc history revert <doc> --history-version-id 5120 --yes
feishu-cli doc history revert <doc> --history-version-id 5120 --wait-timeout-ms 0 --yes   # 只发起不等待

# 查询回滚任务
feishu-cli doc history revert-status <doc> --task-id <task_id>
```

- `list -o json` 输出 `entries[]`（`history_version_id`、`revision_id`、`edit_time`（RFC3339）、`editor_ids` 等）、`has_more`、`page_token`。
- 回滚接口只接受 `history_version_id`（正整数字符串），**不要传 `revision_id`**（传 0 等非法值以退出码 2 拒绝）；
  按 revision_id 或时间点回滚时先在 list 中定位记录，同一 revision_id 命中多条时请用户确认。
- `status=done` 才是成功；`running` 时用 revert-status 继续查询；`partial_failed` / `failed` 以退出码 1 结束并输出 `failed_block_tokens`。
- 回滚以历史内容替换当前正文（当前内容仍在历史中，可再回滚）；回滚后 block id 可能变化，继续编辑前重新 `doc read --with-ids`。

## Markdown 图片

| 命令 | 图片处理 |
|---|---|
| `doc import` | 默认上传本地/网络图片（相对路径按 Markdown 文件所在目录）；表格单元格图片也走导入管线 |
| `doc add` | 显式传 `--upload-images` 上传本地/网络图片；表格单元格图片降级为文字占位 |
| `doc content-update` | 网络图片由服务端下载；本地图片/附件自动上传并绑定（`--upload-images` 可省略，传了只打印提示） |
| `doc create --content` | 仅网络图片；带本地图片的 Markdown 改用 `doc import` |

```bash
# 网络图片原样交给服务端；本地图片相对 --markdown-file 所在目录解析
feishu-cli doc content-update <document_id> --mode append --markdown-file /tmp/with-image.md
```

`content-update` 的本地资源写法（围栏代码与行内代码中的内容不处理）：

| 写法 | 说明 |
|---|---|
| `![说明](./img.png)`、`![说明](/abs/img.png)` | 相对路径基于 `--markdown-file` 所在目录（内联 `--markdown` 时基于当前目录）；说明成为图片标题 |
| `![说明](@./img.png)`、`![说明](<@./带 空格.png>)` | 官方写法，`@` 路径相对**当前目录** |
| `<img path="@./img.png" width="600" height="300"/>` | XML 写法（`--doc-format xml` 时只认这种），可指定显示尺寸，路径相对当前目录 |
| `<source path="@./report.pdf" name="报告.pdf"/>` | 本地附件 |

上传流程：本地资源先改写为占位标签 → 服务端在 `document.new_blocks` 回传占位块 → 以占位块为父节点上传素材
（>20MB 自动分片）→ 绑定。任一资源失败时删除其占位块，命令以退出码 1 结束，
JSON 输出 `local_resources` 逐项明细（`status`=bound/failed、`block_id`、`file_token`、`error`、`cleanup`）。
文件不存在、为空、路径不安全或内容手写了保留占位标记（`@lcli_img_` / `@lcli_file_`）时，在任何网络请求前以退出码 2 报错。

单独插入图片或文件用 `doc media-insert`（插入到文档末尾）：

```bash
feishu-cli doc media-insert <document_id> --file /path/to/image.png --type image --align center --caption "说明"
# 指定显示宽度（只给一边时按原图比例计算另一边；两边都给则按给定值）
feishu-cli doc media-insert <document_id> --file /path/to/chart.png --width 600
feishu-cli doc media-insert <document_id> --file /path/to/report.pdf --type file
```

- `--type` 只接受 `image` / `file`（其他值以退出码 2 拒绝）；视频等其他文件用 `--type file` 作为附件插入。
- `--width/--height` 只用于 `--type image`，取值 1-10000 像素。
- 超过 20MB 的文件自动走分片上传（stderr 打印分片进度，实测 21MB 附件分 6 片）。

## 低层块操作

这些命令的文档参数接受 document_id 或 URL，块参数只接受 block_id；需要熟悉飞书 Block 结构，普通章节编辑优先用 `content-update`。

```bash
# doc add：默认 --content-type json；--source-type file/content；可指定父块和插入位置
feishu-cli doc add <document_id> content.md --content-type markdown --upload-images
feishu-cli doc add <document_id> --content '[{"block_type":2,"text":{"elements":[{"text_run":{"content":"你好"}}]}}]' --source-type content
feishu-cli doc add <document_id> doc.md --content-type markdown --block-id <parent_block_id> --index 0
# 带表格的 Markdown 可指定列宽（仅 markdown 内容类型生效）
feishu-cli doc add <document_id> table.md --content-type markdown --table-column-width auto
# 注意：doc add 不把 ```mermaid / plantuml / svg 代码块转成画板（按代码块写入）；要图表用 doc import 或 board import。
# 导出 Markdown 里的 <whiteboard token> 会复制源画板，单次建块最多 5 个画板，CLI 已自动分批。

feishu-cli doc update <document_id> <block_id> --content-file update.json

# doc delete：父块 ID + 索引范围（左闭右开），或 --all 删全部子块；非交互环境须带 --force 或 --yes，否则以退出码 10 退出且不删除
feishu-cli doc delete <document_id> <parent_block_id> --start 0 --end 3 --yes
feishu-cli doc delete <document_id> <parent_block_id> --all --force

# doc batch-update：默认 --source-type file；--client-token（幂等）与 --document-revision-id（乐观锁，默认 -1=最新）
feishu-cli doc batch-update <document_id> updates.json --source-type file
feishu-cli doc batch-update <document_id> updates.json \
  --client-token "$(uuidgen)" \
  --document-revision-id -1
```

### `doc add-board` 添加画板块

向文档插入一个空画板块，返回 `block_id` 和 `whiteboard_id`，后续用 `feishu-cli board` 系列命令（feishu-cli-visual）操作画板内容。

```bash
# 末尾添加，JSON 输出便于脚本提取 whiteboard_id
feishu-cli doc add-board <document_id> -o json

# 指定父块和位置
feishu-cli doc add-board <document_id> --parent-id <block_id> --index 0
```

### `doc add-callout` 添加高亮块

向文档插入一个 Callout 高亮块并写入文本内容。

| `--callout-type` | 背景色 |
|---|---|
| `info`（默认） | 浅蓝（5） |
| `warning` | 浅黄（3） |
| `error` | 浅红（1） |
| `success` | 浅绿（4） |

> 括号内是飞书 Callout 背景色枚举（1 浅红、2 浅橙、3 浅黄、4 浅绿、5 浅蓝、6 浅紫、7 中灰）。注意 `add-callout --callout-type warning`
> 是**黄色**，而 Markdown `> [!WARNING]` 导入为**红色**。需要 CAUTION（浅橙）/IMPORTANT（浅紫）等其他颜色时用 Markdown
> `> [!CAUTION]` 走 `doc import` / `content-update`。

```bash
# 默认 info 蓝色
feishu-cli doc add-callout <document_id> "这是一条提示信息"

# 警告 + 自定义图标（emoji shortcode）
feishu-cli doc add-callout <document_id> "请注意" --callout-type warning --icon fire
```

## 表格

Markdown 表格导入 docx 时：

- 行数 > 9：CLI 创建 9 行初始表，再用 `insert_table_row` 追加到同一 block。
- 列数 > 9：按列组拆分，保留首列作为标识。
- `doc import` / `doc add` 的主要耗时来自行 > 9 时逐行追加（受单文档写入限流）。`content-update` 由服务端解析 Markdown 表格，不走本地块填充。
- 行数极多（200+）时更适合用 Sheet：`feishu-cli sheet import-md report.md --title "报表"`。

文档内已有表格结构操作（`TABLE_BLOCK_ID` 是 block_type=31 的表格块）：

```bash
# 单次插入一行；插入多行时按目标索引重复调用（--index -1 为末尾）
feishu-cli doc table insert-row DOC_ID TABLE_BLOCK_ID --index 1
feishu-cli doc table delete-rows DOC_ID TABLE_BLOCK_ID --start 1 --end 3
feishu-cli doc table insert-column DOC_ID TABLE_BLOCK_ID --index -1
feishu-cli doc table delete-columns DOC_ID TABLE_BLOCK_ID --start 1 --end 3
feishu-cli doc table merge-cells DOC_ID TABLE_BLOCK_ID --row-start 0 --row-end 2 --col-start 0 --col-end 3
# 取消合并：指定合并区域内任一单元格的行/列索引
feishu-cli doc table unmerge-cells DOC_ID TABLE_BLOCK_ID --row 0 --col 0
```

索引从 0 开始，范围均为左闭右开。

## 扩展语法

`doc import` 与 `content-update` 都认识导出端生成的扩展标签，但处理方式不同：

```html
<mention-user id="ou_xxx"/>
<mention-doc token="doc_token_xxx" type="docx">标题</mention-doc>
<callout type="NOTE">
内容
</callout>
```

- `<callout>`、`<grid>` 这类块级标签必须**开标签独占一行、内容另起行**；写在同一行（`<callout type="NOTE">内容</callout>`）
  时 `doc import` 只会得到一段普通文字。
- `doc import` / `doc add` 由本地转换器解析 `<grid cols>` 分栏：列由服务端自动生成，各列内容写入对应列（列内可含多段落、列表）；
  `content-update` / `doc create --content` 同样可写（多行 `<grid cols="2">` + `<column>` 会转换为 docs_ai 的 `<grid><column width-ratio>`）。
- `content-update` 在发送前把这些标签转换为 docs_ai 写法（`<cite>`、带颜色属性的 `<callout>`、带 `width-ratio` 的 `<grid>`），
  也可直接书写 docs_ai XML 标签（如 `<callout background-color="light-blue" border-color="blue">`）。

Mermaid / PlantUML 在 `doc import` 时转为飞书画板；`content-update` 中的 ```` ```mermaid ```` 代码块同样由服务端生成画板（实测）。
语法限制参考 `../import/references/doc-guide.md`。

## 验证

1. 创建/更新后确认返回 document_id 或成功状态，并检查退出码（`partial_success`、本地资源失败都是非零）。
2. 需要交付时确认 owner 权限已添加。
3. 精确改写后用 `doc read <doc> --with-ids --heading "章节"` 回读确认。
4. 重大覆盖操作前先确认用户明确要求 `overwrite`。
