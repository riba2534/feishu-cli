---
name: feishu-cli-shared
description: >-
  飞书 CLI 共享基础：应用配置初始化、认证登录（auth login）、Token 状态检查、
  scope / 权限错误处理、user 与 bot 身份差异、以及所有其它 feishu-cli 技能的前置规则。
  当用户第一次配置 feishu-cli、遇到权限不足、需要判断该用 App Token 还是 User Token、
  或需要整理认证 / scope / 配置问题时使用。
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书 CLI 共享规则

这个技能提供 feishu-cli 所有其它技能共用的前置约束。先看它，再去看具体模块技能。

## 先做什么

1. 先判断当前任务需要 `bot` 还是 `user` 身份。
2. 先确认是否已完成 `feishu-cli auth login`。
3. 先确认 app id / app secret / config 是否可用。
4. 出现权限错误时，优先判断是缺 scope 还是用了错误身份。

## 身份规则

| 身份 | 说明 | 典型场景 |
|------|------|----------|
| App Token / bot | 应用身份 | 文档创建、消息发送、文件管理、权限操作 |
| User Access Token / user | 用户身份 | 搜索、个人日历、个人消息历史、群聊搜索 |

### 常见判断

- `auth login` 主要用于需要用户授权的能力。
- `config init` 主要用于初始化 app id / app secret。
- 如果命令报权限错，先看当前 skill 是否需要 user 身份。
- 如果是 bot 能力，通常不要引导用户做 `auth login`。

## 常见命令

```bash
feishu-cli config init
feishu-cli auth login
feishu-cli auth status
feishu-cli auth logout
```

## 与其它技能的关系

- 文档读写：看 [feishu-cli-doc](../feishu-cli-doc/SKILL.md)
- 消息发送与会话：看 [feishu-cli-im](../feishu-cli-im/SKILL.md)
- 表格：看 [feishu-cli-sheets](../feishu-cli-sheets/SKILL.md)
- 日历：看 [feishu-cli-calendar](../feishu-cli-calendar/SKILL.md)
- 任务：看 [feishu-cli-task](../feishu-cli-task/SKILL.md)
- Base / 多维表格：看 [feishu-cli-base](../feishu-cli-base/SKILL.md)

