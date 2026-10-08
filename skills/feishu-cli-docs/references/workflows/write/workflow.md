# 飞书文档写入

本技能负责创建和编辑飞书 docx。Markdown 文件导入创建文档见 `../import/workflow.md`；只读/导出走 `../read/workflow.md` / `../export/workflow.md`。

## 新建文档

```bash
feishu-cli doc create --title "文档标题" --output json
```

以 Bot 身份创建（默认）时，CLI 会自动给当前 CLI 登录用户授予 `full_access`，结果见 JSON 的
`permission_grant.status`（`granted` / `skipped` / `failed`，详见 feishu-cli-storage 的 perm 工作流）。
这只覆盖当前登录用户本人，不替代下面的 owner 交付流程。

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
# 内容默认按 Markdown 解析；--title 以 <title> 前置，优先于内容中的标题
feishu-cli doc create --title "周报" --content-file /tmp/weekly.md -o json
feishu-cli doc create --content '<title>XML 文档</title><p>正文</p>' --doc-format xml
# 位置：--folder / --parent-token（文件夹或知识库节点 token）/ --parent-position my_library 三选一
```

- 带 `--content/--content-file/--doc-format/--parent-token/--parent-position` 任一项即走 docs_ai（`POST /docs_ai/v1/documents`，
  与官方 `docs +create` 同协议）；不带时仍是原来的本地空文档创建（`--title` 必填）。
- 大内容时服务端可能转为异步任务：CLI 自动轮询 `async_tasks`（最长 10 分钟，只重试查询、不重放创建请求）；
  `expired` / `execution_interrupted` 报超时并提示分批。网关超时（约 30s，实测 3000 段落即触发）时文档**通常已在后台生成**，
  CLI 不会重试创建，请先在云空间确认再决定是否追加，避免重复文档。**大文档推荐先建少量内容，再 `content-update --mode append` 分批追加**。
- Markdown 中 `doc export` 的本地方言按 content-update 同样规则转换；本地图片/附件不支持（改用 `doc import`）。
- Bot 身份创建后同样自动给当前登录用户授予 `full_access`（JSON 输出 `permission_grant`）。

## 用 Markdown 创建文档

```bash
feishu-cli doc import /tmp/doc.md --title "标题" --upload-images
```

写入临时 Markdown 后先做编码检查：

```bash
python3 -c "d=open('/tmp/doc.md','rb').read(); assert b'\xef\xbf\xbd' not in d; d.decode('utf-8')"
```

`doc import` 会拒绝非法 UTF-8；合法编码中的 U+FFFD 替换字符不会被 CLI 拦截。上面的生成阶段检查可发现它，避免把乱码写入云文档。

## 编辑已有文档

不要把 `doc import --document-id` 当成更新命令；它会把 Markdown 转成新块追加到文档末尾。已有文档编辑优先用 `doc content-update`
（全部走官方 docs_ai 单操作原子更新 `PUT /open-apis/docs_ai/v1/documents/{id}`，无先删后写破坏窗口，支持 `--revision-id` 乐观锁）。

| 用户意图 | 推荐写法 | 粒度 |
|---|---|---|
| 在末尾新增内容 | `--mode append` | 块 |
| 改几个字/改一个术语 | `--mode replace_all --selection-with-ellipsis "旧文本"`，或 `--mode str_replace --pattern "旧"` | **文本级**：只换文字，段落其余内容保留 |
| 删除一段文字 | `--mode delete_range --selection-with-ellipsis "（草稿）"` 或 `--mode str_replace --pattern "（草稿）" --markdown ""` | 文本级 |
| 替换某个章节 | `--mode replace_range --selection-by-title "## 章节"` | 块（标题到下一个同级/更高级标题） |
| 精确替换/删除某些块 | `--mode replace_range` / `delete_range` + `--block-id A[,B]` 或 `--start-block-id A --end-block-id B` | 块 |
| 在某处前/后插入 | `--mode insert_before` / `insert_after` + 标题/文本/`--block-id` | 块 |
| 调整顺序 / 复制块 | `--mode block_move_after` / `block_copy_insert_after` + `--block-id 锚点 --src-block-ids A,B` | 块 |
| 完全重写 | `--mode overwrite`（会丢评论；Markdown 首个 `# 标题` 会成为文档标题） | 全文 |

官方指令名 `block_replace` / `block_delete` / `block_insert_after` 可直接作为 `--mode` 使用（分别等价 replace_range / delete_range / insert_after）。
block id 用 `feishu-cli doc read <doc> --with-ids [--heading "章节"]` 获取；每次写入后旧 block id 可能失效，再次操作前重新读取。

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

# 在章节后插入；追加到末尾；XML 写入
feishu-cli doc content-update <doc> --mode insert_after --selection-by-title "## 目标章节" --markdown "## 新增章节\n\n内容"
feishu-cli doc content-update <doc> --mode append --markdown-file /tmp/append.md
feishu-cli doc content-update <doc> --mode append --doc-format xml --content '<p>XML <b>段落</b></p>'
```

关键规则：
- **选择器粒度**：`--selection-with-ellipsis "纯文本"`（不含 `...`）是**文本级**——replace_all 替换全部命中，
  replace_range / delete_range 要求全文唯一命中，命中多处报错（exit 2）。纯文字替换自动走 XML 序列化以保留样式；
  替换内容含 Markdown 语法（如 `**新**`）时按 Markdown 序列化替换，所在段落的下划线/文字颜色会丢失（stderr 有提示）。
  文本级替换按服务端序列化逐字匹配：跨样式的文字（如部分加粗）匹配不到，改用 `--block-id` 整块改写；
  只出现在文档标题里的文本会被拒绝（文本级替换不改标题）。
- **块级定位**：`--selection-by-title`、`"开头...结尾"`、`--block-id`、`--start-block-id/--end-block-id`。
  `开头...结尾` 从含"开头"的块到其后**最近**一个含"结尾"的块；replace_range / delete_range / insert_* 命中多处时
  报错，不会静默取第一处；replace_all 处理全部命中（倒序逐个原子替换，中途失败非零退出并报告已完成数）。
  ⚠️ 无 `#` 的模糊标题选择器若同时命中**父标题与其子标题**，命令 fail-closed——改用带级别的选择器（`"## 部署检查"`）。
- **本地导出方言自动转换**：`doc export` 产出的 `> [!NOTE]` 等高亮块、`<image token>`、`<file token>`、`<mention-user>`、
  `<mention-doc>`、`<grid cols>`、`<span style>` 颜色，发送前自动转换为 docs_ai 写法（stderr 提示转换统计）。
  无法无损写回的占位会 **fail-closed（exit 2）并列出行号**：`<whiteboard token=… type="blank"/>`（画板）、
  `<bitable>`、`<sheet token>`、未下载的视频、展开的内嵌电子表格、`<!-- 不支持的块类型 -->`、未展开的同步块。
  处理方式：只改写不含这些结构的章节（`--block-id` 精确定位），或删除对应行明确放弃该内容；
  画板可改写为 ```` ```mermaid ```` 代码块或 `<whiteboard type="mermaid">…</whiteboard>` 重新生成。
- **结果判定**：服务端 `result=partial_success` 或 `failed` 时命令以退出码 1 结束，错误信息含 warnings 与 log_id；
  `-o json` 仍输出完整响应。`result=success` 但带 warnings 时在 stderr 打印 warnings 与 log_id。
- **本地资源**：`content-update` 暂不支持本地图片/附件；用网络图片 URL，或写入后用 `doc media-insert` 插入本地文件。
- 用户说"修改/替换/更新某段"时用 replace_range / replace_all / str_replace，不要 append 导致重复。

`--table-column-width`：**`content-update` 不支持自定义列宽**（原子更新协议限制），传非 `auto` 值或内容中含 `<!-- feishu-colwidth: ... -->` 注释都会 fail-closed 报错；需要控制列宽请改用 `feishu-cli doc import`。`doc add` 仍支持该 flag，取值与注释的完整规则（单位/优先级/clamp）以 `../import/references/doc-guide.md` 表格章节为权威。

## Markdown 图片

| 命令 | 图片处理 |
|---|---|
| `doc import` | 默认上传本地/网络图片；表格单元格图片也走导入管线 |
| `doc add` | 显式传 `--upload-images` 上传本地/网络图片；表格单元格图片降级为文字占位 |
| `doc content-update` | 使用网络图片 URL，不传 `--upload-images`；本地资源和该 flag 都会被拒绝 |

```bash
# with-image.md 中使用 ![说明](https://example.com/image.png) 这样的网络图片
feishu-cli doc content-update <document_id> --mode append \
  --markdown-file /tmp/with-image.md
```

单独插入图片或文件用 `doc media-insert`：

```bash
feishu-cli doc media-insert <document_id> --file /path/to/image.png --type image --align center --caption "说明"
# 指定显示宽度（只给一边时按原图比例计算另一边；两边都给则按给定值）
feishu-cli doc media-insert <document_id> --file /path/to/chart.png --width 600
feishu-cli doc media-insert <document_id> --file /path/to/report.pdf --type file
```

超过 20MB 的文件自动走分片上传（upload_prepare / upload_part / upload_finish，stderr 打印分片进度）；
`doc import` 中超过 20MB 的视频同样走分片上传。`--width/--height` 只用于 `--type image`（1-10000 像素）。

## 低层块操作

```bash
# doc add：默认 --content-type json；--source-type file/content；可指定父块和插入位置
feishu-cli doc add <document_id> content.md --content-type markdown --upload-images
feishu-cli doc add <document_id> --content '[{"block_type":2,...}]' --source-type content
feishu-cli doc add <document_id> doc.md --content-type markdown --block-id <parent_block_id> --index 0
# 带表格的 Markdown 可指定列宽（仅 markdown 内容类型生效）
feishu-cli doc add <document_id> table.md --content-type markdown --table-column-width auto

feishu-cli doc update <document_id> <block_id> --content-file update.json

# doc delete：必须指定父块 ID + 索引范围（左闭右开），或用 --all 删全部子块；--force/--yes 跳过确认（非交互未确认时退出码 10 且不删除）
feishu-cli doc delete <document_id> <parent_block_id> --start 0 --end 3
feishu-cli doc delete <document_id> <parent_block_id> --all --force

# doc batch-update：默认 --source-type file；可传 --client-token（UUIDv4 幂等）和
# --document-revision-id（乐观锁，默认 -1=最新；指定具体版本时如版本不匹配会失败）
feishu-cli doc batch-update <document_id> updates.json --source-type file
feishu-cli doc batch-update <document_id> updates.json \
  --client-token "$(uuidgen)" \
  --document-revision-id -1
```

低层 JSON 需要熟悉飞书 Block 结构；普通章节编辑优先用 `content-update`。

### `doc add-board` 添加画板块

向文档插入一个空画板块（block_type=43），返回 `block_id` 和 `whiteboard_id`，后续可用 `feishu-cli board` 系列命令操作画板内容。

| flag | 说明 | 默认 |
|---|---|---|
| `--parent-id` | 父块 ID | 空（文档根节点） |
| `--index` | 插入位置索引 | -1（末尾） |
| `--output, -o` | 输出格式 (json) | 文本 |

```bash
# 末尾添加，JSON 输出便于脚本提取 whiteboard_id
feishu-cli doc add-board <document_id> -o json

# 指定父块和位置
feishu-cli doc add-board <document_id> --parent-id <block_id> --index 0
```

### `doc add-callout` 添加高亮块

向文档插入一个 Callout 高亮块（block_type=19），并自动写入文本内容（API 会自动生成空子块，CLI 已处理为直接更新而非额外创建）。

| `--callout-type` | 背景色 | 视觉含义 |
|---|---|---|
| `info`（默认） | 浅蓝（5） | 信息提示，灯泡图标 |
| `warning` | 浅黄（3） | 警告提示 |
| `error` | 浅红（1） | 错误提示 |
| `success` | 浅绿（4） | 成功提示 |

> 注：括号内是飞书 Callout 背景色枚举（1 浅红、2 浅橙、3 浅黄、4 浅绿、5 浅蓝、6 浅紫、7 中灰，经 docs_ai 读写实测校准）。
> CLI 当前仅暴露 4 种 type；如需 CAUTION（浅橙）/IMPORTANT（浅紫）等其他颜色，请用 Markdown `> [!CAUTION]` 或
> `<callout type="CAUTION">...</callout>` 走 `doc import` / `content-update`。

| flag | 说明 | 默认 |
|---|---|---|
| `--callout-type` | 类型 (info/warning/error/success) | info |
| `--icon` | 自定义图标（emoji shortcode，如 `bulb` `fire`） | 空 |
| `--parent-id` | 父块 ID | 空（文档根节点） |
| `--index` | 插入位置索引 | -1（末尾） |
| `--output, -o` | 输出格式 (json) | 文本 |

```bash
# 默认 info 蓝色
feishu-cli doc add-callout <document_id> "这是一条提示信息"

# 警告 + 自定义图标
feishu-cli doc add-callout <document_id> "请注意" --callout-type warning --icon fire
```

## 表格

Markdown 表格导入 docx 时：

- 行数 > 9：CLI 创建 9 行初始表，再用 `insert_table_row` 追加到同一 block。
- 列数 > 9：按列组拆分，保留首列作为标识。
- `doc import` / `doc add` 的单元格填充走 `batch_update` 批量写入（每批 ≤30 个，含追加行的新 cell）；主要耗时来自行 > 9 时 `insert_table_row` 逐行串行追加（受单文档 3 QPS 节流）。`content-update` 由原子更新 API 解析 Markdown，不走这条本地块填充管线。
- 行数极多（200+）时更适合用 Sheet：`feishu-cli sheet import-md report.md --title "报表"`。

文档内已有表格结构操作：

```bash
# 单次插入一行；插入多行时按目标索引重复调用
feishu-cli doc table insert-row DOC_ID TABLE_BLOCK_ID --index 1
feishu-cli doc table delete-rows DOC_ID TABLE_BLOCK_ID --start 1 --end 3
feishu-cli doc table insert-column DOC_ID TABLE_BLOCK_ID --index -1
feishu-cli doc table delete-columns DOC_ID TABLE_BLOCK_ID --start 1 --end 3
feishu-cli doc table merge-cells DOC_ID TABLE_BLOCK_ID --row-start 0 --row-end 2 --col-start 0 --col-end 3
# 取消合并：指定合并区域内任一单元格的行/列索引
feishu-cli doc table unmerge-cells DOC_ID TABLE_BLOCK_ID --row 0 --col 0
```

## 扩展语法

`doc import` / `content-update` 支持常见 Markdown，以及导出端生成的 HTML 扩展标签：

```html
<mention-user id="ou_xxx"/>
<mention-doc token="doc_token_xxx" type="docx">标题</mention-doc>
<callout type="NOTE">内容</callout>
<grid cols="2"><column>左</column><column>右</column></grid>
```

`doc import` 由本地转换器直接解析；`content-update` 在发送前把它们转换为 docs_ai 写法（`<cite>`、带颜色属性的 `<callout>`、
带 `width-ratio` 的 `<grid>`），也可直接书写 docs_ai XML 标签（如 `<callout background-color="light-blue" border-color="blue">`）。

Mermaid / PlantUML 会在导入时转为飞书画板（`content-update` 中的 ```` ```mermaid ```` 代码块同样由服务端生成画板）；
语法限制参考 `../import/references/doc-guide.md`。

## 验证

1. 创建/更新后确认返回 document_id 或成功状态。
2. 需要交付时确认 owner 权限已添加。
3. 图片/表格/图表较多时查看命令输出中的成功统计。
4. 重大覆盖操作前先确认用户明确要求 `overwrite`。
