---
name: feishu-cli-perm
description: 为飞书云文档添加或更新协作者权限。支持用户/群/部门/群组/知识空间成员授予查看、编辑或管理权限。当用户需要添加文档权限、分享文档给他人时使用。
argument-hint: <doc_token> --perm <view|edit|full_access>
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书权限管理技能

为飞书云文档添加或更新协作者权限。

## 适用场景

- 给飞书文档添加协作者权限
- 批量授权脚本化
- 明确协作者类型、权限级别、是否通知

## 命令格式

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

## 参数说明

### doc-type（云文档类型）

| 值 | 说明 |
|----|------|
| docx | 新版文档 |
| doc | 旧版文档 |
| sheet | 电子表格 |
| bitable | 多维表格 |
| wiki | 知识库 |
| file | 文件 |
| folder | 文件夹 |
| mindnote | 思维笔记 |
| minutes | 妙记 |
| slides | 幻灯片 |

### member-type（协作者 ID 类型）

| 值 | 说明 | 示例 |
|----|------|------|
| email | 邮箱 | user@example.com |
| openid | Open ID | ou_xxx |
| unionid | Union ID | on_xxx |
| userid | User ID | 123456 |
| openchat | 群聊 ID | oc_xxx |
| opendepartmentid | 部门 ID | od_xxx |
| groupid | 群组 ID | gc_xxx |
| wikispaceid | 知识空间 ID | ws_xxx |

### perm（权限角色）

| 值 | 说明 |
|----|------|
| view | 查看权限 |
| edit | 编辑权限 |
| full_access | 完全访问权限（可管理） |

### 可选参数

- `--notification`：添加权限后通知对方

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

### 按 Open ID 添加用户查看权限

```bash
feishu-cli perm add docx_xxxxxx \
  --doc-type docx \
  --member-type openid \
  --member-id ou_xxxxxx \
  --perm view
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

### 更新已有权限

```bash
feishu-cli perm update docx_xxxxxx \
  --doc-type docx \
  --member-type email \
  --member-id user@example.com \
  --perm full_access
```

## 执行流程

1. **收集文档信息**
   - 获取文档 Token（从 URL 或用户提供）
   - 确定 doc-type（根据 Token 前缀判断）

2. **收集协作者信息**
   - 确定 member-type（邮箱最常用）
   - 获取 member-id

3. **选择权限级别**
   - view：仅查看
   - edit：可编辑
   - full_access：完全访问

4. **执行命令**
   - 可选添加 `--notification` 通知对方

## Token 前缀对应关系

| 前缀 | doc-type |
|------|----------|
| docx_ | docx |
| doccn | doc |
| sht_ | sheet |
| bascn | bitable |
| wikicn | wiki |
| fldcn | folder |

## 常见默认操作

**创建文档后自动授权**：

```bash
# 创建文档后，给指定用户添加完全访问权限
feishu-cli perm add <doc_token> \
  --doc-type docx \
  --member-type email \
  --member-id user@example.com \
  --perm full_access \
  --notification
```

## 参考

详细参数枚举见 `references/add_permission.md`
