# 飞书 OKR 工作流

覆盖 OKR 周期查询（`cycle list/detail`）、目标与关键结果创建/更新（`objective` / `key-result`）、进展记录
（`progress list/get/create/update/delete`、`upload-image`）、评论（`comment list/create`）；量化指标等未封装能力走 `feishu-cli api` 透传。
完整参数以 `feishu-cli okr <cmd> --help` 为准。

## 目录

1. [核心概念](#核心概念)
2. [身份与权限](#身份与权限)
3. [命令速查](#命令速查)
4. [周期：cycle list / detail](#周期cycle-list--detail)
5. [进展记录：progress](#进展记录progress)
6. [目标 / 关键结果写入与评论（v2）](#目标--关键结果写入与评论v2)
7. [未封装的能力（api 透传）](#未封装的能力api-透传)
8. [错误处理](#错误处理)
9. [相关技能](#相关技能)

## 核心概念

| 层级 | 对象 | 说明 | 命令 |
|------|------|------|------|
| 1 | **Cycle（周期）** | 每个用户有自己的周期（v2 用户周期，`tenant_cycle_id` 指向租户级周期） | `cycle list` / `cycle detail` |
| 2 | **Objective（目标 O）** | 用户周期内的目标 | `objective create/update`；读取用 `cycle detail` |
| 3 | **Key Result（关键结果 KR）** | O 下的可量化结果 | `key-result create/update`；读取用 `cycle detail` |
| 4 | **Progress Record（进展记录）** | O 或 KR 下的一条进展 | `progress *`、`upload-image` |
| - | **Comment（评论）** | 周期 / 进展 / O / KR 上的评论 | `comment list/create` |

- **两种周期 ID 不通用**：`cycle list` 默认返回 v2 **用户周期 ID**（`GET /open-apis/okr/v2/cycles?user_id=`），`cycle detail`
  与 `objective create --cycle-id` 只认它；`cycle list --tenant` 返回 v1 **租户周期**（`/open-apis/okr/v1/periods`，带名称），
  其 ID 不能用于 v2 接口。行为变更：此前版本 `cycle list` 默认走 v1。
- 每条进展记录只挂在一个目标**或**一个关键结果上（`--objective-id` / `--key-result-id` 二选一）。
- **ContentBlock 键名分两套**：进展记录 `progress create/update` 走 v1 `progress_records`，`--content-json` 用 v1 键名
  （`type` / `textRun`）；目标、关键结果、评论走 v2，用 snake_case 键名（`block_element_type` / `paragraph_element_type` / `text_run`）。
  传 `--content` 纯文本时 CLI 自动按对应版本包装；手写 `--content-json` 时不要混用两套键名。

## 身份与权限

OKR 命令组默认 **`--as bot`**（App/Tenant Token，无需 `auth login`，适合 cron/无人值守）；`--as user` 用 User Token，
`--as auto` User 优先、Tenant 兜底。`--as` 是命令组 persistent flag，所有子命令继承。

身份墙按端点分化（实测）：
- 只有 `cycle list --tenant`（v1 periods）**只收 Tenant Token**，User 身份报 99991668。
- `cycle list`（v2）、`cycle detail`、`progress list/get`、`comment list` 都支持 User 与 Tenant：缺 scope 时
  Bot 报 99991672（应用未开通）、User 报 99991679（用户未授权）。
- `comment create` **必需 User Token**，与 `--as` 无关。
- `cycle list` 的 `--user-id` 默认取当前登录用户，只决定查询对象；请求身份仍按 `--as`（默认 bot）。未登录时必须传 `--user-id`。

| 命令 | 所需 scope（任一即可；Bot 在开放平台为应用开通，User 需 `auth login --scope` 授权同名 scope） |
|------|----------------------|
| `cycle list` | `okr:okr.period:readonly`（`--tenant` 时 `okr:okr:readonly`、`okr:okr` 亦可） |
| `cycle detail` | `okr:okr.content:readonly`（实测 Bot/User 均只认此项） |
| `progress list` | `okr:okr.progress:readonly`（实测服务端只认此项） |
| `progress get` | `okr:okr.progress:readonly`、`okr:okr:readonly`、`okr:okr` |
| `progress create` / `update` | `okr:okr`、`okr:okr.progress:writeonly` |
| `progress delete` | `okr:okr`、`okr:okr.progress:delete` |
| `upload-image` | `okr:okr`、`okr:okr.progress.file:upload` |
| `objective` / `key-result` create/update | `okr:okr.content:writeonly` |
| `comment list` / `comment create` | `okr:okr.comment.readonly` / `okr:okr.comment.writeonly` |

## 命令速查

| 子命令 | 说明 | 必填参数 |
|--------|------|---------|
| `okr cycle list` | 用户的 OKR 周期（默认当前登录用户） | — |
| `okr cycle detail <cycle_id>` | 周期下全部目标 + 关键结果（含 ID） | 用户周期 ID |
| `okr progress list` | 某 O/KR 下的进展 | `--objective-id` 或 `--key-result-id` |
| `okr progress get <progress_id>` | 单条进展详情 | 进展 ID |
| `okr progress create` | 创建进展 | O/KR ID（二选一）+ `--content`/`--content-json`（二选一） |
| `okr progress update <progress_id>` | 更新进展内容/进度 | 进展 ID + 内容（二选一） |
| `okr progress delete <progress_id>` | 删除进展（不可恢复，需确认） | 进展 ID |
| `okr upload-image` | 上传进展图片素材（ContentBlock imageList 引用） | `--file` + O/KR ID（二选一） |
| `okr objective create` | 在用户周期下创建目标 | `--cycle-id` + 内容 |
| `okr objective update <id>` | 更新目标内容/备注/得分/截止 | 至少一个字段 |
| `okr key-result create` | 在目标下创建关键结果 | `--objective-id` + 内容 |
| `okr key-result update <id>` | 更新关键结果内容/得分/截止 | 至少一个字段 |
| `okr comment list` | 列出评论 | `--target-type` + `--target-id` |
| `okr comment create` | 创建评论或回复（必需 User Token） | `--target-type` + `--target-id` + 内容 |

**OKR 对组织可见**：所有写命令（`progress create/update/delete`、`objective`/`key-result` create/update、`comment create`）
都支持 `--dry-run`（只打印请求，不联网、不解析身份）。先预览并向用户确认目标对象和内容，再去掉 `--dry-run` 执行。
`progress delete` 非交互环境需 `--yes`（否则 exit 10 且不执行）。

## 周期：cycle list / detail

```bash
# 查自己的周期
feishu-cli okr cycle list

# 查某人的周期，只保留与 2026 上半年重叠的
feishu-cli okr cycle list --user-id ou_xxx --time-range 2026-01--2026-06 --output json

# 租户级周期（v1，带名称；ID 不能用于 cycle detail，只能 Bot 身份）
feishu-cli okr cycle list --tenant

# 周期下全部目标与关键结果
feishu-cli okr cycle detail <cycle_id> -o json
```

`cycle list` 输出（v2 用户周期）：`id`（用户周期 ID）、`tenant_cycle_id`、`owner`、`score`、`start_time` / `end_time`（本地时区）、
`cycle_status`（`default` / `normal` / `invalid` / `hidden`）；JSON 顶层另有 `user_id` 与 `current_active_cycles`
（当前时间落在周期内且状态为 default/normal）。`--tenant` 时输出 v1 字段 `id` / `zh_name` / `en_name` / 起止 / 状态。
`--time-range` 格式 `YYYY-MM--YYYY-MM`。

## 进展记录：progress

```bash
# 查目标 / 关键结果下的进展（自动分页）
feishu-cli okr progress list --objective-id 7123456789012345678
feishu-cli okr progress list --key-result-id 7123456789012345678 --output json

# 创建进展（先 --dry-run）
feishu-cli okr progress create \
  --key-result-id 7123456789012345678 \
  --content "完成 8/10 任务" \
  --progress-percent 80 --progress-status normal \
  --source-title "周报 W18" --source-url "https://xxx.feishu.cn/docx/<doc_token>" \
  --dry-run

# 富文本（v1 ContentBlock 键名）
feishu-cli okr progress create --objective-id 7xxx \
  --content-json '{"blocks":[{"type":"paragraph","paragraph":{"elements":[{"type":"textRun","textRun":{"text":"加粗内容","style":{"bold":true}}}]}}]}'

feishu-cli okr progress update <progress_id> --content "已完成 9/10" --progress-percent 90 --progress-status normal --dry-run
feishu-cli okr progress delete <progress_id> --dry-run
```

- `--content` 与 `--content-json` 互斥；纯文本自动包装为 ContentBlock（paragraph + textRun）。
- `--progress-percent` 传 0-100 的数字（CLI 只校验是数字，不校验范围）；`--progress-status` 取 `normal` / `overdue` / `done`，
  **必须配合** `--progress-percent`，单独传报错。`risky` 已无写入语义，传入会报错并给出提示，不会被映射成 overdue。
- `--source-url` 是服务端必填字段，不传时 CLI 填占位 `https://www.feishu.cn/okr/progress`；建议显式传周报等真实链接，
  进展卡片在 OKR 页面展示 `--source-title` 并跳转到该 URL。
- `progress list` 输出 `progress_id`、`create_time` / `modify_time`（本地时区）与 `progress_rate.percent/status`（若有）。

脚本化批量同步多个 KR：

```bash
for kr_id in 7xxx 7yyy 7zzz; do
  feishu-cli okr progress create --key-result-id "$kr_id" --content "自动同步: 当前推进中" --output json
  sleep 1
done
```

## 目标 / 关键结果写入与评论（v2）

```bash
# 创建目标（cycle_id 是 okr cycle list 默认返回的用户周期 ID，不是 --tenant 的租户周期 ID）
feishu-cli okr objective create --cycle-id <cycle_id> --content "提升交付质量" [--notes "口径说明"] [--category-id <id>] --dry-run

# 在目标下创建关键结果
feishu-cli okr key-result create --objective-id <objective_id> --content "缺陷率降到 1% 以下" --dry-run

# 更新：只改传入字段；--score 0-1 最多一位小数；--deadline 毫秒时间戳或 YYYY-MM-DD
feishu-cli okr objective update <objective_id> --score 0.7 --deadline 2026-12-31 --dry-run
feishu-cli okr key-result update <key_result_id> --content "缺陷率降到 0.5%" --dry-run

# 评论：--target-type 取 cycle | progress | objective | key_result
feishu-cli okr comment list --target-type objective --target-id <objective_id>
feishu-cli okr comment create --target-type objective --target-id <objective_id> --content "口径需要再对齐" --select-all --dry-run
```

- `--content` / `--notes` 纯文本自动包装为 v2 ContentBlock；需要 @人、链接时用 `--content-json` / `--notes-json` 传完整 v2 结构。
- 目标/KR 评论必须且只能指定 `--selected-text` / `--select-all` / `--ref-comment-id` 之一（`--select-all` 发送的 `selected_text` 不是字面量 `"*"`，而是与 `--content` 纯文本等长（按字符计）的 `*` 串；改用 `--content-json` 时 CLI 拿不到纯文本，生成的串为空）。
- 租户强制 OKR 分类时创建目标需带 `--category-id`：先 `feishu-cli api GET /open-apis/okr/v2/categories --as user` 查
  （该端点需用户 scope `okr:okr.setting:read`，缺失报 99991679）。
- 验证状态：测试租户未开通 OKR 写 scope，以上写命令只做了 `--dry-run` 与单元测试，未做真实写入。

## 未封装的能力（api 透传）

目标/KR 的 list/get/delete、排序与权重、对齐关系、评论 get/patch/delete 等走 `feishu-cli api` 透传，
先 `feishu-cli schema okr` 列出方法，再 `feishu-cli schema okr.<resource>.<method>` 查参数。

### 量化指标 indicators

```bash
feishu-cli api GET /open-apis/okr/v2/objectives/<id>/indicators --as user     # O 的指标
feishu-cli api GET /open-apis/okr/v2/key_results/<id>/indicators --as user    # KR 的指标
feishu-cli api PATCH /open-apis/okr/v2/indicators/<indicator_id> --as user --data '{"current_value":8}'
```

字段语义（汇报进度时的防错口径）：
- `current_value_calculate_type`：0=手动 / 2=按 KR 汇总 / 3=按拆解汇总；**仅 0 允许 PATCH 当前值**
- `entity_type`：2=Objective / 3=Key Result；`indicator_status`：-1/0/1/2
- **默认初始指标不带 start/current/target/unit**——此时汇报必须说「未设置进度」，不能说 0%

## 错误处理

| 错误 | 原因 | 解决 |
|------|------|------|
| `必须指定 --objective-id 或 --key-result-id 之一` / `只能填一个` | O/KR ID 缺失或同时传了两个 | 二选一 |
| `必须指定 --content 或 --content-json 之一` / `只能填一个` | 内容缺失或两个都传 | 二选一 |
| `--progress-status 必须配合 --progress-percent 一起使用` | 单独传 status | 加上 `--progress-percent` |
| `--content-json 不是合法 JSON` | JSON 语法错误 | 用 `jq .` 校验后再传 |
| `99991672` | 应用缺少对应 tenant scope（Bot 身份） | 按错误里的开放平台链接为应用开通（见上方 scope 表）并发布版本 |
| `99991679` | 用户未授权该 scope（User 身份） | `feishu-cli auth login --scope "<scope>"` 增量授权 |
| `99991668 user access token not support` | `cycle list --tenant` 只收 Tenant Token | 用默认 `--as bot` 运行 |

## 相关技能

- **feishu-cli-platform** — 认证诊断、`schema` 查询与 `api` 透传
- **feishu-cli-messaging** — 进展同步后通知 leader/小组
- 本技能的 task / calendar 工作流 — 任务、日历等其他周报相关操作
