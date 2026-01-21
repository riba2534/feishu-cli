---
name: feishu-cli-comment
description: 文档评论操作。当用户请求查看、添加飞书文档评论时使用。
argument-hint: <subcommand> <file_token> [args]
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书文档评论技能

管理飞书云文档的评论，包括列出评论和添加新评论。

## 使用方法

```bash
/feishu-comment list <file_token>        # 列出评论
/feishu-comment add <file_token> "评论"  # 添加评论
```

## CLI 命令详解

### 1. 列出文档评论

```bash
# 列出云文档的评论
feishu-cli comment list <file_token> --type docx

# 列出电子表格的评论
feishu-cli comment list <file_token> --type sheet

# 指定用户类型（获取评论者信息）
feishu-cli comment list <file_token> --type docx --user-id-type open_id
```

**参数说明**：
| 参数 | 说明 | 必需 | 默认值 |
|------|------|------|--------|
| `file_token` | 文件 Token | 是 | - |
| `--type` | 文件类型（docx/sheet/bitable） | 是 | - |
| `--user-id-type` | 用户 ID 类型 | 否 | `open_id` |

**输出示例**：
```
文档评论列表（file_token: doccnXxx）:

评论 #1
  ID: 7123456789012345678
  内容: 这个方案很好，建议增加性能测试数据
  作者: ou_xxx (张三)
  时间: 2024-01-21 14:30:25
  状态: 未解决
  回复数: 2

评论 #2
  ID: 7123456789012345679
  内容: 文档结构清晰，LGTM
  作者: ou_yyy (李四)
  时间: 2024-01-21 10:15:00
  状态: 已解决
  回复数: 0

共 2 条评论
```

### 2. 添加评论

```bash
# 添加全文评论（不关联特定内容）
feishu-cli comment add <file_token> --type docx --text "这是一条评论"

# 添加包含 @ 的评论
feishu-cli comment add <file_token> --type docx --text "请 @张三 review 一下"
```

**参数说明**：
| 参数 | 说明 | 必需 |
|------|------|------|
| `file_token` | 文件 Token | 是 |
| `--type` | 文件类型 | 是 |
| `--text` | 评论内容 | 是 |

**输出示例**：
```
评论添加成功！
  评论 ID: 7123456789012345680
  文件: doccnXxx
  内容: 这是一条评论
```

## 支持的文件类型

| --type 参数 | 说明 |
|-------------|------|
| `docx` | 新版云文档 |
| `doc` | 旧版云文档 |
| `sheet` | 电子表格 |
| `bitable` | 多维表格 |

## 典型工作流

### 代码审查流程

```bash
# 1. 读取文档内容
feishu-cli doc export doccnXxx -o /tmp/doc.md

# 2. 查看现有评论
feishu-cli comment list doccnXxx --type docx

# 3. 添加审查意见
feishu-cli comment add doccnXxx --type docx --text "第三章的流程图需要更新"
```

### 批量查看评论

```bash
# 列出多个文档的评论
for token in doccnXxx doccnYyy doccnZzz; do
  echo "=== 文档: $token ==="
  feishu-cli comment list $token --type docx
done
```

## 权限要求

- `drive:drive.comment:readonly` - 读取评论
- `drive:drive.comment:write` - 添加评论

## 注意事项

1. **评论位置**：当前 CLI 仅支持全文评论，不支持关联特定文字/段落的评论
2. **@ 功能**：在评论中使用 `@用户名` 需要用户有文档访问权限
3. **评论状态**：评论状态（已解决/未解决）需通过飞书客户端修改
