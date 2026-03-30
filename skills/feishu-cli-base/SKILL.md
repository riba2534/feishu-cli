---
name: feishu-cli-base
description: >-
  飞书多维表格（Base / Bitable）：创建、读取、写入、字段、视图、记录、仪表盘、
  工作流和表单管理。适用于和官方 lark-base 对齐的多维表格入口。
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书 Base 技能

这个技能对齐官方 lark-base 的入口，但命令使用 feishu-cli 的 bitable 能力。

## 常用命令

```bash
feishu-cli bitable create --name "新 Base"
feishu-cli bitable get <base_token>
feishu-cli bitable tables <base_token>
feishu-cli bitable fields <base_token> <table_id>
feishu-cli bitable records <base_token> <table_id>
feishu-cli bitable get-record <base_token> <table_id> <record_id>
feishu-cli bitable add-record <base_token> <table_id> --fields '{"名称":"测试"}'
feishu-cli bitable update-record <base_token> <table_id> <record_id> --fields '{"状态":"完成"}'
feishu-cli bitable delete-records <base_token> <table_id> --record-ids rid1,rid2
feishu-cli bitable views <base_token> <table_id>
feishu-cli bitable create-view <base_token> <table_id> --name "新视图"
feishu-cli bitable dashboard list <base_token>
feishu-cli bitable workflow list <base_token>
feishu-cli bitable form list <base_token>
feishu-cli bitable role list <base_token>
feishu-cli bitable data-query <base_token> <table_id> --query query.json
```

## 使用建议

- 用户说“多维表格”优先看这个技能
- 先读表结构，再写字段或记录
- 公式 / 查找引用 / 工作流涉及复杂结构时，先确认字段和 schema
