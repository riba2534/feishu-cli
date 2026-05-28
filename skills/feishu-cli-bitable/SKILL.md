---
name: feishu-cli-bitable
description: >-
  飞书多维表格（Bitable/Base）操作。底层使用 base/v3 新 API，支持视图完整配置写入、
  记录 upsert、记录附件上传下载、记录修改历史、角色 CRUD、高级权限开关、数据聚合查询、
  仪表盘 + 仪表盘块 CRUD、表单 + 表单问题 CRUD、工作流 CRUD 等。
  当用户请求"创建多维表格"、"操作数据表"、"添加记录"、"查询记录"、"管理字段"、
  "多维表格"、"base"、"bitable"、"数据表"、"视图排序"、"视图过滤"、"视图分组"、
  "角色"、"role"、"高级权限"、"advperm"、"数据聚合"、"data query"、
  "仪表盘"、"dashboard"、"表单"、"form"、"工作流"、"workflow"、"记录附件"、
  "复制多维表格"时使用。
argument-hint: "[base_token] [table_id]"
user-invocable: true
allowed-tools: Bash(feishu-cli bitable:*), Bash(feishu-cli auth:*), Read, Write
---

# 飞书多维表格（Bitable / Base）

通过 **base/v3 API** 操作飞书多维表格。`bitable` 也支持 `base` 别名。

> **API 切换**：此技能已从旧的 `bitable/v1` 切换到新的 `base/v3`。字段名 `app_token` 和 `base_token` 在飞书文档里是同一个值的两种叫法（老 v1 叫 app_token，新 v3 叫 base_token），CLI 只认 **`--base-token`**（`--app-token` 已删除）。从多维表格 URL 里 `/base/<token>` 或 `/bitable/<token>` 片段里取 token 即可。

## 前置条件

- **认证**：所有命令**必需 User Access Token**（读类、写类均强制要求 User Token，未登录直接报错）。先执行 `feishu-cli auth login` 登录
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

### 记录 record（14 命令）

```bash
feishu-cli bitable record list        --base-token xxx --table-id tblxxx --view-id viewxxx --limit 100
feishu-cli bitable record get         --base-token xxx --table-id tblxxx --record-id recxxx
feishu-cli bitable record batch-get   --base-token xxx --table-id tblxxx --config-file ids.json
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

# share-link：批量生成记录共享链接（v1.29+ 新增），单次最多 100 条
feishu-cli bitable record share-link  --base-token xxx --table-id tblxxx --record-ids rec_1,rec_2,rec_3

# history-list：GET + query params（不是 POST body），--record-id 必填
feishu-cli bitable record history-list --base-token xxx --table-id tblxxx --record-id recxxx
feishu-cli bitable record history-list --base-token xxx --table-id tblxxx --record-id recxxx --page-size 50 --max-version 20

# 附件：upload/download 为 2 步编排（medias/upload_all + append/get_attachments），单次 ≤50 个
feishu-cli bitable record upload-attachment   --base-token xxx --table-id tblxxx \
  --record-id recxxx --field-id fldxxx --file ./report.pdf --file ./shot.png   # --file 可重复
feishu-cli bitable record download-attachment --base-token xxx --table-id tblxxx \
  --record-id recxxx --output ./downloads/                                     # 省略 --file-token 下全部
feishu-cli bitable record download-attachment --base-token xxx --table-id tblxxx \
  --record-id recxxx --file-token boxcnxxxx --output ./a.pdf                    # 指定单个附件
feishu-cli bitable record remove-attachment   --base-token xxx --table-id tblxxx \
  --record-id recxxx --field-id fldxxx --file-token boxcnxxxx                   # --file-token 可重复
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
  --config '{"filter_info":{"conjunction":"and","conditions":[{"field_id":"fld1","operator":"is","value":["进行中"]}]}}'

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
  --config '{"view_field":[{"field_id":"fld1","visible":true},{"field_id":"fld2","visible":true}]}'

feishu-cli bitable view view-timebar-get       --base-token xxx --table-id tblxxx --view-id viewxxx
feishu-cli bitable view view-timebar-set       --base-token xxx --table-id tblxxx --view-id viewxxx \
  --config '{"timebar":{"start_field_id":"fld_start","end_field_id":"fld_end","title_field_id":"fld_title"}}'

feishu-cli bitable view view-card-get          --base-token xxx --table-id tblxxx --view-id viewxxx
feishu-cli bitable view view-card-set          --base-token xxx --table-id tblxxx --view-id viewxxx \
  --config '{"card":{"cover_field_id":"fld1","display_fields":["fld2","fld3"]}}'
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

### 工作流 workflow（6 命令）

```bash
feishu-cli bitable workflow list   --base-token xxx --page-size 50 --status enabled
feishu-cli bitable workflow list   --base-token xxx --page-token TOKEN
feishu-cli bitable workflow get    --base-token xxx --workflow-id wkfxxxx          # 含 steps
feishu-cli bitable workflow create --base-token xxx --config '{"title":"My Workflow","steps":[...]}'
feishu-cli bitable workflow update --base-token xxx --workflow-id wkfxxxx --config-file wf.json  # PUT 整体替换
feishu-cli bitable workflow enable  --base-token xxx --workflow-id wkfxxxx
feishu-cli bitable workflow disable --base-token xxx --workflow-id wkfxxxx
```

> `update` 是 PUT 整体替换，未提供的字段不保留；`workflow_id` 为 `wkf` 前缀。

### 仪表盘 dashboard（6 命令 + 仪表盘块 block 5 命令）

```bash
# 仪表盘 CRUD（原有 list/copy）；create/update 支持便捷字段 --name/--theme-style 或 --config/--config-file
feishu-cli bitable dashboard list    --base-token xxx
feishu-cli bitable dashboard copy    --base-token xxx --dashboard-id dsbxxxx --name "副本"
feishu-cli bitable dashboard create  --base-token xxx --name "运营看板"
feishu-cli bitable dashboard get     --base-token xxx --dashboard-id dsbxxxx
feishu-cli bitable dashboard update  --base-token xxx --dashboard-id dsbxxxx --name "新名字"
feishu-cli bitable dashboard delete  --base-token xxx --dashboard-id dsbxxxx
feishu-cli bitable dashboard arrange --base-token xxx --dashboard-id dsbxxxx     # 服务端智能排版，无 body

# 仪表盘块 block CRUD；create --type 取值见下
feishu-cli bitable dashboard block create --base-token xxx --dashboard-id dsbxxxx \
  --type column --name "按月统计" --data-config '{"table_name":"任务","group_by":"月份"}'
feishu-cli bitable dashboard block list   --base-token xxx --dashboard-id dsbxxxx
feishu-cli bitable dashboard block get    --base-token xxx --dashboard-id dsbxxxx --block-id blkxxxx
feishu-cli bitable dashboard block update --base-token xxx --dashboard-id dsbxxxx --block-id blkxxxx --name "新块名"
feishu-cli bitable dashboard block delete --base-token xxx --dashboard-id dsbxxxx --block-id blkxxxx
```

> block `--type` 取值：`column|bar|line|pie|ring|area|combo|scatter|funnel|wordCloud|radar|statistics|text`；图表块 `--data-config` 传 `table_name`/`series|count_all`/`group_by`/`filter`，文本块传 `text`。

### 表单 form（7 命令 + 表单问题 field 4 命令）

```bash
# 表单 CRUD（原有 get/patch）
feishu-cli bitable form create --base-token xxx --table-id tblxxx --name "报名表" --description "活动报名"
feishu-cli bitable form get    --base-token xxx --table-id tblxxx --form-id vewxxx
feishu-cli bitable form patch  --base-token xxx --table-id tblxxx --form-id vewxxx --name "新名字"
feishu-cli bitable form delete --base-token xxx --table-id tblxxx --form-id vewxxx   # form_id 即表单视图 view_id

# 按分享 token（shr 前缀）取详情 / 提交，无需 base_token
feishu-cli bitable form detail --share-token shrcnxxxx
feishu-cli bitable form submit --share-token shrcnxxxx --content '{"评分":5,"评价":"很好"}'  # 不处理附件

# 表单问题 field（别名 questions）：list/patch + 批量 create/delete（单次 ≤10）
feishu-cli bitable form field list   --base-token xxx --table-id tblxxx --form-id vewxxx
feishu-cli bitable form field create --base-token xxx --table-id tblxxx --form-id vewxxx \
  --questions '[{"type":"text","title":"你的名字","required":true}]'
feishu-cli bitable form field patch  --base-token xxx --table-id tblxxx --form-id vewxxx --config-file q.json
feishu-cli bitable form field delete --base-token xxx --table-id tblxxx --form-id vewxxx --question-ids fld001,fld002
```

> 表单问题 item 字段：`title`(必填)/`type`(text/number/select/datetime/user/attachment/location)/`description`/`required`/`multiple`/`options` 等；`submit` 如需附件先用 `record upload-attachment` 思路拿 file_token 再写进 `--content`。

## 典型工作流

### 建表 → 加字段 → 写入数据 → 建视图配过滤

```bash
# 1. 创建多维表格
BASE_TOKEN=$(feishu-cli bitable create --name "任务跟踪" -o json | jq -r '.base.base_token')

# 2. 创建数据表
TABLE_ID=$(feishu-cli bitable table create --base-token $BASE_TOKEN --name "待办" | jq -r '.table.table_id')

# 3. 添加字段
feishu-cli bitable field create --base-token $BASE_TOKEN --table-id $TABLE_ID --config '{
  "field": {"name": "状态", "type": "select", "property": {"options": [{"name": "待办"}, {"name": "进行中"}, {"name": "完成"}]}}
}'

# 4. 批量写入记录
feishu-cli bitable record batch-create --base-token $BASE_TOKEN --table-id $TABLE_ID --config-file records.json

# 5. 创建自定义视图
VIEW_ID=$(feishu-cli bitable view create --base-token $BASE_TOKEN --table-id $TABLE_ID --name "进行中" --view-type grid | jq -r '.view.view_id')

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
| 读操作（list/get/search/history） | `base:app:readonly`、`base:table:readonly`、`base:record:readonly`、`base:field:readonly`、`base:view:readonly` |
| 写操作（create/update/delete/batch） | `base:app`、`base:table`、`base:record`、`base:field`、`base:view` |
| 角色管理 | `base:role:readonly` / `base:role` |
| 高级权限 | `base:app_permission` |
| 工作流 | 读 `base:workflow:readonly` / 写 `base:workflow` |
| 仪表盘 / 表单 | 读 `base:dashboard:readonly` / `base:form:readonly`，写 `base:dashboard` / `base:form` |

## 注意事项

- **base/v3 需要 X-App-Id header**：命令自动注入，无需手动设置
- **base_token / app_token 是同一个值**：飞书新旧文档用两种叫法，CLI 只认 `--base-token`（`--app-token` 已删除）
- **--config / --config-file 两种输入**：所有写操作支持 inline JSON 或文件路径
- **批量上限**：`record batch-create` / `batch-update` 由飞书后端限制，建议单批 ≤500 条
- **视图类型**：`view create --view-type` 可选值：`grid / kanban / gallery / gantt / calendar`
- **附件命令为编排命令**：upload 走 `medias/upload_all` + `append_attachments`，download 走 `get_attachments` + `medias/{ft}/download`，单次 ≤50 个
- **form_id = view_id**：表单的 form_id 即表单视图的 view_id；`detail`/`submit` 用 `share-token`（shr 前缀，从分享链接提取）无需 base_token
- **仍走飞书私有扩展 API（公开 OpenAPI 无对应，可用 `feishu-cli api` 裸调兜底）**：view 独立 filter-sort 端点（`base-api.feishu.cn`）
