# 知识库工作流

wiki 使用 node token；普通文档使用 document ID。先解析 URL 再选择命令。

节点解析统一走 `GET /wiki/v2/spaces/node_by_token`：`wiki get/update/move/export/export-tree/delete` 的
`<node_token>` 也可以直接传挂载在知识库中的文档 obj_token，服务端自动识别类型并返回真实 node_token
（旧 `get_node` 对 obj_token 一律报 131005）。常见错误码：131012 节点已删除、131013/131016 token 无效或被截断、
131014 文档不在知识库、131006 当前身份无权读取——均为终态，不要原样重试。

## 查询与导出

```bash
feishu-cli wiki get <node_token>
feishu-cli wiki spaces                       # 默认只取一页；has_more 时 stderr 提示 page_token
feishu-cli wiki spaces --page-all -o json    # 自动翻页拉全（--page-limit 默认 50 页，0 = 不限）
feishu-cli wiki nodes <space_id> [--parent <node_token>]
feishu-cli wiki nodes <space_id> --page-all -o json
feishu-cli wiki nodes <space_id> --page-token <上次提示的 page_token>   # 手动续翻
feishu-cli wiki space-get <space_id>
feishu-cli wiki member list <space_id>
feishu-cli wiki export <node_token> --output doc.md
feishu-cli wiki export-tree <node_token> --output-dir ./backup
```

`wiki spaces` / `wiki nodes` 的 `-o json` 仍输出**数组**（兼容旧脚本）；默认只取一页，服务端 `has_more=true`
时在 stderr 打印 `has_more=true, page_token=...` 续翻提示（stdout 不受影响，管道安全）。需要完整列表时加
`--page-all`（带重复游标防护，达到 `--page-limit` 上限时 stderr 告警结果不完整）。

## 写操作

```bash
feishu-cli wiki create --space-id <space_id> --title "新文档"   # Bot 创建时自动给当前登录用户授予节点 full_access（permission_grant）
# 创建快捷方式节点必须提供 --origin-node-token
feishu-cli wiki create --space-id <space_id> --title "快捷方式" --node-type shortcut --origin-node-token <origin_node_token>
feishu-cli wiki update <node_token> --title "新标题"
feishu-cli wiki move <node_token> --target-space <space_id>
feishu-cli wiki move <node_token> --target-parent <parent_node_token>   # 空间自动取父节点所在空间
feishu-cli wiki node-copy --space-id <src> --node-token <node> --target-space-id <dst> [--as auto|user|bot] [--dry-run]
feishu-cli wiki space-create --name "新知识库"
feishu-cli wiki member add <space_id> --member-id ou_xxx --member-type openid --role member   # --role 枚举仅 admin/member
feishu-cli wiki member add <space_id> --member-id ou_xxx --member-type openid --role member --need-notification=false --dry-run
```

- `wiki move` 传了 `--target-parent` 时先经 node_by_token 解析父节点（也接受文档 obj_token）：
  `--target-space` 与父节点实际所在空间不一致时**直接报错拒绝**（退出码 2），不会移到意料之外的空间；
  只传 `--target-parent` 时自动取父节点所在空间；`--target-space my_library` 会先解析成真实 space_id 再比较。
- `wiki member add` 默认给新成员发送通知（`--need-notification` 默认 true，保持历史行为）；
  静默添加传 `--need-notification=false`。请求体只发 `member_type/member_id/member_role`（与官方一致）。
- `wiki node-copy` / `wiki delete-space` 是写操作，身份走 `--as`（默认 auto：User 优先、未配置 User 时用 Bot；
  **已配置 User 但解析/刷新失败时 fail-closed 报错**，不会像读命令那样告警后静默降级 Bot）。
  `node-copy --dry-run` 不解析身份、不发请求。

删除节点走官方 `DELETE /wiki/v2/spaces/{space}/nodes/{node}` 接口，支持级联删除与异步任务轮询（若未完成或失败非零退出并保留 task_id 与 resume 命令）；删除空间只有显式 `--yes` 才执行，并会轮询异步任务：

```bash
# 删除节点：URL 输入自动推断 --obj-type；裸 token 必须显式 --obj-type（wiki/docx/sheet/...）
# -f/--force 或全局 --yes 跳过确认；非交互未确认时退出码 10 且不删除
feishu-cli wiki delete https://xxx.feishu.cn/wiki/<node_token> [-f]
feishu-cli wiki delete <node_token> --obj-type wiki [--space-id <space_id>] [-f]
feishu-cli wiki delete-space <space_id> --yes [--as auto|user|bot]
```

- `wiki delete` **无论是否传 `--space-id` 都会先 node_by_token 解析节点**：`--space-id` 与节点实际空间不一致、
  `--obj-type` 与节点实际文档类型不一致时直接拒绝（退出码 2）；快捷方式节点只能用 `--obj-type wiki` 删除
  （按文档类型删除会删到源文档）。服务端 131011（节点开启删除审批）/ 131003（子树过大）会给出对应处理提示。
- 异步任务续查统一用 `drive task-result`：`wiki delete` → `--scenario wiki_delete_node`、
  `wiki delete-space` → `--scenario wiki_delete_space`、`wiki move-to-drive` → `--scenario wiki_move_to_drive`、
  `wiki move-docs` → `--scenario wiki_move`（命令超时/`--wait=false` 时会直接打印带 `--task-id` 和 `--as` 的续查命令）。
  `delete-space` 轮询窗口内未完成时 JSON 含 `timed_out=true` 与 `resume_command`；状态查询全部失败时非零退出。

⚠️ **`--include-children` 默认 true（级联删除整棵子树）**。交互确认会明说级联范围
（"将级联删除该节点及其**全部子节点**"）；只删单节点须显式 `--include-children=false`。
用 `-f` 跳过确认时**没有任何提示**，请先确认清楚范围。
JSON 输出的 `ready` / `failed` 如实反映异步任务终态（不再恒为 ready=true），可据此判断是否真正删成功。

### 移出知识库到云盘（move-to-drive）

`wiki move-to-drive` 是 `wiki move-docs`（云盘 → 知识库）的**反向操作**：把知识库节点移出知识空间、
转存到云盘文件夹。底层 `POST /open-apis/wiki/v2/nodes/{node_token}/move_wiki_to_docs` **始终异步**，
返回 task_id；命令默认轮询任务 `move_wiki_to_docs` 直至成功 / 失败 / 超时。

```bash
# 移动到指定云盘文件夹（默认轮询等待）
feishu-cli wiki move-to-drive --node-token wikcnXXXX --folder-token fldcnYYYY

# 省略 --folder-token → 移动到调用方个人空间根目录（通常需用户身份）
feishu-cli wiki move-to-drive --node-token wikcnXXXX --user-access-token u-xxx

# 只提交不等待，输出 task_id 与 resume_command，之后用 drive task-result 续查
feishu-cli wiki move-to-drive --node-token wikcnXXXX --folder-token fldcnYYYY --wait=false
feishu-cli drive task-result --scenario wiki_move_to_drive --task-id <task_id> --as bot
```

关键点：
- `--node-token` 必须是知识库 node_token（`wikcnXXXX`），不是底层文档 obj_token
- 移动后节点脱离知识库树，原继承的知识库权限被目标云盘文件夹的权限模型替换
- `--wait`（默认 true）控制是否轮询，`--timeout`（默认 60 秒）控制轮询上限；超时不代表失败，按输出的 `resume_command` 续查（请勿重复提交）
- 需 `wiki:node:move` 或 `wiki:wiki` + 查询任务的 `wiki:space:read`

身份：读取类优先 User Token、可回落 App Token（User Token 不可用时 stderr 告警）；创建、更新、移动（含 move-to-drive、
move-docs）和成员写操作默认 Bot 身份，只有显式 `--user-access-token` / `FEISHU_USER_ACCESS_TOKEN` 才切换 User；
`wiki delete` / `wiki delete-space` / `wiki node-copy` 走 `--as bot|user|auto`（默认 auto，已配置 User 但不可用时 fail-closed）。
递归导出知识库必须使用 `wiki export-tree`，不要手写遍历脚本替代。
