---
name: feishu-cli-calendar
description: 日历和日程管理。当用户请求查看日历、创建日程、管理日程时使用。支持列出日历、创建/查看/更新/删除日程等操作。
argument-hint: <subcommand> [args]
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书日历操作技能

管理飞书日历和日程，包括列出日历、创建/查看/更新/删除日程等。

## 使用方法

```bash
/feishu-calendar list                                   # 列出所有日历
/feishu-calendar list-events <calendar_id>              # 列出日历中的日程
/feishu-calendar create-event --calendar-id <id> ...    # 创建日程
/feishu-calendar get-event <calendar_id> <event_id>     # 获取日程详情
/feishu-calendar update-event <calendar_id> <event_id>  # 更新日程
/feishu-calendar delete-event <calendar_id> <event_id>  # 删除日程
```

## CLI 命令详解

### 1. 列出日历

```bash
feishu-cli calendar list
```

**输出示例**：
```
日历列表:
  1. 日历ID: CAL_xxx
     名称: 我的日历
     类型: primary

  2. 日历ID: CAL_yyy
     名称: 团队日历
     类型: shared
```

### 2. 创建日程

```bash
# 基本用法
feishu-cli calendar create-event \
  --calendar-id CAL_ID \
  --summary "日程标题" \
  --start 2024-01-21T14:00:00+08:00 \
  --end 2024-01-21T15:00:00+08:00

# 带描述和地点
feishu-cli calendar create-event \
  --calendar-id CAL_ID \
  --summary "项目评审" \
  --start 2024-01-21T14:00:00+08:00 \
  --end 2024-01-21T16:00:00+08:00 \
  --description "Q1 项目进度评审" \
  --location "会议室 A101"

# JSON 格式输出
feishu-cli calendar create-event \
  --calendar-id CAL_ID \
  --summary "会议" \
  --start 2024-01-21T14:00:00+08:00 \
  --end 2024-01-21T15:00:00+08:00 \
  --output json
```

**参数说明**：
| 参数 | 简写 | 说明 | 必填 |
|------|------|------|------|
| `--calendar-id` | `-c` | 日历 ID | 是 |
| `--summary` | `-s` | 日程标题 | 是 |
| `--start` | | 开始时间（RFC3339 格式） | 是 |
| `--end` | | 结束时间（RFC3339 格式） | 是 |
| `--description` | `-d` | 日程描述 | 否 |
| `--location` | `-l` | 地点 | 否 |
| `--output` | `-o` | 输出格式（json） | 否 |

### 3. 列出日程

```bash
# 列出所有日程
feishu-cli calendar list-events CAL_ID

# 指定时间范围
feishu-cli calendar list-events CAL_ID \
  --start-time 2024-01-01T00:00:00+08:00 \
  --end-time 2024-01-31T23:59:59+08:00

# 分页查询
feishu-cli calendar list-events CAL_ID --page-size 20

# JSON 格式输出
feishu-cli calendar list-events CAL_ID --output json
```

**参数说明**：
| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--start-time` | 开始时间过滤 | 无 |
| `--end-time` | 结束时间过滤 | 无 |
| `--page-size` | 每页数量 | 50 |
| `--page-token` | 分页标记 | 无 |
| `--output, -o` | 输出格式 | 文本 |

### 4. 获取日程详情

```bash
feishu-cli calendar get-event CAL_ID EVENT_ID
```

### 5. 更新日程

```bash
# 更新标题
feishu-cli calendar update-event CAL_ID EVENT_ID --summary "新标题"

# 更新时间
feishu-cli calendar update-event CAL_ID EVENT_ID \
  --start 2024-01-21T15:00:00+08:00 \
  --end 2024-01-21T16:00:00+08:00

# 更新多个字段
feishu-cli calendar update-event CAL_ID EVENT_ID \
  --summary "更新后的会议" \
  --description "会议内容已更新" \
  --location "会议室 B202"
```

**参数说明**：
| 参数 | 简写 | 说明 |
|------|------|------|
| `--summary` | `-s` | 新的日程标题 |
| `--start` | | 新的开始时间 |
| `--end` | | 新的结束时间 |
| `--description` | `-d` | 新的日程描述 |
| `--location` | `-l` | 新的地点 |
| `--output` | `-o` | 输出格式（json） |

### 6. 删除日程

```bash
feishu-cli calendar delete-event CAL_ID EVENT_ID
```

## 时间格式说明

使用 **RFC3339** 格式，示例：
- `2024-01-21T14:00:00+08:00` - 北京时间 14:00
- `2024-01-21T06:00:00Z` - UTC 时间 06:00

**常见时区**：
| 时区 | 偏移量 | 示例 |
|------|--------|------|
| 北京时间 | `+08:00` | `2024-01-21T14:00:00+08:00` |
| UTC | `Z` | `2024-01-21T06:00:00Z` |
| 东京时间 | `+09:00` | `2024-01-21T15:00:00+09:00` |

## 典型工作流

### 查看今日日程

```bash
# 1. 获取日历列表
feishu-cli calendar list

# 2. 查看指定日历今日日程
feishu-cli calendar list-events CAL_ID \
  --start-time 2024-01-21T00:00:00+08:00 \
  --end-time 2024-01-21T23:59:59+08:00
```

### 创建会议日程

```bash
# 1. 获取日历 ID
feishu-cli calendar list

# 2. 创建日程
feishu-cli calendar create-event \
  --calendar-id CAL_xxx \
  --summary "周会" \
  --start 2024-01-21T10:00:00+08:00 \
  --end 2024-01-21T11:00:00+08:00 \
  --description "每周团队例会" \
  --location "会议室 101"
```

### 修改日程时间

```bash
# 1. 获取日程列表
feishu-cli calendar list-events CAL_ID

# 2. 更新日程时间
feishu-cli calendar update-event CAL_ID EVENT_ID \
  --start 2024-01-21T15:00:00+08:00 \
  --end 2024-01-21T16:00:00+08:00
```

## 命令别名

`calendar` 命令支持别名 `cal`：

```bash
feishu-cli cal list
feishu-cli cal list-events CAL_ID
```

## 错误处理

| 错误 | 原因 | 解决 |
|------|------|------|
| `Access denied` | 权限不足 | 检查应用是否开通日历权限 |
| `invalid calendar_id` | 日历 ID 无效 | 先用 `calendar list` 获取有效 ID |
| `invalid event_id` | 日程 ID 无效 | 先用 `list-events` 获取有效 ID |
| `invalid time format` | 时间格式错误 | 使用 RFC3339 格式（含时区） |
| `rate limit` | API 限流 | 等待几秒后重试 |

## 注意事项

1. **日历 ID**：创建日程前需要先通过 `calendar list` 获取日历 ID
2. **时间格式**：必须使用 RFC3339 格式，包含时区信息
3. **权限要求**：需要应用具有日历相关权限
4. **创建日历**：当前 CLI 不支持创建新日历，只能使用已有日历创建日程

## 权限要求（重要）

⚠️ 日历功能需要在飞书开放平台为应用开通以下权限之一：

- `calendar:calendar:readonly` - 只读权限
- `calendar:calendar` - 读写权限
- `calendar:calendar.calendar:readonly` - 日历只读
- `calendar:calendar:read` - 日历读取

**申请链接**：在飞书开放平台应用后台 → 权限管理 → 申请对应权限

如果遇到 `Access denied` 错误，请检查应用权限配置。
