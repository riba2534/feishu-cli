---
name: feishu-cli-sheet
description: 飞书电子表格操作：创建表格、读写单元格、V3 富文本 API、行列操作、样式设置、查找替换。支持 V2 简单数据和 V3 富文本两种 API。当用户需要操作飞书表格、批量处理数据、设置样式时使用。
argument-hint: <spreadsheet_token> [command] [args]
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书电子表格操作技能

使用 `feishu-cli` 操作飞书电子表格，支持 V2 和 V3 API。

## 功能概览

| 功能 | 命令 | API 版本 |
|------|------|----------|
| 创建表格 | `sheet create` | V3 |
| 获取信息 | `sheet get` | V3 |
| 列出工作表 | `sheet list-sheets` | V3 |
| 读取单元格 | `sheet read` | V2 |
| 写入单元格 | `sheet write` | V2 |
| 追加数据 | `sheet append` | V2 |
| **获取纯文本** | `sheet read-plain` | **V3** |
| **获取富文本** | `sheet read-rich` | **V3** |
| **写入富文本** | `sheet write-rich` | **V3** |
| **插入数据** | `sheet insert` | **V3** |
| **追加富文本** | `sheet append-rich` | **V3** |
| **清除内容** | `sheet clear` | **V3** |
| 行列操作 | `sheet add-rows/cols` | V2 |
| 样式设置 | `sheet style` | V2 |
| 合并单元格 | `sheet merge/unmerge` | V2 |
| 查找替换 | `sheet find/replace` | V3 |

## 基本操作

```bash
# 创建电子表格
feishu-cli sheet create --title "销售数据"

# 获取表格信息
feishu-cli sheet get <spreadsheet_token>

# 列出工作表
feishu-cli sheet list-sheets <spreadsheet_token>
```

## V2 API（简单数据）

V2 API 使用二维数组格式，适合简单数据读写。

```bash
# 读取单元格
feishu-cli sheet read <token> "Sheet1!A1:C10"

# 写入单元格
feishu-cli sheet write <token> "Sheet1!A1:B2" \
  --data '[["姓名","年龄"],["张三",25]]'

# 追加数据
feishu-cli sheet append <token> "Sheet1!A:B" \
  --data '[["新行1","数据1"]]'
```

## V3 API（富文本）

V3 API 支持富文本内容，包括 @用户、@文档、图片、链接、公式等元素类型。

### 读取数据

```bash
# 获取纯文本内容（批量获取多个范围）
feishu-cli sheet read-plain <token> <sheet_id> "sheet!A1:C10" "sheet!E1:E5"

# 获取富文本内容（返回结构化数据）
feishu-cli sheet read-rich <token> <sheet_id> "sheet!A1:C10" -o json

# 指定渲染选项
feishu-cli sheet read-rich <token> <sheet_id> "sheet!A1:C10" \
  --datetime-render formatted_string \
  --value-render formatted_value
```

### 写入数据

```bash
# 简单模式（二维数组自动转换）
feishu-cli sheet insert <token> <sheet_id> "sheet!A2:B2" \
  --data '[["新数据1","新数据2"]]' --simple

# 富文本模式（从 JSON 文件）
feishu-cli sheet write-rich <token> <sheet_id> --data-file data.json

# 追加富文本
feishu-cli sheet append-rich <token> <sheet_id> "sheet!A1:B1" \
  --data '[["追加数据"]]' --simple
```

### 清除内容

```bash
# 清除单元格内容（保留样式）
feishu-cli sheet clear <token> <sheet_id> "sheet!A1:B3"

# 清除多个范围（最多 10 个）
feishu-cli sheet clear <token> <sheet_id> "sheet!A1:A10" "sheet!C1:C10"
```

## V3 富文本数据格式

### 元素类型

| 类型 | 说明 | 是否独占 |
|------|------|----------|
| `text` | 文本（支持样式） | 否 |
| `value` | 数值 | 是 |
| `date_time` | 日期时间 | 是 |
| `mention_user` | @用户 | 否 |
| `mention_document` | @文档 | 否 |
| `image` | 图片 | 是 |
| `file` | 附件 | 否 |
| `link` | 链接 | 否 |
| `formula` | 公式 | 是 |
| `reminder` | 提醒 | 是 |

### 数据格式示例

**write-rich 的 value_ranges 格式：**

```json
[
  {
    "range": "Sheet1!A1:B2",
    "values": [
      [
        [{"type": "text", "text": {"text": "标题"}}],
        [{"type": "value", "value": {"value": "100"}}]
      ],
      [
        [{"type": "text", "text": {"text": "内容", "segment_style": {"style": {"bold": true, "fore_color": "#FF0000"}}}}],
        [{"type": "formula", "formula": {"formula": "=SUM(A1:A10)"}}]
      ]
    ]
  }
]
```

**insert/append-rich 的 values 格式：**

```json
[
  [
    [{"type": "text", "text": {"text": "A1"}}],
    [{"type": "value", "value": {"value": "123"}}]
  ],
  [
    [{"type": "mention_user", "mention_user": {"user_id": "ou_xxx", "notify": true}}],
    [{"type": "link", "link": {"text": "点击", "link": "https://example.com"}}]
  ]
]
```

### 文本样式

```json
{
  "type": "text",
  "text": {
    "text": "带样式的文本",
    "segment_style": {
      "style": {
        "bold": true,
        "italic": true,
        "strike_through": true,
        "underline": true,
        "fore_color": "#FF0000",
        "font_size": 14
      },
      "affected_text": "带样式"
    }
  }
}
```

## 行列操作

```bash
# 添加行/列
feishu-cli sheet add-rows <token> <sheet_id> -n 5
feishu-cli sheet add-cols <token> <sheet_id> -n 3

# 插入行
feishu-cli sheet insert-rows <token> <sheet_id> --start 2 --count 3

# 删除行/列
feishu-cli sheet delete-rows <token> <sheet_id> --start 0 --end 5
feishu-cli sheet delete-cols <token> <sheet_id> --start 0 --end 3
```

## 格式和样式

```bash
# 合并/取消合并单元格
feishu-cli sheet merge <token> "Sheet1!A1:C3"
feishu-cli sheet unmerge <token> "Sheet1!A1:C3"

# 设置样式
feishu-cli sheet style <token> "Sheet1!A1:C3" \
  --bold --italic \
  --bg-color "#FFFF00" \
  --fore-color "#FF0000"

# 查找内容
feishu-cli sheet find <token> <sheet_id> "关键词"

# 替换内容
feishu-cli sheet replace <token> <sheet_id> "旧值" "新值"
```

## 工作表管理

```bash
# 添加工作表
feishu-cli sheet add-sheet <token> --title "新工作表"

# 删除工作表
feishu-cli sheet delete-sheet <token> <sheet_id>

# 复制工作表
feishu-cli sheet copy-sheet <token> <sheet_id> --title "副本"
```

## API 限制

| 限制项 | V2 API | V3 API |
|--------|--------|--------|
| 单次写入单元格数 | 5000 | 5000 |
| 单元格字符数 | 50000 | 50000 |
| 单次范围数 | - | 10 |
| 单次图片数 | - | 50 |
| 单次 @文档数 | - | 10 |
| 单次提醒数 | - | 100 |
| 频率限制 | 100次/分钟 | 100次/分钟 |

## 错误处理

### 常见错误

| 错误 | 原因 | 解决 |
|------|------|------|
| `range format error` | 范围格式错误 | 使用 `SheetID!A1:B2` 格式 |
| `invalid sheet ID` | 工作表不存在 | 先用 `list-sheets` 确认 |
| `write limit exceeded` | 超出写入限制 | 分批写入，每次不超过 5000 单元格 |
| `parse error` | JSON 格式错误 | 检查 `--data` 参数 JSON 格式 |
| `invalid operation` | 不支持的操作 | 参见已知问题 |
| `rate limit` | API 限流 | 等待几秒后重试 |

### 最佳实践

1. **简单数据用 V2 API**：纯文本/数值使用 `sheet read/write` 更简单
2. **富文本用 V3 API**：需要 @用户、公式等时使用 `sheet read-rich/write-rich`
3. **批量操作**：V3 API 支持一次请求多个范围
4. **使用 --simple 模式**：`insert` 和 `append-rich` 支持 `--simple` 简化输入
5. **范围格式**：始终使用 `SheetID!A1:B2` 格式，避免歧义

## 已知问题

| 问题 | 说明 | 状态 |
|------|------|------|
| `sheet filter` | 需要完整的 col + condition 参数，参数格式复杂 | API 限制 |
| `sheet protect` | V2 API 返回 `invalid operation`，保护工作表功能不可用 | 待修复 |

## 权限要求

- `sheets:spreadsheet` - 电子表格读写

## V3 数据格式补充说明

V3 API 的数据为三层嵌套数组：`行 → 列 → 单元格元素`。

```
values[行索引][列索引] = [元素1, 元素2, ...]  // 单元格内可包含多个元素
```

示例：2 行 2 列的数据：
```json
[
  [ [{"type":"text","text":{"text":"A1"}}], [{"type":"text","text":{"text":"B1"}}] ],
  [ [{"type":"text","text":{"text":"A2"}}], [{"type":"text","text":{"text":"B2"}}] ]
]
```
