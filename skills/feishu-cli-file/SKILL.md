---
name: feishu-cli-file
description: 云空间文件管理。当用户请求列出、创建、移动、复制、删除飞书云空间文件时使用。
argument-hint: <subcommand> [args]
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书云空间文件管理技能

管理飞书云空间（Drive）中的文件和文件夹，包括列出、创建、移动、复制、删除操作。

## 使用方法

```bash
/feishu-file list [folder_token]         # 列出文件
/feishu-file mkdir <name> [--parent]     # 创建文件夹
/feishu-file move <token> --target       # 移动文件
/feishu-file copy <token> --target       # 复制文件
/feishu-file delete <token>              # 删除文件
```

## CLI 命令详解

### 1. 列出文件夹内容

```bash
# 列出根目录（我的空间）
feishu-cli file list

# 列出指定文件夹
feishu-cli file list <folder_token>
```

**输出示例**：
```
文件列表（文件夹: root）:
  1. [文件夹] 项目文档
     Token: fldcnxxx
     修改时间: 2024-01-21 14:30

  2. [docx] 会议纪要
     Token: doccnxxx
     修改时间: 2024-01-20 10:15

  3. [sheet] 数据表格
     Token: shtcnxxx
     修改时间: 2024-01-19 16:45
```

**文件类型**：
| 类型 | 说明 |
|------|------|
| `folder` | 文件夹 |
| `docx` | 云文档 |
| `sheet` | 电子表格 |
| `bitable` | 多维表格 |
| `mindnote` | 思维笔记 |
| `file` | 普通文件 |

### 2. 创建文件夹

```bash
# 在根目录创建
feishu-cli file mkdir "新文件夹"

# 在指定位置创建
feishu-cli file mkdir "子文件夹" --parent <folder_token>
```

**输出示例**：
```
文件夹创建成功！
  名称: 新文件夹
  Token: fldcnAbCdEfGhIjK
  路径: /我的空间/新文件夹
```

### 3. 移动文件或文件夹

```bash
# 移动文档
feishu-cli file move <file_token> --target <folder_token> --type docx

# 移动文件夹
feishu-cli file move <folder_token> --target <parent_folder> --type folder
```

**参数说明**：
| 参数 | 说明 | 必需 |
|------|------|------|
| `file_token` | 要移动的文件/文件夹 Token | 是 |
| `--target` | 目标文件夹 Token | 是 |
| `--type` | 文件类型（docx/sheet/folder/file 等） | 是 |

**输出示例**：
```
文件移动成功！
  文件: doccnAbCdEfGhIjK
  目标: fldcnXyZaBcDeFgH
```

### 4. 复制文件

```bash
# 复制文档
feishu-cli file copy <file_token> --target <folder_token> --type docx

# 复制并重命名
feishu-cli file copy <file_token> --target <folder_token> --type docx --name "副本"
```

**参数说明**：
| 参数 | 说明 | 必需 |
|------|------|------|
| `file_token` | 要复制的文件 Token | 是 |
| `--target` | 目标文件夹 Token | 是 |
| `--type` | 文件类型 | 是 |
| `--name` | 新文件名 | 否 |

**输出示例**：
```
文件复制成功！
  原文件: doccnAbCdEfGhIjK
  新文件: doccnNewCopyToken
  位置: fldcnXyZaBcDeFgH
```

### 5. 删除文件或文件夹

```bash
# 删除文档
feishu-cli file delete <file_token> --type docx

# 删除文件夹（包含内容）
feishu-cli file delete <folder_token> --type folder
```

**注意**：删除操作会将文件移动到回收站，可在 30 天内恢复。

**输出示例**：
```
文件删除成功！
  文件: doccnAbCdEfGhIjK
  类型: docx
```

## 文件类型对照表

| --type 参数 | 说明 | 示例 Token |
|-------------|------|-----------|
| `docx` | 新版云文档 | `doccnXxx` |
| `doc` | 旧版云文档 | `docXxx` |
| `sheet` | 电子表格 | `shtcnXxx` |
| `bitable` | 多维表格 | `bitXxx` |
| `mindnote` | 思维笔记 | `mindXxx` |
| `folder` | 文件夹 | `fldcnXxx` |
| `file` | 普通文件 | `boxcnXxx` |

## 典型工作流

### 整理文档目录

```bash
# 1. 查看当前目录结构
feishu-cli file list

# 2. 创建新文件夹
feishu-cli file mkdir "2024年项目"

# 3. 移动文档到新文件夹
feishu-cli file move doccnXxx --target fldcnXxx --type docx
```

### 备份重要文档

```bash
# 1. 创建备份文件夹
feishu-cli file mkdir "备份-$(date +%Y%m%d)"

# 2. 复制文档
feishu-cli file copy doccnXxx --target fldcnBackup --type docx --name "文档备份"
```

## 权限要求

- `drive:drive:readonly` - 读取文件列表
- `drive:drive` - 文件操作（创建/移动/复制/删除）
