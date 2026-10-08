# 基础文件与素材工作流

基础 file/media 命令适合文件 CRUD、版本和素材操作；大文件分块、resume、目录镜像和异步任务使用
`../drive/workflow.md`。

## File

```bash
feishu-cli file list [folder_token]
feishu-cli file list fldxxx --page-all -o json          # 自动翻页拉全（默认只取一页）
feishu-cli file list fldxxx --page-token '<上次提示的 page_token>'
feishu-cli file list fldxxx --as bot                    # User 缺 drive scope 时切 Bot
feishu-cli file upload ./report.pdf --parent fldxxx
feishu-cli file download <file_token> -o ./report.pdf
feishu-cli file mkdir "新文件夹" --parent fldxxx
feishu-cli file move <file_token> --target fldxxx --type file
feishu-cli file copy <file_token> --target fldxxx --type file
feishu-cli file delete <file_token> --type file --force
feishu-cli file delete <folder_token> --type folder --force -o json   # 异步删除并轮询 task_check
feishu-cli file quota                                                 # 当前用户容量（需 User Token）
feishu-cli file version list <file_token> --obj-type docx
feishu-cli file version revert <file_token> <version>
feishu-cli file meta <token> --doc-type docx
feishu-cli file stats <file_token> --doc-type docx
```

`file list` 默认只取一页（`--page-size` 默认 50、最大 200）；还有更多时在 **stderr** 提示 `has_more=true` 与
续翻用的 `page_token`（已按 shell 规则加引号），`-o json` 的 stdout 仍是文件数组。`--page-all` 自动翻页
（重复游标防护，`--page-limit` 默认 50 页、0 = 不限）。

`file delete` 以 `async=true` 提交；返回 `task_id`（如文件夹）时默认轮询 `task_check` 直到完成，
失败终态（`failed` / 删除任务的 `fail`）报错，超时输出 `drive task-result --scenario task_check` 续查命令；
`--wait=false` 只提交不轮询，`--dry-run` 预览请求。wiki 节点请用 `wiki delete`。

`file quota` 调 `GET /open-apis/drive/v2/quota_details/{user_id}`（只支持 User 身份，自动取当前登录用户
user_id，需 `drive:quota_detail:read_one`），输出配额上限（未设置上限显示"不限"）、各业务用量
（ccm/im/vc/mail/all）、租户是否超限。

`file delete` 默认交互确认（y/N，提示写 stderr）；非交互场景（Agent/管道/cron）未带 `--force`/`--yes` 时**不执行删除**并以退出码 10 失败（需要确认），获得用户同意后追加 `--yes`（或 `--force`）重新运行。交互式回答非 y 时退出码为 1（已取消）。

`file version revert` 把文件回滚到指定历史版本（底层 `POST /open-apis/drive/v1/files/{file_token}/revert`，
请求体 `{"version": version}`）：

```bash
# version 为 drive 版本历史里的长数字版本号（不是 tag）
feishu-cli file version revert boxcnXXXX 7633658129540910621
```

- 回滚后文件当前内容变为该历史版本，且会新增一条 revert 版本记录（原历史版本仍保留）
- `version` 取自文件的版本历史（`feishu-cli drive version-history --file-token <file_token>`；按版本下载用 `drive version-get`）
- 回滚是写操作，默认 Bot 身份；如需用户身份传 `--user-access-token`。需 `drive:file:upload` scope

`file download` 属于读类：已登录时优先 User Token，缺失时可回落 App Token。两种身份都是流式下载
（无 100MB 上限、分片失败有界重试并断点续传、60s 空闲超时、临时文件 + rename）；`--timeout` 只是
可选的总时长上限，默认不限。上传、移动、复制、
删除、版本回滚等写类默认 Bot 身份，只有显式传 User Token 才切换用户身份。

## Media

```bash
feishu-cli media upload image.png --parent-type docx_image --parent-node <document_id>
feishu-cli media download <file_token> --output image.png
```

`--parent-type` 必须匹配素材用途，例如 `docx_image` 或 `docx_file`。不要把消息附件的 file_key 当作
Drive file_token；消息资源应使用 messaging 的 resource-download。

删除、移动和覆盖前先确认 token、类型和目标文件夹。
