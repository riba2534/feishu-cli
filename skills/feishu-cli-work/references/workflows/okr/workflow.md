# 飞书 OKR 查询与进度上报技能

通过 feishu-cli 查询 OKR 周期、列出/创建进度记录。覆盖 OKR 最高频的 3 个操作。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

## 目录

1. [核心概念](#核心概念)
2. [命令速查](#命令速查)
3. [cycle list — 查租户 OKR 周期](#cycle-list--查租户-okr-周期)
4. [progress list — 查进展记录列表](#progress-list--查进展记录列表)
5. [progress create — 创建进展记录](#progress-create--创建进展记录)
6. [关键踩坑](#-关键踩坑)
7. [权限要求](#权限要求应用-token--tenant-scope)
8. [典型工作流](#典型工作流)
9. [未封装的能力：api 透传（含创建 O/KR、量化指标 indicators）](#未封装的能力用-feishu-cli-api-透传)
10. [错误处理](#错误处理)
11. [相关技能](#相关技能)

## 核心概念

### OKR 数据模型

飞书 OKR 由 4 层对象组成；本工作流直接操作周期和进展记录，并引用 Objective/KR 作为进展归属：

| 层级 | 对象 | 说明 | 本技能命令 |
|------|------|------|-----------|
| 1 | **Period（周期）** | 租户级全局，如 2026-Q1。所有成员看到的周期一致 | `cycle list` |
| 2 | **Objective（目标 O）** | 一个周期内的目标，归属用户 | 仅做引用（--objective-id） |
| 3 | **Key Result（关键结果 KR）** | O 下的可量化结果 | 仅做引用（--key-result-id） |
| 4 | **Progress Record（进展记录）** | O 或 KR 下的一条进展更新 | `progress list/create` |

**关键约束**：

- **周期是租户级的**，`cycle list` **没有 user_id 参数**——所有人看到的周期相同。你不能"查某人的 OKR 周期"，只能"查租户当前都有哪些周期"。
- **Objective ID 和 Key Result ID 二选一**（不是同时）：每条进展记录只能挂在一个目标 *或* 一个关键结果上。
- **进展记录可以独立于周期**：API 不需要传 period_id，但通常一个 O/KR 都属于一个 period。

### 身份：默认 Bot（`--as` 可切换）

OKR 命令组默认 **`--as bot`**（App/Tenant Token，无需 `auth login`，cron/无人值守友好）。
身份墙**按端点分化**（实测）：仅 `cycle list`（v1 periods）只收 Tenant Token；其余端点 user/tenant 双支持，
用 `--as user|auto` 可切换（详见下方「身份选择」表）。

- Tenant 路线：在飞书开放平台为应用开通 OKR tenant scopes，本地配置 `FEISHU_APP_ID` / `FEISHU_APP_SECRET`
- 如服务端返回 `99991672`，按错误里的开放平台链接申请对应应用权限

## 命令速查

| 子命令 | 说明 | 必填参数 |
|--------|------|---------|
| `okr cycle list` | 列出当前租户所有 OKR 周期 | — |
| `okr cycle detail <cycle_id>` | 周期详情：全部目标 + 关键结果（含 ID，便于后续挂进展） | 周期 ID |
| `okr progress list` | 列出某 O/KR 下的所有进展 | `--objective-id` *或* `--key-result-id` |
| `okr progress get <progress_id>` | 单条进展详情 | 进展 ID |
| `okr progress create` | 创建一条新进展 | 目标 ID（二选一）+ 内容（二选一） |
| `okr progress update <progress_id>` | 更新进展内容/进度 | 进展 ID + 内容（二选一） |
| `okr progress delete <progress_id>` | 删除进展（`--yes` 跳过确认） | 进展 ID |
| `okr upload-image` | 上传进展图片素材（ContentBlock imageList 引用） | `--file` + 目标 ID（二选一） |

### 身份选择 `--as`（命令组 persistent flag）

| `--as` | 说明 |
|--------|------|
| `bot`（默认） | App/Tenant Token，无需 `auth login`，scope 在应用后台开通 |
| `user` | User Token（登录时需带 okr scope，否则 99991679） |
| `auto` | User 优先、Tenant 兜底 |

> **身份墙按端点分化（实测）**：`cycle list`（v1 periods）**只收 Tenant Token**（user 身份报 99991668）；
> `cycle detail` / `progress` 系列同时支持 user/tenant 身份。默认 `bot` 对所有端点都成立。

## cycle list — 查租户 OKR 周期

```bash
# 默认文本输出（带名称 + 时间 + 状态）
feishu-cli okr cycle list

# JSON 输出（脚本消费）
feishu-cli okr cycle list --output json
```

**输出字段**：
- `id` — 周期 ID（后续创建 O/KR 时会用，本技能不涉及）
- `zh_name` / `en_name` — 周期名称（如 "2026-Q1"）
- `start_time` / `end_time` — 周期起止时间
- `cycle_status` — 周期状态（如 normal / archived）

**实现细节**：底层走 HTTP 直调 `/open-apis/okr/v1/periods`，自动分页。

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
- `--progress-status` 取值：`normal`（正常）/ `risky`（有风险）/ `overdue`（已延期）— v1 PR 修正：飞书官方枚举不含 `done`，旧文档说的 done 实际就是 overdue
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

### 完整参数表

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--objective-id` | 目标 ID（与 `--key-result-id` 二选一） | — |
| `--key-result-id` | 关键结果 ID（与 `--objective-id` 二选一） | — |
| `--content` | 纯文本内容（与 `--content-json` 二选一） | — |
| `--content-json` | 原始 ContentBlock JSON（与 `--content` 二选一） | — |
| `--progress-percent` | 进度百分比（数字） | — |
| `--progress-status` | 进度状态：`normal` / `risky` / `overdue` | — |
| `--source-title` | 来源标题（flag 注册默认空字符串，运行时 client 层注入默认值 `created by feishu-cli`） | `created by feishu-cli` |
| `--source-url` | 来源 URL（⚠️ API 必填；flag 注册默认空字符串，运行时 client 层注入默认值 `https://www.feishu.cn/okr/progress`） | `https://www.feishu.cn/okr/progress` |
| `--user-id-type` | 用户 ID 类型 | `open_id` |
| `-o, --output` | 输出格式：`json` | 文本 |

## ⚠️ 关键踩坑

### 1. `source_url` 字段 API 强制必填

飞书 OKR `progress_record/create` API 在 source 字段下强制要求 `url`，不传会直接报错。CLI 已经默认填了占位值 `https://www.feishu.cn/okr/progress`，但建议显式覆盖为有意义的 URL（如周报文档地址），这样进展卡片在 OKR 页面才有真正的跳转价值。

### 2. cycle 路径是 `v1/periods` 不是 `v2/cycles`

历史上有过混淆——飞书 OKR 周期 OpenAPI 的正确路径是：

```
GET /open-apis/okr/v1/periods   ✅ 真实存在
GET /open-apis/okr/v2/cycles    ❌ 不存在，404
```

`cycle list` 命令的 `cycle` 是 CLI 子命令名（更符合用户直觉），实际调用走 `v1/periods`。如果手动拼 HTTP 请求时不要写错。

### 3. `progress create` 走 SDK，`cycle list` / `progress list` 走 HTTP 直调

实现层面有分工：

| 命令 | 实现方式 |
|------|---------|
| `progress create` | 飞书 Open SDK v3.5.3 的 `Okr.ProgressRecord.Create` |
| `cycle list` | 通用 HTTP client 直调 `/open-apis/okr/v1/periods` |
| `progress list` | 通用 HTTP client 直调（按 target 类型分两条路径）：<br>• OKRTargetObjective: `/open-apis/okr/v2/objectives/{id}/progresses`<br>• OKRTargetKeyResult: `/open-apis/okr/v2/key_results/{id}/progresses` |

SDK v3.5.3 暴露 Create/Get/Update/Delete，但没有适合当前列表语义的统一 List；CLI 当前只公开
`progress create/list`，其中 create 走 SDK，list 按 Objective/KR 类型直调对应 HTTP endpoint。

### 4. cycle 是租户级，没有 user_id 参数

不要试图传 `--user-id` 给 `cycle list`——周期是全租户共享的，所有成员看到的都是同一份列表。这点容易被"OKR 是个人目标"的直觉误导。

## 权限要求（应用 Token / Tenant scope）

| 命令 | 所需 scope（任一即可） |
|------|----------------------|
| `cycle list` | `okr:okr:readonly` 或 `okr:okr.period:readonly` |
| `cycle detail` | `okr:okr:readonly`（user 身份为 `okr:okr.content:readonly`） |
| `progress list` / `get` | `okr:okr:readonly` 或 `okr:okr.progress:readonly` |
| `progress create` / `update` | `okr:okr` 或 `okr:okr.progress:writeonly` |
| `progress delete` | `okr:okr` 或 `okr:okr.progress:delete` |
| `upload-image` | `okr:okr` 或 `okr:okr.progress.file:upload` |

这些是应用权限，不是 OAuth 用户授权；用开放平台应用权限管理页面开通。

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

## 未封装的能力（用 `feishu-cli api` 透传）

`cycle list/detail`、`progress list/get/create/update/delete`、`upload-image` 均已是一等命令（见命令速查）。
以下能力未做专用命令，属季度级低频操作，用 `feishu-cli api` 透传即可（先 `feishu-cli schema okr.<resource>.<method>` 查参数）：

### 创建 Objective / Key Result（api 透传配方）

scope `okr:okr.content:writeonly`；写端点建议 `--as user`（官方对写端点均以 user 身份验证；tenant 身份未实测）。

```bash
# 创建 Objective（cycle_id 从 okr cycle list 拿）
feishu-cli api POST /open-apis/okr/v2/cycles/<cycle_id>/objectives --as user --data '{
  "content": {"blocks":[{"type":"paragraph","paragraph":{"elements":[{"type":"textRun","textRun":{"text":"目标内容","style":{}}}]}}]}
}'

# 在 Objective 下创建 Key Result
feishu-cli api POST /open-apis/okr/v2/objectives/<objective_id>/key_results --as user --data '{
  "content": {"blocks":[{"type":"paragraph","paragraph":{"elements":[{"type":"textRun","textRun":{"text":"KR 内容","style":{}}}]}}]}
}'
```

租户强制 OKR 分类时创建 Objective 需带 `category_id`（先 `feishu-cli api GET /open-apis/okr/v2/categories --as user` 查，
该端点需 user scope `okr:okr.setting:read`，缺失报 99991679——实测确认）。

> 验证状态：上述端点路径与请求形状来自官方 OpenAPI（okr/v2）；本地租户未开通 okr 写 scope，
> 仅实测了鉴权关卡（99991672/99991679 及所需 scope 名），未做真实创建。首次使用前先
> `feishu-cli auth check --scope "okr:okr.content:writeonly"` 预检。

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

### 其他未封装端点

`objective list/get`、`key-result list/get`、`review list/query`（评审/复盘）同样走 `feishu-cli api` 透传。

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
- **feishu-cli-work** — 综合工具箱（任务、日历等其他周报相关工具）
