---
name: feishu-cli-perm
description: >-
  飞书云文档权限管理。支持添加/更新/删除/查看协作者、公开权限管理、分享密码、批量添加、
  权限检查、转移所有权。当用户请求"添加权限"、"权限管理"、"共享文档"、"授权"、
  "协作者"、"full_access"、"转移所有权"时使用。
argument-hint: <doc_token> --perm <view|edit|full_access>
user-invocable: true
allowed-tools: Bash(feishu-cli perm:*), Read
---

# 飞书权限管理技能

飞书云文档权限管理：添加/更新/删除/查看协作者、公开权限管理、分享密码、批量添加、权限检查、转移所有权。

## 适用场景

- 给飞书文档添加/更新/删除协作者权限
- 查看文档协作者列表
- 管理文档公开权限（外部访问、链接分享）
- 设置/刷新/删除分享密码
- 批量添加协作者
- 检查用户对文档的权限
- 转移文档所有权

## 前置条件

### 安装与认证

- **feishu-cli**：如尚未安装，请前往 [riba2534/feishu-cli](https://github.com/riba2534/feishu-cli) 获取安装方式
- **认证**：使用 **App Token（应用身份）**，需配置 App ID 和 App Secret（环境变量或 `~/.feishu-cli/config.yaml`）。无需 `auth login`（User Token 不适用于权限管理 API）。

### 所需权限 scope

| scope | 说明 |
|-------|------|
| `docs:permission.member:create` | 添加协作者 |
| `docs:permission.member:retrieve` | 查看协作者列表 |
| `docs:permission.member:update` | 更新协作者权限 |
| `docs:permission.member:delete` | 删除协作者 |
| `docs:permission.member:transfer` | 转移所有权 |
| `docs:permission.member:auth` | 检查用户权限 |
| `docs:permission.setting:read` / `docs:permission.setting:readonly` | 读取公开权限设置 |
| `docs:permission.setting:write_only` | 更新公开权限、密码管理 |

## 快速开始

```bash
# 给用户添加编辑权限（最常用操作）
feishu-cli perm add <TOKEN> --doc-type docx --member-type email --member-id user@example.com --perm edit --notification

# 查看文档协作者列表
feishu-cli perm list <TOKEN> --doc-type docx

# 删除指定协作者
feishu-cli perm delete <TOKEN> --doc-type docx --member-type email --member-id user@example.com
```

## 命令总览

### 一、基础操作

| 命令 | 说明 |
|------|------|
| `perm add` | 添加协作者权限 |
| `perm update` | 更新已有协作者的权限级别 |
| `perm list` | 查看协作者列表 |
| `perm delete` | 删除协作者 |

### 二、高级操作

| 命令 | 说明 |
|------|------|
| `perm batch-add` | 从 JSON 文件批量添加协作者 |
| `perm transfer-owner` | 转移文档所有权 |
| `perm auth` | 检查当前用户对文档的权限 |

### 三、公开设置

| 命令 | 说明 |
|------|------|
| `perm public-get` | 查看文档公开权限设置 |
| `perm public-update` | 更新公开权限（外部访问、链接分享等） |
| `perm password create` | 创建分享密码 |
| `perm password update` | 刷新分享密码 |
| `perm password delete` | 删除分享密码 |

## 命令详情

### 添加权限

```bash
feishu-cli perm add <TOKEN> \
  --doc-type <DOC_TYPE> \
  --member-type <MEMBER_TYPE> \
  --member-id <MEMBER_ID> \
  --perm <PERM> \
  [--notification]
```

### 更新权限

```bash
feishu-cli perm update <TOKEN> \
  --doc-type <DOC_TYPE> \
  --member-type <MEMBER_TYPE> \
  --member-id <MEMBER_ID> \
  --perm <PERM>
```

### 查看协作者列表

```bash
feishu-cli perm list <TOKEN> --doc-type <DOC_TYPE>
```

### 删除协作者

```bash
feishu-cli perm delete <TOKEN> \
  --doc-type <DOC_TYPE> \
  --member-type <MEMBER_TYPE> \
  --member-id <MEMBER_ID>
```

### 批量添加协作者

```bash
feishu-cli perm batch-add <TOKEN> \
  --doc-type <DOC_TYPE> \
  --members-file <members.json> \
  [--notification]
```

`--doc-type` 默认 `docx`，非 docx 文档（sheet / bitable / file / wiki / slides 等）必须显式指定，否则会按 docx 处理导致 API 报错。

members.json 格式（顶层为 JSON 数组）：
```json
[
  {"member_type": "email", "member_id": "user1@example.com", "perm": "edit"},
  {"member_type": "email", "member_id": "user2@example.com", "perm": "view"}
]
```

### 转移所有权

```bash
feishu-cli perm transfer-owner <TOKEN> \
  --member-type <MEMBER_TYPE> \
  --member-id <MEMBER_ID> \
  [--notification] \
  [--remove-old-owner] \
  [--stay-put] \
  [--old-owner-perm <view|edit|full_access>]
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--notification` | true | 通知新所有者 |
| `--remove-old-owner` | false | 移除原所有者权限 |
| `--stay-put` | false | 文档保留在原位置 |
| `--old-owner-perm` | full_access | 原所有者保留权限（仅 remove-old-owner=false 时生效） |

### 权限检查

```bash
feishu-cli perm auth <TOKEN> --action <ACTION> [--doc-type <DOC_TYPE>]
```

可用的 action 值：`view`、`edit`、`share`、`comment`、`export`

### 查看公开权限

```bash
feishu-cli perm public-get <TOKEN>
```

### 更新公开权限

```bash
feishu-cli perm public-update <TOKEN> \
  [--external-access] \
  [--link-share-entity <anyone_readable|anyone_editable|...>]
```

### 分享密码管理

```bash
feishu-cli perm password create <TOKEN> [--doc-type <DOC_TYPE>]
feishu-cli perm password update <TOKEN> [--doc-type <DOC_TYPE>]
feishu-cli perm password delete <TOKEN> [--doc-type <DOC_TYPE>]
```

## 参数说明

### perm（权限级别）

| 值 | 说明 | 使用场景 |
|----|------|----------|
| `view` | 查看权限 | 只读分享给外部人员或大范围分享，保护文档不被误改 |
| `edit` | 编辑权限 | 团队协作编辑，日常最常用的权限级别 |
| `full_access` | 完全访问权限 | 管理员权限，包含管理协作者、文档设置、导出、查看历史版本等全部能力 |

### member-type（协作者 ID 类型）

| 值 | 别名（IM 风格） | 说明 | 使用场景 | 示例 |
|----|-----------------|------|----------|------|
| `email` | — | 邮箱 | **最常用**，精确到个人 | user@example.com |
| `openid` | `open_id` | Open ID | 通过开放平台获取的用户 ID | ou_xxx |
| `userid` | `user_id` | User ID | 企业内部用户 ID | 123456 |
| `unionid` | `union_id` | Union ID | 跨应用统一 ID | on_xxx |
| `openchat` | `chat_id` | 群聊 ID | **按群聊授权**，群内所有成员获得权限 | oc_xxx |
| `opendepartmentid` | — | 部门 ID | **按部门授权**，部门内所有成员获得权限 | od_xxx |
| `groupid` | — | 群组 ID | 用户组 | gc_xxx |
| `wikispaceid` | — | 知识空间 ID | 知识库空间 | ws_xxx |

> IM API 风格别名（`open_id`、`user_id`、`union_id`、`chat_id`）会自动映射为标准值，两种写法等效。

### doc-type（云文档类型）

| 值 | 说明 |
|----|------|
| `docx` | 新版文档（默认） |
| `doc` | 旧版文档 |
| `sheet` | 电子表格 |
| `bitable` | 多维表格 |
| `wiki` | 知识库 |
| `file` | 文件 |
| `folder` | 文件夹 |
| `mindnote` | 思维笔记 |
| `minutes` | 妙记 |
| `slides` | 幻灯片 |

### Token 前缀对应关系

| 前缀 | doc-type |
|------|----------|
| docx_ | docx |
| doccn | doc |
| sht_ | sheet |
| bascn | bitable |
| wikicn | wiki |
| fldcn | folder |

## 示例

### 按邮箱添加用户为编辑者

```bash
feishu-cli perm add docx_xxxxxx \
  --doc-type docx \
  --member-type email \
  --member-id user@example.com \
  --perm edit \
  --notification
```

### 更新已有权限为完全访问

```bash
feishu-cli perm update docx_xxxxxx \
  --doc-type docx \
  --member-type email \
  --member-id user@example.com \
  --perm full_access
```

### 给群聊添加编辑权限

```bash
feishu-cli perm add sht_xxxxxx \
  --doc-type sheet \
  --member-type openchat \
  --member-id oc_xxxxxx \
  --perm edit
```

### 按部门添加查看权限

```bash
feishu-cli perm add sht_xxxxxx \
  --doc-type sheet \
  --member-type opendepartmentid \
  --member-id od_xxxxxx \
  --perm view
```

### 转移所有权并保留原所有者查看权限

```bash
feishu-cli perm transfer-owner docx_xxxxxx \
  --member-type email \
  --member-id user@example.com \
  --old-owner-perm view
```

### 设置文档为"链接可读"

```bash
feishu-cli perm public-update docx_xxxxxx \
  --external-access \
  --link-share-entity anyone_readable
```

### 创建文档后标准授权流程

```bash
# 1. 授予完全访问权限
feishu-cli perm add <TOKEN> \
  --doc-type docx \
  --member-type email \
  --member-id user@example.com \
  --perm full_access \
  --notification

# 2. 转移文档所有权
feishu-cli perm transfer-owner <TOKEN> \
  --doc-type docx \
  --member-type email \
  --member-id user@example.com \
  --notification
```

## 错误排障

| 错误 | 原因 | 解决方法 |
|------|------|----------|
| `Permission denied` / 权限不足 | App 未开通相关权限 scope | 在飞书开放平台 -> 权限管理中申请 `docs:permission.member:create` 等权限 |
| `doc-type mismatch` / Token 无效 | doc-type 与实际文档类型不匹配 | 检查 Token 前缀：`docx_` -> docx、`sht_` -> sheet、`bascn` -> bitable |
| `member not found` | member-id 不存在或 member-type 不正确 | 确认邮箱/ID 正确，注意 email 类型需要用户的飞书注册邮箱 |
| `password create: Permission denied` | 分享密码为企业版功能 | 确认企业是否开通此功能，或联系管理员开启 |
| `transfer-owner: no permission` | 只有文档所有者或管理员可转移 | 先用 `perm list` 确认当前 App 身份，确保是文档创建者 |

## 参考文档

`perm add` 命令的详细参数枚举和输入检查清单见 `references/add_permission.md`。
