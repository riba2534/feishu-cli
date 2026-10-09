# 飞书多维表格（Bitable / Base）

通过 **base/v3 API** 操作飞书多维表格。`bitable` 也支持 `base` 别名。

> **API 切换**：此技能已从旧的 `bitable/v1` 切换到新的 `base/v3`。字段名 `app_token` 和 `base_token` 在飞书文档里是同一个值的两种叫法（老 v1 叫 app_token，新 v3 叫 base_token），CLI 只认 **`--base-token`**（`--app-token` 已删除）。`--base-token` 既接受裸 token，也接受多维表格链接（`/base/<token>`、`/bitable/<token>`）与挂在知识库里的多维表格链接（`/wiki/<node_token>`，自动经 `node_by_token` 换出底层 base_token 并校验类型）。记录分享（`/record/`）、表单分享链接以及 URL 中的 `?table=` 选中块，先用 [`bitable resolve`](#链接解析-resolve--顶层块-block2-命令) 解析。

> **表 / 字段 / 视图可以直接用名称寻址（实测）**：`--table-id`、`--field-id`、`--view-id` 都可以传名称（如 `--table-id 订单 --view-id 进行中 --field-id 金额`），不必先查 ID；名称有重复时改用 ID。

## 目录

- [前置条件](#前置条件)
- [身份选择](#身份选择---as命令组-persistent-flag所有子命令通用)
- [命令速查](#命令速查)
  - [链接解析 resolve / 顶层块 block](#链接解析-resolve--顶层块-block2-命令)
  - [记录 record（分页、批量、附件）](#记录-record14-命令)
  - [视图 view 与视图配置](#视图-view5-命令--12-配置命令)
  - [数据聚合 data-query](#数据聚合-data-query1-命令)
  - [仪表盘 dashboard](#仪表盘-dashboard7-命令--仪表盘块-block-6-命令--分享-share-2-命令)
  - [表单 form](#表单-form7-命令--表单问题-field-4-命令--分享-share-2-命令)
- [典型工作流](#典型工作流)
- [权限要求](#权限要求)
- [filter DSL](#filter-dslrecord-list--record-search-结构化过滤实测验证)
- [注意事项](#注意事项)

## 前置条件

- **认证**：所有命令支持 `--as bot|user|auto` 身份切换（详见下方「身份选择」），默认 `auto`（User 优先、Tenant 兜底）。已登录用 User Token，未配置 User Token 时回落 App Token；已登录但 User Token 解析/刷新失败时直接报错（fail-closed，不静默切 Bot）。要稳定用 App Token 跑（cron）显式加 `--as bot`
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

> **`--as bot` 报 `91403 you don't have permission`**：不是 token 问题，是 **Bot 还不是这张多维表格的协作者**。以有权限的用户身份把 Bot 加为协作者即可（实测加完 `--as bot` 立即可读）：
>
> ```bash
> BOT_ID=$(feishu-cli api GET /open-apis/bot/v3/info --as bot --jq '.bot.open_id' | tr -d '"')
> feishu-cli perm add <base_token> --doc-type bitable --member-type openid --member-id "$BOT_ID" --perm full_access --as user
> ```
>
> Bot 自己创建的 base 默认就有权限。

## 命令速查

### 链接解析 resolve / 顶层块 block（2 命令）

```bash
# 把链接解析成坐标（对齐官方 base +url-resolve）
feishu-cli bitable resolve --url "https://xxx.feishu.cn/base/bascnxxx?table=tblxxx&view=vewxxx"
feishu-cli bitable resolve --url "https://xxx.feishu.cn/wiki/wikcnxxx"       # 知识库中的多维表格
feishu-cli bitable resolve --url "https://xxx.feishu.cn/record/xxxx"          # 记录分享链接 → base/table/record
feishu-cli bitable resolve --url "https://xxx.feishu.cn/share/base/shrxxx"    # 表单分享链接 → share_token

# 列出顶层块：数据表 / 仪表盘 / 工作流 / 文件夹 / 文档（服务端一次返回全量）
feishu-cli bitable block list --base-token bscnxxxx
feishu-cli bitable block list --base-token bscnxxxx --type dashboard --jq '.blocks[] | {id, name}'
feishu-cli bitable block list --base-token bscnxxxx --parent-id <folder_block_id>
```

> **URL 里的 `?table=` 不一定是数据表**：它是"当前选中的顶层块"，可能是数据表、仪表盘、工作流、文件夹或文档。`resolve` 会调 `blocks/list` 判型：只有 `block_type=table` 才输出 `table_id`（以及 `view_id`/`record_id`），仪表盘输出 `dashboard_id`、工作流输出 `workflow_id`、文档输出 `docx_token`。不要把 `table=` 的值直接当 `--table-id`。
> 视图分享、仪表盘分享、工作区、新增记录、BaseApp 链接 CLI 无法解析，会以用法错误（退出码 2）明确提示。

### 基础（4 命令）

```bash
# 创建多维表格（以 Bot 身份创建时自动给当前 CLI 登录用户授予 full_access，
# JSON 输出 permission_grant.status = granted / skipped / failed；bitable copy 同理）
feishu-cli bitable create --name "项目管理" --time-zone Asia/Shanghai
feishu-cli bitable create --name "销售" --folder-token fldxxx
# 建表时直接定好第一张数据表：--fields 按 schema 新建该表并删除平台默认表；只给 --table-name 则重命名默认表
feishu-cli bitable create --name "任务跟踪" --table-name "任务" \
  --fields '[{"name":"标题","type":"text"},{"name":"状态","type":"select","options":[{"name":"Todo"},{"name":"Done"}]}]'

# 获取多维表格信息
feishu-cli bitable get --base-token bscnxxxx

# 复制多维表格
feishu-cli bitable copy --base-token bscnxxxx --name "副本"
feishu-cli bitable copy --base-token bscnxxxx --name "空白副本" --without-content

# 更新多维表格本体：重命名 / 开关高级权限（仅显式设置的字段才提交）
feishu-cli bitable update --base-token bscnxxxx --name "新表名"
feishu-cli bitable update --base-token bscnxxxx --is-advanced         # 开启高级权限；关闭写 --is-advanced=false
```

> `update` 走 `bitable/v1`（`PUT apps/{app_token}`，base/v3 无更新本体端点；app_token 即 base_token），仅支持云空间文件夹内的多维表格。`--is-advanced` / `--is-advanced=false` 等价于 `advperm enable/disable`。
> **布尔 flag 不能写成 `--flag false`**：`--is-advanced false`、`form patch --shared false` 会被解析成 `true`（`false` 被当成多余参数忽略，`--dry-run` 实测请求体为 `true`）。开启写 `--flag`，关闭写 `--flag=false`。
> `create/copy` 的 `-o json` 输出是**平铺**结构：`{"base_token","name","url","folder_token",...}`（实测，不是 `{"base":{...}}`），取 token 用 `jq -r '.base_token'`；带 `--fields/--table-name` 时额外输出 `table`、`fields`、`default_table_deleted`/`default_table_renamed`。

### 数据表 table（5 命令）

```bash
feishu-cli bitable table list   --base-token bscnxxxx                         # 自动翻页取全部
feishu-cli bitable table get    --base-token bscnxxxx --table-id tblxxx
feishu-cli bitable table create --base-token bscnxxxx --name "任务表"
feishu-cli bitable table create --base-token bscnxxxx --name "任务表" \
  --fields '[{"name":"标题","type":"text"},{"name":"金额","type":"number"}]'   # 建表时一次带上字段
feishu-cli bitable table create --base-token bscnxxxx --config-file table.json
feishu-cli bitable table update --base-token bscnxxxx --table-id tblxxx --name "新名字"
feishu-cli bitable table delete --base-token bscnxxxx --table-id tblxxx
```

> **列表分页（table / field / view list）**：服务端按 offset/limit 分页、只返回 `total`（不返回 has_more），不传 limit 时只给 20 条。CLI 默认按 `total` 自动取完（每页 500），输出 `{"tables|fields|views":[...],"total":N}`。实测**字段列表每次请求的顺序不稳定**，跨页拼接会重复/遗漏，因此 CLI 用服务端最大页一次取完并按 id 去重。`table list` 显式传 `--offset/--limit` 时只取单页，还有剩余时输出 `has_more`/`next_offset` 并在 stderr 提示。
> `table create` 的响应是平铺的表对象 `{"id","name","fields","views"}`，取 ID 用 `jq -r '.id'`（不是 `.table.table_id`）。

### 字段 field（6 命令）

```bash
feishu-cli bitable field list           --base-token xxx --table-id tblxxx     # 自动翻页取全部
feishu-cli bitable field get            --base-token xxx --table-id tblxxx --field-id fldxxx
feishu-cli bitable field create         --base-token xxx --table-id tblxxx --config-file field.json
feishu-cli bitable field update         --base-token xxx --table-id tblxxx --field-id fldxxx --config '...'
feishu-cli bitable field delete         --base-token xxx --table-id tblxxx --field-id fldxxx
feishu-cli bitable field search-options --base-token xxx --table-id tblxxx --field-id fldxxx --query "关键词"
```

> **v3 字段 JSON 形状（实测）**：判别式是顶层 `type`，属性也在顶层（select 的 `options`/`multiple`、
> auto_number 的 `style.rules`），**不要包 `property`**（v1 形状，v3 报 Unrecognized key）。
>
> **自动编号 auto_number（实测）**：创建
> `{"name":"编号","type":"auto_number","style":{"rules":[{"type":"text","text":"TASK-"},{"type":"incremental_number","length":3}]}}`，
> rules 支持 `text` / `created_time`（`date_format` 如 yyyyMM）/ `incremental_number`（`length` 前导零位数）。
> 建字段后**存量记录自动获得编号**；用同一 `PUT /fields/:field_id` 提交新 `style.rules` 即可改规则，
> 无需 `auto_serial` / `reformat_existing_records` 等参数（不存在，不要臆造），也不要绕道 bitable/v1。
> 改规则后**新记录用新格式，存量记录的 API 值保持旧编号不重排**（计数器延续，如旧 1-5、新 TASK-006）。
> 2026-10 复测确认：建字段时已有的 3 条记录拿到 `1/2/3`（未按 `length` 补零）；改为 `TASK-`+3 位后新记录为 `TASK-004`，
> 再改为 `BUG-`+4 位后存量仍是 `1/2/3/TASK-004`、新记录为 `BUG-0005`。官方技能"改规则后存量重排"的说法与 API 实测不符。
> 改字段类型或计算型字段（formula/lookup/link/auto_number）后用 `field get` 读回验证，必要时抽样记录值。

### 记录 record（14 命令）

```bash
feishu-cli bitable record list        --base-token xxx --table-id tblxxx --view-id viewxxx --limit 100
# 默认每页 100 条（服务端不传 limit 只给 20 条）；has_more=true 时输出 next_offset 并在 stderr 提示
feishu-cli bitable record list        --base-token xxx --table-id tblxxx --offset 100   # 续翻
feishu-cli bitable record list        --base-token xxx --table-id tblxxx --page-all     # 自动翻页取全部
# list 支持结构化过滤/排序（无需关键词，纯条件筛选首选；DSL 语法见下方「filter DSL」节）
feishu-cli bitable record list        --base-token xxx --table-id tblxxx \
  --filter-json '{"logic":"and","conditions":[["状态","==",["Doing"]],["分数",">=",70]]}' \
  --sort-json '[{"field":"分数","desc":true}]'
feishu-cli bitable record get         --base-token xxx --table-id tblxxx --record-id recxxx
feishu-cli bitable record batch-get   --base-token xxx --table-id tblxxx --record-ids recxxx,recyyy
# batch-get 可选 flag：返回分享链接 / 自动计算字段 / 指定用户字段 ID 类型
feishu-cli bitable record batch-get   --base-token xxx --table-id tblxxx --record-ids recxxx,recyyy \
  --with-shared-url --automatic-fields --user-id-type open_id   # user-id-type: open_id|union_id|user_id
# 字段投影（读大表控输出体积）：list/search/batch-get 均支持 --field-id（可重复，值为字段名或字段 ID）
# list/batch-get 最多 100 个、search 最多 50 个；list 走 field_id query 参数，search/batch-get 走 body select_fields
feishu-cli bitable record list        --base-token xxx --table-id tblxxx --field-id 名称 --field-id 状态

# record search：便捷模式必须同时提供 --keyword 和至少一个 --search-field
feishu-cli bitable record search      --base-token xxx --table-id tblxxx --keyword 测试 --search-field 名称
# 多个搜索字段重复传 --search-field；字段名含逗号时仍作为一个完整字段名
feishu-cli bitable record search      --base-token xxx --table-id tblxxx --keyword Alice \
  --search-field 'Last, First' --search-field 邮箱
# search 的 --filter-json 叠加在 keyword 搜索上做交集（服务端强制要求 keyword+search_fields）；
# 纯结构化筛选（不需要关键词）用 record list 的 --filter-json
feishu-cli bitable record search      --base-token xxx --table-id tblxxx --keyword 测试 \
  --search-field 名称 --filter-json '{"logic":"and","conditions":[["状态","==",["启用"]]]}'
# --config/--config-file 与所有便捷 flag（包括 --offset/--limit）互斥

# upsert：不传 --record-id 则 POST 创建；传 --record-id 则 PATCH 更新（官方无专用 upsert 端点）
feishu-cli bitable record upsert      --base-token xxx --table-id tblxxx --config '{"fields":{"名称":"测试"}}'
feishu-cli bitable record upsert      --base-token xxx --table-id tblxxx --record-id recxxx --config '{"fields":{"状态":"完成"}}'
# select 字段写入（实测）：单选写字符串 "Todo" 或数组 ["Todo"] 均可（服务端归一化）；多选写数组。
# 未知选项行为按端点分化：单条端点（upsert 的 POST/PATCH）静默自动创建新选项（拼错即产生脏选项且不报错！），
# 批量端点（batch_create/batch_update）拒绝并报 not_found（hint 列出可用选项）。
# 写前先 field list / field search-options 确认选项存在。
# 层级关系（子记录）：用 link 字段写父记录引用数组，如 {"父任务":[{"id":"rec_xxx"}]}；
# 不存在 parent_record_id 参数或独立的子记录 API，不要去找。

# batch-create 两种 body 形态（实测均可用），推荐 create_records 行式（每条记录独立字段 map，
# 可各带不同字段、无需 null 占位，返回 record_id_list）：
#   行式（推荐）：  {"create_records":[{"名称":"Task A","状态":"Todo"},{"名称":"Task B"}]}
#   列式（备选）：  {"fields":["名称","状态"],"rows":[["Task A","Todo"],["Task B",null]]}
feishu-cli bitable record batch-create --base-token xxx --table-id tblxxx --config-file records.json
# batch-update 两种 body 形态（实测均可用）：
#   统一 patch：      {"record_id_list":["rec1","rec2"],"patch":{"状态":["Done"]}}
#   逐记录差异化：    {"update_records":{"rec1":{"分数":88},"rec2":{"分数":77,"状态":["Blocked"]}}}
feishu-cli bitable record batch-update --base-token xxx --table-id tblxxx --config-file records.json
feishu-cli bitable record delete      --base-token xxx --table-id tblxxx --record-id recxxx

# batch-delete：POST /records/batch_delete，服务端单次最多 200 条（实测 201 报 800010701）；
# 超过 200 条 CLI 自动按 200 条一批**串行**提交（同表并发写会触发 1254291），某批失败即停并报告已删除条数
# --record-ids CSV 或 --from-file 任选其一；服务端不校验 record_id 是否存在，需要确认时读回
feishu-cli bitable record batch-delete --base-token xxx --table-id tblxxx --record-ids rec_1,rec_2,rec_3
feishu-cli bitable record batch-delete --base-token xxx --table-id tblxxx --from-file ids.txt   # 每行一个 record_id

# share-link：批量生成记录共享链接（v1.29+ 新增），单次最多 100 条
feishu-cli bitable record share-link  --base-token xxx --table-id tblxxx --record-ids rec_1,rec_2,rec_3

# history-list：GET + query params（不是 POST body），--record-id 必填
feishu-cli bitable record history-list --base-token xxx --table-id tblxxx --record-id recxxx
feishu-cli bitable record history-list --base-token xxx --table-id tblxxx --record-id recxxx --page-size 50 --max-version 20

# 附件：upload 为 3 步编排（校验附件字段 → medias 上传 → append_attachments），单次 ≤50 个文件；
# 单文件 >20MB 自动走 upload_prepare/upload_part/upload_finish 分片上传（上限 2GB）；
# --field-id 可传字段名，非附件字段会在上传前直接报错
feishu-cli bitable record upload-attachment   --base-token xxx --table-id tblxxx \
  --record-id recxxx --field-id 附件 --file ./report.pdf --file ./shot.png     # --file 可重复
feishu-cli bitable record download-attachment --base-token xxx --table-id tblxxx \
  --record-id recxxx --output ./downloads/                                     # 省略 --file-token 下全部
feishu-cli bitable record download-attachment --base-token xxx --table-id tblxxx \
  --record-id recxxx --file-token boxcnxxxx --output ./a.pdf --overwrite       # 指定单个附件，已存在则覆盖
feishu-cli bitable record remove-attachment   --base-token xxx --table-id tblxxx \
  --record-id recxxx --field-id fldxxx --file-token boxcnxxxx                   # --file-token 可重复
```

> **record list 分页（实测）**：服务端 limit 范围 1-2000，响应是矩阵结构（`fields`/`field_id_list`/`field_type_list`/`record_id_list`/`data`/`has_more`/`rev`/`query_context`），`data[i]` 与 `record_id_list[i]` 一一对应；服务端不返回 total，CLI 在单页 `has_more=true` 时补 `next_offset`（续翻传 `--offset <next_offset>`）。`--page-all` 未指定 `--limit` 时每页 500 条、最多 1000 页，合并为同一矩阵输出（`has_more=false`）；分页期间表数据被改动（`rev` 变化）会在 stderr 告警并输出 `rev_changed: true`，表结构被改动则报错退出（列不同无法合并）。带 `--view-id` 时只返回该视图筛选后的记录与可见字段（`query_context` 标明范围）。
> 附件文件名：`download-attachment` 用附件**原始文件名**保存（不再用 file_token 命名）；目标已存在会直接报错，加 `--overwrite` 覆盖。三个附件命令均支持 `--dry-run`（写前预览请求体）；`upload/remove-attachment` 支持 `--format/--jq`，`download-attachment` 不支持（仅打印 JSON）。

### 视图 view（5 命令 + 12 配置命令）

```bash
# 基础 CRUD
feishu-cli bitable view list   --base-token xxx --table-id tblxxx
feishu-cli bitable view get    --base-token xxx --table-id tblxxx --view-id viewxxx
feishu-cli bitable view create --base-token xxx --table-id tblxxx --name "看板视图" --view-type kanban
feishu-cli bitable view delete --base-token xxx --table-id tblxxx --view-id viewxxx
feishu-cli bitable view rename --base-token xxx --table-id tblxxx --view-id viewxxx --name "新名字"

# 视图配置 get/set（6 种 × 2 = 12 命令）— set 方法是 PUT（全量替换），请求体是 base/v3 结构（实测）
# 字段一律可用字段名或字段 ID（键名是 field，不是 v1 的 field_id）；get 与 set 都输出配置本体：
# group/sort/visible-fields 为数组，filter/timebar/card 为对象
feishu-cli bitable view view-filter-get        --base-token xxx --table-id tblxxx --view-id viewxxx
feishu-cli bitable view view-filter-set        --base-token xxx --table-id tblxxx --view-id viewxxx \
  --config '{"logic":"and","conditions":[["状态","intersects",["进行中"]],["截止","empty"]]}'

feishu-cli bitable view view-sort-get          --base-token xxx --table-id tblxxx --view-id viewxxx
# sort/group/visible-fields 的 --config 可传数组，自动包装为 {"sort_config":[...]} / {"group_config":[...]} / {"visible_fields":[...]}
feishu-cli bitable view view-sort-set          --base-token xxx --table-id tblxxx --view-id viewxxx \
  --config '[{"field":"截止时间","desc":false}]'

feishu-cli bitable view view-group-get         --base-token xxx --table-id tblxxx --view-id viewxxx
feishu-cli bitable view view-group-set         --base-token xxx --table-id tblxxx --view-id viewxxx \
  --config '[{"field":"状态","desc":false}]'

feishu-cli bitable view view-visible-fields-get --base-token xxx --table-id tblxxx --view-id viewxxx
# 完整有序列表：未列出的字段被隐藏（不删除数据）
feishu-cli bitable view view-visible-fields-set --base-token xxx --table-id tblxxx --view-id viewxxx \
  --config '["任务名称","负责人","截止时间"]'

# timebar 仅 gantt / calendar 视图支持
feishu-cli bitable view view-timebar-get       --base-token xxx --table-id tblxxx --view-id viewxxx
feishu-cli bitable view view-timebar-set       --base-token xxx --table-id tblxxx --view-id viewxxx \
  --config '{"start_time":"开始时间","end_time":"结束时间","title":"任务名称"}'

# card 仅 gallery / kanban 视图支持
feishu-cli bitable view view-card-get          --base-token xxx --table-id tblxxx --view-id viewxxx
feishu-cli bitable view view-card-set          --base-token xxx --table-id tblxxx --view-id viewxxx \
  --config '{"cover_field":"产品图片"}'
```

> **视图配置自动包装规则**（减少用户样板）：
> - `view-sort-set` 可直接传 `[{...}]` 数组，自动包成 `{"sort_config":[...]}`（最多 10 条，本地校验）
> - `view-group-set` 可直接传 `[{...}]` 数组，自动包成 `{"group_config":[...]}`（最多 3 条）
> - `view-visible-fields-set` 可直接传字段名数组，自动包成 `{"visible_fields":[...]}`
> - filter / timebar / card 传对象
> - 旧版 v1 结构（`filter_info`/`conjunction`、`field_id` 键、`view_field`、`timebar.start_field_id`、`card.cover_field_id`）在 base/v3 会报 800010701（Unrecognized key / Required），不要再用

**视图配置 JSON Schema 速查（base/v3，实测）**：

```jsonc
// view-filter（与 record list --filter-json 同一套 tuple DSL）；清空传 {"conditions":[]}
{"logic": "and", "conditions": [["状态", "intersects", ["进行中"]], ["截止", "empty"]]}

// view-sort（数组顺序即优先级，最多 10 条；空数组清除）
{"sort_config": [{"field": "截止时间", "desc": false}]}

// view-group（最多 3 条；空数组清除；支持 grid/kanban/gantt）
{"group_config": [{"field": "状态", "desc": false}]}

// view-visible-fields（有序完整列表）
{"visible_fields": ["任务名称", "负责人", "截止时间"]}

// view-timebar（gantt / calendar）
{"start_time": "开始时间", "end_time": "结束时间", "title": "任务名称"}

// view-card（gallery / kanban；cover_field 用附件字段，null 清除封面）
{"cover_field": "产品图片"}
```

> 新建视图后立即写配置偶发 `800030501 not_found`（写后读延迟），等 1-2 秒重试即可；连续写视图配置偶发 `800004135 ... limited`（按接口方法限流），CLI 已按 1s/2s/4s 自动重试。

### 角色 role（5 命令 + 协作者 member 5 命令）

```bash
feishu-cli bitable role list   --base-token xxx
feishu-cli bitable role get    --base-token xxx --role-id rolxxx
feishu-cli bitable role create --base-token xxx --config-file role.json
feishu-cli bitable role update --base-token xxx --role-id rolxxx --config '...'
feishu-cli bitable role delete --base-token xxx --role-id rolxxx
```

#### 角色协作者 role member（5 命令）

把用户/群/部门加入或移出某个角色（走 `bitable/v1` 协作者端点 `apps/{app_token}/roles/{role_id}/members`）。

```bash
feishu-cli bitable role member list         --base-token xxx --role-id rolxxx           # 支持 --page-size(≤100)/--page-token
feishu-cli bitable role member create       --base-token xxx --role-id rolxxx --member-id ou_xxx
feishu-cli bitable role member delete       --base-token xxx --role-id rolxxx --member-id ou_xxx
# 批量增删：--member-ids 逗号分隔，单次 ≤100
feishu-cli bitable role member batch-create --base-token xxx --role-id rolxxx --member-ids ou_a,ou_b,ou_c
feishu-cli bitable role member batch-delete --base-token xxx --role-id rolxxx --member-ids ou_a,ou_b
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
feishu-cli bitable data-query --base-token xxx --config '{
  "datasource": {"type": "table", "table": {"tableId": "tblxxx"}},
  "dimensions": [{"field_name": "状态", "alias": "status"}],
  "measures":   [{"field_name": "金额", "aggregation": "sum", "alias": "total"}],
  "sort":       [{"field_name": "total", "order": "desc"}],
  "shaper":     {"format": "flat"}
}'
```

底层调用：`POST /open-apis/base/v3/bases/{base_token}/data/query`

DSL 要点（官方 LiteQuery 协议，实测可用；旧示例 `{"dimensions":[{"field_id":...}],"measures":[{"type":"sum"}]}` 实测报 800004006 `datasource.table is required`）：

| 字段 | 说明 |
|---|---|
| `datasource` | **必填**，`{"type":"table","table":{"tableId":"tbl..."}}` 或 `{"tableName":"表名"}` |
| `dimensions` | 分组维度 `[{"field_name","alias"}]`；与 `measures` 至少一个 |
| `measures` | 度量 `[{"field_name","aggregation","alias"}]`，aggregation: `sum/avg/min/max/count/count_all/distinct_count` |
| `filters` | `{"type":1,"conjunction":"and","conditions":[{"field_name","operator","value":[...]}]}`，operator: `is/isNot/contains/doesNotContain/isEmpty/isNotEmpty/isGreater/isGreaterEqual/isLess/isLessEqual`（与 record list 的 tuple DSL 不同）；`value` 是**字符串数组**（数字也写 `["6"]`，写 `[6]` 报 800004006 failed to parse lite filter），`isEmpty/isNotEmpty` 传 `[]` |
| `sort` | `[{"field_name":"字段名或 alias","order":"asc|desc"}]` |
| `pagination` | `{"limit":N}`，最大 5000，不支持 offset |
| `shaper` | 固定 `{"format":"flat"}` |

- 字段用 **`field_name`（字段名，区分大小写）**，不是 field_id；`alias` 只能用英文且全局唯一
- 结果在 `main_data` 数组，每格形如 `{"value": ...}`；`sum/avg` 等结果可能是字符串（实测 `"30.00"`），维度为空的记录单独成一组（值为 `null`）
- 刚写入的记录可能尚未计入聚合（写后读延迟），结果异常时稍等几秒重查
- CLI 本地校验 `datasource` 与 `dimensions/measures` 是否存在；高级权限多维表格需要完全访问（FA）权限

### 工作流 workflow（6 命令）

```bash
feishu-cli bitable workflow list   --base-token xxx --status enabled           # 自动翻页取全部
feishu-cli bitable workflow list   --base-token xxx --page-size 50 --page-token TOKEN   # 只取指定页
feishu-cli bitable workflow get    --base-token xxx --workflow-id wkfxxxx          # 含 steps
feishu-cli bitable workflow create --base-token xxx --config '{"title":"My Workflow","steps":[...]}'
feishu-cli bitable workflow update --base-token xxx --workflow-id wkfxxxx --config-file wf.json  # PUT 整体替换
feishu-cli bitable workflow enable  --base-token xxx --workflow-id wkfxxxx
feishu-cli bitable workflow disable --base-token xxx --workflow-id wkfxxxx
```

> `update` 是 PUT 整体替换，未提供的字段不保留；`workflow_id` 为 `wkf` 前缀。
> `create/update` 会本地预检 AI 步骤：`AIAnalysisAction` 的 `analysis_table_names` 必须是字符串数组、`identity_type` 只能是 `maker|triggerPersonal`；`AIClassificationBranch` 不支持 `mode`（只有互斥模式），`classes` 至少 2 个、`name` 非空不重复、`desc` 为字符串。
> 提醒触发器 `ReminderTrigger` 的 `offset`：触发时间 = 日期字段时间 + `offset` × `unit`，**负数 = 提前、正数 = 延后**（如 `{"offset":-1,"unit":"DAY","hour":9}` 是截止前一天 9 点）。
> `get/create/update/enable/disable` 支持 `--dry-run/--format/--jq`；`workflow list` 两者都不支持（直接输出 JSON）。`steps` 的结构复杂（触发器、动作、分支），建议先 `workflow get` 一个已有工作流作模板再改。

### 仪表盘 dashboard（7 命令 + 仪表盘块 block 6 命令 + 分享 share 2 命令）

```bash
# 仪表盘 CRUD；create/update 支持便捷字段 --name/--theme-style 或 --config/--config-file
feishu-cli bitable dashboard list    --base-token xxx                              # 自动翻页取全部
feishu-cli bitable dashboard copy    --base-token xxx --dashboard-id blkxxxx --name "副本"
feishu-cli bitable dashboard create  --base-token xxx --name "运营看板"
feishu-cli bitable dashboard get     --base-token xxx --dashboard-id blkxxxx
feishu-cli bitable dashboard update  --base-token xxx --dashboard-id blkxxxx --name "新名字"
feishu-cli bitable dashboard delete  --base-token xxx --dashboard-id blkxxxx
feishu-cli bitable dashboard arrange --base-token xxx --dashboard-id blkxxxx     # 服务端智能排版，无 body

# 仪表盘块 block CRUD；create --type 取值见下
feishu-cli bitable dashboard block create --base-token xxx --dashboard-id blkxxxx \
  --type column --name "按状态统计" --data-config '{"table_name":"任务","count_all":true,"group_by":[{"field_name":"状态","mode":"integrated"}]}'
# --position：12 列栅格中的位置与大小，x/y/w/h 必须同时给出且为数字；省略则服务端自动布局
feishu-cli bitable dashboard block create --base-token xxx --dashboard-id blkxxxx \
  --type statistics --name "记录数" --data-config '{"table_name":"任务","count_all":true}' --position '{"x":0,"y":0,"w":6,"h":4}'
# 排行榜 ranking / NPS 图 nps 必须带 --data-config；nps 的 group_by 必须是评分（rating）字段，
# category_range 为 [min, 贬损上限, 被动上限, max]，首尾须等于该评分字段的 min/max
feishu-cli bitable dashboard block create --base-token xxx --dashboard-id blkxxxx --type ranking --name "Top 负责人" \
  --data-config '{"table_name":"订单","group_by":[{"field_name":"负责人"}],"series":[{"field_name":"金额","rollup":"SUM"}]}'
feishu-cli bitable dashboard block create --base-token xxx --dashboard-id blkxxxx --type nps --name "满意度" \
  --data-config '{"table_name":"问卷","group_by":[{"field_name":"评分"}],"category_range":[0,6,8,10]}'
feishu-cli bitable dashboard block list   --base-token xxx --dashboard-id blkxxxx
feishu-cli bitable dashboard block get    --base-token xxx --dashboard-id blkxxxx --block-id chtxxxx
feishu-cli bitable dashboard block get-data --base-token xxx --block-id chtxxxx         # 读取图表计算结果（不需要 dashboard-id）
feishu-cli bitable dashboard block update --base-token xxx --dashboard-id blkxxxx --block-id chtxxxx --name "新块名"
feishu-cli bitable dashboard block update --base-token xxx --dashboard-id blkxxxx --block-id chtxxxx --position '{"x":0,"y":4,"w":12,"h":4}'
feishu-cli bitable dashboard block delete --base-token xxx --dashboard-id blkxxxx --block-id chtxxxx

# 仪表盘分享（base/v3 share；update 每次只改一个字段——同时传多个报用法错误，退出码 2；布尔值用 =false 关闭）
feishu-cli bitable dashboard share get    --base-token xxx --dashboard-id blkxxxx
feishu-cli bitable dashboard share update --base-token xxx --dashboard-id blkxxxx --enabled
feishu-cli bitable dashboard share update --base-token xxx --dashboard-id blkxxxx --access-scope tenant   # invite|tenant|anyone
feishu-cli bitable dashboard share update --base-token xxx --dashboard-id blkxxxx --show-source=false
```

> block `--type` 取值：`column|bar|line|pie|ring|area|combo|scatter|funnel|wordCloud|radar|ranking|statistics|nps|text`；图表块 `--data-config` 传 `table_name`/`series|count_all`/`group_by`（用 `field_name` 字段名）/`filter`，文本块传 `text`。NPS 的 `group_by` 用非评分字段时服务端报 `code=1 NPS 快照包含无法公开表达的兼容配置`（实测）。`block get-data` 返回图表协议 JSON（`dimensions`/`measures`/`main_data`），不支持计算数据的图表类型改用 `data-query` 按同样的表、维度、度量查询。`--data-config` 的内部结构由飞书图表 schema 定义、本地无离线校验（`--dry-run` 也不校验内部字段），建议用 `block get` 先取一个已有图表块的结构作模板再改。
> `--theme-style` 写入 `theme.theme_style`，合法取值由飞书仪表盘主题 schema 定义（CLI 不做枚举校验）；不确定时省略此字段用默认主题（实测为 `default`），或 `dashboard get` 一个已配好主题的看板看其真实取值。
> ID 形态（实测）：`dashboard create` 返回的 `dashboard_id` 形如 `blk…`，图表块 `block_id` 形如 `cht…`；`block list` 输出 `{"items":[...],"total","has_more"}`，按 `--page-token` 手动翻页。开启分享后 `share_url` 形如 `https://xxx.feishu.cn/share/base/dashboard/shrxxx`。
> dashboard / block / share 全部写命令（create/update/delete/copy/arrange、share update）均支持 `--dry-run` 预览请求体；全部命令支持 `--format json|pretty|table|ndjson|csv` 与 `--jq`。

### 表单 form（7 命令 + 表单问题 field 4 命令 + 分享 share 2 命令）

```bash
# 表单 CRUD（form_id 即表单视图的 view_id，create 输出的 id）
feishu-cli bitable form create --base-token xxx --table-id tblxxx --name "报名表" --description "活动报名"
feishu-cli bitable form list   --base-token xxx --table-id tblxxx                    # 自动翻页列出全部表单；--page-token 只取指定页
feishu-cli bitable form get    --base-token xxx --table-id tblxxx --form-id vewxxx
feishu-cli bitable form patch  --base-token xxx --table-id tblxxx --form-id vewxxx --name "新名字"
# patch 一次开启共享并限制范围（走 bitable/v1；布尔 flag 写 --shared / --shared=false，不要写 --shared true|false）
feishu-cli bitable form patch  --base-token xxx --table-id tblxxx --form-id vewxxx \
  --shared --shared-limit tenant_editable --submit-limit-once
feishu-cli bitable form delete --base-token xxx --table-id tblxxx --form-id vewxxx   # form_id 即表单视图 view_id

# 按分享 token（shr 前缀）取详情 / 提交，无需 base_token
feishu-cli bitable form detail --share-token shrcnxxxx
feishu-cli bitable form submit --share-token shrcnxxxx --content '{"评分":5,"评价":"很好"}'  # 不处理附件

# 表单问题 field（别名 questions）：list/patch + 批量 create/delete（单次 ≤10）
feishu-cli bitable form field list   --base-token xxx --table-id tblxxx --form-id vewxxx
feishu-cli bitable form field create --base-token xxx --table-id tblxxx --form-id vewxxx \
  --questions '[{"type":"text","title":"你的名字","required":true}]'
feishu-cli bitable form field patch  --base-token xxx --table-id tblxxx --form-id vewxxx --config-file q.json
# ⚠️ 删除题目默认会同时删除底层字段及该列全部记录数据（问题 ID 就是字段 ID，不可恢复）：
#    只想从表单移除题目用 --keep-field（保留字段与数据，可之后以 use_existing_field=true + field_id 加回）；
#    不带 --keep-field 时为危险操作，交互终端需确认，非交互（Agent/管道/cron）必须加 --yes，否则退出码 10 且不执行
feishu-cli bitable form field delete --base-token xxx --table-id tblxxx --form-id vewxxx --question-ids fld001,fld002 --keep-field
feishu-cli bitable form field delete --base-token xxx --table-id tblxxx --form-id vewxxx --question-ids fld001 --yes

# 表单分享（base/v3 share；update 每次只改一个字段，同时传多个报用法错误）
feishu-cli bitable form share get    --base-token xxx --table-id tblxxx --form-id vewxxx
feishu-cli bitable form share update --base-token xxx --table-id tblxxx --form-id vewxxx --enabled
feishu-cli bitable form share update --base-token xxx --table-id tblxxx --form-id vewxxx --access-scope anyone   # invite|tenant|anyone
feishu-cli bitable form share update --base-token xxx --table-id tblxxx --form-id vewxxx --require-login=false   # 另有 --allow-anonymous
```

> 表单问题字段：`title`(必填)/`type`(text/number/select/datetime/user/attachment/location)/`description`/`required`/`multiple`/`options` 等；`submit` 如需附件先用 `record upload-attachment` 思路拿 file_token 再写进 `--content`。
> **form patch 走 `bitable/v1`**（不同于其它 form 命令的 base/v3）：因 `--shared`/`--shared-limit`/`--submit-limit-once` 是 bitable/v1 字段，整体路由到 bitable/v1 让分享相关字段一次生效。`--shared-limit` 取值：`off | tenant_editable | anyone_editable`；仅显式设置的便捷字段才提交，复杂场景可用 `--config/--config-file` 裸传完整请求体。
> **form submit 的 `--content` 是裸字段 map**（如 `{"评分":5}`），**不要**外包 `{"fields":{...}}`——CLI 不再自动解包，多套一层会丢数据。
> 开启分享后 `share_url` 形如 `https://xxx.feishu.cn/share/base/shrxxx`，其中 `shr...` 即 `form detail/submit` 的 `--share-token`（也可用 `bitable resolve --url` 取出）。
> **分享范围要核对**：实测表单首次 `share update --enabled` 后 `access_scope` 变为 `anyone`（互联网可访问），之后再开关保持已设置的范围。只允许组织内填写时，开启后立即 `--access-scope tenant` 并用 `share get` 读回确认。
> form/field 全部写命令（create/patch/delete、field create/patch/delete、share update、detail/submit）支持 `--dry-run` 预览；全部 form 命令支持 `--format/--jq`；`form create/patch`、`field create/delete` 均支持 `--config/--config-file` 裸传完整请求体作为便捷字段的逃生通道。

## 典型工作流

### 建表 → 加字段 → 写入数据 → 建视图配过滤

```bash
# 1. 创建多维表格
BASE_TOKEN=$(feishu-cli bitable create --name "任务跟踪" -o json | jq -r '.base_token')   # 响应是平铺结构

# 2. 创建数据表（响应是平铺的表对象，ID 键为 id）
TABLE_ID=$(feishu-cli bitable table create --base-token $BASE_TOKEN --name "待办" | jq -r '.id')

# 3. 添加字段
feishu-cli bitable field create --base-token $BASE_TOKEN --table-id $TABLE_ID --config '{
  "name": "状态",
  "type": "select",
  "options": [{"name": "待办"}, {"name": "进行中"}, {"name": "完成"}]
}'   # v3 判别式：options/multiple 等属性在顶层，不要包 property（会报 Unrecognized key）

# 4. 批量写入记录
feishu-cli bitable record batch-create --base-token $BASE_TOKEN --table-id $TABLE_ID --config-file records.json

# 5. 创建自定义视图
VIEW_ID=$(feishu-cli bitable view create --base-token $BASE_TOKEN --table-id $TABLE_ID --name "进行中" --view-type grid | jq -r '.id')

# 6. 配置视图过滤（base/v3 tuple DSL；新建视图偶发写后读延迟，not_found 时等 1-2 秒重试）
feishu-cli bitable view view-filter-set --base-token $BASE_TOKEN --table-id $TABLE_ID --view-id $VIEW_ID \
  --config '{"logic":"and","conditions":[["状态","intersects",["进行中"]]]}'

# 7. 配置排序（按截止时间升序；字段可用名称）
feishu-cli bitable view view-sort-set --base-token $BASE_TOKEN --table-id $TABLE_ID --view-id $VIEW_ID \
  --config '{"sort_config":[{"field":"截止时间","desc":false}]}'

# 8. 读取全部记录（自动翻页）
feishu-cli bitable record list --base-token $BASE_TOKEN --table-id $TABLE_ID --page-all
```

## 权限要求

User 身份一次性授权：`feishu-cli auth login --domain bitable --recommend`；执行前可 `feishu-cli auth check --scope "<scope>"` 预检。
Bot 身份需应用开通对应 scope，且 Bot 是目标多维表格的协作者（见上文「身份选择」）。

| 命令 | 所需 scope（与官方 CLI 一致的细粒度 scope） |
|---|---|
| 多维表格 get / create / copy / update | `base:app:read` / `base:app:create` / `base:app:copy` / `base:app:update` |
| 数据表、字段、记录 | `base:table:*`、`base:field:*`、`base:record:*`（`read` / `create` / `update` / `delete` 按操作取） |
| 视图与视图配置 | 读 `base:view:read`，写 `base:view:write_only` |
| 记录修改历史 | `base:history:read` |
| 角色 | `base:role:read` / `create` / `update` / `delete` |
| 角色协作者（role member） | `bitable:app` / `bitable:app:readonly` / `base:collaborator:read` 任一（走 bitable/v1 协作者端点，**不是** `base:role`；缺失时报 99991679 并列出这三个，实测） |
| 高级权限 advperm、`bitable update --is-advanced` | `base:app:update` |
| 工作流 | `base:workflow:read` / `create` / `update` |
| 仪表盘 / 表单 | `base:dashboard:*` / `base:form:*`；`dashboard share get` 需要 `base:dashboard:update`，`form share get` 需要 `base:form:update` |
| 顶层块（block list、resolve 判型） | `base:block:read`；resolve 的 /wiki/ 链接另需 `wiki:node:retrieve`，/record/ 链接需 `base:record:read` |
| 附件上传 | `base:record:update`、`base:field:read`（字段类型预检）、`docs:document.media:upload` |
| data-query | `base:table:read`；普通多维表格有阅读权限即可，开启高级权限的多维表格需要完全访问（FA） |

## filter DSL（record list / record search 结构化过滤，实测验证）

`--filter-json` 的语法（tuple DSL），两个入口共用：

```json
{"logic":"and|or","conditions":[[字段名或字段ID, operator, 值], ...]}
```

- **operator 全集**：`==` `!=` `>` `>=` `<` `<=` `intersects`（文本包含）`disjoint` `empty` `non_empty`
- **empty/non_empty** 可省略值写二元组：`["单选","empty"]`
- **值按字段类型写法**（写错会 0 命中或报错）：

| 字段类型 | 值写法 | 示例 |
|---|---|---|
| 文本 / 公式 / 查找引用 | 字符串（包含匹配用 `intersects`） | `["标题","intersects","发布"]` |
| 数字 | 数字 | `["分数",">=",70]` |
| 单选 / 多选 | **选项名数组（filter 条件里单选也必须数组；与写入 CellValue 不同——写入时单选字符串/数组均可）** | `["状态","==",["Doing"]]` |
| 复选框 | 布尔 | `["完成","==",true]` |
| 人员 / 群 / 关联记录 | 对象数组 | `["负责人","intersects",[{"id":"ou_xxx"}]]` |
| 日期 | `"ExactDate(2026-01-01)"` 或 `"Today"`/`"Yesterday"`/`"Tomorrow"`；**只支持 `==` `>` `<` `empty` `non_empty`，不支持 `>=` `<=`**（实测 800010506），范围用 `>` + `<` 组合 | `["截止","==","Today"]` |

排序 `--sort-json`：`[{"field":"分数","desc":true}]`，数组顺序即优先级，最多 10 条。

**两个入口的差异（重要）**：

| | `record list` | `record search` |
|---|---|---|
| 端点 | GET /records（filter/sort 作 query 参数） | POST /records/search（filter 放 body） |
| keyword | **不需要** | **服务端必填** keyword + search_fields |
| 适用 | 纯结构化条件筛选（按状态/数值/日期/空值） | 关键词全文搜索，filter 做交集收窄 |
| 注意 | — | 新写入记录全文索引有 ~20s 延迟 |

## 注意事项

- **base_token / app_token 是同一个值**：飞书新旧文档用两种叫法，CLI 只认 `--base-token`（`--app-token` 已删除）；base/v3 所需的 `X-App-Id` header 自动注入
- **--config / --config-file 两种输入**：所有写操作支持 inline JSON 或文件路径
- **--dry-run 预览（仅部分命令支持）**：`dashboard` 写（含 copy/arrange）、`dashboard block` 写与 `block get-data`、`dashboard share update`、`form` 写与 `detail/submit`、`form field` 写、`form share update`、`workflow get/create/update/enable/disable`、`role member` 写、`record upload/download/remove-attachment`、`bitable update`、`bitable resolve`。传 `/wiki/` 链接作 `--base-token` 时 dry-run 不解析（路径里以 `<wiki:token>` 占位）。**其余命令不支持**（`record batch-create/batch-update/upsert/delete/batch-delete`、`table/field/view/role create·update·delete`、`view-*-set`、`advperm enable/disable`、`bitable create/copy`、`data-query` 等传 `--dry-run` 会报 `unknown flag`）。download-attachment 的 dry-run 只打 stdout，不写 `--output`
- **--format / --jq 输出控制**：支持 `--format json|pretty|table|ndjson|csv`（默认 json）+ `--jq`（内置 gojq）的是 `record batch-get`、`record upload/remove-attachment`、`bitable update`、`bitable resolve`、`block list`、`dashboard` / `dashboard block` / `form` / `form field` / `role member` 全部命令（含 share）、`workflow`（list 除外）。**其余命令不支持**：`record list/get/search/批量写`、`table/field/view/role list·CRUD`、`view-*-get/set`、`workflow list`、`data-query`、`advperm`、`record download-attachment` 直接打印 JSON；`bitable create/copy` 默认输出文本、加 `-o json` 输出 JSON，`bitable get` 默认即 JSON。需要过滤时接外部 `jq`
- **删除无确认门禁**：`table/field/view/role/record delete`、`record batch-delete`、`dashboard/form delete` 执行即生效、不可恢复，先读回确认目标；唯一需要 `--yes` 的是不带 `--keep-field` 的 `form field delete`
- **批量上限**：`record batch-create` / `batch-update` 单批 ≤200 条；`batch-delete` 服务端单批 ≤200 条，CLI 超出时自动按 200 条串行分批（输出带 `batch_count`）
- **同一张表的写操作要串行**：并发写会触发 1254291 写冲突；视图配置等接口连续调用偶发 800004135 按方法限流，CLI 自动重试
- **写后读延迟**：刚创建/删除的记录、刚建的视图，立即 `record list` / `data-query` / 写视图配置可能还是旧状态或报 not_found（实测），等 1-2 秒再读
- **列表命令默认取全**：`table/field/view list` 按 total 取完；`form/workflow/dashboard list` 未传 `--page-token` 时按 page_token 自动翻页；`record list` 默认 100 条，`--page-all` 取全部；`dashboard block list`、`role member list`、`form field list` 按 `--page-token` 手动翻页
- **角色接口输出已解包**：`role list` 输出 `{"base_roles":[{"role_id","role_name","role_type"}],"total"}`，`role get` 直接输出角色对象（`table_rule_map` 等），不再有外层 `data`、`base_roles` 项也不再是 JSON 字符串；内层 code≠0 时报错
- **视图类型**：`view create --view-type` 可选值：`grid / kanban / gallery / gantt / calendar`
- **附件**：上传/移除单次 ≤50 个文件，单文件 >20MB 自动分片（上限 2GB）；download 省略 `--file-token` 时下载记录全部附件，`--output` 必须是已存在目录
- **form_id = view_id**：表单的 form_id 即表单视图的 view_id；`detail`/`submit` 用 `share-token`（shr 前缀，从分享链接提取）无需 base_token
