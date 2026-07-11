# 评论工作流

支持 doc/docx/sheet/bitable 评论、回复和解决状态。

```bash
feishu-cli comment list <file_token> --type docx
feishu-cli comment add <file_token> --type docx --text "评论内容"
feishu-cli comment resolve <file_token> <comment_id> --type docx
feishu-cli comment unresolve <file_token> <comment_id> --type docx
feishu-cli comment reply list <file_token> <comment_id> --type docx
feishu-cli comment reply add <file_token> <comment_id> --type docx --text "回复内容"
feishu-cli comment reply delete <file_token> <comment_id> <reply_id> --type docx
```

飞书不支持直接删除整条评论；`comment delete` 会提示改用删除回复或 resolve。

默认使用 App Token。个人文档若返回 `1069302/1069303`，先登录并显式传 User Token；回复删除时
使用与回复作者一致的身份。需要 wiki URL 自动解析、局部评论或富文本元素时读取
`../drive/workflow.md` 的 `drive add-comment`。
