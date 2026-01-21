---
name: feishu-cli-create
description: 快速创建飞书云文档。当用户请求"创建文档"、"新建文档"、"写一份文档"时使用。
argument-hint: <title>
user-invocable: true
allowed-tools: Bash
---

# 飞书文档创建技能

快速创建一个新的飞书云文档。

## 使用方法

```bash
/feishu-create "文档标题"
```

## 执行流程

1. **创建空文档**
   ```bash
   feishu-cli doc create --title "<title>" --output json
   ```

2. **添加默认权限**
   ```bash
   feishu-cli perm add <document_id> --doc-type docx --member-type email --member-id user@example.com --perm full_access
   ```

3. **发送通知**
   使用 `/lark-send-msg` 技能发送消息通知用户

4. **返回结果**
   - 文档 ID
   - 文档 URL

## 输出格式

```
已创建文档！
  文档 ID: <document_id>
  文档链接: https://feishu.cn/docx/<document_id>
```

## 可选参数

| 参数 | 说明 |
|------|------|
| --folder | 指定父文件夹 Token |

## 示例

```bash
# 创建简单文档
/feishu-create "项目计划"

# 创建带时间戳的文档
/feishu-create "会议纪要-$(date +%Y%m%d)"
```
