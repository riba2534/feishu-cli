# 知识库工作流

wiki 使用 node token；普通文档使用 document ID。先解析 URL 再选择命令。

## 查询与导出

```bash
feishu-cli wiki get <node_token>
feishu-cli wiki spaces
feishu-cli wiki nodes <space_id>
feishu-cli wiki space-get <space_id>
feishu-cli wiki member list <space_id>
feishu-cli wiki export <node_token> --output doc.md
feishu-cli wiki export-tree <node_token> --output-dir ./backup
```

## 写操作

```bash
feishu-cli wiki create --space-id <space_id> --title "新文档"
feishu-cli wiki update <node_token> --title "新标题"
feishu-cli wiki move <node_token> --target-space <space_id>
feishu-cli wiki node-copy --space-id <src> --node-token <node> --target-space-id <dst>
feishu-cli wiki space-create --name "新知识库"
feishu-cli wiki member add <space_id> --member-id ou_xxx --member-type openid --role editor
```

删除节点、成员或整个空间前必须确认目标。删除空间只有显式 `--yes` 才执行，并会轮询异步任务：

```bash
feishu-cli wiki delete-space <space_id> --yes
```

读取类优先 User Token、可回落 App Token；创建、更新、移动和成员写操作默认 Bot 身份，只有显式
User Token 才切换身份。递归导出知识库必须使用 `wiki export-tree`，不要手写遍历脚本替代。
