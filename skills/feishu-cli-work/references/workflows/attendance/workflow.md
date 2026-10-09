# 飞书考勤数据查询

通过 `feishu-cli attendance`（别名 `att`）查询打卡记录（`user-task query`）与考勤统计（`user-stats query`），
只覆盖这两个查询接口。完整参数以 `feishu-cli attendance <group> query --help` 为准。

## 决策规则

| 场景 | 命令 | 身份 |
|---|---|---|
| 查**本人**打卡 | `user-task query --start ... --end ...`（不传 `--user-ids`、不传 `--employee-type`） | 必须 User Token：CLI 自动用 `employee_no` + 空 `user_ids` 走本人自查 |
| 查**指定员工**打卡 | `user-task query --employee-type employee_id\|employee_no --user-ids ...` | `--as bot\|user\|auto`，默认 auto |
| 查出勤/迟到/请假等统计 | `user-stats query --user-ids ... --stats-type daily\|month` | 只走 Tenant Token（无 `--as`），`--user-ids` 必填 |

- `--employee-type` **只支持** `employee_id`（默认）和 `employee_no`（工号），**不支持 `open_id`**（传入报用法错误 exit 2）。
  `employee_id` 是通讯录里的用户 ID（user_id），不是 `ou_` open_id；可用 `feishu-cli user search --email user@example.com` 查到 `user_id`
  （见 feishu-cli-platform 的 directory 工作流）。
- 显式传了 `--employee-type employee_id` 却不传 `--user-ids` 会报错；查本人时两者都不传即可。
- 日期只接受 `YYYY-MM-DD` 或 `YYYYMMDD`（内部转为 `yyyyMMdd` 整数），**不能**套用日历的 RFC3339 时间点。
- 上限：`user-task` 单次 ≤ 50 人、不限日期跨度；`user-stats` 单次 ≤ 200 人、起止跨度 ≤ 31 天（本地预校验，超出不发请求）。

## 命令

```bash
# 查本人打卡（User Token）
feishu-cli attendance user-task query --start 2026-05-01 --end 2026-05-18

# 按工号查打卡
feishu-cli attendance user-task query \
    --employee-type employee_no --user-ids 10001,10002 \
    --start 2026-05-01 --end 2026-05-18

# 多人 + 加班班段 + JSON（应用身份）
feishu-cli attendance user-task query --as bot \
    --employee-type employee_id --user-ids 2847xxxx,2848xxxx \
    --start 20260501 --end 20260518 --need-overtime -o json

# 日度统计（--user-ids 必填，不支持留空自查本人）
feishu-cli attendance user-stats query \
    --employee-type employee_no --user-ids <本人工号> \
    --stats-type daily --start 2026-05-01 --end 2026-05-31

# 月度统计 + JSON；新考勤系统用户需 --current-user-id（与 --employee-type 同类型）
feishu-cli attendance user-stats query \
    --employee-type employee_id --user-ids 2847xxxx --current-user-id 2847xxxx \
    --stats-type month --start 2026-05-01 --end 2026-05-31 -o json
```

`user-task query` 的非显然开关：`--need-overtime`（含加班班段，默认 false）、`--ignore-invalid-users`（默认 true，
忽略无效/无权限用户只返回有效数据）、`--include-terminated`（含离职员工，默认 false）。
`user-stats query` 另有 `--locale zh|en|ja`、`--need-history`、`--current-group-only`。

## 输出

- `-o json` 输出归一化结构：`user-task` 为 `{user_task_results, invalid_user_ids, unauthorized_user_ids}`；
  `user-stats` 为 `{user_datas[].{name,user_id,datas[].{title,value}}, invalid_user_list}`。
- `user-task query` 文本模式：每条为「姓名 (user_id) 日期」+ 考勤组/班次/打卡记录 ID，下列上下班（或加班）打卡时间与结果
  （`Normal` / `Late` / `Early` / `Lack` 等）；末尾以 ⚠ 列出无效/无权限用户。
- `user-stats query` 文本模式：每个用户列出「统计字段标题 = 值」（出勤天数、迟到次数、请假时长等）。

## 何时不用本工作流

| 场景 | 替代方案 |
|------|---------|
| 请假/加班等审批单据查询或审批 | 本技能的 approval 工作流 |
| 排班、班次定义、考勤组成员、补卡等管理面或写操作 | CLI 未封装；用 `feishu-cli api` 透传对应 OpenAPI（见 feishu-cli-platform 的 api 工作流），需 `attendance:task` 等写权限 |

## 常见错误与排查

| 现象 | 原因 | 解决 |
|------|------|------|
| `不支持的employee-type "open_id"` | 考勤接口不接受 open_id | 改用 `employee_id`（user_id）或 `employee_no`（工号） |
| `--employee-type 为 employee_id 时必须指定 --user-ids` | 查他人未传 `--user-ids` | 传 `--user-ids`；查本人时 `--employee-type` 与 `--user-ids` 都不传 |
| `--user-ids 单次最多 50 个`（user-task）/ `200 个`（user-stats） | 超过本地上限 | 分批 |
| `--start 到 --end 跨度不能超过 31 天` | 仅 user-stats 有此限制 | 拆成多次查询 |
| `日期 "xxx" 不是 YYYYMMDD 8 位数字` / `解析日期失败` | 日期格式不对（含 RFC3339、`2026/09/01`） | 用 `YYYY-MM-DD` 或 `YYYYMMDD` |
| `99991672` | 应用未开通考勤 scope（Bot 身份） | 开放平台为应用开通 `attendance:task:readonly` 并发布版本 |
| `99991679` | 用户未授权考勤 scope（User 身份） | `feishu-cli auth login --scope "attendance:task:readonly"` 增量授权 |
| `current_user_id is invalid`（user-stats） | 新系统用户未传 `--current-user-id` | 补上 `--current-user-id`，类型与 `--employee-type` 一致 |

## 权限要求

两个命令都需要 `attendance:task:readonly`（推荐，只读）或 `attendance:task`。应用侧在开放平台开通后需重新发布版本才生效；
考勤数据涉及员工隐私，企业管理员通常会要求审批后才放权。
