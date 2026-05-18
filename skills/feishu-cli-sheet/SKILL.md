---
name: feishu-cli-sheet
description: >-
  飞书电子表格高级能力（v1.23+ 新增筛选视图 + 下拉单元格）。
  filter-view create/list/delete 管理筛选视图（V3 SpreadsheetSheetFilterView）；
  dropdown set 给单元格设下拉框（V2 dataValidation HTTP 直调）。
  基础读写（read/write/style/add-rows/add-sheet）仍在 feishu-cli 主命令 sheet/bitable，
  本 skill 专注 v1.23 新增高级能力。
  当用户请求"筛选视图"、"加下拉框"、"数据验证"、"列下拉"时使用。
argument-hint: filter-view create/list/delete | dropdown set
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书电子表格高级能力（v1.23+）

`feishu-cli sheet` 子命令组的 v1.23 新增高级能力——**筛选视图 CRUD** + **单元格下拉菜单**。两块此前是 `lark-cli` 独占的能力，现已补齐到 `feishu-cli`。

> **范围划分**：基础读写（`sheet read` / `write` / `style` / `add-rows` / `add-sheet` 等）和 V3 富文本走主命令 `feishu-cli sheet` / `feishu-cli bitable`，本 skill **只覆盖 v1.23 新增的 filter-view + dropdown**。其他子命令查询 `feishu-cli sheet --help`。

## 前置条件

- **认证**：`sheets:spreadsheet` scope，User Token 或 App Token 均可。命令默认走 `resolveOptionalUserTokenWithFallback`：
  - 已 `feishu-cli auth login` → 自动用 User Token
  - 未登录或显式 `--user-access-token` 留空 → 落回 App Token（Bot 身份）
- **token / sheet-id 来源**：电子表格 URL `https://xxx.feishu.cn/sheets/<token>?sheet=<sheet-id>` 中分别取。

## 命令速查

### 筛选视图 filter-view（V3 API，3 命令）

```bash
# create —— --name / --filter-view-id 可选，飞书侧自动生成
feishu-cli sheet filter-view create \
  --token shtcnxxxxxx --sheet-id 0b1212 \
  --range "0b1212!A1:H14" --name "我的视图"

# range 不带 sheetId 前缀时自动补全为 <sheet-id>!<range>
feishu-cli sheet filter-view create --token shtcnxxxxxx --sheet-id 0b1212 --range "A1:H14"

# list
feishu-cli sheet filter-view list --token shtcnxxxxxx --sheet-id 0b1212
feishu-cli sheet filter-view list --token shtcnxxxxxx --sheet-id 0b1212 -o json

# delete
feishu-cli sheet filter-view delete --token shtcnxxxxxx --sheet-id 0b1212 --filter-view-id pH9hbVcCXA
```

**关键参数**：

| flag | 必填 | 说明 |
|---|---|---|
| `--token` | 是 | 电子表格 token（URL `/sheets/<token>`） |
| `--sheet-id` | 是 | 工作表 ID（URL `?sheet=<sheet-id>`） |
| `--range` | create 必填 | `"<sheetId>!A1:H14"`，不带 `!` 前缀自动补 |
| `--name` | 否 | 视图名称 ≤ 100 字符 |
| `--filter-view-id` | create/delete | create 时自定义 ID（10 位字母数字）；delete 时定位视图 |
| `-o json` | 否 | 输出原始 JSON |
| `--user-access-token` | 否 | 显式覆盖登录态 |

底层调用：SDK `client.Sheets.SpreadsheetSheetFilterView.{Create, Query, Delete}`。

### 下拉菜单 dropdown（V2 dataValidation API，1 命令）

```bash
# 简单选项（CSV 逗号分隔）
feishu-cli sheet dropdown set --token shtcnxxxxxx --range "0b1212!A1:A100" \
  --options "待办,处理中,已完成"

# 多选 + 高亮（colors 数量需与 options 一致，自动开启 highlightValidData）
feishu-cli sheet dropdown set --token shtcnxxxxxx --range "0b1212!B1:B100" \
  --options "P0,P1,P2" --multiple --colors "#FF4D4F,#FAAD14,#52C41A"

# 选项内含逗号 → 必须改用 --options-json（JSON 数组，绕过 CSV 解析）
feishu-cli sheet dropdown set --token shtcnxxxxxx --range "0b1212!C1:C100" \
  --options-json '["a, b","c"]'
```

**关键参数**：

| flag | 必填 | 说明 |
|---|---|---|
| `--token` | 是 | 电子表格 token |
| `--range` | 是 | **必须带 sheetId 前缀**，例如 `"0b1212!A1:A100"`（不带 `!` 直接报错） |
| `--options` | 二选一 | CSV 逗号分隔，每项 ≤ 100 字符 |
| `--options-json` | 二选一 | JSON 数组字符串，选项内含逗号时用 |
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

# 创建"P0 高优"筛选视图
feishu-cli sheet filter-view create --token $TOKEN --sheet-id $SHEET \
  --range "$SHEET!A1:H1000" --name "P0 高优"
# 在飞书 UI 里给该视图配置筛选条件（CLI 暂不支持写条件，只能创建空视图后 UI 配）
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
- **dropdown `--range` 必须带 `!` 前缀**：不带前缀直接报错（`--range 必须包含 sheetId 前缀`），不像 filter-view 会自动补
- **`--colors` 长度必须 = options 长度**：例如 3 个选项就要 3 个颜色，少一个报错 `--colors 长度(X) 必须与选项数(Y) 一致`；传 colors 自动开启 `highlightValidData: true`
- **dropdown 用英文逗号分隔 不是中文「，」**：`--options` CSV 解析器只识别 ASCII `,`，中文逗号会让多选项合并成一个，容易踩
- **filter-view 只能创建空视图，无法 CLI 配条件**：飞书 V3 SDK 暂未暴露写筛选条件的接口，创建后需到飞书 Web UI 里手动配筛选；想用 CLI 写复杂条件请走 **`feishu-cli bitable view view-filter-set`**（多维表格而非电子表格）
- **filter-view 范围 v.s. 单元格写入限制**：filter-view `--range` 仅圈定视图作用域，**不写入数据**；写入受 V3 单 cell ≤ 50000 字符 / 单批 ≤ 5000 cells / 10 ranges 限制（详见主 `feishu-cli sheet` 命令）
- **`-o json` 仅 filter-view 支持**：dropdown set 只回吐成功摘要文本（API 本身只返回 code/msg）

## 何时该转主命令

| 需求 | 走哪 |
|---|---|
| 读单元格、整行整列、富文本 | `feishu-cli sheet read` / `read-rich` |
| 写入数据、追加行 | `feishu-cli sheet write` / `append` / `add-rows` |
| 单元格样式、合并、保护、查找替换 | `feishu-cli sheet style` / `merge` / `protect` / `find` / `replace` |
| 工作表增删改、复制、导出 | `feishu-cli sheet add-sheet` / `copy-sheet` / `export` |
| Markdown 表格批量导入 | `feishu-cli sheet import-md`（参考 `feishu-cli-toolkit`） |
| 多维表格的视图过滤/排序/分组 | `feishu-cli bitable view view-*-set`（语义更强，能配条件） |

## 权限要求

- `sheets:spreadsheet`（读写均覆盖；User Token / App Token 均可）

## 底层实现位置

| 命令 | 客户端实现 | CLI 入口 |
|---|---|---|
| `filter-view create/list/delete` | `internal/client/sheets.go: CreateFilterView/ListFilterViews/DeleteFilterView` | `cmd/sheet_filter_view.go` |
| `dropdown set` | `internal/client/sheets.go: SetDropdown` | `cmd/sheet_dropdown.go` |

filter-view 走 SDK `larksheets.SpreadsheetSheetFilterView`；dropdown 走通用 HTTP（`v2APICallWithToken`）直调 V2 dataValidation 端点（SDK 未封装）。
