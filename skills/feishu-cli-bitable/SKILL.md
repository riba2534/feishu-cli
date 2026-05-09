---
name: feishu-cli-bitable
description: >-
  飞书多维表格（Bitable/Base）操作。底层使用 base/v3 新 API，支持视图完整配置写入、
  记录 upsert、记录修改历史、角色 CRUD、高级权限开关、数据聚合查询等。
  当用户请求"创建多维表格"、"操作数据表"、"添加记录"、"查询记录"、"管理字段"、
  "多维表格"、"base"、"bitable"、"数据表"、"视图排序"、"视图过滤"、"视图分组"、
  "角色"、"role"、"高级权限"、"advperm"、"数据聚合"、"data query"、
  "复制多维表格"时使用。
argument-hint: "[base_token] [table_id]"
user-invocable: true
allowed-tools: Bash, Read, Write
---

# 飞书多维表格（Bitable / Base）

通过 **base/v3 API** 操作飞书多维表格。`bitable` 也支持 `base` 别名。

> **API 切换**：此技能已从旧的 `bitable/v1` 切换到新的 `base/v3`。字段名 `app_token` 和 `base_token` 在飞书文档里是同一个值的两种叫法（老 v1 叫 app_token，新 v3 叫 base_token），CLI 只认 **`--base-token`**（`--app-token` 已删除）。从多维表格 URL 里 `/base/<token>` 或 `/bitable/<token>` 片段里取 token 即可。

## 前置条件

- **认证**：所有命令默认使用 **User Access Token**（执行 `feishu-cli auth login` 登录）
- **App 凭证**：应用 App ID + App Secret（base/v3 需要 `X-App-Id` header，自动注入）

## 命令速查

### 基础（3 命令）

```bash
# 创建多维表格
feishu-cli bitable create --name "项目管理" --time-zone Asia/Shanghai
feishu-cli bitable create --name "销售" --folder-token fldxxx

# 获取多维表格信息
feishu-cli bitable get --base-token bscnxxxx

# 复制多维表格
feishu-cli bitable copy --base-token bscnxxxx --name "副本"
feishu-cli bitable copy --base-token bscnxxxx --name "空白副本" --without-content
```

### 数据表 table（5 命令）

```bash
feishu-cli bitable table list   --base-token bscnxxxx
feishu-cli bitable table get    --base-token bscnxxxx --table-id tblxxx
feishu-cli bitable table create --base-token bscnxxxx --name "任务表"
feishu-cli bitable table create --base-token bscnxxxx --config-file table.json
feishu-cli bitable table update --base-token bscnxxxx --table-id tblxxx --name "新名字"
feishu-cli bitable table delete --base-token bscnxxxx --table-id tblxxx
```

### 字段 field（6 命令）

```bash
feishu-cli bitable field list           --base-token xxx --table-id tblxxx
feishu-cli bitable field get            --base-token xxx --table-id tblxxx --field-id fldxxx
feishu-cli bitable field create         --base-token xxx --table-id tblxxx --config-file field.json
feishu-cli bitable field update         --base-token xxx --table-id tblxxx --field-id fldxxx --config '...'
feishu-cli bitable field delete         --base-token xxx --table-id tblxxx --field-id fldxxx
feishu-cli bitable field search-options --base-token xxx --table-id tblxxx --field-id fldxxx --query "关键词"
```

### 记录 record（8 命令）

```bash
feishu-cli bitable record list        --base-token xxx --table-id tblxxx --view-id viewxxx --limit 100
feishu-cli bitable record get         --base-token xxx --table-id tblxxx --record-id recxxx
feishu-cli bitable record search      --base-token xxx --table-id tblxxx --config-file search.json

# upsert：不传 --record-id 则 POST 创建；传 --record-id 则 PATCH 更新（官方无专用 upsert 端点）
feishu-cli bitable record upsert      --base-token xxx --table-id tblxxx --config '{"fields":{"名称":"测试"}}'
feishu-cli bitable record upsert      --base-token xxx --table-id tblxxx --record-id recxxx --config '{"fields":{"状态":"完成"}}'

feishu-cli bitable record batch-create --base-token xxx --table-id tblxxx --config-file records.json
feishu-cli bitable record batch-update --base-token xxx --table-id tblxxx --config-file records.json
feishu-cli bitable record delete      --base-token xxx --table-id tblxxx --record-id recxxx

# batch-delete：POST /records/batch_delete，单次最多 500 条；--record-ids CSV 或 --from-file 任选其一
feishu-cli bitable record batch-delete --base-token xxx --table-id tblxxx --record-ids rec_1,rec_2,rec_3
feishu-cli bitable record batch-delete --base-token xxx --table-id tblxxx --from-file ids.txt   # 每行一个 record_id

# history-list：GET + query params（不是 POST body），--record-id 必填
feishu-cli bitable record history-list --base-token xxx --table-id tblxxx --record-id recxxx
feishu-cli bitable record history-list --base-token xxx --table-id tblxxx --record-id recxxx --page-size 50 --max-version 20
```

### 视图 view（5 命令 + 12 配置命令）

```bash
# 基础 CRUD
feishu-cli bitable view list   --base-token xxx --table-id tblxxx
feishu-cli bitable view get    --base-token xxx --table-id tblxxx --view-id viewxxx
feishu-cli bitable view create --base-token xxx --table-id tblxxx --name "看板视图" --view-type kanban
feishu-cli bitable view delete --base-token xxx --table-id tblxxx --view-id viewxxx
feishu-cli bitable view rename --base-token xxx --table-id tblxxx --view-id viewxxx --name "新名字"

# 视图配置 get/set（6 种 × 2 = 12 命令）— set 方法是 PUT（全量替换）
feishu-cli bitable view view-filter-get        --base-token xxx --table-id tblxxx --view-id viewxxx
feishu-cli bitable view view-filter-set        --base-token xxx --table-id tblxxx --view-id viewxxx \
  --config '{"conjunction":"and","conditions":[{"field_id":"fld1","operator":"is","value":["进行中"]}]}'

feishu-cli bitable view view-sort-get          --base-token xxx --table-id tblxxx --view-id viewxxx
# sort/group 的 --config 可传数组，自动包装为 {"sort_config":[...]} / {"group_config":[...]}
feishu-cli bitable view view-sort-set          --base-token xxx --table-id tblxxx --view-id viewxxx \
  --config '[{"field_id":"fld1","desc":false}]'

feishu-cli bitable view view-group-get         --base-token xxx --table-id tblxxx --view-id viewxxx
feishu-cli bitable view view-group-set         --base-token xxx --table-id tblxxx --view-id viewxxx \
  --config '[{"field_id":"fld1"}]'

feishu-cli bitable view view-visible-fields-get --base-token xxx --table-id tblxxx --view-id viewxxx
# visible-fields 必须传完整对象（不会自动包装）
feishu-cli bitable view view-visible-fields-set --base-token xxx --table-id tblxxx --view-id viewxxx \
  --config '{"visible_fields":["fld1","fld2"]}'

feishu-cli bitable view view-timebar-get       --base-token xxx --table-id tblxxx --view-id viewxxx
feishu-cli bitable view view-timebar-set       --base-token xxx --table-id tblxxx --view-id viewxxx \
  --config '{"start_field_id":"fld_start","end_field_id":"fld_end","title_field_id":"fld_title"}'

feishu-cli bitable view view-card-get          --base-token xxx --table-id tblxxx --view-id viewxxx
feishu-cli bitable view view-card-set          --base-token xxx --table-id tblxxx --view-id viewxxx \
  --config '{"cover_field_id":"fld1","display_fields":["fld2","fld3"]}'
```

> **视图配置自动包装规则**（减少用户样板）：
> - `view-sort-set` 可直接传 `[{...}]` 数组，自动包成 `{"sort_config":[...]}`
> - `view-group-set` 可直接传 `[{...}]` 数组，自动包成 `{"group_config":[...]}`
> - 其他配置（filter/visible-fields/timebar/card）必须传完整对象

**视图配置 JSON Schema 速查**：

```jsonc
// view-filter（过滤条件）
{
  "filter_info": {
    "conjunction": "and",
    "conditions": [
      {"field_id": "fldxxx", "operator": "is", "value": ["进行中"]}
    ]
  }
}

// view-sort（排序）
{
  "sort_config": [
    {"field_id": "fldxxx", "desc": false}
  ]
}

// view-group（分组）
{
  "group_config": [{"field_id": "fldxxx"}]
}

// view-visible-fields（可见字段）
{
  "view_field": [{"field_id": "fld1", "visible": true}, {"field_id": "fld2", "visible": false}]
}

// view-timebar（甘特图时间轴）
{"timebar": {"start_field_id": "fld1", "end_field_id": "fld2", "title_field_id": "fld3"}}

// view-card（卡片/画册视图）
{"card": {"cover_field_id": "fld1", "display_fields": ["fld2", "fld3"]}}
```

### 角色 role（5 命令）

```bash
feishu-cli bitable role list   --base-token xxx
feishu-cli bitable role get    --base-token xxx --role-id roxxx
feishu-cli bitable role create --base-token xxx --config-file role.json
feishu-cli bitable role update --base-token xxx --role-id roxxx --config '...'
feishu-cli bitable role delete --base-token xxx --role-id roxxx
```

### 高级权限 advperm（2 命令）

```bash
feishu-cli bitable advperm enable  --base-token xxx
feishu-cli bitable advperm disable --base-token xxx
```

### 数据聚合 data-query（1 命令）

⚠️ base/v3 的 data-query 端点挂在 **base 级**（不是 table 级），所以**不需要** `--table-id`。

```bash
feishu-cli bitable data-query --base-token xxx --config-file query.json
feishu-cli bitable data-query --base-token xxx --config '{"dimensions":[{"field_id":"fld_cat"}],"measures":[{"field_id":"fld_amt","type":"sum"}]}'
```

底层调用：`POST /open-apis/base/v3/bases/{base_token}/data/query`

查询 body 示例：
```json
{
  "group_by": [{"field_id": "fld_category"}],
  "aggregate": [{"field_id": "fld_amount", "type": "sum"}]
}
```

### 工作流 workflow（1 命令，仅 list）

```bash
feishu-cli bitable workflow list --base-token xxx --page-size 50 --status enabled
feishu-cli bitable workflow list --base-token xxx --page-token TOKEN
```

## 典型工作流

### 建表 → 加字段 → 写入数据 → 建视图配过滤

```bash
# 1. 创建多维表格
BASE_TOKEN=$(feishu-cli bitable create --name "任务跟踪" -o json | jq -r '.base.base_token')

# 2. 创建数据表
TABLE_ID=$(feishu-cli bitable table create --base-token $BASE_TOKEN --name "待办" -o json | jq -r '.table.table_id')

# 3. 添加字段
feishu-cli bitable field create --base-token $BASE_TOKEN --table-id $TABLE_ID --config '{
  "field": {"name": "状态", "type": "select", "property": {"options": [{"name": "待办"}, {"name": "进行中"}, {"name": "完成"}]}}
}'

# 4. 批量写入记录
feishu-cli bitable record batch-create --base-token $BASE_TOKEN --table-id $TABLE_ID --config-file records.json

# 5. 创建自定义视图
VIEW_ID=$(feishu-cli bitable view create --base-token $BASE_TOKEN --table-id $TABLE_ID --name "进行中" --view-type grid -o json | jq -r '.view.view_id')

# 6. 配置视图过滤
feishu-cli bitable view view-filter-set --base-token $BASE_TOKEN --table-id $TABLE_ID --view-id $VIEW_ID --config '{
  "filter_info": {
    "conjunction": "and",
    "conditions": [{"field_id": "fld_status", "operator": "is", "value": ["进行中"]}]
  }
}'

# 7. 配置排序（按创建时间降序）
feishu-cli bitable view view-sort-set --base-token $BASE_TOKEN --table-id $TABLE_ID --view-id $VIEW_ID --config '{
  "sort_config": [{"field_id": "fld_create_time", "desc": true}]
}'
```

## 权限要求

| 命令 | 所需 scope |
|---|---|
| 读操作（list/get/search/history） | `base:base:readonly`、`base:table:readonly`、`base:record:readonly` 等对应 readonly |
| 写操作（create/update/delete/batch） | `base:base`、`base:table`、`base:record`、`base:field`、`base:view` 等全权限 |
| 角色管理 | `base:role:readonly` / `base:role` |
| 高级权限 | `base:app_permission` |
| 工作流 | `base:workflow:readonly` |

## 注意事项

- **base/v3 需要 X-App-Id header**：命令自动注入，无需手动设置
- **base_token / app_token 是同一个值**：飞书新旧文档用两种叫法，CLI 只认 `--base-token`（`--app-token` 已删除）
- **--config / --config-file 两种输入**：所有写操作支持 inline JSON 或文件路径
- **批量上限**：`record batch-create` / `batch-update` 由飞书后端限制，建议单批 ≤500 条
- **视图类型**：`view create --view-type` 可选值：`grid / kanban / gallery / gantt / calendar`
- **未实现的 v3 命令**（后续迭代）：
  - dashboard CRUD + dashboard-block CRUD
  - form CRUD + form-questions CRUD
  - workflow get/create/update/enable/disable
  - record upload-attachment
  
  当前可通过飞书 Web 界面配合管理这些功能。
