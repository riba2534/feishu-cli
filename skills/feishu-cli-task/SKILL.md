---
name: feishu-cli-task
description: 任务管理。当用户请求创建任务、查看任务、完成任务或管理任务时使用。支持创建/查看/更新/删除/完成任务等操作。
argument-hint: <subcommand> [args]
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书任务操作技能

管理飞书任务，包括创建、查看、更新、删除和完成任务。

## 使用方法

```bash
/feishu-task create --summary "任务标题"       # 创建任务
/feishu-task list                              # 列出任务
/feishu-task list --completed                  # 列出已完成任务
/feishu-task get <task_id>                     # 获取任务详情
/feishu-task update <task_id> --summary "新标题"  # 更新任务
/feishu-task complete <task_id>                # 完成任务
/feishu-task delete <task_id>                  # 删除任务
```

## CLI 命令详解

### 1. 创建任务

```bash
# 创建简单任务
feishu-cli task create --summary "完成项目文档"

# 创建带描述的任务
feishu-cli task create --summary "代码审查" --description "审查 PR #123"

# 创建带截止时间的任务
feishu-cli task create --summary "提交报告" --due "2024-12-31 18:00:00"

# 创建带来源链接的任务
feishu-cli task create --summary "处理 Issue" \
  --origin-href "https://github.com/example/repo/issues/1"

# JSON 格式输出
feishu-cli task create --summary "测试任务" --output json
```

**参数说明**：
| 参数 | 简写 | 说明 | 必填 |
|------|------|------|------|
| `--summary` | `-s` | 任务标题 | 是 |
| `--description` | `-d` | 任务描述 | 否 |
| `--due` | | 截止时间 | 否 |
| `--origin-href` | | 任务来源链接 | 否 |
| `--origin-platform` | | 任务来源平台名称 | 否（默认 `feishu-cli`） |
| `--output` | `-o` | 输出格式（json） | 否 |

**截止时间格式**：
- `2024-12-31 18:00:00` - 完整格式
- `2024-12-31` - 仅日期

### 2. 列出任务

```bash
# 列出所有任务
feishu-cli task list

# 列出已完成的任务
feishu-cli task list --completed

# 列出未完成的任务
feishu-cli task list --uncompleted

# 分页查询
feishu-cli task list --page-size 10

# JSON 格式输出
feishu-cli task list --output json
```

**参数说明**：
| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--completed` | 只显示已完成任务 | false |
| `--uncompleted` | 只显示未完成任务 | false |
| `--page-size` | 每页数量 | 50 |
| `--page-token` | 分页标记 | 无 |
| `--output, -o` | 输出格式 | 文本 |

### 3. 获取任务详情

```bash
feishu-cli task get <task_id>

# JSON 格式输出
feishu-cli task get <task_id> --output json
```

**输出示例**：
```
任务详情:
  ID:       e297ddff-06ca-4166-b917-4ce57cd3a7a0
  标题:     完成项目文档
  描述:     编写 Q1 项目技术文档
  状态:     未完成
  截止时间: 2024-12-31 18:00:00
  创建时间: 2024-01-15 10:30:00
```

### 4. 更新任务

```bash
# 更新任务标题
feishu-cli task update <task_id> --summary "新标题"

# 更新任务描述
feishu-cli task update <task_id> --description "新描述"

# 更新截止时间
feishu-cli task update <task_id> --due "2024-12-31 18:00:00"

# 通过 update 标记任务为已完成
feishu-cli task update <task_id> --completed

# JSON 格式输出
feishu-cli task update <task_id> --summary "新标题" --output json
```

**参数说明**：
| 参数 | 简写 | 说明 |
|------|------|------|
| `--summary` | `-s` | 新的任务标题 |
| `--description` | `-d` | 新的任务描述 |
| `--due` | | 新的截止时间 |
| `--completed` | | 标记任务为已完成 |
| `--output` | `-o` | 输出格式（json） |

### 5. 完成任务

```bash
feishu-cli task complete <task_id>
```

### 6. 删除任务

```bash
feishu-cli task delete <task_id>
```

## 任务状态说明

| 状态 | 说明 |
|------|------|
| 未完成 | 任务创建后的默认状态 |
| 已完成 | 通过 `complete` 或 `update --completed` 标记 |

## 典型工作流

### 创建并管理任务

```bash
# 1. 创建任务
feishu-cli task create --summary "完成代码审查" \
  --description "审查 feature/new-api 分支" \
  --due "2024-01-25 18:00:00"

# 2. 查看任务列表
feishu-cli task list --uncompleted

# 3. 完成任务
feishu-cli task complete <task_id>
```

### 从 GitHub Issue 创建任务

```bash
feishu-cli task create \
  --summary "修复登录 Bug #42" \
  --description "用户反馈登录偶尔失败" \
  --origin-href "https://github.com/company/repo/issues/42" \
  --due "2024-01-20"
```

### 批量查看任务状态

```bash
# 查看所有未完成任务
feishu-cli task list --uncompleted

# 查看已完成任务
feishu-cli task list --completed

# 导出为 JSON 便于处理
feishu-cli task list --output json
```

### 更新任务并完成

```bash
# 1. 更新任务描述
feishu-cli task update <task_id> --description "已完成 80%，待最终测试"

# 2. 修改截止时间
feishu-cli task update <task_id> --due "2024-01-22 12:00:00"

# 3. 完成任务
feishu-cli task complete <task_id>
```

## 注意事项

1. **任务 ID**：UUID 格式，如 `e297ddff-06ca-4166-b917-4ce57cd3a7a0`
2. **截止时间**：支持 `YYYY-MM-DD HH:mm:ss` 或 `YYYY-MM-DD` 格式
3. **来源链接**：可关联外部系统（如 GitHub、Jira）的链接
4. **权限要求**：需要应用具有任务相关权限

## 权限要求（重要）

⚠️ 任务功能需要在飞书开放平台为应用开通以下权限：

- `task:task:read` - 任务读取权限（用于 list/get）
- `task:task:write` - 任务写入权限（用于 create/update/delete/complete）

**申请链接**：在飞书开放平台应用后台 → 权限管理 → 申请对应权限

### 已验证功能

以下命令已通过测试验证：
- `task create` ✅ - 创建任务正常
- `task complete` ✅ - 完成任务正常
- `task delete` ✅ - 删除任务正常
- `task list` ✅ - 列出任务正常（需 `task:task:read` 权限）
- `task get` ✅ - 获取任务详情正常（需 `task:task:read` 权限）

## 错误处理

| 错误 | 原因 | 解决 |
|------|------|------|
| `Access denied` | 权限不足 | 确认应用已开通 `task:task:read` 和 `task:task:write` 权限 |
| `task not found` | 任务不存在 | 检查 task_id 是否正确（UUID 格式） |
| `invalid parameter` | 参数错误 | 检查 --due 时间格式、--summary 是否为空 |
