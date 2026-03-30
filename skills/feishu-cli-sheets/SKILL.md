---
name: feishu-cli-sheets
description: >-
  飞书电子表格：创建表格、读取和写入单元格、追加行、调整列、样式、合并、
  查找替换，以及 V3 富文本表格读写。适用于需要按表格维度直接操作飞书 Sheets 的场景。
user-invocable: true
allowed-tools: Bash, Read
---

# 飞书电子表格技能

这个技能对齐官方 lark-sheets 的入口。

## 常用命令

```bash
feishu-cli sheet create --title "新表格"
feishu-cli sheet read <token> "Sheet1!A1:C10"
feishu-cli sheet write <token> "Sheet1!A1:B2" --data '[["姓名","年龄"],["张三",25]]'
feishu-cli sheet append <token> "Sheet1!A:B" --data '[["新行1","新行2"]]'
feishu-cli sheet read-rich <token> <sheet_id> "Sheet1!A1:C10"
feishu-cli sheet write-rich <token> <sheet_id> --data-file data.json
feishu-cli sheet add-rows <token> <sheet_id> --count 5
feishu-cli sheet delete-cols <token> <sheet_id> --start 1 --end 3
feishu-cli sheet merge <token> "Sheet1!A1:B2"
feishu-cli sheet find <token> <sheet_id> "关键词" --range "A1:C10"
feishu-cli sheet export <token> -o output.xlsx
```

## 重点

- 纯读取先用 `read` / `read-rich`
- 结构调整先确认范围再执行
- 需要导出时默认优先 XLSX，CSV 要单独指定 `--sheet-id`

