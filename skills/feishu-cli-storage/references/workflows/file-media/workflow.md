# 基础文件与素材工作流

基础 file/media 命令适合文件 CRUD、版本和素材操作；大文件分块、resume、目录镜像和异步任务使用
`../drive/workflow.md`。

## File

```bash
feishu-cli file list [folder_token]
feishu-cli file upload ./report.pdf --parent fldxxx
feishu-cli file download <file_token> -o ./report.pdf
feishu-cli file mkdir "新文件夹" --parent fldxxx
feishu-cli file move <file_token> --target fldxxx --type file
feishu-cli file copy <file_token> --target fldxxx --type file
feishu-cli file delete <file_token> --type file
feishu-cli file version list <file_token> --obj-type docx
feishu-cli file version revert <file_token> <version>
feishu-cli file meta <token> --doc-type docx
feishu-cli file stats <file_token> --doc-type docx
```

`file version revert` 把文件回滚到指定历史版本（底层 `POST /open-apis/drive/v1/files/{file_token}/revert`，
请求体 `{"version": version}`）：

```bash
# version 为 drive 版本历史里的长数字版本号（不是 tag）
feishu-cli file version revert boxcnXXXX 7633658129540910621
```

- 回滚后文件当前内容变为该历史版本，且会新增一条 revert 版本记录（原历史版本仍保留）
- `version` 取自文件的版本历史（可用 `feishu-cli api GET /open-apis/drive/v1/files/{file_token}/history` 查询 `data.items[].version`）
- 回滚是写操作，默认 Bot 身份；如需用户身份传 `--user-access-token`。需 `drive:file:upload` scope

`file download` 属于读类：已登录时优先 User Token，缺失时可回落 App Token。上传、移动、复制、
删除、版本回滚等写类默认 Bot 身份，只有显式传 User Token 才切换用户身份。

## Media

```bash
feishu-cli media upload image.png --parent-type docx_image --parent-node <document_id>
feishu-cli media download <file_token> --output image.png
```

`--parent-type` 必须匹配素材用途，例如 `docx_image` 或 `docx_file`。不要把消息附件的 file_key 当作
Drive file_token；消息资源应使用 messaging 的 resource-download。

删除、移动和覆盖前先确认 token、类型和目标文件夹。
