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

### 移出知识库到云盘（move-to-drive）

`wiki move-to-drive` 是 `wiki move-docs`（云盘 → 知识库）的**反向操作**：把知识库节点移出知识空间、
转存到云盘文件夹。底层 `POST /open-apis/wiki/v2/nodes/{node_token}/move_wiki_to_docs` **始终异步**，
返回 task_id；命令默认轮询任务 `move_wiki_to_docs` 直至成功 / 失败 / 超时。

```bash
# 移动到指定云盘文件夹（默认轮询等待）
feishu-cli wiki move-to-drive --node-token wikcnXXXX --folder-token fldcnYYYY

# 省略 --folder-token → 移动到调用方个人空间根目录（通常需用户身份）
feishu-cli wiki move-to-drive --node-token wikcnXXXX --user-access-token u-xxx

# 只提交不等待，拿 task_id 自行查询
feishu-cli wiki move-to-drive --node-token wikcnXXXX --folder-token fldcnYYYY --wait=false
```

关键点：
- `--node-token` 必须是知识库 node_token（`wikcnXXXX`），不是底层文档 obj_token
- 移动后节点脱离知识库树，原继承的知识库权限被目标云盘文件夹的权限模型替换
- `--wait`（默认 true）控制是否轮询，`--timeout`（默认 60 秒）控制轮询上限；超时不代表失败，可凭 task_id 再查
- 需 `wiki:node:move` 或 `wiki:wiki` + 查询任务的 `wiki:space:read`

读取类优先 User Token、可回落 App Token；创建、更新、移动（含 move-to-drive）和成员写操作默认 Bot 身份，
只有显式 User Token 才切换身份。递归导出知识库必须使用 `wiki export-tree`，不要手写遍历脚本替代。
