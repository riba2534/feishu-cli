---
name: feishu-cli-sheet
description: >-
  飞书电子表格高级能力（筛选视图 + 筛选条件 + 下拉单元格 + 浮动图片 + 批量样式）。
  filter-view CRUD 管理筛选视图，filter-view condition CRUD 写筛选条件（V3 API）；
  dropdown set/get/update/delete 管理单元格下拉框（V2 dataValidation）；
  image get/update/media-upload/write-image 操作浮动图片与单元格写图；
  batch-set-style 批量设置多范围单元格样式。
  基础读写（read/write/style/add-rows/add-sheet）仍在 feishu-cli 主命令 sheet/bitable，
  本 skill 专注高级能力。
  当用户请求"筛选视图"、"筛选条件"、"加下拉框"、"数据验证"、"列下拉"、"浮动图片"、"插入图片"、"批量样式"时使用。
argument-hint: filter-view [condition] | dropdown | image | batch-set-style
user-invocable: true
allowed-tools: Bash(feishu-cli sheet:*), Bash(feishu-cli bitable:*), Read
---

# 飞书电子表格高级能力

`feishu-cli sheet` 子命令组的高级能力——**筛选视图 CRUD + 筛选条件 CRUD** + **单元格下拉菜单 CRUD** + **浮动图片 / 单元格写图** + **批量样式**。这些此前多是 `lark-cli` 独占的能力，现已补齐到 `feishu-cli`。

> **范围划分**：基础读写（`sheet read` / `write` / `style` / `add-rows` / `add-sheet` 等）和 V3 富文本走主命令 `feishu-cli sheet` / `feishu-cli bitable`，本 skill **覆盖 filter-view（含 condition）+ dropdown + image + batch-set-style**。其他子命令查询 `feishu-cli sheet --help`。

## 前置条件

- **认证**：`sheets:spreadsheet` scope，User Token 或 App Token 均可。命令默认走 `resolveOptionalUserTokenWithFallback`：
  - 已 `feishu-cli auth login` → 自动用 User Token
  - 未登录或显式 `--user-access-token` 留空 → 落回 App Token（Bot 身份）
- **token / sheet-id 来源**：电子表格 URL `https://xxx.feishu.cn/sheets/<token>?sheet=<sheet-id>` 中分别取。

## 命令速查

### 筛选视图 filter-view（V3 API，5 命令）

```bash
# create —— --name / --filter-view-id 可选，飞书侧自动生成
feishu-cli sheet filter-view create \
  --spreadsheet-token shtcnxxxxxx --sheet-id 0b1212 \
  --range "0b1212!A1:H14" --name "我的视图"

# range 不带 sheetId 前缀时自动补全为 <sheet-id>!<range>
feishu-cli sheet filter-view create --token shtcnxxxxxx --sheet-id 0b1212 --range "A1:H14"

# get —— 取单个视图 id/name/range
feishu-cli sheet filter-view get --token shtcnxxxxxx --sheet-id 0b1212 --filter-view-id pH9hbVcCXA

# update —— --name / --range 至少一个
feishu-cli sheet filter-view update --token shtcnxxxxxx --sheet-id 0b1212 \
  --filter-view-id pH9hbVcCXA --name "新名字"
feishu-cli sheet filter-view update --token shtcnxxxxxx --sheet-id 0b1212 \
  --filter-view-id pH9hbVcCXA --range "0b1212!A1:H20"

# list
feishu-cli sheet filter-view list --token shtcnxxxxxx --sheet-id 0b1212
feishu-cli sheet filter-view list --token shtcnxxxxxx --sheet-id 0b1212 -o json

# delete
feishu-cli sheet filter-view delete --token shtcnxxxxxx --sheet-id 0b1212 --filter-view-id pH9hbVcCXA
```

### 筛选条件 filter-view condition（V3 API，5 命令）

CLI 现已支持给筛选视图写**筛选条件**（按列字母定位），不再只能创建空视图后到 Web UI 配。

```bash
# create —— condition-id 为列字母（如 E）；expected 为筛选参数 JSON 数组
feishu-cli sheet filter-view condition create --token shtcnxxxxxx --sheet-id 0b1212 \
  --filter-view-id pH9hbVcCXA --condition-id E --filter-type number --compare-type less --expected '["6"]'

feishu-cli sheet filter-view condition get    --token shtcnxxxxxx --sheet-id 0b1212 \
  --filter-view-id pH9hbVcCXA --condition-id E
feishu-cli sheet filter-view condition update --token shtcnxxxxxx --sheet-id 0b1212 \
  --filter-view-id pH9hbVcCXA --condition-id E --filter-type number --compare-type less --expected '["6"]'
feishu-cli sheet filter-view condition delete --token shtcnxxxxxx --sheet-id 0b1212 \
  --filter-view-id pH9hbVcCXA --condition-id E
feishu-cli sheet filter-view condition list   --token shtcnxxxxxx --sheet-id 0b1212 --filter-view-id pH9hbVcCXA
```

> `--filter-type` 可选 `hiddenValue / number / text / color`；`--compare-type` 如 `less / beginsWith / between`；`--expected` 是 JSON 数组（如 `'["6"]'`）；`--condition-id` 是列字母（如 `E`）。

**关键参数**：

| flag | 必填 | 说明 |
|---|---|---|
| `--token` / `--spreadsheet-token` | 是 | 电子表格 token（URL `/sheets/<token>`）；`--spreadsheet-token` 对齐官方 lark-cli |
| `--sheet-id` | 是 | 工作表 ID（URL `?sheet=<sheet-id>`） |
| `--range` | create 必填 | `"<sheetId>!A1:H14"`，不带 `!` 前缀自动补 |
| `--name` | 否 | 视图名称 ≤ 100 字符 |
| `--filter-view-id` | create/delete | create 时自定义 ID（10 位字母数字）；delete 时定位视图 |
| `-o json` | 否 | 输出原始 JSON |
| `--user-access-token` | 否 | 显式覆盖登录态 |

底层调用：SDK `client.Sheets.SpreadsheetSheetFilterView.{Create, Query, Delete}`。

### 下拉菜单 dropdown（V2 dataValidation API，4 命令）

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

# get —— 读取区域的下拉菜单设置（range 必须带 sheetId 前缀）
feishu-cli sheet dropdown get --token shtcnxxxxxx --range "0b1212!A1:A100"

# update —— 更新下拉（--sheet-id + --ranges 多范围，支持 --multiple / --colors）
feishu-cli sheet dropdown update --token shtcnxxxxxx --sheet-id 0b1212 \
  --ranges "0b1212!A1:A100,0b1212!B1:B100" --options "P0,P1,P2" --multiple --colors "#FF4D4F,#FAAD14,#52C41A"

# delete —— 删除下拉（--ranges 逗号分隔，每个带前缀，最多 100 个）
feishu-cli sheet dropdown delete --token shtcnxxxxxx --ranges "0b1212!A1:A100,0b1212!B1:B100"
```

> `set` 用 `--range`（单个），`update`/`delete` 用 `--ranges`（多范围逗号分隔，`update` 还需 `--sheet-id`）。

### 浮动图片 image（7 命令）

```bash
# media-upload —— 上传本地图片素材，返回 file_token（再用于 image add）
feishu-cli sheet image media-upload shtcnxxxxxx ./logo.png
feishu-cli sheet image media-upload shtcnxxxxxx ./logo.png --name banner.png -o json

# write-image —— 把本地图片直接写入单元格（值类型为图片，非浮动图片；起止单元格须相同）
feishu-cli sheet image write-image shtcnxxxxxx 0b1212 --range "0b1212!A1" --image ./logo.png

# get —— 获取单个浮动图片
feishu-cli sheet image get shtcnxxxxxx 0b1212 ScDmuyHm

# update —— 更新浮动图片锚点 / 尺寸 / 偏移（仅更新显式传入的字段）
feishu-cli sheet image update shtcnxxxxxx 0b1212 ScDmuyHm --width 200 --height 150
feishu-cli sheet image update shtcnxxxxxx 0b1212 ScDmuyHm --range "0b1212!B2:B2" --offset-x 5

# 原有：add / list / delete
feishu-cli sheet image list shtcnxxxxxx 0b1212
```

> 浮动图片（float image，可拖动覆盖在单元格上）≠ 单元格写图（write-image，图片作为单元格值）。`media-upload` 的 parent_type 固定 `sheet_image`，write-image 的目标范围起止单元格须相同。

### 批量样式 batch-set-style（1 命令）

```bash
# --data 为 {ranges, style} 对象的 JSON 数组，每个 range 须带 sheetId 前缀
feishu-cli sheet batch-set-style shtcnxxxxxx \
  --data '[{"ranges":["0b1212!A1:A2"],"style":{"font":{"bold":true},"backColor":"#FF0000"}}]'

# 多个范围块
feishu-cli sheet batch-set-style shtcnxxxxxx \
  --data '[{"ranges":["0b1212!A1:A2"],"style":{"font":{"bold":true}}},{"ranges":["0b1212!B1:B2"],"style":{"backColor":"#00FF00"}}]'
```

> `style` 字段沿用飞书 V2 `styles_batch_update` 原始结构（`font` / `hAlign` / `vAlign` / `backColor` / `foreColor` / `formatter` / `clean`）。

**关键参数**：

| flag | 必填 | 说明 |
|---|---|---|
| `--token` | 是 | 电子表格 token |
| `--range` | 是 | **必须带 sheetId 前缀**，例如 `"0b1212!A1:A100"`（不带 `!` 直接报错） |
| `--options` | 二选一 | CSV 逗号分隔，每项 ≤ 100 字符，总数 ≤ 500 项 |
| `--options-json` | 二选一 | JSON 数组字符串，选项内含逗号时用；同样受 ≤ 500 项 / 每项 ≤ 100 字符限制 |
| `--multiple` | 否 | 启用多选（默认单选） |
| `--colors` | 否 | RGB hex CSV，长度与 options 一致；传值自动开启高亮 |

底层调用：`POST /open-apis/sheets/v2/spreadsheets/{token}/dataValidation`，`dataValidationType: list`。

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

# 创建"P0 高优"筛选视图 + 写筛选条件（B 列 = P0）
FV=$(feishu-cli sheet filter-view create --token $TOKEN --sheet-id $SHEET \
  --range "$SHEET!A1:H1000" --name "P0 高优" -o json | jq -r '.filter_view_id')
feishu-cli sheet filter-view condition create --token $TOKEN --sheet-id $SHEET \
  --filter-view-id $FV --condition-id B --filter-type text --compare-type equal --expected '["P0"]'
```

### 2. 清理工作表筛选视图

```bash
# list → 拿到 ID → 批量 delete
feishu-cli sheet filter-view list --token $TOKEN --sheet-id $SHEET -o json | \
  jq -r '.[].filter_view_id' | \
  xargs -I{} feishu-cli sheet filter-view delete --token $TOKEN --sheet-id $SHEET --filter-view-id {}
```

## 踩坑

- **`--options` / `--options-json` 互斥**：同时传会报错 `--options 和 --options-json 不能同时使用，请选其一`；含逗号的选项必走 `--options-json`，否则会被 CSV 切碎
- **dropdown 选项数量 ≤ 500 项**：飞书 V2 dataValidation 单次最多 500 个 list 选项（见 `internal/client/sheets.go:2331`），超出 API 直接报错；批量场景请按业务维度拆多列下拉
- **dropdown 每个选项 ≤ 100 字符**：单选项超 100 字符会被服务端拒；如果用 `--options-json` 注入长文案务必先截断或换成"短码 + 注释列"模式
- **dropdown `--range` 必须带 `!` 前缀**：不带前缀直接报错（`--range 必须包含 sheetId 前缀`），不像 filter-view 会自动补
- **`--colors` 长度必须 = options 长度**：例如 3 个选项就要 3 个颜色，少一个报错 `--colors 长度(X) 必须与选项数(Y) 一致`；传 colors 自动开启 `highlightValidData: true`
- **dropdown 用英文逗号分隔 不是中文「，」**：`--options` CSV 解析器只识别 ASCII `,`，中文逗号会让多选项合并成一个，容易踩
- **filter-view 条件已可用 CLI 写**：用 `filter-view condition create/update`（按列字母 `--condition-id` 定位）；多维表格（非电子表格）的复杂条件仍走 **`feishu-cli bitable view view-filter-set`**
- **filter-view 范围 v.s. 单元格写入限制**：filter-view `--range` 仅圈定视图作用域，**不写入数据**；写入受 V3 单 cell ≤ 50000 字符 / 单批 ≤ 5000 cells / 10 ranges 限制（详见主 `feishu-cli sheet` 命令）
- **`-o json` 仅 filter-view 支持**：dropdown set 只回吐成功摘要文本（API 本身只返回 code/msg）

## 何时该转主命令

本 skill 覆盖 `filter-view`（含 `condition`）+ `dropdown` + `image` + `batch-set-style`。`feishu-cli sheet` 下其余子命令全部走主命令，不属于本 skill 范围：

| 需求分组 | 走哪（主命令 `feishu-cli sheet <cmd>`） |
|---|---|
| 创建 / 元信息 | `create` / `get` / `meta` / `list-sheets` |
| 读取（普通 + 富文本） | `read` / `read-plain` / `read-rich` |
| 写入 / 追加 / 插入 / 清除 | `write` / `write-rich` / `append` / `append-rich` / `insert` / `clear` |
| 行列管理 | `add-rows` / `add-cols` / `insert-rows` / `delete-rows` / `delete-cols` |
| 工作表管理 | `add-sheet` / `copy-sheet` / `delete-sheet` |
| 单范围样式 / 合并 / 保护 | `style` / `merge` / `unmerge` / `protect` / `unprotect`（多范围批量样式走本 skill `batch-set-style`） |
| 查找 / 替换 / 简单筛选 | `find` / `replace` / `filter`（注意：与 `filter-view` 不同，`filter` 是临时筛选） |
| 导出 / Markdown 导入 | `export`（XLSX/CSV/MD）/ `import-md`（参考 `feishu-cli-toolkit`） |
| 浮动图片 add/list/delete | `image add` / `image list` / `image delete`（get/update/media-upload/write-image 见本 skill） |
| 多维表格的视图过滤/排序/分组 | `feishu-cli bitable view view-*-set`（语义更强，能配条件） |

> 速查：`feishu-cli sheet --help` 子命令以 `--help` 实测为准；本 skill 负责 `filter-view`（含 `condition`）/ `dropdown` / `image` / `batch-set-style`，其余转主命令。

## 权限要求

- `sheets:spreadsheet`（读写均覆盖；User Token / App Token 均可）

## 底层实现位置

| 命令 | CLI 入口 |
|---|---|
| `filter-view create/list/delete` | `cmd/sheet_filter_view.go` |
| `filter-view get/update` + `filter-view condition *` | `cmd/sheet_filter_view_ext.go` / `cmd/sheet_filter_view_condition.go` |
| `dropdown set` | `cmd/sheet_dropdown.go` |
| `dropdown get/update/delete` | `cmd/sheet_dropdown_ext.go` |
| `image add/list/delete` | `cmd/sheet_image.go` |
| `image get/update/media-upload/write-image` | `cmd/sheet_float_image_ext.go` |
| `batch-set-style` | `cmd/sheet_style_batch.go` |

filter-view 走 SDK `larksheets.SpreadsheetSheetFilterView`；dropdown / batch-set-style 走通用 HTTP 直调 V2 端点（SDK 未封装）。
