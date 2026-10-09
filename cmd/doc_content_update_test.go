package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/internal/config"
	"github.com/riba2534/feishu-cli/internal/converter"
	"github.com/spf13/viper"
)

func TestResolveMarkdownContentInlineUnescapesNewlines(t *testing.T) {
	got, err := resolveMarkdownContent("# 标题\\n\\n内容", "")
	if err != nil {
		t.Fatalf("resolveMarkdownContent() 返回错误: %v", err)
	}
	want := "# 标题\n\n内容"
	if got != want {
		t.Fatalf("resolveMarkdownContent() = %q，期望 %q", got, want)
	}
}

func TestResolveMarkdownContentFilePreservesLatexBackslash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "content.md")
	want := "$$\n\\nu + 1\n$$"
	if err := os.WriteFile(path, []byte(want), 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	got, err := resolveMarkdownContent("", path)
	if err != nil {
		t.Fatalf("resolveMarkdownContent() 返回错误: %v", err)
	}
	if got != want {
		t.Fatalf("resolveMarkdownContent() = %q，期望 %q", got, want)
	}
}

func makeHeadingBlock(id string, level int, text string) *larkdocx.Block {
	bt := int(converter.BlockTypeHeading1) + level - 1
	txt := &larkdocx.Text{
		Elements: []*larkdocx.TextElement{
			{TextRun: &larkdocx.TextRun{Content: &text}},
		},
	}
	b := &larkdocx.Block{
		BlockId:   &id,
		BlockType: &bt,
	}
	switch level {
	case 1:
		b.Heading1 = txt
	case 2:
		b.Heading2 = txt
	case 3:
		b.Heading3 = txt
	case 4:
		b.Heading4 = txt
	case 5:
		b.Heading5 = txt
	case 6:
		b.Heading6 = txt
	}
	return b
}

func makeTextBlock(id string, text string) *larkdocx.Block {
	bt := int(converter.BlockTypeText)
	return &larkdocx.Block{
		BlockId:   &id,
		BlockType: &bt,
		Text: &larkdocx.Text{
			Elements: []*larkdocx.TextElement{
				{TextRun: &larkdocx.TextRun{Content: &text}},
			},
		},
	}
}

func TestFindByTitleWithoutHashMatchesAnyHeadingLevel(t *testing.T) {
	children := []*larkdocx.Block{
		makeHeadingBlock("b0", 1, "总览"),
		makeTextBlock("b1", "这是总览介绍"),
		makeHeadingBlock("b2", 2, "架构设计"),
		makeTextBlock("b3", "架构内容描述"),
		makeHeadingBlock("b4", 3, "实现细节"),
		makeTextBlock("b5", "细节A"),
		makeHeadingBlock("b6", 2, "总结与展望"),
		makeTextBlock("b7", "结论"),
	}

	// 1. 无 # 匹配 H2 标题 "架构设计"
	r2, err := findByTitle(children, "架构设计")
	if err != nil {
		t.Fatalf("findByTitle(架构设计) 返回错误: %v", err)
	}
	if len(r2) != 1 || r2[0].startIndex != 2 || r2[0].endIndex != 6 {
		t.Fatalf("findByTitle(架构设计) = %+v, 期望 [2, 6)", r2)
	}

	// 2. 无 # 匹配 H3 标题 "实现细节"
	r3, err := findByTitle(children, "实现细节")
	if err != nil {
		t.Fatalf("findByTitle(实现细节) 返回错误: %v", err)
	}
	if len(r3) != 1 || r3[0].startIndex != 4 || r3[0].endIndex != 6 {
		t.Fatalf("findByTitle(实现细节) = %+v, 期望 [4, 6)", r3)
	}

	// 3. 无 # 匹配 H1 标题 "总览"
	r1, err := findByTitle(children, "总览")
	if err != nil {
		t.Fatalf("findByTitle(总览) 返回错误: %v", err)
	}
	if len(r1) != 1 || r1[0].startIndex != 0 || r1[0].endIndex != 8 {
		t.Fatalf("findByTitle(总览) = %+v, 期望 [0, 8)", r1)
	}

	// 4. 带 ## 仅匹配 H2
	rHash2, err := findByTitle(children, "## 架构设计")
	if err != nil {
		t.Fatalf("findByTitle(## 架构设计) 返回错误: %v", err)
	}
	if len(rHash2) != 1 || rHash2[0].startIndex != 2 || rHash2[0].endIndex != 6 {
		t.Fatalf("findByTitle(## 架构设计) = %+v, 期望 [2, 6)", rHash2)
	}

	// 5. 带 ### 匹配 H2 应该失败
	if _, err := findByTitle(children, "### 架构设计"); err == nil {
		t.Fatalf("findByTitle(### 架构设计) 应该未找到，但未报错")
	}

	// 6. 普通文本内容不应该被当作标题匹配
	if _, err := findByTitle(children, "总览介绍"); err == nil {
		t.Fatalf("findByTitle(总览介绍) 不应匹配非标题文本块")
	}
}

func TestParseTitleSelectorLevels(t *testing.T) {
	tests := []struct {
		input     string
		wantLevel int
		wantText  string
	}{
		{"## 二级标题", 2, "二级标题"},
		{"# 一级标题", 1, "一级标题"},
		{"###   三级标题  ", 3, "三级标题"},
		{"纯文本标题", 0, "纯文本标题"},
	}

	for _, tt := range tests {
		lvl, txt := parseTitleSelector(tt.input)
		if lvl != tt.wantLevel || txt != tt.wantText {
			t.Errorf("parseTitleSelector(%q) = (%d, %q), 期望 (%d, %q)",
				tt.input, lvl, txt, tt.wantLevel, tt.wantText)
		}
	}
}

func initDocUpdateTestConfig(t *testing.T, baseURL string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := fmt.Sprintf("app_id: cli_test\napp_secret: test_secret\nbase_url: %q\n", baseURL)
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("写测试配置失败: %v", err)
	}
	if err := config.Init(configPath); err != nil {
		t.Fatalf("初始化测试配置失败: %v", err)
	}
}

// TestOverwriteAtomicProtocol 验证 overwrite 走官方 PUT /docs_ai 单操作原子覆盖协议，彻底杜绝先删后写破坏窗口
func TestOverwriteAtomicProtocol(t *testing.T) {
	putCalled := false
	batchDeleteCalled := false
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.Method == "PUT" && r.URL.Path == "/open-apis/docs_ai/v1/documents/doc-123":
			putCalled = true
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document":{"revision_id":6}}}`)
		case strings.Contains(r.URL.Path, "batch_delete"):
			batchDeleteCalled = true
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok"}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	err := doOverwrite("doc-123", "# 全新内容\n\n测试", "", "", 5)
	if err != nil {
		t.Fatalf("doOverwrite 执行失败: %v", err)
	}

	if !putCalled {
		t.Fatal("未发起官方 PUT /open-apis/docs_ai/v1/documents/{id} 请求")
	}
	if batchDeleteCalled {
		t.Fatal("严禁调用 batch_delete！必须走单操作原子覆盖")
	}
	if gotBody["command"] != "overwrite" {
		t.Fatalf("command = %v, 期望 overwrite", gotBody["command"])
	}
	if gotBody["format"] != "markdown" {
		t.Fatalf("format = %v, 期望 markdown", gotBody["format"])
	}
	if gotBody["revision_id"] != float64(5) {
		t.Fatalf("revision_id = %v, 期望 5", gotBody["revision_id"])
	}
}

// TestReplaceRangeAtomicProtocol 验证 replace_range 映射 title selector 为 start_block_id/end_block_id 并发起原子 block_replace
func TestReplaceRangeAtomicProtocol(t *testing.T) {
	putCalled := false
	batchDeleteCalled := false
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/children"):
			// 返回包含 H2 标题和内容的块
			_, _ = fmt.Fprint(w, `{
				"code":0,"msg":"ok",
				"data":{
					"items":[
						{"block_id":"b_intro","block_type":2,"text":{"elements":[{"text_run":{"content":"前言"}}]}},
						{"block_id":"b_h2","block_type":4,"heading2":{"elements":[{"text_run":{"content":"目标章节"}}]}},
						{"block_id":"b_p1","block_type":2,"text":{"elements":[{"text_run":{"content":"正文段落"}}]}},
						{"block_id":"b_next_h2","block_type":4,"heading2":{"elements":[{"text_run":{"content":"下一章节"}}]}}
					],
					"has_more":false
				}
			}`)
		case r.Method == "PUT" && r.URL.Path == "/open-apis/docs_ai/v1/documents/doc-replace":
			putCalled = true
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document":{"revision_id":7}}}`)
		case strings.Contains(r.URL.Path, "batch_delete"):
			batchDeleteCalled = true
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok"}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	err := doReplaceRange("doc-replace", "## 新章节内容", "## 目标章节", "", "", "", 6)
	if err != nil {
		t.Fatalf("doReplaceRange 执行失败: %v", err)
	}

	if !putCalled {
		t.Fatal("未发起官方 PUT /open-apis/docs_ai/v1/documents/{id} 请求")
	}
	if batchDeleteCalled {
		t.Fatal("严禁调用 batch_delete！必须走单操作原子 block_replace")
	}
	if gotBody["command"] != "block_replace" {
		t.Fatalf("command = %v, 期望 block_replace", gotBody["command"])
	}
	if gotBody["start_block_id"] != "b_h2" {
		t.Fatalf("start_block_id = %v, 期望 b_h2", gotBody["start_block_id"])
	}
	if gotBody["end_block_id"] != "b_p1" {
		t.Fatalf("end_block_id = %v, 期望 b_p1", gotBody["end_block_id"])
	}
	if gotBody["revision_id"] != float64(6) {
		t.Fatalf("revision_id = %v, 期望 6", gotBody["revision_id"])
	}
}

// TestDeleteRangeAtomicProtocol 验证 delete_range 映射 block_delete 原子操作
func TestDeleteRangeAtomicProtocol(t *testing.T) {
	putCalled := false
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/children"):
			_, _ = fmt.Fprint(w, `{
				"code":0,"msg":"ok",
				"data":{
					"items":[
						{"block_id":"b_del_start","block_type":3,"heading1":{"elements":[{"text_run":{"content":"废弃章节"}}]}},
						{"block_id":"b_del_body","block_type":2,"text":{"elements":[{"text_run":{"content":"废弃正文"}}]}}
					],
					"has_more":false
				}
			}`)
		case r.Method == "PUT" && r.URL.Path == "/open-apis/docs_ai/v1/documents/doc-del":
			putCalled = true
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document":{"revision_id":8}}}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	err := doDeleteRange("doc-del", "废弃章节", "", "", "", 7)
	if err != nil {
		t.Fatalf("doDeleteRange 执行失败: %v", err)
	}

	if !putCalled {
		t.Fatal("未发起原子 block_delete PUT 请求")
	}
	if gotBody["command"] != "block_delete" {
		t.Fatalf("command = %v, 期望 block_delete", gotBody["command"])
	}
	if gotBody["start_block_id"] != "b_del_start" || gotBody["end_block_id"] != "b_del_body" {
		t.Fatalf("block_delete 范围异常: start=%v, end=%v", gotBody["start_block_id"], gotBody["end_block_id"])
	}
}

// TestReplaceAllAtomicProtocolWithPartialFailure 验证 replace_all 倒序原子替换，且部分失败时非零退出并报告已完成项
func TestReplaceAllAtomicProtocolWithPartialFailure(t *testing.T) {
	putCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/children"):
			// 返回 2 个匹配项
			_, _ = fmt.Fprint(w, `{
				"code":0,"msg":"ok",
				"data":{
					"items":[
						{"block_id":"item_1","block_type":2,"text":{"elements":[{"text_run":{"content":"待替换词项"}}]}},
						{"block_id":"item_2","block_type":2,"text":{"elements":[{"text_run":{"content":"待替换词项"}}]}}
					],
					"has_more":false
				}
			}`)
		case r.Method == "PUT" && r.URL.Path == "/open-apis/docs_ai/v1/documents/doc-rep-all":
			putCount++
			if putCount == 1 {
				// 第一次（倒序 item_2）成功
				_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document":{"revision_id":11}}}`)
			} else {
				// 第二次（item_1）模拟服务端失败
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = fmt.Fprint(w, `{"code":99991400,"msg":"rate limited"}`)
			}
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	err := doReplaceAll("doc-rep-all", "新词", "", "待替换...词项", "", "", 10)
	if err == nil {
		t.Fatal("部分失败时必须返回非零错误！")
	}

	// 验证错误信息明确指出了已成功完成项
	if !strings.Contains(err.Error(), "已成功完成 1 处") {
		t.Fatalf("错误信息必须明确报告已成功完成项，实际错误: %v", err)
	}
	if putCount != 2 {
		t.Fatalf("PUT 调用次数 = %d，期望 2", putCount)
	}
}

// TestFailClosedOnLocalResources 验证本地资源检测（--upload-images 或相对路径图片）时 fail closed 并给出迁移提示
func TestFailClosedOnLocalResources(t *testing.T) {
	// 1. --upload-images fail closed
	err1 := validateNoLocalResources(true, "普通内容")
	if err1 == nil || !strings.Contains(err1.Error(), "feishu-cli doc import") {
		t.Fatalf("upload-images=true 应 fail closed 并给出迁移提示，得到: %v", err1)
	}

	// 2. 本地图片语法 fail closed
	localMD := "一段文字\n![本地图](./assets/pic.png)\n结尾"
	err2 := validateNoLocalResources(false, localMD)
	if err2 == nil || !strings.Contains(err2.Error(), "feishu-cli doc import") {
		t.Fatalf("含本地图片应 fail closed 并给出迁移提示，得到: %v", err2)
	}

	// 3. 网络图片允许通行
	remoteMD := "一段文字\n![网络图](https://example.com/pic.png)\n结尾"
	if err := validateNoLocalResources(false, remoteMD); err != nil {
		t.Fatalf("网络图片应允许通行，但报错: %v", err)
	}
}

// TestAppendWireBodyUsesBlockInsertAfterSentinel 验证 append 模式在 wire 上正确转换为 block_insert_after + block_id="-1"
func TestAppendWireBodyUsesBlockInsertAfterSentinel(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.Method == "PUT" && r.URL.Path == "/open-apis/docs_ai/v1/documents/doc-append":
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document":{"revision_id":1}}}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	err := doAppend("doc-append", "追加的内容", "", "", -1)
	if err != nil {
		t.Fatalf("doAppend 失败: %v", err)
	}

	if gotBody["command"] != "block_insert_after" {
		t.Fatalf("command = %v, 期望 block_insert_after", gotBody["command"])
	}
	if gotBody["block_id"] != "-1" {
		t.Fatalf("block_id = %v, 期望 -1", gotBody["block_id"])
	}
	if gotBody["revision_id"] != float64(-1) {
		t.Fatalf("默认 revision_id 应当发送 -1，实际发送: %v", gotBody["revision_id"])
	}
}

// TestDestructiveModesFailWhenResultFailed 验证所有破坏性模式在服务端返回 result="failed" 时必须退出非零
func TestDestructiveModesFailWhenResultFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/children"):
			_, _ = fmt.Fprint(w, `{
				"code":0,"msg":"ok",
				"data":{
					"items":[
						{"block_id":"b1","block_type":3,"heading1":{"elements":[{"text_run":{"content":"章节1"}}]}},
						{"block_id":"b2","block_type":2,"text":{"elements":[{"text_run":{"content":"内容1"}}]}}
					],
					"has_more":false
				}
			}`)
		case r.Method == "PUT":
			// 返回 code=0 但 data.result="failed"
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"result":"failed"}}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	// 1. overwrite
	if err := doOverwrite("doc-1", "新内容", "", "", -1); err == nil {
		t.Fatal("overwrite 在 result=failed 时必须非零报错")
	}

	// 2. replace_range
	if err := doReplaceRange("doc-1", "新内容", "章节1", "", "", "", -1); err == nil {
		t.Fatal("replace_range 在 result=failed 时必须非零报错")
	}

	// 3. delete_range
	if err := doDeleteRange("doc-1", "章节1", "", "", "", -1); err == nil {
		t.Fatal("delete_range 在 result=failed 时必须非零报错")
	}

	// 4. replace_all
	if err := doReplaceAll("doc-1", "新内容", "章节1", "", "", "", -1); err == nil {
		t.Fatal("replace_all 在 result=failed 时必须非零报错")
	}
}

// TestDestructiveModesFailWhenEmptyData 验证所有破坏性模式在服务端返回空 data 时必须退出非零
func TestDestructiveModesFailWhenEmptyData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/children"):
			_, _ = fmt.Fprint(w, `{
				"code":0,"msg":"ok",
				"data":{
					"items":[
						{"block_id":"b1","block_type":3,"heading1":{"elements":[{"text_run":{"content":"章节1"}}]}}
					],
					"has_more":false
				}
			}`)
		case r.Method == "PUT":
			// 返回 code=0 但 data 为 null/空
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":null}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	if err := doOverwrite("doc-1", "新内容", "", "", -1); err == nil {
		t.Fatal("overwrite 在 data 为空时必须非零报错")
	}
	if err := doReplaceRange("doc-1", "新内容", "章节1", "", "", "", -1); err == nil {
		t.Fatal("replace_range 在 data 为空时必须非零报错")
	}
	if err := doDeleteRange("doc-1", "章节1", "", "", "", -1); err == nil {
		t.Fatal("delete_range 在 data 为空时必须非零报错")
	}
	if err := doReplaceAll("doc-1", "新内容", "章节1", "", "", "", -1); err == nil {
		t.Fatal("replace_all 在 data 为空时必须非零报错")
	}
}

// TestNegativeRevisionIDRejected 验证负数非法 revision-id（<-1）被校验拒绝
func TestNegativeRevisionIDRejected(t *testing.T) {
	initDocUpdateTestConfig(t, "http://127.0.0.1:9999")
	_ = docContentUpdateCmd.Flags().Set("mode", "overwrite")
	_ = docContentUpdateCmd.Flags().Set("markdown", "test")
	_ = docContentUpdateCmd.Flags().Set("revision-id", "-2")
	defer func() {
		_ = docContentUpdateCmd.Flags().Set("mode", "")
		_ = docContentUpdateCmd.Flags().Set("markdown", "")
		_ = docContentUpdateCmd.Flags().Set("revision-id", "-1")
	}()
	err := docContentUpdateCmd.RunE(docContentUpdateCmd, []string{"doc-1"})
	if err == nil {
		t.Fatal("--revision-id -2 必须被拒绝报错")
	}
	if !strings.Contains(err.Error(), "--revision-id 必须 >= -1") {
		t.Fatalf("错误信息应说明 >= -1，实际得到: %v", err)
	}
}

// TestTableColumnWidthCustomFailsClosed 验证自定义 --table-column-width 时 fail closed 拒绝并提供迁移提示
func TestTableColumnWidthCustomFailsClosed(t *testing.T) {
	initDocUpdateTestConfig(t, "http://127.0.0.1:9999")
	_ = docContentUpdateCmd.Flags().Set("mode", "overwrite")
	_ = docContentUpdateCmd.Flags().Set("markdown", "test")
	_ = docContentUpdateCmd.Flags().Set("table-column-width", "100,200")
	defer func() {
		_ = docContentUpdateCmd.Flags().Set("mode", "")
		_ = docContentUpdateCmd.Flags().Set("markdown", "")
		_ = docContentUpdateCmd.Flags().Set("table-column-width", "auto")
	}()
	err := docContentUpdateCmd.RunE(docContentUpdateCmd, []string{"doc-1"})
	if err == nil {
		t.Fatal("自定义 --table-column-width 必须 fail closed 报错")
	}
	if !strings.Contains(err.Error(), "feishu-cli doc import") {
		t.Fatalf("错误信息必须包含迁移提示 doc import，实际得到: %v", err)
	}
}

// TestEllipsisSelectorRejectsEmptyEndpoints 验证省略号定位端点为空或仅为省略号时拒绝报错
func TestEllipsisSelectorRejectsEmptyEndpoints(t *testing.T) {
	tests := []string{
		"...",
		"...结尾",
		"开头...",
		"   ...   ",
	}

	for _, tt := range tests {
		err := validateContentUpdateParams("replace_range", "新内容", "", tt)
		if err == nil {
			t.Errorf("端点为空或单省略号 %q 必须报错拒绝", tt)
		}
	}

	// 正常端点允许通过
	if err := validateContentUpdateParams("replace_range", "新内容", "", "开头...结尾"); err != nil {
		t.Fatalf("合法端点应当通过: %v", err)
	}
}

// TestReplaceAllAbortsWhenNextRevisionMissing 验证多步 replace_all 如果未返回新的 revision_id 则非零退出并停止后续替换
func TestReplaceAllAbortsWhenNextRevisionMissing(t *testing.T) {
	putCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/children"):
			_, _ = fmt.Fprint(w, `{
				"code":0,"msg":"ok",
				"data":{
					"items":[
						{"block_id":"item_1","block_type":2,"text":{"elements":[{"text_run":{"content":"待替换内容"}}]}},
						{"block_id":"item_2","block_type":2,"text":{"elements":[{"text_run":{"content":"待替换内容"}}]}}
					],
					"has_more":false
				}
			}`)
		case r.Method == "PUT":
			putCount++
			// 成功，但 data 缺少 revision_id
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document":{}}}`)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	err := doReplaceAll("doc-rep", "新内容", "", "待替换...内容", "", "", 5)
	if err == nil {
		t.Fatal("缺少新的 revision_id 时必须非零中止，绝不能未受保护继续执行")
	}
	if !strings.Contains(err.Error(), "未返回新的 revision_id") {
		t.Fatalf("错误应说明未返回新的 revision_id，实际得到: %v", err)
	}
	if putCount != 1 {
		t.Fatalf("PUT 应当在第 1 处后中止，实际调用了 %d 次", putCount)
	}
}

// TestRevisionIDWireCondition 验证 revision-id 在 wire 上的条件：默认 -1 发送，正整数发送，显式 0 省略
func TestRevisionIDWireCondition(t *testing.T) {
	// 1. 默认 -1 发送
	bodyMinusOne := map[string]any{"command": "overwrite"}
	injectRevisionID(bodyMinusOne, -1)
	if bodyMinusOne["revision_id"] != -1 {
		t.Fatalf("revisionID=-1 应当发送 -1，实际为: %v", bodyMinusOne["revision_id"])
	}

	// 2. 显式 0 省略
	bodyZero := map[string]any{"command": "overwrite"}
	injectRevisionID(bodyZero, 0)
	if _, ok := bodyZero["revision_id"]; ok {
		t.Fatalf("revisionID=0 必须在 wire 上省略，实际却存在: %v", bodyZero["revision_id"])
	}

	// 3. 正整数发送
	bodyPositive := map[string]any{"command": "overwrite"}
	injectRevisionID(bodyPositive, 42)
	if bodyPositive["revision_id"] != 42 {
		t.Fatalf("revisionID=42 应当发送 42，实际为: %v", bodyPositive["revision_id"])
	}
}

// TestReplaceAllRevisionExtractionFallback 验证 document 对象存在但缺 revision_id 时，能独立从顶层成功回退提取
func TestReplaceAllRevisionExtractionFallback(t *testing.T) {
	putCount := 0
	var receivedRevisions []any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/children"):
			_, _ = fmt.Fprint(w, `{
				"code":0,"msg":"ok",
				"data":{
					"items":[
						{"block_id":"item_1","block_type":2,"text":{"elements":[{"text_run":{"content":"测试词"}}]}},
						{"block_id":"item_2","block_type":2,"text":{"elements":[{"text_run":{"content":"测试词"}}]}}
					],
					"has_more":false
				}
			}`)
		case r.Method == "PUT":
			putCount++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			receivedRevisions = append(receivedRevisions, body["revision_id"])

			if putCount == 1 {
				// 关键 fixture：document 对象存在但没有 revision_id，而顶层包含 revision_id=12！
				_, _ = fmt.Fprint(w, `{
					"code": 0,
					"msg": "ok",
					"data": {
						"document": {},
						"revision_id": 12
					}
				}`)
			} else {
				// 第二次成功
				_, _ = fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"document":{"revision_id":13}}}`)
			}
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	err := doReplaceAll("doc-rep-fallback", "新词", "", "测试...词", "", "", 10)
	if err != nil {
		t.Fatalf("即使 document 对象缺少 revision_id，也应独立从顶层 revision_id 提取成功，但报错: %v", err)
	}

	if putCount != 2 {
		t.Fatalf("全流程应当完成 2 处替换，实际调用了 %d 次", putCount)
	}
	// 验证第二步使用的 revision 正确继承了顶层返回的 12
	if len(receivedRevisions) != 2 || receivedRevisions[1] != float64(12) {
		t.Fatalf("第二步使用的 revision 应当为 12，实际为: %v", receivedRevisions)
	}
}

// TestDocContentUpdateInvalidOutputZeroNetwork 验证非法 --output 在任何网络请求前 fail closed
func TestDocContentUpdateInvalidOutputZeroNetwork(t *testing.T) {
	initDocUpdateTestConfig(t, "http://127.0.0.1:59997")

	_ = docContentUpdateCmd.Flags().Set("mode", "overwrite")
	_ = docContentUpdateCmd.Flags().Set("markdown", "test")
	_ = docContentUpdateCmd.Flags().Set("output", "yaml")
	defer func() {
		_ = docContentUpdateCmd.Flags().Set("mode", "")
		_ = docContentUpdateCmd.Flags().Set("markdown", "")
		_ = docContentUpdateCmd.Flags().Set("output", "")
	}()

	err := docContentUpdateCmd.RunE(docContentUpdateCmd, []string{"doc-123"})
	if err == nil {
		t.Fatal("非法 --output yaml 必须立即报错")
	}
	if !strings.Contains(err.Error(), "不支持的 --output") {
		t.Fatalf("错误信息应说明不支持的 output，得到: %v", err)
	}
}

// TestValidateNoColumnWidthDirective 验证列宽指令的两条入口都 fail-closed。
// 回归防护：曾只拦 --table-column-width flag，内容里的
// `<!-- feishu-colwidth: ... -->`（CLAUDE.md 记载的等价入口）被静默丢弃，
// 表格以默认列宽落地且无任何提示。
func TestValidateNoColumnWidthDirective(t *testing.T) {
	tableMD := "| A | B |\n|---|---|\n| 1 | 2 |\n"

	// flag 入口
	if err := validateNoColumnWidthDirective(true, "100,200", tableMD); err == nil {
		t.Error("显式 --table-column-width 应报错")
	}
	if err := validateNoColumnWidthDirective(true, "auto", tableMD); err != nil {
		t.Errorf("--table-column-width=auto 应放行: %v", err)
	}
	if err := validateNoColumnWidthDirective(false, "auto", tableMD); err != nil {
		t.Errorf("未传 flag 且无注释应放行: %v", err)
	}

	// 内容注释入口
	withComment := []string{
		"<!-- feishu-colwidth: 80,200,*,30% -->\n\n" + tableMD,
		tableMD + "\n<!-- feishu-colwidth: 100, 100 -->\n",
		"  <!--  feishu-colwidth : 80 -->  \n" + tableMD,
	}
	for i, md := range withComment {
		if err := validateNoColumnWidthDirective(false, "auto", md); err == nil {
			t.Errorf("用例 %d: 含 feishu-colwidth 注释应报错", i)
		}
	}

	// 不应误判：行内提及但非指令注释
	notDirective := []string{
		"正文提到 feishu-colwidth 这个名字但不是注释\n" + tableMD,
		"`<!-- feishu-colwidth: 80 -->` 出现在行内代码里\n" + tableMD,
	}
	for i, md := range notDirective {
		if err := validateNoColumnWidthDirective(false, "auto", md); err != nil {
			t.Errorf("用例 %d: 非指令形态不应报错: %v", i, err)
		}
	}
}

// TestFindByTitle_RejectsAmbiguousNestedMatch 验证模糊标题选择器同时命中父子标题时
// fail-closed，而不是静默替换父范围。
//
// 回归防护：不带级别的 "部署" 会同时命中 H1「部署总览」[0,6) 与 H2「部署检查」[2,4)。
// doReplaceAll 逆序 block_replace 时外层范围吞掉内层与其后未匹配的兄弟章节——
// 实测 7 块文档一次替换后只剩 2 块，无关的「其它章节」被销毁，
// 而命令报告"成功替换 2 处"且 exit 0。
func TestFindByTitle_RejectsAmbiguousNestedMatch(t *testing.T) {
	children := []*larkdocx.Block{
		makeHeadingBlock("h1", 1, "部署总览"),
		makeTextBlock("t1", "总览正文。"),
		makeHeadingBlock("h2a", 2, "部署检查"),
		makeTextBlock("t2", "检查正文。"),
		makeHeadingBlock("h2b", 2, "其它章节"),
		makeTextBlock("t3", "其它正文。"),
	}

	// 模糊选择器命中 H1 + H2（父含子）→ 必须报错
	_, err := findByTitle(children, "部署")
	if err == nil {
		t.Fatal("模糊选择器同时命中父子标题时应 fail-closed")
	}
	for _, want := range []string{"歧义", "部署"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误信息应包含 %q，得到: %v", want, err)
		}
	}

	// 带级别的精确选择器只命中一个范围 → 正常返回
	got, err := findByTitle(children, "## 部署检查")
	if err != nil {
		t.Fatalf("精确选择器应正常工作: %v", err)
	}
	if len(got) != 1 || got[0].startIndex != 2 || got[0].endIndex != 4 {
		t.Errorf("精确选择器范围 = %+v，want [{2 4}]", got)
	}

	// 同级多处匹配（互不包含）→ 正常返回多个范围
	siblings := []*larkdocx.Block{
		makeHeadingBlock("sa", 2, "部署 A"),
		makeTextBlock("ta", "a"),
		makeHeadingBlock("sb", 2, "部署 B"),
		makeTextBlock("tb", "b"),
	}
	got, err = findByTitle(siblings, "## 部署")
	if err != nil {
		t.Fatalf("同级多处匹配应正常工作: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("同级匹配应返回 2 个范围，得到 %+v", got)
	}
}

// TestFirstNestedPair 单测嵌套检测本身
func TestFirstNestedPair(t *testing.T) {
	if _, _, ok := firstNestedPair([]blockRange{{0, 6}, {2, 4}}); !ok {
		t.Error("父含子应判为嵌套")
	}
	if _, _, ok := firstNestedPair([]blockRange{{0, 2}, {2, 4}}); ok {
		t.Error("相邻不重叠不应判为嵌套")
	}
	if _, _, ok := firstNestedPair([]blockRange{{0, 4}}); ok {
		t.Error("单个范围不应判为嵌套")
	}
	if _, _, ok := firstNestedPair(nil); ok {
		t.Error("空输入不应判为嵌套")
	}
}
