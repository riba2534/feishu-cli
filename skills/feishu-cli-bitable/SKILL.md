---
name: feishu-cli-bitable
description: >-
  飞书多维表格（Bitable/Base）操作。底层使用 base/v3 新 API，支持视图完整配置写入、
  记录 upsert、记录批量获取、记录附件上传下载、记录修改历史、角色 CRUD + 协作者增删、
  多维表格本体重命名/高级权限开关、数据聚合查询、
  仪表盘 + 仪表盘块 CRUD、表单 + 表单问题 CRUD、工作流 CRUD 等。
  当用户请求"创建多维表格"、"操作数据表"、"添加记录"、"查询记录"、"管理字段"、
  "多维表格"、"base"、"bitable"、"数据表"、"视图排序"、"视图过滤"、"视图分组"、
  "角色"、"role"、"高级权限"、"advperm"、"数据聚合"、"data query"、
  "仪表盘"、"dashboard"、"表单"、"form"、"工作流"、"workflow"、"记录附件"、
  "复制多维表格"时使用。
  支持 --as bot|user|auto 身份切换：默认 auto（User 优先、Tenant 兜底），
  --as bot 用 App Token 操作多维表格，无需 auth login、永不过期，
  适合 cron / 无人值守 / 脚本自动抓取多维表格内容。
  凡涉及"App Token 读写 bitable"、"不登录抓多维表格"、"cron 定时同步多维表格"、
  "bitable 报需要 User Token / 91403 没权限"时也应使用本技能。
argument-hint: "[base_token] [table_id]"
user-invocable: true
allowed-tools: Bash(feishu-cli bitable:*), Bash(feishu-cli auth:*), Read, Write
---

# 飞书多维表格（Bitable / Base）

通过 **base/v3 API** 操作飞书多维表格。`bitable` 也支持 `base` 别名。

> **API 切换**：此技能已从旧的 `bitable/v1` 切换到新的 `base/v3`。字段名 `app_token` 和 `base_token` 在飞书文档里是同一个值的两种叫法（老 v1 叫 app_token，新 v3 叫 base_token），CLI 只认 **`--base-token`**（`--app-token` 已删除）。从多维表格 URL 里 `/base/<token>` 或 `/bitable/<token>` 片段里取 token 即可。

## 前置条件

- **认证**：所有命令支持 `--as bot|user|auto` 身份切换（详见下方「身份选择」），默认 `auto`（User 优先、Tenant 兜底）。已登录用 User Token，未登录自动回落 App Token；要稳定用 App Token 跑（cron）显式加 `--as bot`
- **App 凭证**：应用 App ID + App Secret（base/v3 需要 `X-App-Id` header，自动注入）。`--as bot` / `auto` 回落 Tenant 时只靠 App 凭证，无需 `auth login`

## 身份选择 `--as`（命令组 persistent flag，所有子命令通用）

飞书 `base/v3` 与 `bitable/v1` API **本身同时支持 User 和 Tenant(App) 两种身份**，本技能据此提供三档：

| `--as` | 身份 | 何时用 | 是否需 `auth login` |
|--------|------|--------|---------------------|
| `auto`（默认） | User 优先、Tenant 兜底 | 交互式日常使用 | 否（未登录自动用 App Token） |
| `bot`（= `tenant`/`app`） | 强制 App Token | **cron / 无人值守 / 脚本自动抓取**，永不过期 | **否** |
| `user` | 强制 User Token | 必须以本人身份操作（个人 base、协作者权限） | 是（缺失报错） |

```bash
# cron 场景：App 凭证走环境变量，App Token 抓多维表格，不依赖登录、不会过期
export FEISHU_APP_ID=cli_xxx FEISHU_APP_SECRET=xxx
feishu-cli bitable table list  --base-token bscnxxxx --as bot
feishu-cli bitable record list --base-token bscnxxxx --table-id tblxxx --as bot
feishu-cli bitable record upsert --base-token bscnxxxx --table-id tblxxx \
  --config '{"fields":{"文本":"hello"}}' --as bot
```

> **`--as bot` 报 `91403 you don't have permission`**：不是 token 问题，是 **Bot 还不是这张多维表格的协作者**。把 Bot 加为协作者（`feishu-cli perm add <base_token> --doc-type bitable --member-type open_id --member-id <bot_open_id> --perm full_access`）或把文档可见性调到组织可见即可。Bot 自己创建的 base 默认就有权限。
> **历史背景**：旧版本 bitable 命令在 CLI 侧硬性强制 User Token（未登录直接报错），其实底层 API 一直支持 Tenant Token——现已按官方 `lark-cli` 的 `--as` 模式放开。

## 命令速查

### 基础（4 命令）

```bash
# 创建多维表格
feishu-cli bitable create --name "项目管理" --time-zone Asia/Shanghai
feishu-cli bitable create --name "销售" --folder-token fldxxx

# 获取多维表格信息
feishu-cli bitable get --base-token bscnxxxx

# 复制多维表格
feishu-cli bitable copy --base-token bscnxxxx --name "副本"
feishu-cli bitable copy --base-token bscnxxxx --name "空白副本" --without-content

# 更新多维表格本体：重命名 / 开关高级权限（仅显式设置的字段才提交）
feishu-cli bitable update --base-token bscnxxxx --name "新表名"
feishu-cli bitable update --base-token bscnxxxx --is-advanced true   # 开启高级权限
```

> `update` 走 `bitable/v1`（`PUT apps/{app_token}`，base/v3 无更新本体端点；app_token 即 base_token），仅支持云空间文件夹内的多维表格。`--is-advanced true/false` 等价于 `advperm enable/disable` 的高级权限开关。

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
feishu-cli bitable record batch-get   --base-token xxx --table-id tblxxx --record-ids recxxx,recyyy
# batch-get 可选 flag：返回分享链接 / 自动计算字段 / 指定用户字段 ID 类型
feishu-cli bitable record batch-get   --base-token xxx --table-id tblxxx --record-ids recxxx,recyyy \
  --with-shared-url --automatic-fields --user-id-type open_id   # user-id-type: open_id|union_id|user_id
# record search：用 --keyword（推荐），不要混用 upsert 的 {"fields":{...}}
feishu-cli bitable record search      --base-token xxx --table-id tblxxx --keyword 测试 --search-field 名称
# 结构化过滤用 --filter-json（遵循 base/v3 records/search 规范）或 --config 传完整请求体

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
  --record-id recxxx --file-token boxcnxxxx --output ./a.pdf --overwrite       # 指定单个附件，已存在则覆盖
feishu-cli bitable record remove-attachment   --base-token xxx --table-id tblxxx \
  --record-id recxxx --field-id fldxxx --file-token boxcnxxxx                   # --file-token 可重复
```

> 附件文件名：`download-attachment` 用附件**原始文件名**保存（不再用 file_token 命名）；目标已存在会直接报错，加 `--overwrite` 覆盖。三个附件命令均支持 `--dry-run`（写前预览请求体）；`upload/remove-attachment` 支持 `--format/--jq`，`download-attachment` 不支持（仅打印 JSON）。

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

### 角色 role（5 命令 + 协作者 member 5 命令）

```bash
feishu-cli bitable role list   --base-token xxx
feishu-cli bitable role get    --base-token xxx --role-id roxxx
feishu-cli bitable role create --base-token xxx --config-file role.json
feishu-cli bitable role update --base-token xxx --role-id roxxx --config '...'
feishu-cli bitable role delete --base-token xxx --role-id roxxx
```

#### 角色协作者 role member（5 命令）

把用户/群/部门加入或移出某个角色（走 `bitable/v1` 协作者端点 `apps/{app_token}/roles/{role_id}/members`）。

```bash
feishu-cli bitable role member list         --base-token xxx --role-id roxxx           # 支持 --page-size(≤100)/--page-token
feishu-cli bitable role member create       --base-token xxx --role-id roxxx --member-id ou_xxx
feishu-cli bitable role member delete       --base-token xxx --role-id roxxx --member-id ou_xxx
# 批量增删：--member-ids 逗号分隔，单次 ≤100
feishu-cli bitable role member batch-create --base-token xxx --role-id roxxx --member-ids ou_a,ou_b,ou_c
feishu-cli bitable role member batch-delete --base-token xxx --role-id roxxx --member-ids ou_a,ou_b
```

> `--member-id-type` 默认 `open_id`，可选 `open_id|union_id|user_id|chat_id|department_id|open_department_id`（与 `--member-id`/`--member-ids` 的 ID 类型对应）。member 写命令（create/delete/batch-create/batch-delete）均支持 `--dry-run/--format/--jq`。
> **scope 不同**：role member 走协作者端点，所需 scope 是 `bitable:app` / `bitable:app:readonly` / `base:collaborator:read`，**不是** `base:role`——只申请 `base:role` 会撞 99991679。

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

查询 body 示例（LiteQuery DSL：`dimensions` / `measures` / `filters`，与上面 `--config` 一致）：
```json
{
  "dimensions": [{"field_id": "fld_category"}],
  "measures": [{"field_id": "fld_amount", "type": "sum"}]
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
> **私有端点**：公开 OpenAPI 仅文档化 `workflow list`（只读），`create/get/update/enable/disable` 走 base/v3 私有扩展端点（如撞限制可用 `feishu-cli api` 裸调兜底）。写命令（create/update/enable/disable）均支持 `--dry-run/--format/--jq`。

### 仪表盘 dashboard（7 命令 + 仪表盘块 block 5 命令）

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
  --type column --name "按月统计" --data-config '{"table_name":"任务","group_by":[{"field_id":"fld_month"}]}'
feishu-cli bitable dashboard block list   --base-token xxx --dashboard-id dsbxxxx
feishu-cli bitable dashboard block get    --base-token xxx --dashboard-id dsbxxxx --block-id blkxxxx
feishu-cli bitable dashboard block update --base-token xxx --dashboard-id dsbxxxx --block-id blkxxxx --name "新块名"
feishu-cli bitable dashboard block delete --base-token xxx --dashboard-id dsbxxxx --block-id blkxxxx
```

> block `--type` 取值：`column|bar|line|pie|ring|area|combo|scatter|funnel|wordCloud|radar|statistics|text`；图表块 `--data-config` 传 `table_name`/`series|count_all`/`group_by`/`filter`，文本块传 `text`。`--data-config` 的内部结构由飞书图表 schema 定义、本地无离线校验（`--dry-run` 也不校验内部字段），建议用 `block get` 先取一个已有图表块的结构作模板再改。
> `--theme-style` 写入 `theme.theme_style`，合法取值由飞书仪表盘主题 schema 定义（CLI 不做枚举校验）；不确定时省略此字段用默认主题，或 `dashboard get` 一个已配好主题的看板看其真实取值。
> dashboard / block 全部写命令（create/update/delete/copy/arrange）均支持 `--dry-run` 预览请求体；全部命令支持 `--format json|pretty|table|ndjson|csv` 与 `--jq`。

### 表单 form（7 命令 + 表单问题 field 4 命令）

```bash
# 表单 CRUD（原有 get/patch）
feishu-cli bitable form create --base-token xxx --table-id tblxxx --name "报名表" --description "活动报名"
feishu-cli bitable form list   --base-token xxx --table-id tblxxx                    # 列出表下所有表单，支持 --page-size(≤100)/--page-token
feishu-cli bitable form get    --base-token xxx --table-id tblxxx --form-id vewxxx
feishu-cli bitable form patch  --base-token xxx --table-id tblxxx --form-id vewxxx --name "新名字"
# patch 一键开启共享（走 bitable/v1，shared 系字段才生效）
feishu-cli bitable form patch  --base-token xxx --table-id tblxxx --form-id vewxxx \
  --shared true --shared-limit anyone_editable --submit-limit-once true
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

> 表单问题字段：`title`(必填)/`type`(text/number/select/datetime/user/attachment/location)/`description`/`required`/`multiple`/`options` 等；`submit` 如需附件先用 `record upload-attachment` 思路拿 file_token 再写进 `--content`。
> **form patch 走 `bitable/v1`**（不同于其它 form 命令的 base/v3）：因 `--shared`/`--shared-limit`/`--submit-limit-once` 是 bitable/v1 字段，整体路由到 bitable/v1 让分享相关字段一次生效。`--shared-limit` 取值：`off | tenant_editable | anyone_editable`；仅显式设置的便捷字段才提交，复杂场景可用 `--config/--config-file` 裸传完整请求体。
> **form submit 的 `--content` 是裸字段 map**（如 `{"评分":5}`），**不要**外包 `{"fields":{...}}`——CLI 不再自动解包，多套一层会丢数据。
> form/field 全部写命令（create/patch/delete、field create/patch/delete、detail/submit）支持 `--dry-run` 预览；全部 form 命令支持 `--format/--jq`；`form create/patch`、`field create/delete` 均支持 `--config/--config-file` 裸传完整请求体作为便捷字段的逃生通道。

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
| 角色协作者（role member） | `bitable:app:readonly` / `bitable:app` / `base:collaborator:read`（走 bitable/v1 协作者端点，**不是** `base:role`） |
| 高级权限 | `base:app_permission` |
| 工作流 | 读 `base:workflow:readonly` / 写 `base:workflow` |
| 仪表盘 / 表单 | 读 `base:dashboard:readonly` / `base:form:readonly`，写 `base:dashboard` / `base:form` |

## 注意事项

- **base/v3 需要 X-App-Id header**：命令自动注入，无需手动设置
- **base_token / app_token 是同一个值**：飞书新旧文档用两种叫法，CLI 只认 `--base-token`（`--app-token` 已删除）
- **--config / --config-file 两种输入**：所有写操作支持 inline JSON 或文件路径
- **--dry-run 预览（仅本轮新增写命令支持）**：`--dry-run` 只在本轮新增的写命令上可用——`dashboard`/`block` 写、`form`/`form field` 写、`workflow create/update/enable/disable`、`role member` 写、`record upload/download/remove-attachment`、`bitable update`。**旧写命令不支持**（`record batch-create/batch-update/upsert/delete`、`table/field/view/role create·update·delete`、`advperm enable/disable`、`bitable create/copy` 等传 `--dry-run` 会报 `unknown flag`）。新命令的 dry-run 也尊重 `--format/--jq`（download-attachment 的 dry-run 始终打 stdout，不写 `--output`）。
- **--format / --jq 输出控制（约 4 成命令支持，多为本轮新增）**：支持 `--format json|pretty|table|ndjson|csv`（默认 json）+ `--jq`（内置 gojq）的主要是本轮新增命令（`record batch-get`、`bitable update`、`dashboard`/`form`/`role member`/`workflow` 各命令等），可 `--jq '.items[].name'` 提取或 `--format table` 表格化。**大量旧命令不支持**：`record list/get/search/批量写`、`table/field/view/role list·CRUD`、`view-*-get/set`、`bitable create/copy/data-query/advperm`、`record download-attachment` 等——这些直接打印 JSON（部分用旧式 `-o json`），需要过滤时改用 `feishu-cli api ... --jq` 或外部 jq。
- **批量上限**：`record batch-create` / `batch-update` 由飞书后端限制，建议单批 ≤500 条
- **视图类型**：`view create --view-type` 可选值：`grid / kanban / gallery / gantt / calendar`
- **附件命令为编排命令**：upload 走 `medias/upload_all` + `append_attachments`，download 走 `get_attachments` + `medias/{ft}/download`，单次 ≤50 个
- **form_id = view_id**：表单的 form_id 即表单视图的 view_id；`detail`/`submit` 用 `share-token`（shr 前缀，从分享链接提取）无需 base_token
- **仍走飞书私有扩展 API（公开 OpenAPI 无对应，可用 `feishu-cli api` 裸调兜底）**：view 独立 filter-sort 端点（`base-api.feishu.cn`）
