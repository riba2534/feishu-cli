---
name: feishu-cli-okr
description: >-
  飞书 OKR 查询与进度上报。okr cycle list 列租户级 OKR 周期；
  okr progress list/create 查/创建进度记录。
  ⚠️ source_url 字段必填（API 强制），默认占位 https://www.feishu.cn/okr/progress 可改。
  使用 SDK Okr.ProgressRecord + v1/periods HTTP 直调（v2/cycles 不存在）。
  当用户请求"查 OKR 周期"、"上报 OKR 进度"、"OKR 更新"时使用。
argument-hint: cycle list | progress list | progress create
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书 OKR 查询与进度上报技能

通过 feishu-cli 查询 OKR 周期、列出/创建进度记录。覆盖 OKR 最高频的 3 个操作。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

## 核心概念

### OKR 数据模型

飞书 OKR 由 4 层对象组成，本技能涉及前 3 层：

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

### 身份：应用 Token / Tenant Token

OKR 模块当前强制使用 **应用 Token / Tenant Token**。实测 OKR 服务端拒绝 User Token（`99991668 user access token not support`），因此命令不读取 `--user-access-token`，也不需要 `auth login`。

- 需要在飞书开放平台为应用开通 OKR tenant scopes
- 本地只需配置 `FEISHU_APP_ID` / `FEISHU_APP_SECRET` 或 `config.yaml`
- 如服务端返回 `99991672`，按错误里的开放平台链接申请对应应用权限

## 命令速查

| 子命令 | 说明 | 必填参数 |
|--------|------|---------|
| `okr cycle list` | 列出当前租户所有 OKR 周期 | — |
| `okr progress list` | 列出某 O/KR 下的所有进展 | `--objective-id` *或* `--key-result-id` |
| `okr progress create` | 创建一条新进展 | 目标 ID（二选一）+ 内容（二选一） |

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
| `--source-title` | 来源标题 | `created by feishu-cli` |
| `--source-url` | 来源 URL（⚠️ API 必填，CLI 已填默认占位） | `https://www.feishu.cn/okr/progress` |
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
| `progress list` | 通用 HTTP client 直调 `/open-apis/okr/v2/...` |

原因：SDK 在 v3.5.3 版本只暴露了 progress record 的 Create 方法（没有 List/Get/Update/Delete），其他能力都得自己拼 HTTP。

### 4. cycle 是租户级，没有 user_id 参数

不要试图传 `--user-id` 给 `cycle list`——周期是全租户共享的，所有成员看到的都是同一份列表。这点容易被"OKR 是个人目标"的直觉误导。

## 权限要求（应用 Token / Tenant scope）

| 命令 | 所需 scope（任一即可） |
|------|----------------------|
| `cycle list` | `okr:okr:readonly` 或 `okr:okr.period:readonly` |
| `progress list` | `okr:okr:readonly` 或 `okr:okr.progress:readonly` |
| `progress create` | `okr:okr` 或 `okr:okr.progress:writeonly` |

这些是应用权限，不是 OAuth 用户授权；用开放平台应用权限管理页面开通。

## 典型工作流

### 周报同步进展

```bash
# 1. 确保已登录
feishu-cli auth status

# 2. 查当前有哪些周期（可选，确认正在哪个 Q）
feishu-cli okr cycle list

# 3. 看某个目标历史进展（可选，回顾上次说了啥）
feishu-cli okr progress list --objective-id 7xxx

# 4. 同步本周进展
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

## 未实现的能力（按需后续补 CLI）

以下 OKR API 飞书都支持，但本 PR 的 MVP 范围只覆盖 3 个最高频动词。`internal/client` 层已经部分实现，CLI 子命令暂未暴露：

| 能力 | 状态 |
|------|------|
| `okr progress get <progress-id>` | ❌ 未实现 |
| `okr progress update <progress-id>` | ❌ 未实现 |
| `okr progress delete <progress-id>` | ❌ 未实现 |
| `okr progress image upload` | ❌ 未实现（图片素材上传） |
| `okr objective list/get` | ❌ 未实现 |
| `okr key-result list/get` | ❌ 未实现 |
| `okr review list/query`（评审/复盘） | ❌ 未实现 |

如有需要，提 issue 或 PR 时参考 `cmd/okr_progress_create.go` 的模式扩展。

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
| `99991668 user access token not support` | 使用了 User Token | 不传用户 token，使用应用身份运行 |

## 相关技能

- **feishu-cli-auth** — 通用认证诊断；OKR scopes 需要在开放平台应用权限管理开通
- **feishu-cli-msg** — 发飞书消息（进展同步后通知 leader/小组）
- **feishu-cli-toolkit** — 综合工具箱（任务、日历等其他周报相关工具）
