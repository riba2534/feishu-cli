package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/v2/internal/converter"
)

// docxMock 模拟导入涉及的 docx / drive / board 接口，并按 2026-10 实测的服务端约束拒绝非法建块：
//   - 带 token 的 Image/File/Board/Sheet/Bitable、带 name 的 File → 1770001
//   - 在 Grid 下显式创建子块 → 1770028；QuoteContainer/Callout 作为 QuoteContainer 子块 → 1770030
//   - 单次请求超过 5 个画板块 → 1770035 resource count exceed limit
//
// Grid 建块时服务端自动生成两列（响应 children），File 建块返回外层 View 块（children[0] 为 File 块）。
type docxMock struct {
	t           *testing.T
	mu          sync.Mutex
	next        int
	types       map[string]int // block_id → block_type
	created     map[string][]map[string]any
	fatal       bool // 所有建块请求返回 1770002 not found
	patches     map[string]map[string]any
	uploads     []map[string]string
	diagrams    []string
	maxBoards   int // 单次建块请求中出现过的最多画板块数
	over1770035 int // 因画板超限被拒的请求数
}

func newDocxMock(t *testing.T) *docxMock {
	return &docxMock{t: t, types: map[string]int{}, created: map[string][]map[string]any{}, patches: map[string]map[string]any{}}
}

func (m *docxMock) newID(prefix string) string {
	m.next++
	return fmt.Sprintf("%s_%d", prefix, m.next)
}

func mockWriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// rejectReason 返回该子块按服务端约束会被拒绝的错误码（0 表示接受）。
func (m *docxMock) rejectReason(parentType int, c map[string]any) int {
	bt := int(c["block_type"].(float64))
	switch parentType {
	case int(converter.BlockTypeGrid):
		return 1770028
	case int(converter.BlockTypeQuoteContainer):
		if bt == int(converter.BlockTypeQuoteContainer) || bt == int(converter.BlockTypeCallout) {
			return 1770030
		}
	}
	nonEmpty := func(section string, keys ...string) bool {
		sec, _ := c[section].(map[string]any)
		for _, k := range keys {
			if v, ok := sec[k].(string); ok && v != "" {
				return true
			}
		}
		return false
	}
	switch {
	case bt == int(converter.BlockTypeImage) && nonEmpty("image", "token"),
		bt == int(converter.BlockTypeBoard) && nonEmpty("board", "token"),
		bt == int(converter.BlockTypeFile) && nonEmpty("file", "token", "name"),
		bt == int(converter.BlockTypeSheet) && nonEmpty("sheet", "token"),
		bt == int(converter.BlockTypeBitable) && nonEmpty("bitable", "token"):
		return 1770001
	}
	if bt == int(converter.BlockTypeSheet) {
		if sec, _ := c["sheet"].(map[string]any); sec != nil {
			if rows, _ := sec["row_size"].(float64); rows <= 0 {
				return 99992402
			}
		}
	}
	return 0
}

func (m *docxMock) handler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/open-apis/auth/v3/tenant_access_token/internal":
		mockWriteJSON(w, 200, map[string]any{"code": 0, "msg": "ok", "tenant_access_token": "t-test", "expire": 7200})

	case r.Method == http.MethodPost && strings.HasSuffix(path, "/children") && strings.Contains(path, "/docx/v1/documents/"):
		parts := strings.Split(path, "/")
		parent := parts[len(parts)-2]
		var req struct {
			Children []map[string]any `json:"children"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.fatal {
			mockWriteJSON(w, 404, map[string]any{"code": 1770002, "msg": "not found"})
			return
		}
		for _, c := range req.Children {
			if code := m.rejectReason(m.types[parent], c); code != 0 {
				mockWriteJSON(w, 400, map[string]any{"code": code, "msg": "rejected"})
				return
			}
		}
		boards := 0
		for _, c := range req.Children {
			if bt, _ := c["block_type"].(float64); int(bt) == int(converter.BlockTypeBoard) {
				boards++
			}
		}
		if boards > 5 {
			m.over1770035++
			mockWriteJSON(w, 400, map[string]any{"code": 1770035, "msg": "resource count exceed limit"})
			return
		}
		m.maxBoards = max(m.maxBoards, boards)
		out := make([]map[string]any, 0, len(req.Children))
		for _, c := range req.Children {
			bt := int(c["block_type"].(float64))
			id := m.newID("blk")
			m.types[id] = bt
			resp := map[string]any{}
			for k, v := range c {
				resp[k] = v
			}
			resp["block_id"] = id
			switch bt {
			case int(converter.BlockTypeGrid):
				cols := []string{id + "_c1", id + "_c2"}
				if g, _ := c["grid"].(map[string]any); g != nil {
					if n, _ := g["column_size"].(float64); n == 3 {
						cols = append(cols, id+"_c3")
					}
				}
				for _, col := range cols {
					m.types[col] = int(converter.BlockTypeGridColumn)
				}
				resp["children"] = cols
			case int(converter.BlockTypeFile):
				fileID := id + "_file"
				m.types[id] = int(converter.BlockTypeView)
				m.types[fileID] = int(converter.BlockTypeFile)
				resp = map[string]any{"block_id": id, "block_type": int(converter.BlockTypeView), "children": []string{fileID}}
			case int(converter.BlockTypeBoard):
				resp["board"] = map[string]any{"token": "wb_" + id}
			}
			out = append(out, resp)
		}
		index := -1
		if v, ok := reqIndex(body); ok {
			index = v
		}
		if index < 0 || index > len(m.created[parent]) {
			m.created[parent] = append(m.created[parent], out...)
		} else {
			rest := append([]map[string]any(nil), m.created[parent][index:]...)
			m.created[parent] = append(append(m.created[parent][:index], out...), rest...)
		}
		mockWriteJSON(w, 200, map[string]any{"code": 0, "msg": "ok", "data": map[string]any{"children": out}})

	case r.Method == http.MethodGet && strings.HasSuffix(path, "/children"):
		parts := strings.Split(path, "/")
		m.mu.Lock()
		items := append([]map[string]any(nil), m.created[parts[len(parts)-2]]...)
		m.mu.Unlock()
		mockWriteJSON(w, 200, map[string]any{"code": 0, "msg": "ok", "data": map[string]any{"items": items, "has_more": false}})

	case r.Method == http.MethodDelete && strings.HasSuffix(path, "/children/batch_delete"):
		parts := strings.Split(path, "/")
		parent := parts[len(parts)-3]
		var req struct {
			StartIndex int `json:"start_index"`
			EndIndex   int `json:"end_index"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		m.mu.Lock()
		kids := m.created[parent]
		if req.StartIndex >= 0 && req.EndIndex <= len(kids) && req.StartIndex < req.EndIndex {
			m.created[parent] = append(append([]map[string]any(nil), kids[:req.StartIndex]...), kids[req.EndIndex:]...)
		}
		m.mu.Unlock()
		mockWriteJSON(w, 200, map[string]any{"code": 0, "msg": "ok", "data": map[string]any{}})

	case r.Method == http.MethodPatch && strings.Contains(path, "/docx/v1/documents/"):
		parts := strings.Split(path, "/")
		var req map[string]any
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		m.mu.Lock()
		m.patches[parts[len(parts)-1]] = req
		m.mu.Unlock()
		mockWriteJSON(w, 200, map[string]any{"code": 0, "msg": "ok", "data": map[string]any{}})

	case strings.HasSuffix(path, "/medias/batch_get_tmp_download_url"):
		mockWriteJSON(w, 200, map[string]any{"code": 0, "msg": "ok", "data": map[string]any{"tmp_download_urls": []any{}}})

	case strings.HasPrefix(path, "/open-apis/drive/v1/medias/") && strings.HasSuffix(path, "/download"):
		token := strings.TrimSuffix(strings.TrimPrefix(path, "/open-apis/drive/v1/medias/"), "/download")
		if strings.HasPrefix(token, "missing") {
			mockWriteJSON(w, 404, map[string]any{"code": 1061004, "msg": "not found"})
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Disposition", `attachment; filename="src.png"`)
		_, _ = w.Write(tinyPNG())

	case strings.HasSuffix(path, "/medias/upload_all"):
		mediaType, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		fields := map[string]string{}
		if strings.HasPrefix(mediaType, "multipart/") {
			mr := multipart.NewReader(r.Body, params["boundary"])
			for {
				part, err := mr.NextPart()
				if err != nil {
					break
				}
				if part.FormName() != "file" {
					b, _ := io.ReadAll(part)
					fields[part.FormName()] = string(b)
				}
			}
		}
		m.mu.Lock()
		m.uploads = append(m.uploads, fields)
		n := len(m.uploads)
		m.mu.Unlock()
		mockWriteJSON(w, 200, map[string]any{"code": 0, "msg": "ok", "data": map[string]any{"file_token": fmt.Sprintf("newtok_%d", n)}})

	case strings.HasSuffix(path, "/nodes") && strings.Contains(path, "/board/v1/whiteboards/"):
		mockWriteJSON(w, 200, map[string]any{"code": 0, "msg": "", "data": map[string]any{"nodes": []map[string]any{{
			"id": "t1:1", "type": "section",
			"syntax": map[string]any{"code": "@startuml\nA -> B\n@enduml", "syntax_type": 1},
		}, {"id": "r1:1", "type": "life_line", "parent_id": "t1:1"}}}})

	case strings.HasSuffix(path, "/nodes/plantuml"):
		body, _ := io.ReadAll(r.Body)
		m.mu.Lock()
		m.diagrams = append(m.diagrams, strings.Split(path, "/")[5]+" "+string(body))
		m.mu.Unlock()
		mockWriteJSON(w, 200, map[string]any{"code": 0, "msg": "ok", "data": map[string]any{"ticket_id": "tk"}})

	default:
		http.Error(w, "unexpected "+r.Method+" "+path, http.StatusNotFound)
	}
}

// reqIndex 读取建块请求体中的 index（-1 表示末尾）。
func reqIndex(body []byte) (int, bool) {
	var req struct {
		Index *int `json:"index"`
	}
	if json.Unmarshal(body, &req) != nil || req.Index == nil {
		return 0, false
	}
	return *req.Index, true
}

func tinyPNG() []byte {
	// 1x1 PNG
	return []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41, 0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x03, 0x01, 0x01, 0x00, 0x18, 0xdd, 0x8d, 0xb0, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
		0x44, 0xae, 0x42, 0x60, 0x82}
}

type importJSON struct {
	DocumentID      string          `json:"document_id"`
	URL             string          `json:"url"`
	PartialFailure  bool            `json:"partial_failure"`
	Failures        []importFailure `json:"failures"`
	Blocks          int             `json:"blocks"`
	ImageSuccess    int             `json:"image_success"`
	VideoSuccess    int             `json:"video_success"`
	FileSuccess     int             `json:"file_success"`
	FileFailed      int             `json:"file_failed"`
	WhiteboardOK    int             `json:"whiteboard_success"`
	BlocksFailed    int             `json:"blocks_failed"`
	WhiteboardTotal int             `json:"whiteboard_total"`
}

// runImportJSON 以 --document-id doc1 -o json 运行 doc import，返回解析后的 JSON 与 RunE 错误。
func runImportJSON(t *testing.T, m *docxMock, markdown string, files map[string][]byte) (importJSON, error, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(m.handler))
	t.Cleanup(server.Close)
	initDocUpdateTestConfig(t, server.URL)

	dir := t.TempDir()
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mdPath := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(mdPath, []byte(markdown), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = importMarkdownCmd.Flags().Set("document-id", "doc1")
	_ = importMarkdownCmd.Flags().Set("output", "json")
	t.Cleanup(func() { resetCmdFlag(importMarkdownCmd, "document-id", "output") })

	var runErr error
	stdout := captureStdout(t, func() {
		runErr = importMarkdownCmd.RunE(importMarkdownCmd, []string{mdPath})
	})
	var out importJSON
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout 应是 JSON（err=%v）:\n%s", runErr, stdout)
	}
	return out, runErr, stdout
}

// TestImportOutputsJSONWhenBlockCreationFails 建块阶段的致命错误（文档不存在 / 无权限等）也要输出 JSON
// （document_id、url、partial_failure、failures），并保持非零退出——此前直接返回错误，-o json 无任何输出。
func TestImportOutputsJSONWhenBlockCreationFails(t *testing.T) {
	m := newDocxMock(t)
	m.fatal = true
	out, runErr, _ := runImportJSON(t, m, "第一段\n\n第二段\n", nil)
	if runErr == nil {
		t.Fatal("建块失败时必须非零退出")
	}
	if out.DocumentID != "doc1" || !strings.Contains(out.URL, "/docx/doc1") || !out.PartialFailure {
		t.Fatalf("JSON 应包含文档信息与 partial_failure: %+v", out)
	}
	if len(out.Failures) != 1 || out.Failures[0].Kind != "blocks" || !strings.Contains(out.Failures[0].Error, "1770002") {
		t.Fatalf("failures 应记录建块失败: %+v", out.Failures)
	}
}

// TestImportIsolatesRejectedBlocks 一批块里只有个别块被服务端拒绝时，逐块隔离：其余内容照常写入，
// 被拒的块计入 failures（kind=blocks），而不是整篇导入中止。
func TestImportIsolatesRejectedBlocks(t *testing.T) {
	m := newDocxMock(t)
	out, runErr, _ := runImportJSON(t, m, "第一段\n\n<sheet rows=\"0\" cols=\"0\"/>\n\n第二段\n", nil)
	if runErr == nil {
		t.Fatal("有被拒的块时应以非零退出")
	}
	if out.Blocks != 2 || out.BlocksFailed != 1 {
		t.Fatalf("应写入 2 个段落、跳过 1 个被拒块: %+v", out)
	}
	if len(out.Failures) != 1 || out.Failures[0].Kind != "blocks" || !strings.Contains(out.Failures[0].Error, "Sheet") {
		t.Fatalf("failures = %+v", out.Failures)
	}
	if got := len(m.created["doc1"]); got != 2 {
		t.Fatalf("文档根下应创建 2 个块, got %d", got)
	}
}

// TestImportGridWritesIntoServerColumns 分栏：不能显式创建 GridColumn（1770028），列内容要写进服务端自动生成的列。
func TestImportGridWritesIntoServerColumns(t *testing.T) {
	m := newDocxMock(t)
	md := "<grid cols=\"2\">\n<column>\n左栏\n\n- 列表\n</column>\n<column>\n右栏\n</column>\n</grid>\n"
	out, runErr, stdout := runImportJSON(t, m, md, nil)
	if runErr != nil || out.PartialFailure {
		t.Fatalf("分栏导入不应失败: err=%v\n%s", runErr, stdout)
	}
	var gridID string
	for id, bt := range m.types {
		if bt == int(converter.BlockTypeGrid) {
			gridID = id
		}
	}
	if gridID == "" || len(m.created[gridID]) != 0 {
		t.Fatalf("不应向 Grid 显式创建子块: grid=%q created=%v", gridID, m.created[gridID])
	}
	left, right := m.created[gridID+"_c1"], m.created[gridID+"_c2"]
	if len(left) != 2 || len(right) != 1 {
		t.Fatalf("列内容应写入服务端列: left=%v right=%v", left, right)
	}
	if !strings.Contains(fmt.Sprint(left[0]), "左栏") || !strings.Contains(fmt.Sprint(right[0]), "右栏") {
		t.Fatalf("列内容错位: left=%v right=%v", left, right)
	}
}

// TestImportNestedQuoteNotRejected 嵌套引用扁平化后不再触发 1770030，外层内容完整写入。
func TestImportNestedQuoteNotRejected(t *testing.T) {
	m := newDocxMock(t)
	out, runErr, stdout := runImportJSON(t, m, "> 外层\n> > 内层\n>\n> 外层第二段\n", nil)
	if runErr != nil || out.PartialFailure {
		t.Fatalf("嵌套引用不应失败: err=%v\n%s", runErr, stdout)
	}
	var quoteID string
	for id, bt := range m.types {
		if bt == int(converter.BlockTypeQuoteContainer) {
			quoteID = id
		}
	}
	got := fmt.Sprint(m.created[quoteID])
	for _, want := range []string{"外层", "内层", "外层第二段"} {
		if !strings.Contains(got, want) {
			t.Fatalf("引用内容缺少 %q: %s", want, got)
		}
	}
}

// TestImportReusesTokenMediaAndCopiesBoard doc export 输出的 <image token>/<file token>/<whiteboard token>
// 导入时：建空块 → 下载原素材重新上传并绑定（保留原显示尺寸）、附件上传到 View 下的 File 块、
// 画板按节点里的图表源码重新导入。全部成功时退出码 0。
func TestImportReusesTokenMediaAndCopiesBoard(t *testing.T) {
	m := newDocxMock(t)
	md := "<image token=\"srcImg\" width=\"120\" height=\"80\" align=\"center\"/>\n\n" +
		"<file token=\"srcFile\" name=\"报告.pdf\"/>\n\n" +
		"<whiteboard token=\"srcBoard\" type=\"blank\"/>\n"
	out, runErr, stdout := runImportJSON(t, m, md, nil)
	if runErr != nil || out.PartialFailure {
		t.Fatalf("token 资源应复用成功: err=%v\n%s", runErr, stdout)
	}
	if out.ImageSuccess != 1 || out.FileSuccess != 1 || out.WhiteboardOK != 1 {
		t.Fatalf("统计异常: %+v", out)
	}

	var imageID, viewID string
	for id, bt := range m.types {
		switch bt {
		case int(converter.BlockTypeImage):
			imageID = id
		case int(converter.BlockTypeView):
			viewID = id
		}
	}
	rep, _ := m.patches[imageID]["replace_image"].(map[string]any)
	if rep == nil || !strings.HasPrefix(fmt.Sprint(rep["token"]), "newtok_") || rep["width"] != float64(120) || rep["height"] != float64(80) || rep["align"] != float64(2) {
		t.Fatalf("图片应绑定新上传素材并保留原显示尺寸/对齐: %v", m.patches[imageID])
	}
	fileRep, _ := m.patches[viewID+"_file"]["replace_file"].(map[string]any)
	if fileRep == nil {
		t.Fatalf("附件应 replace_file 到 View 的子 File 块: %v", m.patches)
	}
	var fileUpload map[string]string
	for _, u := range m.uploads {
		if u["parent_type"] == "docx_file" {
			fileUpload = u
		}
	}
	if fileUpload == nil || fileUpload["parent_node"] != viewID+"_file" || fileUpload["file_name"] != "报告.pdf" {
		t.Fatalf("附件应上传到 File 块并沿用原文件名: %v", m.uploads)
	}
	if len(m.diagrams) != 1 || !strings.Contains(m.diagrams[0], "plant_uml_code") || !strings.Contains(m.diagrams[0], "@startuml") {
		t.Fatalf("画板应按源码重新导入: %v", m.diagrams)
	}
}

// TestImportSplitsBoardsPerRequest doc export 的 Markdown 含大量 <whiteboard token> 时，同一批建块不能超过 5 个画板块
// （实测第 6 个即 1770035，此前整篇在第二批中止）。12 个画板应全部建出并复制成功，且每次请求 ≤5 个画板。
func TestImportSplitsBoardsPerRequest(t *testing.T) {
	m := newDocxMock(t)
	var md strings.Builder
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&md, "段落 %d\n\n<whiteboard token=\"srcBoard\" type=\"blank\"/>\n\n", i)
	}
	out, runErr, stdout := runImportJSON(t, m, md.String(), nil)
	if runErr != nil || out.PartialFailure {
		t.Fatalf("12 个画板应全部导入成功: err=%v\n%s", runErr, stdout)
	}
	if out.WhiteboardOK != 12 || m.maxBoards == 0 || m.maxBoards > 5 || m.over1770035 != 0 {
		t.Fatalf("画板复制 %d/12，单次请求最多画板 %d（应 1–5），1770035 被拒 %d 次（应 0：分批时就切开，不靠逐块兜底）",
			out.WhiteboardOK, m.maxBoards, m.over1770035)
	}
	if got := len(m.created["doc1"]); got != 24 {
		t.Fatalf("文档根下应创建 24 个块（12 段落 + 12 画板）, got %d", got)
	}
}

func TestBlockBatchRanges(t *testing.T) {
	board, text := int(converter.BlockTypeBoard), int(converter.BlockTypeText)
	mk := func(types ...int) []*larkdocx.Block {
		var bs []*larkdocx.Block
		for _, ty := range types {
			ty := ty
			bs = append(bs, &larkdocx.Block{BlockType: &ty})
		}
		return bs
	}
	cases := []struct {
		name  string
		types []int
		max   int
		want  [][2]int
	}{
		{"空", nil, 50, nil},
		{"纯文本按块数切", []int{text, text, text, text, text}, 2, [][2]int{{0, 2}, {2, 4}, {4, 5}}},
		{"第 6 个画板另起一批", []int{board, board, board, board, board, text, board, text}, 50, [][2]int{{0, 6}, {6, 8}}},
		{"画板与块数双约束", []int{board, board, board, board, board, board, board, board, board, board, board}, 50, [][2]int{{0, 5}, {5, 10}, {10, 11}}},
	}
	for _, c := range cases {
		got := blockBatchRanges(mk(c.types...), c.max)
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

// TestImportTokenMediaDownloadFailureDegrades 素材无法下载（不存在 / 无源文档权限）时，降级为占位文本并计入 failures，
// 其余内容照常导入。
func TestImportTokenMediaDownloadFailureDegrades(t *testing.T) {
	m := newDocxMock(t)
	out, runErr, _ := runImportJSON(t, m, "前文\n\n<image token=\"missingImg\"/>\n\n后文\n", nil)
	if runErr == nil {
		t.Fatal("素材复用失败时应非零退出")
	}
	if out.Blocks != 3 || len(out.Failures) != 1 || out.Failures[0].Kind != "image" || out.Failures[0].Source != "feishu://media/missingImg" {
		t.Fatalf("应只记录图片复用失败: %+v", out)
	}
	if strings.Contains(out.Failures[0].Error, "Header:map") {
		t.Fatalf("失败信息不应带完整响应头: %s", out.Failures[0].Error)
	}
	// 空 Image 块被替换为原位置的占位文本
	root := fmt.Sprint(m.created["doc1"])
	if !strings.Contains(root, "[图片未能复用: feishu://media/missingImg]") || strings.Contains(root, "block_type:27") {
		t.Fatalf("空图片块应替换为占位文本: %s", root)
	}
	if !strings.Contains(fmt.Sprint(m.created["doc1"][1]), "图片未能复用") {
		t.Fatalf("占位文本应在原位置（第 2 个块）: %v", m.created["doc1"])
	}
}

// TestImportVideoUploadsToFileChildOfView 本地视频：建块只带空 token（带 name 被拒），素材上传到 View 下的 File 块。
func TestImportVideoUploadsToFileChildOfView(t *testing.T) {
	m := newDocxMock(t)
	out, runErr, stdout := runImportJSON(t, m, "<video src=\"./demo.mp4\" data-name=\"演示.mp4\"></video>\n",
		map[string][]byte{"demo.mp4": []byte("\x00\x00\x00\x18ftypisom")})
	if runErr != nil || out.VideoSuccess != 1 {
		t.Fatalf("视频导入失败: err=%v\n%s", runErr, stdout)
	}
	if len(m.uploads) != 1 || !strings.HasSuffix(m.uploads[0]["parent_node"], "_file") || m.uploads[0]["file_name"] != "演示.mp4" {
		t.Fatalf("视频应上传到 File 块并使用 data-name: %v", m.uploads)
	}
}

func TestDiagramSourcesOfBoard(t *testing.T) {
	syntaxOnly := []map[string]any{
		{"id": "s", "type": "section", "syntax": map[string]any{"code": "flowchart LR\nA-->B", "syntax_type": float64(2)}},
		{"id": "n1", "type": "composite_shape", "parent_id": "s"},
		{"id": "n2", "type": "connector", "parent_id": "n1"},
	}
	src := diagramSourcesOfBoard(syntaxOnly)
	if len(src) != 1 || src[0].syntax != "mermaid" || !strings.Contains(src[0].code, "A-->B") {
		t.Fatalf("应识别 Mermaid 源码: %#v", src)
	}
	mixed := append(append([]map[string]any(nil), syntaxOnly...), map[string]any{"id": "free", "type": "text_shape"})
	if got := diagramSourcesOfBoard(mixed); got != nil {
		t.Fatalf("含手工节点时不能只按源码重建: %#v", got)
	}
	if got := diagramSourcesOfBoard([]map[string]any{{"id": "x", "type": "svg"}}); got != nil {
		t.Fatalf("无源码画板应返回 nil: %#v", got)
	}
}

func TestConciseDownloadError(t *testing.T) {
	long := fmt.Errorf("下载素材失败: response content-type not json, response: StatusCode: 404, Header:map[X:[%s]]", strings.Repeat("y", 3000))
	if got := conciseDownloadError(long); got != "HTTP 404" {
		t.Fatalf("got %q", got)
	}
	if got := conciseDownloadError(fmt.Errorf("下载素材失败: code=1061004, msg=forbidden")); !strings.Contains(got, "1061004") {
		t.Fatalf("got %q", got)
	}
}
