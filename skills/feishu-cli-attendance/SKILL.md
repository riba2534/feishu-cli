---
name: feishu-cli-attendance
description: >-
  飞书考勤数据查询（user-task / user-stats）。user-task 查打卡任务/班次；
  user-stats 查考勤统计（出勤/迟到/早退/请假）。
  ⚠️ 仅支持 Tenant Token，SDK v3.5.3 限制不接受 User Token；
  单次查询跨度上限 31 天，超出会拒绝。
  当用户请求"查考勤"、"查打卡记录"、"出勤统计"、"考勤明细"时使用。
argument-hint: user-task query | user-stats query
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书考勤数据查询

通过 `feishu-cli attendance`（别名 `att`）查询用户考勤打卡记录与统计数据，对接飞书考勤 OpenAPI。

> **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式。

## 核心概念

### Tenant Token 限制（关键）

- **全部命令走 `tenant_access_token`**（应用身份），**无需** `feishu-cli auth login`。
- `larksuite/oapi-sdk-go v3.5.3` 中 `Attendance.UserTask.Query` 与
  `Attendance.UserStatsData.Query` 的 `SupportedAccessTokenTypes` 仅含 `Tenant`，
  传入 User Access Token **会被 SDK 直接拒绝**。
- 权限通过飞书开放平台「应用权限管理」页面授予应用：
  - `attendance:task:readonly`（推荐，仅查询）
  - `attendance:task`（含写入）
- 应用身份起跑前提：`FEISHU_APP_ID` + `FEISHU_APP_SECRET`（环境变量或 `~/.feishu-cli/config.yaml`）。

### 日期格式

接受 `YYYY-MM-DD` 或 `YYYYMMDD` 两种写法，CLI 内部统一转为 OpenAPI 要求的 `yyyyMMdd` 整数。

### 跨度上限（关键）

- **`user-stats query` 起止跨度 ≤ 31 天**，超出本地直接拒绝（不发请求）。
- `user-task query` 没有 31 天上限，但仍受 50 用户上限约束。

## 命令速查

### 1. 查询打卡记录 `user-task query`

```bash
feishu-cli attendance user-task query \
    --employee-type <type> --user-ids <ids> \
    --start <date> --end <date> [选项]
```

底层走 `POST /open-apis/attendance/v1/user_tasks/query`。返回上下班实际打卡结果（含加班班段可选）。

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `--employee-type` | string | - | 用户 ID 类型：`employee_id` (默认) / `open_id` / `user_id` / `employee_no` |
| `--user-ids` | CSV | ✓ | 用户 ID 列表，逗号分隔，**最多 50 个** |
| `--start` | string | ✓ | 起始工作日（YYYY-MM-DD 或 YYYYMMDD）|
| `--end` | string | ✓ | 结束工作日（YYYY-MM-DD 或 YYYYMMDD）|
| `--need-overtime` | bool | - | 是否包含加班班段打卡结果（默认 false）|
| `--ignore-invalid-users` | bool | - | 忽略无效/无权限用户（默认 true）|
| `--include-terminated` | bool | - | 包含离职员工数据（默认 false）|
| `-o, --output` | string | - | `text`（默认）/ `json` |

### 2. 查询考勤统计 `user-stats query`

```bash
feishu-cli attendance user-stats query \
    --employee-type <type> --user-ids <ids> --current-user-id <id> \
    --stats-type <daily|month> --start <date> --end <date> [选项]
```

底层走 `POST /open-apis/attendance/v1/user_stats_datas/query`。返回出勤/迟到/早退/请假等聚合统计字段（出勤天数、迟到次数、加班时长……）。

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `--employee-type` | string | - | 用户 ID 类型（同上，默认 `employee_id`）|
| `--stats-type` | string | - | `daily`（日度，默认）/ `month`（月度）|
| `--user-ids` | CSV | ✓ | 查询的用户 ID 列表，**最多 200 个** |
| `--current-user-id` | string | - | 发起请求的用户 ID（新系统用户必填，对应「查询统计设置」`user_id`）|
| `--start` | string | ✓ | 起始日期 |
| `--end` | string | ✓ | 结束日期（跨度 ≤ 31 天）|
| `--locale` | string | - | 语言：`zh` / `en` / `ja` |
| `--need-history` | bool | - | 是否返回历史数据（默认 false）|
| `--current-group-only` | bool | - | 仅展示当前考勤组（默认 false）|
| `-o, --output` | string | - | `text`（默认）/ `json` |

## 使用示例

```bash
# 查询本人最近一周打卡
feishu-cli attendance user-task query \
    --employee-type open_id --user-ids ou_xxxxxxxx \
    --start 2026-05-01 --end 2026-05-18

# 多人 + 加班 + JSON
feishu-cli attendance user-task query \
    --employee-type open_id \
    --user-ids ou_aaa,ou_bbb \
    --start 20260501 --end 20260518 \
    --need-overtime -o json

# 查本人 5 月日度统计
feishu-cli attendance user-stats query \
    --employee-type open_id \
    --user-ids ou_xxxxxxxx --current-user-id ou_xxxxxxxx \
    --stats-type daily --start 2026-05-01 --end 2026-05-31

# 查月度统计 + JSON
feishu-cli attendance user-stats query \
    --employee-type open_id --user-ids ou_xxx --current-user-id ou_xxx \
    --stats-type month --start 2026-05-01 --end 2026-05-31 -o json

# 兼容 alias：att 等价 attendance
feishu-cli att user-task query --user-ids ou_xxx --start 2026-05-01 --end 2026-05-18
```

## 输出字段

### `user-task query` 文本模式

```
共 N 条打卡任务:

[i] <姓名> (<user_id>)  日期: YYYY-MM-DD
    考勤组: <group_id>   班次: <shift_id>   打卡记录 ID: <result_id>
    [j] (上下班 / 加班)
        上班: HH:MM  结果: <Normal/Late/Early/Lack/...>  (补充说明)
        下班: HH:MM  结果: <...>                          (补充说明)

⚠ 无效用户 ID (n): id1, id2
⚠ 无权限用户 ID (n): id3, id4
```

### `user-stats query` 文本模式

```
共 N 个用户的统计数据:

[i] <姓名> (<user_id>)
    <统计字段标题>     = <值>
    出勤天数            = 22
    迟到次数            = 1
    请假时长            = 2.5h
    ...

⚠ 无权限用户 (n): id1, id2
```

JSON 模式直出归一化结构体（`AttendanceQueryUserTaskResult` / `AttendanceQueryUserStatsResult`），便于 AI Agent 与脚本消费。

## 何时不用本 skill

下列场景**不要**走 `feishu-cli attendance`，本模块只覆盖「查询」两个接口：

| 场景 | 替代方案 |
|------|---------|
| 管理面查/改排班、班次定义、考勤组 | 飞书开放平台 OpenAPI Explorer，或直接调 `/open-apis/attendance/v1/shifts`、`/groups` 等 |
| 审批假勤（请假/加班单据查询/审批） | `feishu-cli approval` 模块 |
| 补卡/手动打卡 | OpenAPI `/open-apis/attendance/v1/user_task_remedys`，本 CLI 暂未封装 |
| 「查考勤组成员」类场景 | OpenAPI `/open-apis/attendance/v1/groups/{group_id}` |
| 任何需要写操作（补卡审批、修改班次） | 直接调 OpenAPI（需 `attendance:task` 写权限）|

## 常见错误与排查

| 现象 | 原因 | 解决 |
|------|------|------|
| `unsupported access token type, only support: Tenant` | 误传了 User Token | 移除 `--user-access-token` / `FEISHU_USER_ACCESS_TOKEN`；本模块只走 Tenant |
| `--user-ids 单次最多 50 个 / 200 个` | 超过本地预校验上限 | user-task ≤ 50，user-stats ≤ 200，超出请分批 |
| `--start 到 --end 跨度不能超过 31 天` | user-stats 跨度超限 | 拆成多次查询，每次 ≤ 31 天 |
| `日期 "xxx" 不是 YYYYMMDD 8 位数字` | 日期格式不对 | 用 `YYYY-MM-DD` 或纯 8 位数字 `YYYYMMDD` |
| `99991663` / `attendance:task` 权限错误 | 应用未开通考勤 scope | 飞书开放平台 → 应用权限管理 → 申请 `attendance:task:readonly` 并发布新版本 |
| `current_user_id is invalid`（user-stats） | 新系统用户未传 `--current-user-id` | 补上 `--current-user-id`，值与 user-ids 中目标用户保持同源 |
| `invalid_user_ids` / `unauthorized_user_ids` 非空 | 部分 user-id 不存在或应用对该用户无权限 | 核对 user-id 类型与应用可见范围设置 |

## 权限要求

| 命令 | 必需 scope（tenant 级）|
|------|------------------------|
| `attendance user-task query` | `attendance:task:readonly`（推荐）或 `attendance:task` |
| `attendance user-stats query` | `attendance:task:readonly`（推荐）或 `attendance:task` |

权限在飞书开放平台「应用权限管理」页面开通后**应用需重新发布版本**才生效；考勤数据涉及员工隐私，企业管理员通常会要求审批后才放权。
