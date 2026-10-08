package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
	"github.com/riba2534/feishu-cli/internal/clierr"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// ---------- 测试辅助 ----------

// runSheetCmdForTest 设置 flag 后直接执行命令 RunE 并捕获 stdout，结束后把 flag 复位（命令是全局变量）。
func runSheetCmdForTest(t *testing.T, c *cobra.Command, args []string, flags map[string]string) (string, error) {
	t.Helper()
	resetSheetCmdFlags(c) // 同一测试内多次调用时先清掉上一次的 flag
	t.Cleanup(func() { resetSheetCmdFlags(c) })
	for k, v := range flags {
		if err := c.Flags().Set(k, v); err != nil {
			t.Fatalf("设置 --%s=%s 失败: %v", k, v, err)
		}
	}
	var runErr error
	out := captureStdoutConcurrent(t, func() { runErr = c.RunE(c, args) })
	return out, runErr
}

// captureStdoutConcurrent 边执行边读取 stdout，避免输出超过管道缓冲区（64KB）时写端阻塞死锁。
func captureStdoutConcurrent(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	defer func() { os.Stdout = oldStdout }()
	fn()
	_ = w.Close()
	return <-done
}

func resetSheetCmdFlags(c *cobra.Command) {
	c.Flags().VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
}

type sheetCmdRequest struct {
	Method, Path, RawPath, Query string
	Body                         []byte
}

// sheetMockServer 模拟飞书 sheets 接口：鉴权、子表列表（Sheet1→s1、数据→s2）与通用成功响应，并记录业务请求。
type sheetMockServer struct {
	mu      sync.Mutex
	reqs    []sheetCmdRequest
	sheets  string // /sheets/query 的 sheets JSON
	respond func(w http.ResponseWriter, r *http.Request, body []byte) bool
}

func newSheetMockServer(t *testing.T) *sheetMockServer {
	t.Helper()
	m := &sheetMockServer{
		sheets: `[{"sheet_id":"s1","title":"Sheet1","index":0,"grid_properties":{"row_count":200,"column_count":20}},` +
			`{"sheet_id":"s2","title":"数据","index":1,"grid_properties":{"row_count":200,"column_count":20}}]`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mockAuthHandler(w, r) {
			return
		}
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/sheets/query") {
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"sheets":`+m.sheets+`}}`)
			return
		}
		m.mu.Lock()
		m.reqs = append(m.reqs, sheetCmdRequest{Method: r.Method, Path: r.URL.Path, RawPath: r.URL.EscapedPath(), Query: r.URL.RawQuery, Body: body})
		m.mu.Unlock()
		if m.respond != nil && m.respond(w, r, body) {
			return
		}
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{}}`)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(setupCmdTestConfig(t, srv.URL))
	return m
}

func (m *sheetMockServer) requests() []sheetCmdRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]sheetCmdRequest(nil), m.reqs...)
}

func decodeBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("解析请求体失败: %v (%s)", err, raw)
	}
	return m
}

// ---------- 范围与子表解析 ----------

func stubSheetList(t *testing.T, sheets []*client.SheetInfo, err error) *int {
	t.Helper()
	calls := 0
	orig := sheetListFunc
	sheetListFunc = func(ctx context.Context, token, uat string) ([]*client.SheetInfo, error) {
		calls++
		return sheets, err
	}
	t.Cleanup(func() { sheetListFunc = orig })
	return &calls
}

// TestSheetQualifyRange 范围前缀可以是 sheetId 或子表名（"Sheet1!A1:C10" 曾实测 90215 not found sheetId）。
func TestSheetQualifyRange(t *testing.T) {
	two := []*client.SheetInfo{{SheetID: "s1", Title: "Sheet1"}, {SheetID: "s2", Title: "数据"}}
	cases := []struct {
		name                   string
		sheets                 []*client.SheetInfo
		rng, sheetID, sheetNm  string
		urlSheet               string
		want                   string
		wantUsageErr, wantList bool
	}{
		{name: "子表名前缀换成 sheetId", sheets: two, rng: "Sheet1!A1:C10", want: "s1!A1:C10", wantList: true},
		{name: "中文子表名 + 全角分隔符", sheets: two, rng: "数据！B2", want: "s2!B2", wantList: true},
		{name: "带引号子表名", sheets: two, rng: "'数据'!A:C", want: "s2!A:C", wantList: true},
		{name: "zsh 转义", sheets: two, rng: `Sheet1\!A1`, want: "s1!A1", wantList: true},
		{name: "sheetId 前缀原样", sheets: two, rng: "s2!A1", want: "s2!A1", wantList: true},
		{name: "前缀等于已知 sheetId 不发请求", sheets: two, rng: "s9!A1", sheetID: "s9", want: "s9!A1"},
		{name: "无前缀用 --sheet-id", sheets: two, rng: "A1:B2", sheetID: "s2", want: "s2!A1:B2"},
		{name: "无前缀用 --sheet-name", sheets: two, rng: "A1:B2", sheetNm: "数据", want: "s2!A1:B2", wantList: true},
		{name: "无前缀用 URL ?sheet=", sheets: two, rng: "A1:B2", urlSheet: "s2", want: "s2!A1:B2"},
		{name: "无前缀且唯一子表", sheets: two[:1], rng: "A1:B2", want: "s1!A1:B2", wantList: true},
		{name: "无前缀且多子表报用法错误", sheets: two, rng: "A1:B2", wantUsageErr: true, wantList: true},
		{name: "未知子表报用法错误", sheets: two, rng: "不存在!A1", wantUsageErr: true, wantList: true},
		{name: "整个子表（子表名）", sheets: two, rng: "数据", want: "s2", wantList: true},
		{name: "整个子表（已知 sheetId）", sheets: two, rng: "s9", sheetID: "s9", want: "s9"},
		{name: "空前缀报错", sheets: two, rng: "!A1", wantUsageErr: true},
		{name: "形如单元格的 sheetId 精确命中按整表", sheets: []*client.SheetInfo{{SheetID: "abc123", Title: "T"}}, rng: "abc123", want: "abc123", wantList: true},
		{name: "单元格写法未命中子表时按单元格", sheets: []*client.SheetInfo{{SheetID: "s1", Title: "T"}}, rng: "B2", want: "s1!B2", wantList: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			calls := stubSheetList(t, c.sheets, nil)
			target := &sheetTarget{Token: "shtTok", URLSheetID: c.urlSheet}
			got, err := target.qualifyRange(c.rng, c.sheetID, c.sheetNm)
			if c.wantUsageErr {
				if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
					t.Fatalf("应返回用法错误，得到 %q, %v", got, err)
				}
			} else if err != nil || got != c.want {
				t.Fatalf("qualifyRange(%q) = (%q, %v), want %q", c.rng, got, err, c.want)
			}
			if (*calls > 0) != c.wantList {
				t.Errorf("子表列表请求次数 = %d, wantList=%v", *calls, c.wantList)
			}
		})
	}
}

// TestSheetQualifyRange_ListFailureKeepsOldBehavior 子表列表获取失败时原样透传（不引入新的失败模式）。
func TestSheetQualifyRange_ListFailureKeepsOldBehavior(t *testing.T) {
	stubSheetList(t, nil, errors.New("boom"))
	target := &sheetTarget{Token: "shtTok"}
	for in, want := range map[string]string{"Sheet1!A1": "Sheet1!A1", "A1:B2": "A1:B2"} {
		got, err := target.qualifyRange(in, "", "")
		if err != nil || got != want {
			t.Errorf("qualifyRange(%q) = (%q, %v), want %q", in, got, err, want)
		}
	}
}

// TestResolveSheetUserToken --as 强制身份；不传时保持旧行为（User 优先）。
func TestResolveSheetUserToken(t *testing.T) {
	t.Cleanup(setupCmdTestConfig(t, "https://open.feishu.cn"))
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "u-env-token")
	newCmd := func() *cobra.Command {
		c := &cobra.Command{Use: "x"}
		c.Flags().String("as", "", "")
		c.Flags().String("user-access-token", "", "")
		return c
	}
	cases := []struct {
		as        string
		want      string
		wantUsage bool
	}{
		{"", "u-env-token", false}, // 未传 --as：旧行为，User 优先
		{"bot", "", false},         // 已登录也强制 Bot（Bot 自有表格曾因无法强制 Bot 报 1310213）
		{"user", "u-env-token", false},
		{"auto", "u-env-token", false},
		{"robot", "", true},
	}
	for _, c := range cases {
		cmd := newCmd()
		if c.as != "" {
			_ = cmd.Flags().Set("as", c.as)
		}
		got, err := resolveSheetUserToken(cmd)
		if c.wantUsage {
			if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
				t.Errorf("--as %s 应返回用法错误，得到 %v", c.as, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("--as %q → (%q, %v), want %q", c.as, got, err, c.want)
		}
	}

	// --as user 且未配置 User Token：报错而不是静默用 Bot
	t.Setenv("FEISHU_USER_ACCESS_TOKEN", "")
	cmd := newCmd()
	_ = cmd.Flags().Set("as", "user")
	if _, err := resolveSheetUserToken(cmd); err == nil {
		t.Error("--as user 缺少 User Token 时应报错")
	}
}

// TestSheetCmdPersistentAsFlag 命令组级 --as 对所有 sheet 子命令生效。
func TestSheetCmdPersistentAsFlag(t *testing.T) {
	if sheetCmd.PersistentFlags().Lookup("as") == nil {
		t.Fatal("sheet 命令组应注册持久 --as")
	}
	for _, sub := range []*cobra.Command{sheetReadCmd, sheetWriteCmd, sheetListSheetsCmd, sheetImageAddCmd, sheetFilterViewCreateCmd} {
		if sub.InheritedFlags().Lookup("as") == nil {
			t.Errorf("%s 未继承 --as", sub.CommandPath())
		}
	}
}

// TestParseSpreadsheetArg 表格参数接受裸 token、/sheets/、/spreadsheets/ 与 /wiki/ URL，拒绝其他类型。
func TestParseSpreadsheetArg(t *testing.T) {
	cases := []struct {
		in, token, sheet string
		wiki, wantErr    bool
	}{
		{in: "shtTok", token: "shtTok"},
		{in: "https://xxx.feishu.cn/sheets/shtTok?sheet=s2", token: "shtTok", sheet: "s2"},
		{in: "https://xxx.feishu.cn/spreadsheets/shtTok", token: "shtTok"},
		{in: "https://xxx.larkoffice.com/wiki/wikTok", token: "wikTok", wiki: true},
		{in: "https://xxx.feishu.cn/docx/docTok", wantErr: true},
		{in: "https://evil.example.com/sheets/shtTok", wantErr: true},
		{in: "sht/Tok", wantErr: true},
	}
	for _, c := range cases {
		token, sheet, wiki, err := parseSpreadsheetArg(c.in, "")
		if c.wantErr {
			if err == nil || !clierr.HasKind(err, clierr.KindUsage) {
				t.Errorf("parseSpreadsheetArg(%q) 应返回用法错误，得到 %v", c.in, err)
			}
			continue
		}
		if err != nil || token != c.token || sheet != c.sheet || wiki != c.wiki {
			t.Errorf("parseSpreadsheetArg(%q) = (%q,%q,%v,%v)", c.in, token, sheet, wiki, err)
		}
	}
}

// TestNewSheetTarget_WikiURL /wiki/ URL 通过 node_by_token 换出底层表格 token。
func TestNewSheetTarget_WikiURL(t *testing.T) {
	m := newSheetMockServer(t)
	m.respond = func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		if strings.HasSuffix(r.URL.Path, "/wiki/v2/spaces/get_node") || strings.HasSuffix(r.URL.Path, "/node_by_token") {
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"node":{"node_token":"wikTok","obj_token":"shtReal","obj_type":"sheet","space_id":"1"}}}`)
			return true
		}
		return false
	}
	target, err := newSheetTargetWithToken("https://xxx.feishu.cn/wiki/wikTok", "<spreadsheet_token>", "")
	if err != nil {
		t.Fatal(err)
	}
	if target.Token != "shtReal" {
		t.Errorf("wiki 应解包为底层表格 token，得到 %q", target.Token)
	}

	m.respond = func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"node":{"node_token":"wikTok","obj_token":"docReal","obj_type":"docx","space_id":"1"}}}`)
		return true
	}
	if _, err := newSheetTargetWithToken("https://xxx.feishu.cn/wiki/wikTok", "<spreadsheet_token>", ""); err == nil {
		t.Error("wiki 节点不是电子表格时应报错")
	}
}

// ---------- 读写命令端到端（httptest） ----------

// TestSheetRead_SheetNamePrefix "Sheet1!A1:B2" 先换算为 sheetId 再请求。
func TestSheetRead_SheetNamePrefix(t *testing.T) {
	m := newSheetMockServer(t)
	m.respond = func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"valueRange":{"range":"s1!A1:B1","values":[[1234567890123456789,"x"]]}}}`)
		return true
	}
	out, err := runSheetCmdForTest(t, sheetReadCmd, []string{"shtTok", "Sheet1!A1:B1"}, map[string]string{"output": "json"})
	if err != nil {
		t.Fatal(err)
	}
	reqs := m.requests()
	if len(reqs) != 1 {
		t.Fatalf("请求数 = %d", len(reqs))
	}
	path, _ := url.PathUnescape(reqs[0].RawPath)
	if !strings.HasSuffix(path, "/values/s1!A1:B1") {
		t.Errorf("读取路径应使用 sheetId，得到 %s", path)
	}
	if !strings.Contains(out, "1234567890123456789") {
		t.Errorf("输出数字精度丢失: %s", out)
	}
}

// TestSheetWrite_BatchesAndPreservesNumbers 超过 5000 行自动分批；数字原样写入。
func TestSheetWrite_BatchesAndPreservesNumbers(t *testing.T) {
	m := newSheetMockServer(t)
	m.respond = func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		var req struct {
			ValueRange struct {
				Range string `json:"range"`
			} `json:"valueRange"`
		}
		_ = json.Unmarshal(body, &req)
		fmt.Fprintf(w, `{"code":0,"msg":"ok","data":{"updatedRange":%q}}`, req.ValueRange.Range)
		return true
	}
	var b strings.Builder
	b.WriteString("[")
	for i := 1; i <= 12000; i++ {
		if i > 1 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `[%d,1234567890123456789]`, i)
	}
	b.WriteString("]")

	out, err := runSheetCmdForTest(t, sheetWriteCmd, []string{"shtTok", "数据!A1"}, map[string]string{"data": b.String(), "output": "json"})
	if err != nil {
		t.Fatal(err)
	}
	reqs := m.requests()
	var ranges []string
	for _, r := range reqs {
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected %s %s", r.Method, r.Path)
		}
		body := decodeBody(t, r.Body)
		ranges = append(ranges, body["valueRange"].(map[string]any)["range"].(string))
		if !strings.Contains(string(r.Body), "1234567890123456789") {
			t.Errorf("请求体大整数被改写")
		}
	}
	want := []string{"s2!A1:B5000", "s2!A5001:B10000", "s2!A10001:B12000"}
	if strings.Join(ranges, ",") != strings.Join(want, ",") {
		t.Errorf("分批范围 = %v, want %v", ranges, want)
	}
	if !strings.Contains(out, `"range": "s2!A1:B12000"`) || !strings.Contains(out, `"batches": 3`) {
		t.Errorf("输出 = %s", out[:200])
	}
}

// TestSheetAppend_BatchesAnchoredAfterPrevious 分批追加：后一批锚定在上一批实际写入位置之后。
func TestSheetAppend_BatchesAnchoredAfterPrevious(t *testing.T) {
	m := newSheetMockServer(t)
	next := 11 // 模拟已有 10 行数据
	m.respond = func(w http.ResponseWriter, r *http.Request, body []byte) bool {
		var req struct {
			ValueRange struct {
				Values [][]any `json:"values"`
			} `json:"valueRange"`
		}
		_ = json.Unmarshal(body, &req)
		n := len(req.ValueRange.Values)
		fmt.Fprintf(w, `{"code":0,"msg":"ok","data":{"updates":{"updatedRange":"s1!A%d:B%d"}}}`, next, next+n-1)
		next += n
		return true
	}
	rows := make([]string, 6000)
	for i := range rows {
		rows[i] = fmt.Sprintf(`[%d,"r"]`, i)
	}
	out, err := runSheetCmdForTest(t, sheetAppendCmd, []string{"shtTok", "s1!A:B"}, map[string]string{"data": "[" + strings.Join(rows, ",") + "]"})
	if err != nil {
		t.Fatal(err)
	}
	reqs := m.requests()
	if len(reqs) != 2 {
		t.Fatalf("应分 2 批，得到 %d", len(reqs))
	}
	r0 := decodeBody(t, reqs[0].Body)["valueRange"].(map[string]any)
	r1 := decodeBody(t, reqs[1].Body)["valueRange"].(map[string]any)
	if r0["range"] != "s1!A:B" || r1["range"] != "s1!A5011:B6010" {
		t.Errorf("追加范围 = %v / %v", r0["range"], r1["range"])
	}
	if len(r0["values"].([]any)) != 5000 || len(r1["values"].([]any)) != 1000 {
		t.Errorf("分批行数错误")
	}
	if !strings.Contains(out, "s1!A11:B6010") {
		t.Errorf("输出范围 = %s", out)
	}
}

// TestSheetInsertSimple_NumberLiterals insert --simple 的 V3 value 元素不出现 1e+06。
func TestSheetInsertSimple_NumberLiterals(t *testing.T) {
	m := newSheetMockServer(t)
	if _, err := runSheetCmdForTest(t, sheetInsertCmd, []string{"shtTok", "s1", "A1:B1"},
		map[string]string{"simple": "true", "data": `[[1000000, 1234567890123456789]]`}); err != nil {
		t.Fatal(err)
	}
	reqs := m.requests()
	if len(reqs) != 1 {
		t.Fatalf("请求数 = %d", len(reqs))
	}
	raw := string(reqs[0].Body)
	if !strings.Contains(raw, `"value":"1000000"`) || !strings.Contains(raw, `"value":"1234567890123456789"`) {
		t.Errorf("V3 value 元素被改写: %s", raw)
	}
	path, _ := url.PathUnescape(reqs[0].RawPath)
	if !strings.Contains(path, "/values/s1!A1:B1/insert") {
		t.Errorf("无前缀范围应补上 sheet_id: %s", path)
	}
}
