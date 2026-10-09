# 基础文件与素材工作流

基础 file/media 命令适合文件夹浏览、文件 CRUD、元数据、容量、版本和文档素材操作；大文件分块、URL/wiki 解包、
原地覆盖、上传文件版本历史、异步导入导出和目录镜像使用 `../drive/workflow.md`。wiki 节点用 `../wiki/workflow.md`。

## 身份

- 读类（`file list/download/meta/stats`、`file version list/get`）：User 优先，不可用时 stderr 告警后回退 App/Bot。
  `file list` 另有 `--as bot|user|auto`，不传保持此默认。
- 写类（`file upload/mkdir/move/copy/shortcut/delete`、`file version create/delete/revert`）：默认 App/Bot，
  只有显式 `--user-access-token`（或 `FEISHU_USER_ACCESS_TOKEN`）才以用户身份执行；这些命令没有 `--as`。
- `file quota` 必须 User Token（无 `--as`）；`media upload/download` 只用 App 身份。
- Bot 身份 `file upload/mkdir/copy` 新建资源后，自动给当前 CLI 登录用户授予 `full_access`（JSON 带 `permission_grant`，
  见 `../perm/workflow.md`）。
- Bot 对用户自有文件（用户上传、用户创建的文档）通常没有管理权限：移动、删除、回滚这类文件要传用户身份。

## File

```bash
feishu-cli file list [folder_token]
feishu-cli file list fldxxx --page-all -o json          # 自动翻页拉全（默认只取一页）
feishu-cli file list fldxxx --page-token '<上次提示的 page_token>'
feishu-cli file list fldxxx --as bot                    # User 缺 drive 读 scope（99991679）时切 Bot
feishu-cli file upload ./report.pdf --parent fldxxx
feishu-cli file download <file_token> -o ./report.pdf
feishu-cli file mkdir "新文件夹" --parent fldxxx -o json
feishu-cli file move <file_token> --target fldxxx --type file
feishu-cli file copy <file_token> --target fldxxx --type docx --name "副本"
feishu-cli file shortcut <file_token> --target fldxxx --type docx       # 在目标文件夹创建快捷方式（不复制内容）
feishu-cli file delete <file_token> --type file --yes
feishu-cli file delete <folder_token> --type folder --yes -o json      # 异步删除并轮询 task_check
feishu-cli file delete <folder_token> --type folder --dry-run           # 预览请求，不确认、不删除
feishu-cli file quota -o json                                           # 当前登录用户容量（必须 User Token）
feishu-cli file meta <token> [token...] --doc-type docx
feishu-cli file stats <file_token> --doc-type docx
```

### 列表分页

`file list` 默认只取一页（`--page-size` 默认 50、最大 200）；还有更多时在 **stderr** 提示 `has_more=true` 与续翻用的
`page_token`（已按 shell 规则加引号），`-o json` 的 stdout 仍是文件数组（每项 `token/name/type/parent_token/url/
created_time/modified_time/owner_id`）。`--page-all` 自动翻页（重复游标防护，`--page-limit` 默认 50 页、0 = 不限）。
不传 folder_token 时列根目录。

### 删除

- `file delete` 把文件移入回收站，以 `async=true` 提交：返回 `task_id`（实测普通文件也会返回）时默认轮询 `task_check` 直到完成；
  失败终态（`failed` / 删除任务的 `fail`）报错，超时输出 `drive task-result --scenario task_check --task-id ... --as bot|user`
  续查命令。`--wait=false` 只提交不轮询；`--dry-run` 只打印请求。wiki 节点不能用 `file delete`，改用 `wiki delete`。
- 确认门禁：交互终端默认 y/N 确认（提示写 stderr，回答非 y 时退出码 1）；非交互场景（Agent/管道/cron）未带
  `--yes`/`--force` 时**不执行删除**并以退出码 10 失败。获得用户同意后追加 `--yes`（或 `-f/--force`）重新运行。
- 异步删除失败时服务端不返回原因：以 Bot 删除用户自有文件是最常见原因（实测 task 状态为 `fail`，CLI 会提示），
  此时用用户身份重试：`feishu-cli file delete <file_token> --type file --yes --user-access-token <User Token>`，
  或先 `feishu-cli perm list <token> --doc-type <type> --as user` 确认应用是否有管理权限。

### 容量

`file quota` 调 `GET /open-apis/drive/v2/quota_details/{user_id}`，自动取当前登录用户的 user_id，需
`drive:quota_detail:read_one`。JSON 输出 `total`/`used`/`unlimited`（未设置上限时为 true，文本模式显示"不限"）、
`biz_infos[]`（ccm/im/vc/mail/all 各业务用量）、`is_tenant_quota_exceeded`、`user_quota`，有部门配额时一并输出。

### 版本

两套版本不要混用：

| 命令 | 对象 | 版本号来源 |
|---|---|---|
| `file version list/create/get/delete --obj-type doc\|docx\|sheet\|bitable` | 在线文档的命名版本（`--obj-type` 默认 docx） | `file version list` |
| `file version revert <file_token> <version>` | 上传文件（type=file）的历史版本 | `drive version-history` 输出的 `version`（长数字，不是 tag） |

```bash
feishu-cli drive version-history --file-token boxcnxxx -o json
feishu-cli file version revert boxcnxxx 7633658129540910621
# 在线文档命名版本（--obj-type 默认 docx；version_id 取自 list/create 输出）
feishu-cli file version list doxcnxxx --obj-type docx -o json
feishu-cli file version create doxcnxxx --obj-type docx --name "v1.0"
feishu-cli file version get doxcnxxx <version_id> --obj-type docx -o json
feishu-cli file version delete doxcnxxx <version_id> --obj-type docx
```

- `file version revert` 底层 `POST /open-apis/drive/v1/files/{file_token}/revert`（请求体 `{"version": ...}`），需
  `drive:file:upload`；回滚后当前内容变为该版本，并新增一条 `action_type=revert` 的版本记录，原历史版本保留（实测）。
- 按版本下载用 `drive version-get`（见 drive 工作流）。
- `file version create/delete` 是写类（默认 Bot，显式 `--user-access-token` 才切 User），`list/get` 是读类（User 优先）；
  `create` 的 `--name` 必填；version_id 是短字符串（如 `WLLqZN`），不是上传文件的长数字 version。scope：
  `drive:drive:version`（读可用 `drive:drive:version:readonly`），User Token 未授权时返回 99991679。
- ⚠️ 当前版本 `file version create` 会在服务端**已创建版本后**报 `cannot unmarshal number ... status` 并以退出码 1 结束（实测）。
  不要直接重试（会建出重复版本），先核对：
  `feishu-cli api GET /open-apis/drive/v1/files/<token>/versions --params '{"obj_type":"docx","page_size":20}' --as bot`。

### 下载

`file download` 是读类：已登录时优先 User Token，不可用时回落 App Token。两种身份都是流式下载（无 100MB 上限、
分片失败有界重试并断点续传、60 秒空闲超时、临时文件 + rename）；`--timeout` 只是可选的总时长上限，默认不限。
`-o` 省略时保存到当前目录。需要 URL 输入、wiki 解包或目录默认文件名时用 `drive download`。

## Media

```bash
feishu-cli media upload image.png --parent-type docx_image --parent-node <document_id>
feishu-cli media upload report.pdf --parent-type docx_file --parent-node <document_id> -o json
# 上传到文档块下时携带 --doc-id：写入 extra={"drive_route_token":...}，素材按该文档路由鉴权
feishu-cli media upload image.png --parent-type docx_image --parent-node <block_id> --doc-id <document_id>
feishu-cli media download <file_token> --output image.png
feishu-cli media download <file_token> -o large.bin --timeout 30m      # 默认超时 5m
```

- `--parent-type` 必须匹配素材用途：`docx_image`（默认）/ `docx_file` / `doc_image` / `doc_file`；`--parent-node` 必填（文档 ID）。
- `--doc-id` 可选：裸文档 ID 原样使用，`/docx/` URL 取 token，`/wiki/` URL 以 App 身份解析为底层 docx；不传时不带 extra（旧行为）。
  上传素材只得到 file_token，不会出现在文档里；要把图片/附件插入文档正文用 `feishu-cli-docs` 的 `doc media-insert`。
- 不要把消息附件的 file_key 当作 Drive file_token（两者不可互换）；消息资源用 `feishu-cli-messaging` 的 `msg resource-download`。

删除、移动和覆盖前先确认 token、类型和目标文件夹。
