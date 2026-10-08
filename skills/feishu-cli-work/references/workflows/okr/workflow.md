# 飞书 OKR 查询与进度上报技能

通过 feishu-cli 查询 OKR 周期与周期详情（`cycle list/detail`）、管理进展记录（`progress list/get/create/update/delete`）、上传进展图片（`upload-image`），并写入目标 / 关键结果与评论（`objective` / `key-result` / `comment`），与「命令速查」表一一对应。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

## 目录

1. [核心概念](#核心概念)
2. [命令速查](#命令速查)
3. [cycle list — 查用户 OKR 周期](#cycle-list--查用户-okr-周期)
4. [progress list — 查进展记录列表](#progress-list--查进展记录列表)
5. [progress create — 创建进展记录](#progress-create--创建进展记录)
6. [关键踩坑](#-关键踩坑)
7. [权限要求](#权限要求应用-token--tenant-scope)
8. [典型工作流](#典型工作流)
9. [目标 / 关键结果写入与评论](#目标--关键结果写入与评论v2)；[未封装的能力：api 透传（量化指标 indicators 等）](#未封装的能力用-feishu-cli-api-透传)
10. [错误处理](#错误处理)
11. [相关技能](#相关技能)

## 核心概念

### OKR 数据模型

飞书 OKR 由 4 层对象组成；本工作流直接操作周期和进展记录，并引用 Objective/KR 作为进展归属：

| 层级 | 对象 | 说明 | 本技能命令 |
|------|------|------|-----------|
| 1 | **Cycle（周期）** | 每个用户有自己的周期（v2 `cycles`，`tenant_cycle_id` 指向租户级周期 period） | `cycle list` |
| 2 | **Objective（目标 O）** | 一个周期内的目标，归属用户 | 仅做引用（--objective-id） |
| 3 | **Key Result（关键结果 KR）** | O 下的可量化结果 | 仅做引用（--key-result-id） |
| 4 | **Progress Record（进展记录）** | O 或 KR 下的一条进展更新 | `progress list/create` |

**关键约束**：

- **周期 ID 有两种**：v2 **用户周期 ID**（`okr/v2/cycles?user_id=`，`cycle detail` 与创建目标只认它）和 v1 **租户周期 ID**（`okr/v1/periods`，带名称）。
  `cycle list` 默认返回当前登录用户（或 `--user-id` 指定用户）的用户周期；`--tenant` 才返回租户周期（旧行为，其 ID 不能用于 `cycle detail`）。
- **Objective ID 和 Key Result ID 二选一**（不是同时）：每条进展记录只能挂在一个目标 *或* 一个关键结果上。
- **进展记录可以独立于周期**：API 不需要传 period_id，但通常一个 O/KR 都属于一个 period。

### 身份：默认 Bot（`--as` 可切换）

OKR 命令组默认 **`--as bot`**（App/Tenant Token，无需 `auth login`，cron/无人值守友好）。
身份墙**按端点分化**（实测）：仅 `cycle list --tenant`（v1 periods）只收 Tenant Token（user 报 99991668）；
`cycle list` 默认走的 v2 cycles 与其余端点 user/tenant 双支持（缺 scope 时分别报 99991672 / 99991679），
用 `--as user|auto` 可切换（详见下方「身份选择」表）。

- Tenant 路线：在飞书开放平台为应用开通 OKR tenant scopes，本地配置 `FEISHU_APP_ID` / `FEISHU_APP_SECRET`
- 如服务端返回 `99991672`，按错误里的开放平台链接申请对应应用权限

## 命令速查

| 子命令 | 说明 | 必填参数 |
|--------|------|---------|
| `okr cycle list` | 列出用户的 OKR 周期（默认当前登录用户） | — |
| `okr cycle detail <cycle_id>` | 周期详情：全部目标 + 关键结果（含 ID，便于后续挂进展） | 周期 ID |
| `okr progress list` | 列出某 O/KR 下的所有进展 | `--objective-id` *或* `--key-result-id` |
| `okr progress get <progress_id>` | 单条进展详情 | 进展 ID |
| `okr progress create` | 创建一条新进展 | 目标 ID（二选一）+ 内容（二选一） |
| `okr progress update <progress_id>` | 更新进展内容/进度 | 进展 ID + 内容（二选一） |
| `okr progress delete <progress_id>` | 删除进展（`--yes` 跳过确认；`--dry-run` 只预览） | 进展 ID |
| `okr upload-image` | 上传进展图片素材（ContentBlock imageList 引用） | `--file` + 目标 ID（二选一） |
| `okr objective create` | 在周期下创建目标（v2，先 `--dry-run`） | `--cycle-id` + `--content`/`--content-json` |
| `okr objective update <id>` | 更新目标内容/备注/得分/截止 | 至少一个字段 |
| `okr key-result create` | 在目标下创建关键结果 | `--objective-id` + 内容 |
| `okr key-result update <id>` | 更新关键结果内容/得分/截止 | 至少一个字段 |
| `okr comment list` | 列出周期/进展/目标/KR 的评论 | `--target-type` + `--target-id` |
| `okr comment create` | 创建评论或回复（必需 User Token） | `--target-type` + `--target-id` + 内容 |

### 身份选择 `--as`（命令组 persistent flag）

| `--as` | 说明 |
|--------|------|
| `bot`（默认） | App/Tenant Token，无需 `auth login`，scope 在应用后台开通 |
| `user` | User Token（登录时需带 okr scope，否则 99991679） |
| `auto` | User 优先、Tenant 兜底 |

> **身份墙按端点分化（实测）**：`cycle list --tenant`（v1 periods）**只收 Tenant Token**（user 身份报 99991668）；
> `cycle list`（v2 cycles）、`cycle detail` / `progress` 系列同时支持 user/tenant 身份。默认 `bot` 对所有端点都成立。

## cycle list — 查用户 OKR 周期

```bash
# 查自己的周期（--user-id 默认取当前登录用户，请求身份仍按 --as，默认 bot）
feishu-cli okr cycle list

# 查某人的周期，只保留与 2026 上半年重叠的
feishu-cli okr cycle list --user-id ou_xxx --time-range 2026-01--2026-06 --output json

# 租户级周期（v1 periods，带名称；ID 不能用于 cycle detail）
feishu-cli okr cycle list --tenant
```

**输出字段**（默认 v2 用户周期）：
- `id` — 用户周期 ID（用于 `cycle detail`，以及后文创建 O/KR 的 api 配方）
- `tenant_cycle_id` — 对应的租户周期 ID
- `owner` — 周期所属用户；`score` — 周期得分
- `start_time` / `end_time` — 周期起止时间（本地时区）
- `cycle_status` — `default`(0) / `normal`(1) / `invalid`(2) / `hidden`(3)
- JSON 顶层另有 `user_id` 与 `current_active_cycles`（当前时间落在周期内且状态 default/normal）
- `--tenant` 时输出 v1 字段：`id` / `zh_name` / `en_name` / 起止 / 状态

**实现细节**：默认 `GET /open-apis/okr/v2/cycles?user_id=...&user_id_type=open_id`（自动分页）；`--tenant` 走 `GET /open-apis/okr/v1/periods`。
**行为变更**：此前版本默认走 v1 periods，返回的租户周期 ID 传给 `cycle detail` 查不到目标。

## progress list — 查进展记录列表

```bash
# 查目标下的所有进展
feishu-cli okr progress list --objective-id 7123456789012345678

# 查关键结果下的所有进展
feishu-cli okr progress list --key-result-id 7123456789012345678

# JSON 输出
feishu-cli okr progress list --objective-id 7xxx --output json
```

**参数**：

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--objective-id` | 目标 ID（与 `--key-result-id` **二选一**） | — |
| `--key-result-id` | 关键结果 ID（与 `--objective-id` **二选一**） | — |
| `--user-id-type` | 用户 ID 类型：`open_id` / `union_id` / `user_id` | `open_id` |
| `-o, --output` | 输出格式：`json` | 文本 |

**输出字段**：
- `progress_id` — 进展 ID
- `create_time` / `modify_time` — 创建/修改时间（已转本地时区 `YYYY-MM-DD HH:MM:SS`）
- `progress_rate.percent` / `progress_rate.status` — 进度百分比和状态（如果有）

## progress create — 创建进展记录

最常用：周报/日报中手动同步进度。

### 最简形式（纯文本）

```bash
feishu-cli okr progress create \
  --objective-id 7123456789012345678 \
  --content "本周完成核心模块联调，下周开始联调测试"
```

CLI 会自动把纯文本包装成飞书 ContentBlock 富文本 JSON（paragraph + textRun）。

### 带进度百分比

```bash
feishu-cli okr progress create \
  --key-result-id 7123456789012345678 \
  --content "完成 8/10 任务" \
  --progress-percent 80 \
  --progress-status normal
```

- `--progress-percent` 数字（0-100）
- `--progress-status` 取值：`normal`(0) / `overdue`(1) / `done`(2)。`risky` 已删除写入语义，传入会得到兼容提示而不会映射成 overdue
- ⚠️ `--progress-status` **必须配合** `--progress-percent` 使用，单独传 status 会报错

### 富文本（ContentBlock JSON）

需要 @某人、嵌入链接、加粗等富文本场景：

```bash
feishu-cli okr progress create \
  --objective-id 7xxx \
  --content-json '{"blocks":[{"type":"paragraph","paragraph":{"elements":[{"type":"textRun","textRun":{"text":"加粗内容","style":{"bold":true}}}]}}]}'
```

`--content` 和 `--content-json` **互斥**，只能填一个。

### 自定义 source（来源标题 + URL）

```bash
feishu-cli okr progress create \
  --objective-id 7xxx \
  --content "本周完成 X" \
  --source-title "周报：W18" \
  --source-url "https://xxx.feishu.cn/docx/abc123"
```

进展卡片在飞书 OKR 页面会展示来源标题，点击跳转 URL。

写命令（`progress create/update/delete`、`objective`/`key-result` create/update、`comment create`）都支持 `--dry-run`：
只打印将发出的请求，不联网、不解析身份。OKR 对组织可见，先预览再执行。

### 完整参数表

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--objective-id` | 目标 ID（与 `--key-result-id` 二选一） | — |
| `--key-result-id` | 关键结果 ID（与 `--objective-id` 二选一） | — |
| `--content` | 纯文本内容（与 `--content-json` 二选一） | — |
| `--content-json` | 原始 ContentBlock JSON（与 `--content` 二选一） | — |
| `--progress-percent` | 进度百分比（数字） | — |
| `--progress-status` | 进度状态：`normal` / `overdue` / `done` | — |
| `--source-title` | 来源标题（flag 注册默认空字符串，运行时 client 层注入默认值 `created by feishu-cli`） | `created by feishu-cli` |
| `--source-url` | 来源 URL（⚠️ API 必填；flag 注册默认空字符串，运行时 client 层注入默认值 `https://www.feishu.cn/okr/progress`） | `https://www.feishu.cn/okr/progress` |
| `--user-id-type` | 用户 ID 类型 | `open_id` |
| `-o, --output` | 输出格式：`json` | 文本 |

## ⚠️ 关键踩坑

### 1. `source_url` 字段 API 强制必填

飞书 OKR `progress_record/create` API 在 source 字段下强制要求 `url`，不传会直接报错。CLI 已经默认填了占位值 `https://www.feishu.cn/okr/progress`，但建议显式覆盖为有意义的 URL（如周报文档地址），这样进展卡片在 OKR 页面才有真正的跳转价值。

### 2. `v2/cycles` 与 `v1/periods` 都存在，ID 不通用

此前文档称 `GET /open-apis/okr/v2/cycles` 不存在（404）——**这是错的**：带上 `user_id` 参数实测返回的是
99991672（应用缺 `okr:okr.period:readonly`）或 99991679（用户未授权该 scope），即端点存在、只是缺权限。

| 端点 | 粒度 | 身份 | ID 用途 |
|------|------|------|---------|
| `GET /open-apis/okr/v2/cycles?user_id=` | 用户周期 | user / tenant | `cycle detail`、`/v2/cycles/{id}/objectives` |
| `GET /open-apis/okr/v1/periods` | 租户周期（带名称） | 仅 tenant | 不能用于 v2 接口 |

### 3. `progress create` 走 SDK，`cycle list` / `progress list` 走 HTTP 直调

实现层面有分工：

| 命令 | 实现方式 |
|------|---------|
| `progress create` | 飞书 Open SDK v3.5.3 的 `Okr.ProgressRecord.Create` |
| `cycle list` | 通用 HTTP client 直调 `/open-apis/okr/v2/cycles`（`--tenant` 时 `/open-apis/okr/v1/periods`） |
| `progress list` | 通用 HTTP client 直调（按 target 类型分两条路径）：<br>• OKRTargetObjective: `/open-apis/okr/v2/objectives/{id}/progresses`<br>• OKRTargetKeyResult: `/open-apis/okr/v2/key_results/{id}/progresses` |

CLI 已公开完整的进展 CRUD 与配套命令（见「命令速查」）：`progress create/get/update/delete` 与
`upload-image` 走 SDK v3.5.3（`ProgressRecord.*` / `Image.Upload`）；SDK 没有适合当前列表语义的
统一 List，故 `progress list` 按 Objective/KR 类型直调对应 HTTP endpoint，`cycle list` 同为 HTTP 直调。

### 4. 查"某人的周期"用 `--user-id`

`cycle list` 默认查当前登录用户；查别人传 `--user-id`（配合 `--user-id-type`）。只想看租户都有哪些周期名称时用 `--tenant`。

## 权限要求（应用 Token / Tenant scope）

| 命令 | 所需 scope（任一即可） |
|------|----------------------|
| `cycle list` | `okr:okr.period:readonly`（`--tenant` 时 `okr:okr:readonly` 亦可） |
| `cycle detail` | `okr:okr:readonly`（user 身份为 `okr:okr.content:readonly`） |
| `progress list` / `get` | `okr:okr:readonly` 或 `okr:okr.progress:readonly` |
| `progress create` / `update` | `okr:okr` 或 `okr:okr.progress:writeonly` |
| `progress delete` | `okr:okr` 或 `okr:okr.progress:delete` |
| `upload-image` | `okr:okr` 或 `okr:okr.progress.file:upload` |

默认 Bot 路线在应用权限管理页面开通上述 tenant scopes。选择 `--as user` 的端点，
还需对应的用户 scope 和 OAuth 授权；按服务端返回的 scope 提示预检、增量登录。
`cycle list --tenant` 只收 Tenant Token，不应用用户授权替代。

## 典型工作流

### 周报同步进展

```bash
# 1. 查当前有哪些周期（可选，确认正在哪个 Q）
feishu-cli okr cycle list

# 2. 看某个目标历史进展（可选，回顾上次说了啥）
feishu-cli okr progress list --objective-id 7xxx

# 3. 同步本周进展
feishu-cli okr progress create \
  --objective-id 7xxx \
  --content "W18: 完成 X 和 Y，下周冲刺 Z" \
  --progress-percent 60 \
  --progress-status normal \
  --source-title "周报 W18" \
  --source-url "https://xxx.feishu.cn/docx/<your-weekly-doc-id>"
```

### 脚本化批量同步多个 KR 进展

```bash
for kr_id in 7xxx 7yyy 7zzz; do
  feishu-cli okr progress create \
    --key-result-id "$kr_id" \
    --content "自动同步: 当前推进中" \
    --output json
  sleep 1
done
```

## 目标 / 关键结果写入与评论（v2）

OKR 对组织可见：**先 `--dry-run` 预览**，确认目标对象和内容后再去掉 `--dry-run` 执行。

```bash
# 创建目标（cycle_id 是 okr cycle list 默认返回的用户周期 ID，不是 --tenant 的租户周期 ID）
feishu-cli okr objective create --cycle-id <cycle_id> --content "提升交付质量" [--notes "口径说明"] [--category-id <id>] --dry-run

# 在目标下创建关键结果
feishu-cli okr key-result create --objective-id <objective_id> --content "缺陷率降到 1% 以下" --dry-run

# 更新：只改传入字段；--score 0-1 最多一位小数；--deadline 毫秒时间戳或 YYYY-MM-DD
feishu-cli okr objective update <objective_id> --score 0.7 --deadline 2026-12-31 --dry-run
feishu-cli okr key-result update <key_result_id> --content "缺陷率降到 0.5%" --dry-run

# 评论（创建必需 User Token；目标/KR 评论必须且只能指定 --selected-text / --select-all / --ref-comment-id 之一）
feishu-cli okr comment list --target-type objective --target-id <objective_id>
feishu-cli okr comment create --target-type objective --target-id <objective_id> --content "口径需要再对齐" --select-all --dry-run
```

- `--content` / `--notes` 的纯文本会包装为 **v2 ContentBlock**（`block_element_type` / `paragraph_element_type` / `text_run`，snake_case）；
  需要 @人、链接时用 `--content-json` 传完整结构。**v2 与 v1 进展记录的 ContentBlock 键名不同**（v1 为 `type` / `textRun`），
  此前文档里用 v1 键名调用 v2 创建接口的 api 透传配方是错的，已改为上面的一等命令。
- 身份沿用命令组 `--as`（默认 bot）；`comment create` 只支持 User Token。
- scope：目标/KR 写入 `okr:okr.content:writeonly`，评论读 `okr:okr.comment.readonly`、写 `okr:okr.comment.writeonly`。
- 验证状态：本地租户未开通 OKR 写 scope，以上写命令只做了 dry-run 与单元测试，未做真实创建。

## 未封装的能力（用 `feishu-cli api` 透传）

`objective list/get`、`key-result list/get`、`reorder/weight`、评论 `get/patch/delete/solve`、`review list/query`（评审/复盘）
走 `feishu-cli api` 透传（先 `feishu-cli schema okr.<resource>.<method>` 查参数）。

租户强制 OKR 分类时创建 Objective 需带 `category_id`（先 `feishu-cli api GET /open-apis/okr/v2/categories --as user` 查，
该端点需 user scope `okr:okr.setting:read`，缺失报 99991679——实测确认）。

### 量化指标 indicators（api 透传）

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
| `必须指定 --objective-id 或 --key-result-id 之一` | 没传目标 ID | 二选一传入 |
| `--objective-id 和 --key-result-id 只能填一个` | 同时传了两个 | 只保留一个 |
| `必须指定 --content 或 --content-json 之一` | 没传内容 | 二选一传入 |
| `--content 和 --content-json 只能填一个` | 同时传了两个 | 只保留一个 |
| `--progress-status 必须配合 --progress-percent 一起使用` | 单独传 status | 加上 `--progress-percent` |
| `--content-json 不是合法 JSON` | JSON 语法错误 | 用 `jq .` 校验后再传 |
| `source_url is required` 或类似 | 飞书 API 强制必填 | CLI 已默认填占位，理论上不会触发；如出现请显式传 `--source-url` |
| `99991672` / `scope not authorized` | 应用缺少 OKR tenant scope | 到开放平台应用权限管理开通 `okr:okr*` 相关权限 |
| `99991668 user access token not support` | 该端点只收 Tenant Token（如 `cycle list`） | 用默认 `--as bot`（应用身份）运行 |

## 相关技能

- **feishu-cli-platform** — 通用认证诊断；OKR scopes 需要在开放平台应用权限管理开通
- **feishu-cli-messaging** — 发飞书消息（进展同步后通知 leader/小组）
- 本领域的 task / calendar 工作流 — 任务、日历等其他周报相关操作
