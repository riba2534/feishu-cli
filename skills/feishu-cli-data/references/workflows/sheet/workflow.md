# 飞书电子表格工作流

本工作流覆盖普通电子表格（Sheet）的全部命令。本页展开高级能力：筛选视图与条件（filter-view）、下拉菜单（dropdown）、
浮动图片与单元格图片（image）、批量样式（batch-set-style），并汇总全部命令的决策要点。

> **按需阅读**：基础读写、行列与工作表管理、`table-get` / `table-put` 类型保真读写、Markdown 互转、保护、身份与 API 限制
> 见 `references/basic-commands.md`。多维表格另见 `../bitable/workflow.md`。

## 目录

- [前置条件与通用约定](#前置条件与通用约定)
- [筛选视图 filter-view](#筛选视图-filter-view5-命令)
- [筛选条件取值](#筛选条件取值filter-view-condition-与-filter-通用)
- [下拉菜单 dropdown](#下拉菜单-dropdownv2-datavalidation4-命令)
- [浮动图片与单元格图片 image](#浮动图片与单元格图片-image)
- [批量样式 batch-set-style](#批量样式-batch-set-style)
- [典型工作流](#典型工作流)
- [踩坑](#踩坑)
- [其他 Sheet 命令](#其他-sheet-命令)
- [权限要求](#权限要求)

## 前置条件与通用约定

- **身份**：所有 sheet 命令支持 `--as bot|user|auto`：
  - 不传 `--as`：User 优先（已 `feishu-cli auth login`），User Token 不可用时 stderr 告警后回退 Bot
  - `--as bot`：已登录也强制 Bot（Bot 自有表格、cron 无人值守）；Bot 不是协作者的表格报 `91403 Forbidden`（实测）
  - `--as user`：强制 User，缺 Token 报错；`--as auto`：User 优先，已配置但刷新失败时 fail-closed，不回退 Bot
  - `sheet create` / `import-md` 以 Bot 身份创建时，自动给当前 CLI 登录用户授予 full_access，结果见 JSON 输出的
    `permission_grant.status`（`granted` / `skipped` / `failed`）
- **表格参数**：可传裸 token，也可直接传 URL（`/sheets/<token>?sheet=<sheetId>`、`/spreadsheets/<token>`、
  `/wiki/<node_token>` 自动换出底层表格）。
- **范围前缀**：可写 sheetId 或子表名（`Sheet1!A1:C10` 自动换算为 sheetId）。不带前缀时依次使用 `--sheet-id` /
  `--sheet-name` / URL `?sheet=` / 唯一子表；表格有多个子表又无法确定时报用法错误（退出码 2）并列出全部子表。

## 命令速查

### 筛选视图 filter-view（5 命令）

filter-view 系列用 `--token`（兼容别名 `--spreadsheet-token`）+ `--sheet-id` 定位，不用位置参数。

```bash
# create —— --name / --filter-view-id 可选（不传时由飞书生成 ID）；--range 前缀可写 sheetId 或子表名
feishu-cli sheet filter-view create --token shtcnxxxxxx --sheet-id 0b1212 \
  --range "0b1212!A1:H14" --name "我的视图"
feishu-cli sheet filter-view create --token shtcnxxxxxx --sheet-id 0b1212 --range "A1:H14"   # 不带前缀自动补 --sheet-id

# get / list / delete
feishu-cli sheet filter-view get    --token shtcnxxxxxx --sheet-id 0b1212 --filter-view-id pH9hbVcCXA
feishu-cli sheet filter-view list   --token shtcnxxxxxx --sheet-id 0b1212 -o json
feishu-cli sheet filter-view delete --token shtcnxxxxxx --sheet-id 0b1212 --filter-view-id pH9hbVcCXA

# update —— --name / --range 至少一个
feishu-cli sheet filter-view update --token shtcnxxxxxx --sheet-id 0b1212 \
  --filter-view-id pH9hbVcCXA --range "0b1212!A1:H20"
```

> `create -o json` 输出 `{"filter_view_id","filter_view_name","range"}`；`list -o json` 是同结构对象的数组。
> `--name` ≤ 100 字符；显式指定 `--filter-view-id` 时必须是 10 位字母数字。

#### 筛选条件 filter-view condition（5 命令）

按列字母定位（`--condition-id E`），一列一个条件。

```bash
feishu-cli sheet filter-view condition create --token shtcnxxxxxx --sheet-id 0b1212 \
  --filter-view-id pH9hbVcCXA --condition-id E --filter-type number --compare-type less --expected '["6"]'
feishu-cli sheet filter-view condition update --token shtcnxxxxxx --sheet-id 0b1212 \
  --filter-view-id pH9hbVcCXA --condition-id E --filter-type number --compare-type greater --expected '["10"]'
feishu-cli sheet filter-view condition get    --token shtcnxxxxxx --sheet-id 0b1212 --filter-view-id pH9hbVcCXA --condition-id E
feishu-cli sheet filter-view condition list   --token shtcnxxxxxx --sheet-id 0b1212 --filter-view-id pH9hbVcCXA
feishu-cli sheet filter-view condition delete --token shtcnxxxxxx --sheet-id 0b1212 --filter-view-id pH9hbVcCXA --condition-id E
```

### 筛选条件取值（filter-view condition 与 filter 通用）

`--filter-type` 决定可用的 `--compare-type`；`--expected` 是 JSON 字符串数组。以下组合均为实测可用：

| `--filter-type` | `--compare-type` | `--expected` |
|---|---|---|
| `number` | `equal` `notEqual` `less` `lessOrEqual` `greater` `greaterOrEqual` `between` `notBetween` | 数字字符串，如 `'["6"]'`；between 类传两个边界 `'["2","6"]'` |
| `text` | `beginsWith` `notBeginsWith` `endsWith` `notEndsWith` `contains` `notContains` | 字符串，如 `'["P0"]'` |
| `multiValue` | `equal` | 要保留的值（精确匹配其中任一），如 `'["P0","P1"]'` |
| `color` | `backColor` `foreColor` | 颜色，如 `'["#FF0000"]'` |

- `text` **没有**等值比较（`equal` / `equals` 报 1310236 Wrong Filter Value）；按值精确筛选用 `multiValue` + `equal`
  （`--filter-type` 取值由 CLI 原样透传给服务端）。
- `lessEqual` / `greaterEqual` / `lessThan` 等写法报 1310236，用 `lessOrEqual` / `greaterOrEqual`。
- `hiddenValue` 实测不可用：filter-view 条件报 `1310251 break change not support hiddenValue filter type`，
  `sheet filter create` 报 1310236。要隐藏某些值，改用 `multiValue` 列出要保留的值。

### 下拉菜单 dropdown（V2 dataValidation，4 命令）

```bash
# set —— 简单选项（CSV 逗号分隔）
feishu-cli sheet dropdown set --token shtcnxxxxxx --range "0b1212!A1:A100" \
  --options "待办,处理中,已完成"

# set —— 多选 + 高亮（colors 数量需与 options 一致，自动开启 highlightValidData）
feishu-cli sheet dropdown set --token shtcnxxxxxx --range "0b1212!B1:B100" \
  --options "P0,P1,P2" --multiple --colors "#FF4D4F,#FAAD14,#52C41A"

# set —— 选项内含逗号 → 必须改用 --options-json（JSON 数组，绕过 CSV 解析）
feishu-cli sheet dropdown set --token shtcnxxxxxx --range "0b1212!C1:C100" \
  --options-json '["a, b","c"]'

# get —— 读取区域的下拉菜单设置（只输出 JSON）
feishu-cli sheet dropdown get --token shtcnxxxxxx --range "0b1212!A1:A100"

# update —— 多范围（--sheet-id + --ranges），支持 --multiple / --colors / --highlight
feishu-cli sheet dropdown update --token shtcnxxxxxx --sheet-id 0b1212 \
  --ranges "0b1212!A1:A100,0b1212!B1:B100" --options "P0,P1,P2" --multiple --colors "#FF4D4F,#FAAD14,#52C41A"

# update —— 仅开启上色高亮（--highlight 是 update 独有，不传 --colors 也能高亮）
feishu-cli sheet dropdown update --token shtcnxxxxxx --sheet-id 0b1212 \
  --ranges "0b1212!A1:A100" --options "P0,P1,P2" --highlight

# delete —— --ranges 逗号分隔，最多 100 个
feishu-cli sheet dropdown delete --token shtcnxxxxxx --ranges "0b1212!A1:A100,0b1212!B1:B100"
```

> `set` 用 `--range`（单个），`update` / `delete` 用 `--ranges`（逗号分隔，`update` 还需 `--sheet-id`）。
> `get` / `update` / `delete` 接受 `--spreadsheet-token` 作为 `--token` 的兼容别名，`set` 只认 `--token`。
> 范围前缀可写 sheetId 或子表名；不带前缀时用 `--token` URL 的 `?sheet=` 或唯一子表补全。

### 浮动图片与单元格图片 image

```bash
# media-upload —— 上传本地图片素材，返回 file_token（再用于 image add）
feishu-cli sheet image media-upload shtcnxxxxxx ./logo.png -o json

# add —— 用 media-upload 返回的 file_token 创建浮动图片（--range 为锚点单元格）
feishu-cli sheet image add shtcnxxxxxx 0b1212 --token <file_token> --range "A1:A1" --width 200 --height 150

# get / list / delete
feishu-cli sheet image get    shtcnxxxxxx 0b1212 ScDmuyHm
feishu-cli sheet image list   shtcnxxxxxx 0b1212 -o json
feishu-cli sheet image delete shtcnxxxxxx 0b1212 ScDmuyHm

# update —— 更新浮动图片锚点 / 尺寸 / 偏移（仅提交显式传入的字段；--range 必须是单格）
feishu-cli sheet image update shtcnxxxxxx 0b1212 ScDmuyHm --width 200 --height 150
feishu-cli sheet image update shtcnxxxxxx 0b1212 ScDmuyHm --range "0b1212!B2:B2" --offset-x 5

# write-image —— HTTPS 图片或本地图片写成原生单元格图片（非浮动图片），自动回读验证
feishu-cli sheet image write-image shtcnxxxxxx 0b1212 --range "A1" --image https://example.com/logo.png
feishu-cli sheet image write-image shtcnxxxxxx 0b1212 --range "Sheet1!B2" --image ./logo.png --name logo.png -o json

# write-batch —— 批量写原生图片单元格（manifest 可为文件、stdin '-' 或行内 JSON）
cat >/tmp/images.json <<'JSON'
[
  {"cell":"B2","url":"https://example.com/1.jpg"},
  {"cell":"B3","path":"/tmp/2.png","name":"product-2.png"}
]
JSON
feishu-cli sheet image write-batch shtcnxxxxxx 0b1212 --manifest /tmp/images.json -o json
cat /tmp/images.json | feishu-cli sheet image write-batch shtcnxxxxxx 0b1212 --manifest - -o json
```

> **单元格图片写入规则（单张与批量通用）**：
> 1. 浮动图片（可拖动、覆盖在单元格上方）≠ 原生单元格图片（`write-image` / `write-batch`，图片作为单元格值嵌入）。
> 2. 严禁用 V2 `sheet write` 写 `=IMAGE(...)` 公式，也严禁在 `sheet import-md` 中使用 Markdown 图片语法 `![]()`；
>    两者都无法生成可稳定渲染的原生图片单元格（易显示为 `#ERROR` 或纯文本链接）。
> 3. 网络图片仅接受 HTTPS，默认单张 ≤ 20 MiB（`write-batch` 可用 `--max-image-bytes` 调整）；企业内网 CDN / 对象存储加 `--allow-private-net`。
> 4. JPEG/PNG/GIF 直接写入；BMP/TIFF/WebP 自动转 PNG（原文件不变）；HEIC/BPG 原样提交，能否写入取决于服务端。
>    `--workers` 只控制下载与预处理并发，写入同一表格时串行。
> 5. 写入后自动通过 V3 `read-rich` 回读校验原生 `image_token`，任一单元格校验失败即非零退出（`write-batch` 输出逐格结果）。
> 6. `media-upload` 对 >20MB 文件自动分片上传，但浮动图片接口实测不接受 >20MB 的图片（1310245），作浮动图片前先压缩。
>    导入型 Office 表格会自动切换素材类型，无需额外参数。

### 批量样式 batch-set-style

```bash
# --data 为 {ranges, style} 对象的 JSON 数组，每个 range 带 sheetId 或子表名前缀
feishu-cli sheet batch-set-style shtcnxxxxxx \
  --data '[{"ranges":["0b1212!A1:A2"],"style":{"font":{"bold":true},"backColor":"#FF0000"}}]'

# 多个范围块
feishu-cli sheet batch-set-style shtcnxxxxxx \
  --data '[{"ranges":["0b1212!A1:A2"],"style":{"font":{"bold":true}}},{"ranges":["Sheet1!B1:B2"],"style":{"backColor":"#00FF00"}}]'
```

> `style` 沿用飞书 V2 `styles_batch_update` 原始结构：`font`（`bold` / `italic` / `fontSize` / `clean`）、`hAlign`、`vAlign`、
> `backColor`、`foreColor`、`borderType`、`borderColor`、`formatter`、`clean`。单范围简单样式用 `sheet style`。

## 典型工作流

### 1. 任务表加状态下拉 + 优先级筛选视图

```bash
TOKEN=shtcnxxxxxx
SHEET=0b1212

# 状态列下拉（A 列）：待办 / 处理中 / 已完成
feishu-cli sheet dropdown set --token $TOKEN --range "$SHEET!A2:A1000" \
  --options "待办,处理中,已完成"

# 优先级列下拉（B 列）+ 颜色高亮
feishu-cli sheet dropdown set --token $TOKEN --range "$SHEET!B2:B1000" \
  --options "P0,P1,P2" --colors "#FF4D4F,#FAAD14,#52C41A"

# 创建 "P0 高优" 筛选视图（范围要含表头行），再写条件：B 列精确等于 P0 用 multiValue + equal
FV=$(feishu-cli sheet filter-view create --token $TOKEN --sheet-id $SHEET \
  --range "$SHEET!A1:H1000" --name "P0 高优" -o json | jq -r '.filter_view_id')
feishu-cli sheet filter-view condition create --token $TOKEN --sheet-id $SHEET \
  --filter-view-id $FV --condition-id B --filter-type multiValue --compare-type equal --expected '["P0"]'
```

### 2. 清理工作表筛选视图

```bash
# list → 拿到 ID → 逐个 delete（delete 无确认门禁，立即执行；先看清 list 结果再删）
for FV in $(feishu-cli sheet filter-view list --token $TOKEN --sheet-id $SHEET -o json | jq -r '.[].filter_view_id'); do
  feishu-cli sheet filter-view delete --token $TOKEN --sheet-id $SHEET --filter-view-id "$FV"
done
```

## 踩坑

- **`sheet find` / `sheet replace` 必须带 `--range`**：不传时服务端报 `99992402 find_condition.range is required`
  （`--help` 里不带 `--range` 的示例实测失败）。全表查找就传覆盖数据区的范围，如 `--range "A1:Z1000"`。
  实测隐藏行不参与匹配与替换。
- **`--options` / `--options-json` 互斥**：同时传会报错；含逗号的选项必须走 `--options-json`，否则会被 CSV 切碎。
- **dropdown 用英文逗号分隔**：`--options` 只识别 ASCII `,`，中文 `，` 会让多个选项合并成一个。
- **dropdown 上限**：接口限制单个下拉最多 500 个选项、每项 ≤ 100 字符（CLI 不做本地校验，超限由服务端报错）；
  `--colors` 个数必须等于选项数（CLI 校验），传 `--colors` 自动开启高亮。
- **filter 与 filter-view 区别**：`sheet filter` 是子表唯一的临时筛选（create/update 必须带 `--col` + `--filter-type`，
  缺了 CLI 直接以用法错误退出，退出码 2）；`filter-view` 是可命名、可多个的筛选视图。多维表格的视图筛选走
  `feishu-cli bitable view view-filter-set`（另一套 DSL）。
- **filter-view 范围不写数据**：`--range` 只圈定视图作用域；写数据的限制见 `references/basic-commands.md` 的「API 限制」。
- **`-o json` 支持面**：`filter-view`（含 `condition get/list/create/update`）、`image get/list/add/update/media-upload/write-image/write-batch`
  支持 `-o json`（默认 `text`）；`dropdown get` 只输出 JSON；`dropdown set/update/delete`、`filter-view delete`、
  `condition delete` 没有 `-o`，只输出成功摘要文本。

## 其他 Sheet 命令

以下能力也属于本工作流，示例与口径见 `references/basic-commands.md`，参数以 `feishu-cli sheet <cmd> --help` 为准：

| 需求 | 命令（`feishu-cli sheet <cmd>`） |
|---|---|
| 创建 / 元信息 | `create` / `get` / `meta` / `list-sheets`（含 `resource_type`、冻结与隐藏状态） |
| 读取（普通 + 富文本） | `read` / `read-plain` / `read-rich`（`read` 的日期单元格默认读出序列号，要显示值加 `--value-render FormattedValue`） |
| 写入 / 追加 / 插入 / 清除 | `write` / `write-rich` / `append` / `append-rich` / `insert` / `clear`（`write` / `append` 超过 5000 行自动分批） |
| 整表按列类型保真读写 | `table-get` / `table-put`（dtype 只有 `int*` / `uint*` / `float*` / `complex*` 是数字，`number` 不是合法 dtype 会按文本写；`--mode append`、`--start-cell`；日期时间保留时分秒） |
| 行列管理 | `add-rows` / `add-cols` / `insert-rows` / `insert-cols` / `delete-rows` / `delete-cols`（`--range "3:5"` / `"B:D"` 1 起始两端包含，或 `--start/--end` 0 起始不含 end；删除支持 `--dry-run`） |
| 隐藏行列 / 行高列宽 / 移动行列 | `update-dimension --range "3:5" --hidden` / `--size 40`；`move-dimension --range "2:3" --target 5`（移到原第 5 行之前） |
| 工作表管理 | `add-sheet` / `copy-sheet` / `delete-sheet`（`--dry-run`）/ `update-sheet`（改名、隐藏、位置、冻结）/ `freeze --rows 1 --cols 1` |
| 单范围样式 / 合并 / 保护 | `style` / `merge` / `unmerge` / `protect` / `unprotect`（多范围批量样式用上文 `batch-set-style`） |
| 查找 / 替换 / 简单筛选 | `find` / `replace`（都必须带 `--range`；`--regex` 正则）/ `filter create/update/get/delete` |
| 导出 / Markdown 导入 | `export`（XLSX / CSV / Markdown；`-o` 是输出文件路径）/ `import-md` |
| 浮动图片与单元格写图 | `image add/get/update/list/delete/media-upload/write-image/write-batch`（见上文） |

> 整表结构化处理优先 `table-get` / `table-put`；只改已知区域时使用范围读写命令。
> 删除类命令（`delete-rows/delete-cols/delete-sheet`、`filter-view delete`、`image delete`、`unprotect`）没有确认门禁，
> 执行即生效且不可撤销；行列与子表删除先 `--dry-run` 核对实际行号。

## 权限要求

- User 身份：`feishu-cli auth login --domain sheets --recommend`（`sheets:spreadsheet:read` / `sheets:spreadsheet:write_only` /
  `sheets:spreadsheet:create` 等）；XLSX/CSV 导出另需 `docs:document:export`、`drive:file:download`。
- Bot 身份：应用开通对应 sheets scope，且 Bot 是目标表格的协作者（或表格由 Bot 自己创建）。
