# 电子表格详细参考

## 目录

- 通用约定（URL、子表名前缀、`--as`、数字精度、日期读取）
- 创建与元信息（create、get、list-sheets、meta）
- API 版本说明 / V2 API 命令（read、write、append 自动分批）
- 按列类型保真读写（`table-get` / `table-put`：dtype 映射、append、起点单元格）
- Markdown 互转 / V3 API 命令（富文本元素字段）
- 行列操作（`--range` 与 `--start/--end` 口径、插入/删除/隐藏/移动）
- 样式设置 / 合并拆分 / 查找替换（必须带 `--range`）/ 筛选
- 工作表管理（改名、隐藏、冻结）/ 图片 / 工作表保护
- 身份与 User Access Token / API 限制

## 通用约定

- `<token>` 位置参数（以及 `--token` / `--spreadsheet-token`）可传裸 token，也可直接传表格 URL：
  `/sheets/<token>`、`/spreadsheets/<token>`、`/wiki/<node_token>`（自动通过 node_by_token 换出底层表格，非表格节点报错）。
  URL 带 `?sheet=<sheetId>` 时，范围未带子表前缀会默认使用该子表。
- 范围前缀可以是 sheetId，也可以是子表名：`"0b12!A1:C10"` 与 `"Sheet1!A1:C10"` 等价（子表名自动换算为 sheetId；
  名称含空格等字符时可写 `'My Sheet'!A1`）。不带前缀时依次使用 `--sheet-id` / `--sheet-name` / URL `?sheet=` /
  唯一子表；表格有多个子表又无法确定时报错并列出全部子表。
- 身份：所有 sheet 命令支持 `--as bot|user|auto`。不传时沿用旧行为（User 优先，User Token 不可用时告警后回退 Bot）；
  `--as bot` 强制 App 身份（Bot 自有表格、定时任务），`--as user` 强制 User 身份。
- 数字精度：CLI 读写全程保留数字原始字面量（1000000 不会变成 1e+06），但表格本身以双精度存储数字——
  以 JSON 数字写入的长整数会被服务端舍入（实测 19 位 `1234567890123456789` 读回 `1234567890123456800`，17 位同样被舍入）。
  订单号、身份证号等长 ID 和前导零编码要写成 **JSON 字符串**（如 `"1234567890123456789"`、`"0012"`，实测原样读回）；
  用 `table-put` 时把该列 dtype 设为 `string` / `object`，会同时设置 `@` 文本格式。
- 日期读取：`sheet read` 的日期单元格默认返回序列号（如 `45306.354166666664`），要显示值加
  `--value-render FormattedValue`；`sheet export --format markdown` 同样输出序列号；需要 ISO 日期用 `table-get`。

## 创建与元信息

```bash
feishu-cli sheet create --title "周报数据" --folder <folder_token> -o json   # 输出 spreadsheet_token、url（Bot 创建时另有 permission_grant）
feishu-cli sheet get <token> -o json            # 标题、URL、所有者
feishu-cli sheet list-sheets <token> -o json    # 子表 sheet_id、title、行列数、冻结、隐藏、resource_type
feishu-cli sheet meta <token> -o json           # 表格属性 + 各子表 sheetId/title/行列数/冻结
feishu-cli sheet meta <token> --ext-fields protectedRange -o json   # 附带保护范围（protectId）
```

> 取 sheetId 优先用 `list-sheets`。`meta --ext-fields` 实测只接受 `protectedRange`；帮助里的 `mergedCell` 会报
> `90229 unknown extFiled mergedCell`。

## API 版本说明

| API 版本 | 用途 | 数据格式 |
|---------|------|---------|
| V2 | 基础读写（简单数据） | 二维数组 `[["A1","B1"],["A2","B2"]]` |
| V3 | 富文本读写（格式化数据） | 三维数组 `[[[[{"type":"text","text":{"text":"Hello"}}]]]]` |

## V2 API 命令

### 读取单元格

```bash
feishu-cli sheet read <token> "Sheet1!A1:C10"    # 子表名前缀
feishu-cli sheet read <token> "<sheetId>!A:C"    # 整列
feishu-cli sheet read <token> "<sheetId>!1:3"    # 整行
feishu-cli sheet read <token> "A1:C10" --sheet-name "数据"
```

### 写入单元格

```bash
feishu-cli sheet write <token> "Sheet1!A1:B2" --data '[["姓名","年龄"],["张三",25]]'

# 大数据量：只写左上角单元格，超过 5000 行 / 100 列自动分批
feishu-cli sheet write <token> "Sheet1!A1" --data-file rows.json
```

接口单次最多写 5000 行 × 100 列；超限时 CLI 以范围左上角为锚点自动拆批（stderr 提示批数，JSON 输出带 `batches`）。
显式声明的范围小于数据时直接报错，不会越界写。

### 追加数据

```bash
feishu-cli sheet append <token> "Sheet1!A:B" --data '[["新数据1","新数据2"]]'
```

超过 5000 行自动分批，每批紧接上一批的实际写入位置（stderr 提示批数，JSON 输出带 `batches`）；超过 100 列无法分批追加
（会错位），改用 `sheet write`。

## 按列类型保真读写（table-get / table-put）

形状对齐 pandas `to_json(orient="split")`：`{"sheets":[{"columns":[...],"data":[[...]],"dtypes":{...},"formats":{...}}]}`。
`table-get` 的输出可以直接（或修改后）喂给 `table-put`，适合"读出 → 改 → 写回"。

```bash
feishu-cli sheet table-get <token> <sheet_id> > t.json                 # 缺省读整张已用区域，自动裁空行空列
feishu-cli sheet table-get <token> <sheet_id> --range A1:D50 --no-header
feishu-cli sheet table-put <token> <sheet_id> --sheets-file t.json      # 默认 overwrite，从 A1 起覆盖
feishu-cli sheet table-put <token> <sheet_id> --sheets-file more.json --mode append      # 写到已有数据最后一行之后
feishu-cli sheet table-put <token> <sheet_id> --sheets-file t.json --start-cell C3 -o json
feishu-cli sheet table-put <token> <sheet_id> \
  --sheets '{"sheets":[{"columns":["id","amount","date"],"data":[["A001",12.5,"2024-01-15T08:30:00"]],"dtypes":{"amount":"float64","date":"datetime64[ns]"}}]}'
```

dtype 映射（实测）：

| dtype | 写入为 |
|---|---|
| `int*` / `uint*` / `float*` / `complex*` | 数字（写前重置该列残留的 `@` 文本格式） |
| `bool` / `boolean` | 布尔 |
| `datetime*` | 真日期（Excel 序列号 + 日期 formatter）；带时分秒的值保留时间，格式 `yyyy/MM/dd HH:mm:ss` |
| 其他（`object` / `string` / 未声明 / 拼错） | 文本，并设置 `@` 格式（长 ID、前导零安全） |

- `"number"` **不是**合法 dtype，会按文本写入（实测读回为 `string`）；数字列写 `"int64"` / `"float64"`。
- `overwrite` 只覆盖写入的矩形区域，不清除区域外旧数据；`append` 默认不写表头（空子表首次追加自动写），
  `--start-cell` 在 append 模式只取列。子表行列不足时自动扩容；单批 ≤ 5000 单元格，超出自动分批。
- 一次只能写一个 sheet（payload 含多个 sheet 报错）。number / date 列的空字符串写为空单元格。
- `table-get` 推断：数字 → `float64`，日期 → `datetime64[ns]`（带时间的列 `formats` 为 `yyyy-mm-dd hh:mm:ss`），
  全列 TRUE/FALSE → `bool`，混合类型 → `object`。

## Markdown 互转

### 从 Markdown 表格创建电子表格

```bash
# 默认提取第一张 GFM 表格，标题使用文件名
feishu-cli sheet import-md report.md

# 指定标题、目标文件夹
feishu-cli sheet import-md report.md --title "Q1 销售数据" --folder <folder_token>

# 文件中有多张表时选择第 N 张（0-based）
feishu-cli sheet import-md report.md --table-index 1
```

`sheet import-md` 只提取 GFM 表格，忽略对齐标记；不规则行会按最长行补空字符串。适合把报告里的数据表转成可在线筛选、排序的飞书电子表格。

### 导出电子表格为 Markdown

```bash
# 导出所有可见工作表
feishu-cli sheet export <spreadsheet_token> --format markdown -o report.md

# 只导出指定工作表；md 是 markdown 的别名
feishu-cli sheet export <spreadsheet_token> --format md --sheet-id <sheet_id> -o sheet.md
```

Markdown 导出直接读取工作表数据并写出 Markdown 表格；不指定 `--sheet-id` 时导出所有可见工作表。XLSX/CSV 导出仍使用飞书异步导出任务，CSV 必须指定 `--sheet-id`。

## V3 API 命令

### 读取（纯文本/富文本）

```bash
# 纯文本模式（范围不带前缀时自动补 <sheet_id>）
feishu-cli sheet read-plain <token> <sheet_id> "A1:C10"

# 富文本模式（返回完整格式信息）
feishu-cli sheet read-rich <token> <sheet_id> "<sheet_id>!A1:C10"
```

### 写入富文本

```bash
feishu-cli sheet write-rich <token> <sheet_id> --data-file data.json
```

data.json 格式示例（value_ranges JSON 数组；字段名为 snake_case，写错字段会被服务端静默丢弃成空单元格）：
```json
[
  {
    "range": "Sheet1!A1:B2",
    "values": [
      [
        [{"type": "text", "text": {"text": "加粗文本", "segment_style": {"style": {"bold": true}, "affected_text": "加粗文本"}}}],
        [{"type": "text", "text": {"text": "普通文本"}}]
      ],
      [
        [{"type": "link", "link": {"text": "飞书", "link": "https://www.feishu.cn"}}],
        [{"type": "formula", "formula": {"formula": "=SUM(B2:B5)"}}]
      ]
    ]
  }
]
```

### V3 富文本元素类型

| 类型 | 说明 | 主要字段 |
|------|------|---------|
| `text` | 文本 | `text.text`，局部样式 `text.segment_style.style.bold/italic/underline/strike_through/fore_color/font_size` |
| `value` | 数值 | `value.value`（数字字符串，如 `"123"`） |
| `date_time` | 日期时间 | `date_time.date_time` |
| `mention_user` | @用户 | `mention_user.user_id`、`mention_user.notify` |
| `mention_document` | @文档 | `mention_document.token`、`mention_document.object_type` |
| `image` | 图片 | `image.image_token` |
| `file` | 文件 | `file.file_token`、`file.name` |
| `link` | 链接 | `link.text`、`link.link` |
| `formula` | 公式 | `formula.formula`（读取时另有 `formula_value`） |
| `reminder` | 提醒 | `reminder.notify_date_time`、`reminder.notify_strategy` |

### 插入/追加/清除（V3）

`insert` / `append-rich` 的 `--data-file` 读取的是单个范围的三维 `values` 数组；不要传 `write-rich` 的 value_ranges 包装对象。

```bash
# 在指定位置插入数据（--simple 时数字按原始字面量写入）
feishu-cli sheet insert <token> <sheet_id> "A1:B2" --data '[["a", 1000000]]' --simple
feishu-cli sheet insert <token> <sheet_id> "Sheet1!A1:B2" --data-file data.json

# 追加富文本
feishu-cli sheet append-rich <token> <sheet_id> "A1:B2" --data-file data.json

# 清除范围内容
feishu-cli sheet clear <token> <sheet_id> "A1:C10"
```

## 行列操作

行列命令有两种范围写法（二选一）：

- `--range`：A1 写法，1 起始、两端包含，与表格行号/列字母一致：`"3:5"`、`"3"`（行），`"B:D"`、`"B"`（列）。推荐。
- `--start/--end`：0 起始、`--end` 不包含（保持原有语义）；省略 `--end` 时只操作 1 行/列。

CLI 内部换算成各接口口径（删除/更新/保护接口为 1 起始两端包含，插入接口为 0 起始不含 end），输出显示实际行号。

```bash
# 添加行/列（追加到末尾）
feishu-cli sheet add-rows <token> <sheet_id> --count 5
feishu-cli sheet add-cols <token> <sheet_id> --count 3

# 插入：--range 表示插入后新行/列所在位置
feishu-cli sheet insert-rows <token> <sheet_id> --range "3:4"          # 在原第 3 行前插入 2 行
feishu-cli sheet insert-rows <token> <sheet_id> --start 2 --end 4      # 同上（0 起始写法）
feishu-cli sheet insert-cols <token> <sheet_id> --range "C" --inherit-style BEFORE

# 删除（不可撤销，先 --dry-run 确认实际删除范围）
feishu-cli sheet delete-rows <token> <sheet_id> --range "3:5" --dry-run
feishu-cli sheet delete-rows <token> <sheet_id> --range "3:5"          # 删除第 3、4、5 行
feishu-cli sheet delete-rows <token> <sheet_id> --start 2 --end 5      # 同上（0 起始写法）
feishu-cli sheet delete-cols <token> <sheet_id> --range "B:D"

# 隐藏 / 取消隐藏 / 行高列宽
feishu-cli sheet update-dimension <token> <sheet_id> --range "3:5" --hidden
feishu-cli sheet update-dimension <token> <sheet_id> --range "B:D" --hidden=false
feishu-cli sheet update-dimension <token> <sheet_id> --range "1" --size 40

# 移动：--target 为「移动到原第 N 行 / 原 X 列之前」
feishu-cli sheet move-dimension <token> <sheet_id> --range "2:3" --target 5   # 1..6 行变为 1,4,2,3,5,6
feishu-cli sheet move-dimension <token> <sheet_id> --range "B" --target A
```

## 样式设置

```bash
feishu-cli sheet style <token> "Sheet1!A1:C3" \
  --bold \
  --italic \
  --bg-color "#FFFF00" \
  --fore-color "#FF0000"
```

## 合并/拆分单元格

```bash
feishu-cli sheet merge <token> "Sheet1!A1:B2"
feishu-cli sheet unmerge <token> "Sheet1!A1:B2"
```

## 查找替换

`--range` 实际必填：不传时服务端报 `99992402 find_condition.range is required`。全表操作传覆盖数据区的范围。

```bash
feishu-cli sheet find <token> <sheet_id> "关键词" --range "A1:C10" -o json
feishu-cli sheet replace <token> <sheet_id> "旧文本" "新文本" --range "A1:C10"
feishu-cli sheet replace <token> <sheet_id> "[0-9]{4}" "****" --regex --range "A1:Z1000"
```

> 输出 `matched_cells`（命中单元格）；实测隐藏行不参与查找与替换。

## 筛选（filter，每个子表一个）

```bash
# 接口要求同时给出范围、条件列与条件（缺 --col/--filter-type 时 CLI 以用法错误退出）
feishu-cli sheet filter create <token> <sheet_id> "A1:E100" --col B --filter-type number --compare-type less --expected '["6"]'
feishu-cli sheet filter update <token> <sheet_id> --col B --filter-type number --compare-type greater --expected '["5"]'
feishu-cli sheet filter create <token> <sheet_id> "A1:E100" --col C --filter-type multiValue --compare-type equal --expected '["已完成"]'
feishu-cli sheet filter get <token> <sheet_id> -o json      # {"range","filtered_out_rows":[被隐藏的行号]}
feishu-cli sheet filter delete <token> <sheet_id>
```

> `--filter-type` × `--compare-type` 的可用组合见 `../workflow.md` 的「筛选条件取值」（text 没有等值比较，精确匹配用
> `multiValue` + `equal`；`hiddenValue` 实测不可用）。

## 工作表管理

```bash
# 添加/删除/复制工作表
feishu-cli sheet add-sheet <token> --title "新工作表"
feishu-cli sheet delete-sheet <token> <sheet_id> --dry-run   # 删除不可撤销，先预览
feishu-cli sheet delete-sheet <token> <sheet_id>
feishu-cli sheet copy-sheet <token> <sheet_id> [--title "副本"]

# 改名 / 隐藏 / 移动位置（0 起始）/ 冻结
feishu-cli sheet update-sheet <token> <sheet_id> --title "2024 汇总"
feishu-cli sheet update-sheet <token> <sheet_id> --hidden            # --hidden=false 取消隐藏
feishu-cli sheet update-sheet <token> <sheet_id> --index 0
feishu-cli sheet freeze <token> <sheet_id> --rows 1 --cols 1         # 0 表示取消冻结

# 列出子表（含 resource_type：sheet / bitable 等）
feishu-cli sheet list-sheets <token> -o json
```

## 图片

```bash
# 原生单元格图片（图片作为单元格值）：直接传 HTTPS URL 或本地路径，自动上传并回读校验
feishu-cli sheet image write-image <token> <sheet_id> --range "A1" --image ./logo.png

# 浮动图片：先 media-upload 拿 file_token，再 add（>20MB 会分片上传，但浮动图片接口实测不接受 >20MB 图片）
feishu-cli sheet image media-upload <token> ./logo.png -o json
feishu-cli sheet image add <token> <sheet_id> --token <file_token> --range "A1:A1" --width 200 --height 150
feishu-cli sheet image list <token> <sheet_id>
feishu-cli sheet image delete <token> <sheet_id> <float_image_id>
```

批量写图与写图规则见 `../workflow.md` 的「浮动图片与单元格图片 image」。

## 工作表保护

只能保护整行/整列。范围写法同行列命令：`--range`（1 起始两端包含）或 `--dimension` + `--start/--end`（0 起始、`--end` 不包含）。

```bash
feishu-cli sheet protect <token> <sheet_id> --range "1:5"                          # 保护前 5 行
feishu-cli sheet protect <token> <sheet_id> --dimension ROWS --start 0 --end 5     # 同上
feishu-cli sheet protect <token> <sheet_id> --range "A:C" --users ou_xxx,ou_yyy --lock-info "财务数据"
feishu-cli sheet unprotect <token> <protect_id...>
```

> `protect` 返回 `protect_ids`，把它传给 `unprotect` 即可解除（可一次传多个）。`--users` 为除所有者外允许编辑的用户，
> ID 类型由 `--user-id-type`（open_id / union_id，默认 open_id）指定。

## 身份与 User Access Token

所有 sheet 命令均支持 `--as bot|user|auto` 与 `--user-access-token`：

```bash
feishu-cli sheet list-sheets <token> --as bot    # 已登录也强制 Bot（Bot 自有表格 / 无人值守）
feishu-cli sheet read <token> "Sheet1!A1:C10" --as user   # 强制 User，缺 Token 直接报错
```

`sheet create` / `sheet import-md` 以 Bot 身份创建时，会自动给当前登录用户授予 full_access（结果见 JSON 的
`permission_grant.status`：`granted` / `skipped` / `failed`）。Bot 不是协作者的表格以 `--as bot` 访问会报 `91403 Forbidden`。

`--user-access-token` 用于以用户身份访问无 App 权限但用户有权限的表格。

```bash
# 通过参数指定
feishu-cli sheet read <token> "Sheet1!A1:C10" --user-access-token "u-xxxx"

# 通过环境变量
export FEISHU_USER_ACCESS_TOKEN="u-xxxx"
feishu-cli sheet read <token> "Sheet1!A1:C10"
```

**Token 读取优先级**：

1. `--user-access-token` 命令行参数
2. `FEISHU_USER_ACCESS_TOKEN` 环境变量
3. 当前 profile 的 `token.json`（`auth login` 保存，过期自动刷新）
4. 配置文件中的 `user_access_token`

不传 `--as` 且以上都不可用时回退 App Token（Bot）；Token 损坏或刷新失败会先在 stderr 告警。

## API 限制

| 限制 | 说明 |
|------|------|
| V2 写入 / 追加 | 单次最多 5000 行 × 100 列（`write` / `append` 自动分批） |
| V3 写入 | 单次最多 5000 个单元格、10 个范围 |
| 单元格内容 | 最大 50000 字符（建议 ≤ 40000） |
| 频率限制 | 100 次/秒 |
| 数字精度 | 表格以双精度存储数字，JSON 数字超过约 15 位有效数字会被舍入；长 ID 写成 JSON 字符串 |
| 范围格式 | `<sheetId 或子表名>!A1:C10`，支持整列 `A:C` 和整行 `1:3` |
