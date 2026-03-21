---
name: feishu-cli-bitable
description: >-
  飞书多维表格（Bitable/Base）全功能操作：创建多维表格、数据表管理、字段管理、记录增删改查（单条+批量）、
  视图管理、搜索过滤排序。当用户请求"创建多维表格"、"操作数据表"、"添加记录"、"查询记录"、
  "管理字段"、"多维表格"、"base"、"bitable"、"数据表"时使用。
  也适用于：用户需要创建结构化数据库、批量导入数据、管理表字段和视图的场景。
  注意：电子表格（Sheets）请使用 feishu-cli-toolkit，两者是不同的产品。
argument-hint: "[app_token] [table_id]"
user-invocable: true
allowed-tools: Bash, Read, Write
---

# 飞书多维表格（Bitable）操作

## 前置条件

- **认证**：需要有效的 App Access Token（环境变量 `FEISHU_APP_ID` + `FEISHU_APP_SECRET`，或 `~/.feishu-cli/config.yaml`）
- **权限**：应用需开通 `bitable:app`（多维表格读写）
- **验证**：`feishu-cli auth status` 确认认证状态正常

## 核心概念

| 概念 | 说明 |
|------|------|
| **app_token** | 多维表格 URL 中 `/base/` 后的字符串，标识一个多维表格 |
| **table_id** | 数据表 ID，通过 `bitable tables` 获取 |
| **record_id** | 记录 ID，通过 `bitable records` 获取 |
| **field_id** | 字段 ID，通过 `bitable fields` 获取 |

**bitable ≠ sheet**：`feishu-cli bitable` 操作多维表格（Base），`feishu-cli sheet` 操作电子表格（Sheets），两者是完全不同的产品和 API。

## 命令速查

```bash
# === 多维表格 ===
feishu-cli bitable create --name "项目管理"
feishu-cli bitable create --name "数据库" --folder FOLDER_TOKEN
feishu-cli bitable get <app_token>

# === 数据表 ===
feishu-cli bitable tables <app_token>
feishu-cli bitable create-table <app_token> --name "任务表"
feishu-cli bitable rename-table <app_token> <table_id> --name "新表名"
feishu-cli bitable delete-table <app_token> <table_id>

# === 字段管理 ===
feishu-cli bitable fields <app_token> <table_id>
feishu-cli bitable fields <app_token> <table_id> -o json    # JSON 输出（含完整 property）
feishu-cli bitable create-field <app_token> <table_id> --field '{"field_name":"状态","type":3,"property":{"options":[{"name":"进行中"},{"name":"已完成"}]}}'
feishu-cli bitable update-field <app_token> <table_id> <field_id> --field '{"field_name":"新名称","type":1}'
feishu-cli bitable delete-field <app_token> <table_id> <field_id>

# === 记录操作 ===
feishu-cli bitable records <app_token> <table_id>
feishu-cli bitable records <app_token> <table_id> --page-size 100
feishu-cli bitable records <app_token> <table_id> --filter '{"conjunction":"and","conditions":[{"field_name":"状态","operator":"is","value":["进行中"]}]}'
feishu-cli bitable records <app_token> <table_id> --sort '[{"field_name":"创建时间","desc":true}]'
feishu-cli bitable records <app_token> <table_id> --field-names "名称,状态,金额"
feishu-cli bitable get-record <app_token> <table_id> <record_id>
feishu-cli bitable add-record <app_token> <table_id> --fields '{"名称":"测试","金额":100,"状态":"进行中"}'
feishu-cli bitable add-records <app_token> <table_id> --data '[{"名称":"A","金额":100},{"名称":"B","金额":200}]'
feishu-cli bitable add-records <app_token> <table_id> --data-file records.json
feishu-cli bitable update-record <app_token> <table_id> <record_id> --fields '{"状态":"已完成"}'
feishu-cli bitable delete-records <app_token> <table_id> --record-ids "recXXX,recYYY"

# === 视图管理 ===
feishu-cli bitable views <app_token> <table_id>
feishu-cli bitable create-view <app_token> <table_id> --name "看板" --type kanban
feishu-cli bitable delete-view <app_token> <table_id> <view_id>
```

**别名**：`feishu-cli base` 等同于 `feishu-cli bitable`。

## 字段类型速查

| type | 中文名 | 写入格式 | 示例 |
|------|--------|---------|------|
| 1 | 多行文本 | 字符串 | `"文本内容"` |
| 2 | 数字 | 数值 | `100`（不要传字符串 `"100"`） |
| 3 | 单选 | 字符串 | `"选项A"`（自动创建选项） |
| 4 | 多选 | 字符串数组 | `["A","B"]` |
| 5 | 日期 | 13 位毫秒时间戳 | `1770508800000` |
| 7 | 复选框 | 布尔值 | `true`（建议用单选替代） |
| 11 | 人员 | 对象数组 | `[{"id":"ou_xxx"}]` |
| 15 | 超链接 | 对象 | `{"text":"名称","link":"https://..."}` |
| 18 | 单向关联 | 字符串数组 | `["recuxxx"]` |

**不支持 API 写入**：公式、查找引用、创建时间、修改人、自动编号。

## 过滤运算符

| 运算符 | 说明 | 适用类型 |
|--------|------|---------|
| is | 等于 | 文本/单选/数字 |
| isNot | 不等于 | 文本/单选/数字 |
| contains | 包含 | 文本/多选 |
| doesNotContain | 不包含 | 文本/多选 |
| isEmpty | 为空 | 所有类型 |
| isNotEmpty | 不为空 | 所有类型 |
| isGreater | 大于 | 数字/日期 |
| isLess | 小于 | 数字/日期 |

## 踩坑必读

### 1. PUT fields 更新描述会清空单选选项

使用 `update-field` 更新单选（type=3）字段时，**必须带上完整的 property（含 options 列表）**，否则选项被清空，所有记录中该字段的值丢失。

**正确做法**：先用 `fields -o json` 获取当前字段定义，合并修改后再更新：

```bash
# 1. 获取当前字段定义
feishu-cli bitable fields <app_token> <table_id> -o json

# 2. 更新时带上完整 property
feishu-cli bitable update-field <app_token> <table_id> <field_id> \
  --field '{"field_name":"状态","type":3,"property":{"options":[{"name":"待处理"},{"name":"进行中"},{"name":"已完成"}]},"description":{"text":"任务状态"}}'
```

### 2. 创建 Base 默认表有空行

`bitable create` 创建多维表格时自动创建一张默认表，里面有空记录（约 10 行）。写入数据前建议先清理：

```bash
# 1. 列出所有记录
feishu-cli bitable records <app_token> <table_id> -o json

# 2. 找出空行的 record_id，批量删除
feishu-cli bitable delete-records <app_token> <table_id> --record-ids "rec1,rec2,rec3"
```

### 3. 不要用复选框（Checkbox），用单选替代

复选框在 GUI 上容易误触。改用单选（type=3）配置"是/否"选项：

```bash
feishu-cli bitable create-field <app_token> <table_id> \
  --field '{"field_name":"必填","type":3,"property":{"options":[{"name":"是"},{"name":"否"}]}}'
```

### 4. 主索引列重命名需带 type

重命名 `is_primary=true` 的字段时必须带 `type` 字段，否则报 `99992402`：

```bash
# 错误：缺少 type
feishu-cli bitable update-field ... --field '{"field_name":"新名称"}'

# 正确：带上 type
feishu-cli bitable update-field ... --field '{"field_name":"新名称","type":1}'
```

### 5. API 创建的表格默认不可见

通过 API 创建的多维表格默认只有机器人能看到。创建后必须立即添加权限：

**邮箱来源**：`~/.feishu-cli/config.yaml` 中的 `owner_email`，或环境变量 `FEISHU_OWNER_EMAIL`。

```bash
# 添加 full_access 权限
feishu-cli perm add <app_token> --doc-type bitable --member-type email --member-id <owner_email> --perm full_access --notification
```

如果配置了 `transfer_ownership: true`，还需转移所有权：
```bash
feishu-cli perm transfer-owner <app_token> --doc-type bitable --member-type email --member-id <owner_email>
```

### 6. 关联字段的局限

关联字段是"一列关联一张表"的设计，无法让不同行关联到不同子表。如果主表的不同记录需要对应到不同子表，直接靠**命名约定**（如工具名称 → 同名子表）更实用。

### 7. 数据格式注意

- **数值不要传字符串**：`100` 而非 `"100"`
- **日期必须是 13 位毫秒时间戳**：`1770508800000`
- **批量操作最多 500 条**：超出需要分批处理

## 典型工作流

### 创建项目管理表

```bash
# 1. 创建多维表格
feishu-cli bitable create --name "项目管理" -o json
# → app_token

# 2. 查看默认表
feishu-cli bitable tables <app_token>
# → table_id

# 3. 添加字段
feishu-cli bitable create-field <app_token> <table_id> --field '{"field_name":"负责人","type":1}'
feishu-cli bitable create-field <app_token> <table_id> --field '{"field_name":"状态","type":3,"property":{"options":[{"name":"待处理"},{"name":"进行中"},{"name":"已完成"}]}}'
feishu-cli bitable create-field <app_token> <table_id> --field '{"field_name":"优先级","type":3,"property":{"options":[{"name":"P0"},{"name":"P1"},{"name":"P2"}]}}'
feishu-cli bitable create-field <app_token> <table_id> --field '{"field_name":"截止日期","type":5}'

# 4. 清理默认空行
feishu-cli bitable records <app_token> <table_id> -o json
feishu-cli bitable delete-records <app_token> <table_id> --record-ids "..."

# 5. 批量写入数据
feishu-cli bitable add-records <app_token> <table_id> --data '[
  {"标题":"完成需求文档","负责人":"张三","状态":"进行中","优先级":"P0"},
  {"标题":"代码审查","负责人":"李四","状态":"待处理","优先级":"P1"}
]'

# 6. 添加权限
feishu-cli perm add <app_token> --doc-type bitable --member-type email --member-id user@example.com --perm full_access --notification

# 7. 创建看板视图
feishu-cli bitable create-view <app_token> <table_id> --name "状态看板" --type kanban
```

## API 限制

| 限制 | 说明 |
|------|------|
| 批量操作上限 | 单次最多 500 条记录 |
| 搜索分页 | page_size 最大 500 |
| 字段数量 | 单表最多 200 个字段 |
| 默认空行 | 新建表格自动创建约 10 行空记录 |
| 权限可见性 | API 创建的表格默认仅机器人可见 |

## 权限要求

| 功能 | 所需权限 |
|------|---------|
| 多维表格读写 | `bitable:app` |
| 权限管理 | `docs:permission.member:create` |
| 所有权转移 | `docs:permission.member:create` |
