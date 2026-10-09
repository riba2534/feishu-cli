# 知识库工作流

管理知识空间、节点结构与成员，以及把节点导出为本地 Markdown。读取或总结 wiki 文档正文用 `feishu-cli-docs`；
节点的协作者权限用 `../perm/workflow.md`（`--doc-type wiki`）；wiki 异步任务续查用 `../drive/workflow.md` 的 `drive task-result`。

## 节点 token 与解析

知识库节点有两个 token：`node_token`（`/wiki/` URL 里的那段）和底层文档的 `obj_token`（docx/sheet 等）。
节点解析统一走 `GET /wiki/v2/spaces/node_by_token`：

- `wiki get/update/move/export/export-tree/delete` 的 `<node_token>` 也可以直接传挂载在知识库中的文档 obj_token、
  `/wiki/` URL 或 `/docx/`、`/sheets/` 等文档 URL，服务端自动识别并返回真实 node_token。
- 只接受 node_token 的入口：`wiki nodes --parent`、`wiki node-copy --node-token`、`wiki move-to-drive --node-token`、
  `wiki create --parent-node`、`drive update-title --type wiki`、`perm ... --doc-type wiki`。手里只有 obj_token 或文档 URL 时，
  先 `feishu-cli wiki get <obj_token|url> -o json` 取 `node_token`。
- 常见错误码（均为终态，不要原样重试）：131012 节点已删除、131013/131016 token 无效或被截断、131014 文档不在知识库
  （普通云文档直接用其 docx/sheets token）、131006 当前身份无权读取。

## 身份

| 命令 | 默认身份 |
|---|---|
| `wiki get/nodes/spaces/space-get/member list/export/export-tree` | User 优先，不可用时 stderr 告警后回退 App/Bot |
| `wiki space-list` | `--as bot\|user\|auto`，默认 auto（User 优先、回退 Bot） |
| `wiki create/update/move/move-docs/move-to-drive`、`wiki member add/remove` | App/Bot；只有显式 `--user-access-token` / `FEISHU_USER_ACCESS_TOKEN` 才切 User |
| `wiki delete/delete-space/node-copy` | `--as bot\|user\|auto`，默认 auto；**已配置 User 但解析/刷新失败时 fail-closed 报错**，不静默降级 Bot |
| `wiki space-create` | 必须 User Token（接口不支持 Bot） |

企业知识空间常不接受应用作为成员，Bot 写操作返回无权限时改用用户身份（`--user-access-token` 或 `--as user`）。
Bot 身份 `wiki create` 后会自动给当前 CLI 登录用户授予节点容器 `full_access`（JSON 带 `permission_grant`）。

## 查询与导出

```bash
feishu-cli wiki get <node_token|obj_token|url> -o json      # space_id / node_token / obj_token / obj_type / title / has_child
feishu-cli wiki spaces                                      # 默认只取一页；has_more 时 stderr 提示 page_token
feishu-cli wiki spaces --page-all -o json                   # 自动翻页（--page-limit 默认 50 页，0 = 不限）
feishu-cli wiki space-list --page-all --as user -o json     # 输出对象 {spaces, has_more, page_token, count}
feishu-cli wiki space-get <space_id>
feishu-cli wiki nodes <space_id> [--parent <node_token>]
feishu-cli wiki nodes <space_id> --page-all -o json
feishu-cli wiki nodes <space_id> --page-token '<上次提示的 page_token>'
feishu-cli wiki member list <space_id> -o json
feishu-cli wiki export <node_token|url> --output doc.md [--download-images]
feishu-cli wiki export-tree <node_token|url> --output-dir ./backup [--max-depth 3] [--skip-existing]
```

- `wiki spaces` / `wiki nodes` 的 `-o json` 输出**数组**；默认只取一页，服务端 `has_more=true` 时只在 stderr 打印
  `has_more=true, page_token=...` 续翻提示（stdout 管道安全）。需要完整列表时加 `--page-all`（重复游标防护，达到
  `--page-limit` 上限时 stderr 告警结果不完整）。
- `wiki space-list` 是带 `--as` 的空间列表，输出对象而非数组，`--page-limit` 默认 10；`my_library`（个人知识库）不会出现在列表里。
- `wiki export` / `export-tree` 只支持 docx 与 sheet 转 Markdown；`export-tree` 跳过其他类型并计入 unsupported，
  单节点失败默认继续（`--continue-on-error=false` 立即中断）。**递归导出知识库必须用 `wiki export-tree`**，不要手写遍历脚本。

## 写操作

```bash
feishu-cli wiki create --space-id <space_id> --title "新文档" [--parent-node <node_token>] [--obj-type docx|doc|sheet]
# 快捷方式节点必须提供 --origin-node-token
feishu-cli wiki create --space-id <space_id> --title "快捷方式" --node-type shortcut --origin-node-token <origin_node_token>
feishu-cli wiki update <node_token> --title "新标题"
feishu-cli wiki move <node_token> --target-space <space_id>
feishu-cli wiki move <node_token> --target-parent <parent_node_token>   # 空间自动取父节点所在空间
feishu-cli wiki node-copy --space-id <src_space_id> --node-token <node_token> --target-space-id <dst_space_id> [--title "副本"] [--dry-run]
feishu-cli wiki space-create --name "新知识库" --description "说明" --dry-run
feishu-cli wiki member add <space_id> --member-type openid --member-id ou_xxx --role member   # --role 仅 admin/member
feishu-cli wiki member add <space_id> --member-type openid --member-id ou_xxx --role member --need-notification=false --dry-run
feishu-cli wiki member remove <space_id> --member-type openid --member-id ou_xxx --role member
```

- `wiki update` 只支持 doc/docx/快捷方式节点；其他类型节点可改用 `drive update-title --token <node_token> --type wiki`（见 drive 工作流）。
- `wiki move` 会携带子节点一起移动。传了 `--target-parent` 时先经 node_by_token 解析父节点（也接受文档 obj_token）：
  `--target-space` 与父节点实际所在空间不一致时**直接报错拒绝**（退出码 2）；只传 `--target-parent` 时自动取父节点所在空间；
  `--target-space my_library` 会先解析成真实 space_id 再比较，该别名只支持 User 身份（需显式 `--user-access-token`）。
- `wiki node-copy`：`--target-space-id` 与 `--target-parent-node-token` 二选一；`--dry-run` 不解析身份、不发请求。
- `wiki member add` 默认给新成员发送通知（`--need-notification` 默认 true）；静默添加传 `--need-notification=false`。
  `--role` 只有 admin/member，CLI 不做本地枚举校验，传错由服务端拒绝。

### 云盘 ↔ 知识库

```bash
# 云盘文档挂到知识空间（move-docs）；无权限时 --apply 提交迁入申请
feishu-cli wiki move-docs doxcnxxx --space-id <space_id> [--parent-node <node_token>] [--obj-type docx|doc|sheet|mindnote|bitable|file]
feishu-cli wiki move-docs shtcnxxx --space-id <space_id> --obj-type sheet --apply

# 知识库节点移出到云盘文件夹（move-to-drive，始终异步，默认轮询）
feishu-cli wiki move-to-drive --node-token wikcnxxx --folder-token fldcnxxx
feishu-cli wiki move-to-drive --node-token wikcnxxx --folder-token fldcnxxx --wait=false
feishu-cli drive task-result --scenario wiki_move_to_drive --task-id <task_id> --as bot
```

- `move-docs` 返回三种结果：`wiki_token`（立即完成）/ `task_id`（异步，用 `--scenario wiki_move` 续查）/ `applied=true`（已提交申请）。
  `--obj-type` 默认 docx，非 docx 必须显式指定；调用方须是源文档编辑者 + 目标空间成员，应用不是空间成员时用 User 身份。
- `move-to-drive` 的 `--node-token` 必须是 node_token，不是底层 obj_token；省略 `--folder-token` 移到调用方个人空间根目录
  （通常需用户身份，传 `--user-access-token`）。移出后原继承的知识库权限被目标云盘文件夹的权限模型替换。
  `--timeout` 默认 60 秒；超时不代表失败，按输出的 `resume_command` 续查，不要重复提交。
- scope：`wiki:node:move` 或 `wiki:wiki`；查询任务另需 `wiki:space:read`。

### 删除节点与空间

```bash
# 删除节点：URL 输入自动推断 --obj-type；裸 token 必须显式 --obj-type（wiki/docx/sheet/...）
feishu-cli wiki delete "https://xxx.feishu.cn/wiki/wikcnxxx" --yes
feishu-cli wiki delete wikcnxxx --obj-type wiki --include-children=false --yes   # 只删单节点
feishu-cli wiki delete-space <space_id> --yes [--as auto|user|bot]
```

- ⚠️ **`--include-children` 默认 true（级联删除整棵子树）**。交互确认会明说级联范围；只删单节点须显式
  `--include-children=false`。`-f/--force` 或全局 `--yes` 跳过确认时**没有任何提示**，先确认清楚范围；
  非交互环境未确认时以退出码 10 拒绝且不删除。
- `wiki delete` **无论是否传 `--space-id` 都会先 node_by_token 解析节点**：`--space-id` 与节点实际空间不一致、
  `--obj-type` 与节点实际文档类型不一致时直接拒绝（退出码 2）；快捷方式节点只能用 `--obj-type wiki` 删除
  （按文档类型删除会删到源文档）。服务端 131011（节点开启删除审批）/ 131003（子树过大）会给出对应处理提示。
- 后端可能转为异步任务，命令自动轮询；未完成或失败时非零退出并给出 task_id 与续查命令。JSON 的 `ready` / `failed`
  如实反映异步任务终态，可据此判断是否真正删成功。
- `wiki delete-space` 不带 `--yes` 以退出码 10 拒绝执行；需 `wiki:space:write_only`（Bot 需是该空间管理员）。
  轮询窗口内未完成时 JSON 含 `timed_out=true` 与 `resume_command`；状态查询全部失败时非零退出。
- 异步任务续查统一用 `drive task-result`：`wiki delete` → `--scenario wiki_delete_node`、`wiki delete-space` →
  `--scenario wiki_delete_space`、`wiki move-to-drive` → `--scenario wiki_move_to_drive`、`wiki move-docs` →
  `--scenario wiki_move`，并带上原命令的身份（`--as`）。
