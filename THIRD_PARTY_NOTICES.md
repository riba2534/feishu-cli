# 第三方声明（Third-Party Notices）

本项目部分代码与技能文档改编自以下开源项目，按其许可证要求保留版权与许可声明。

## larksuite/cli

- 来源：https://github.com/larksuite/cli
- 许可证：MIT License
- 改编范围（文件头注明"改编自 larksuite/cli"）：
  - `internal/docxparse/`：DocxXML 解析、画像与字数统计
  - `internal/immarkdown/`：docs_ai Markdown 转 IM Markdown
  - `internal/clipboard/`：剪贴板图片读取
  - `internal/client/doc_cover_url.go`：封面图片 URL 受控下载
  - `cmd/doc_script*.go`、`cmd/doc_write_resources.go`、`cmd/doc_content_local_resources.go`、`cmd/doc_write_dryrun.go`
  - `skills/feishu-cli-docs/references/workflows/author/`：写作工作流、DocxXML 写作规范与文体模板

```text
MIT License

Copyright (c) 2026 Lark Technologies Pte. Ltd.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```
