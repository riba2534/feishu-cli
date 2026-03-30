---
name: feishu-cli-contact
description: >-
  飞书通讯录：查询用户信息、搜索用户、列出部门用户以及获取部门和子部门信息。
  适用于从姓名、邮箱、手机号定位成员，或需要部门树信息的场景。
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书通讯录技能

这个技能对齐官方 lark-contact 的入口。

## 常用命令

```bash
feishu-cli user info <user_id>
feishu-cli user search --email user@example.com
feishu-cli user search --mobile 13800138000
feishu-cli user list --department-id od_xxx
feishu-cli dept get <department_id>
feishu-cli dept children <department_id>
```

## 适用场景

- 找人
- 通过邮箱 / 手机号映射用户
- 通过部门找成员
- 获取部门层级结构
