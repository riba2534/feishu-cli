package apidiag

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const okrDenied = `{"code":99991672,"msg":"Access denied.","error":{"message":"see doc","log_id":"2026LOG","troubleshooter":"排查建议查看(Troubleshooting suggestions): https://open.feishu.cn/search?log_id=2026LOG","permission_violations":[{"type":"action_scope_required","subject":"okr:okr.period:readonly"},{"subject":"okr:okr"},{"subject":"okr:okr"}]}}`

func TestParse(t *testing.T) {
	cases := []struct {
		name   string
		status int
		header http.Header
		body   string
		wantOK bool
		check  func(t *testing.T, info Info)
	}{
		{
			name: "HTTP 400 业务信封仍解析出 code / scope / log_id", status: 400, body: okrDenied, wantOK: true,
			check: func(t *testing.T, info Info) {
				if info.Code != 99991672 || info.HTTPStatus != 400 || info.LogID != "2026LOG" {
					t.Fatalf("解析结果不符: %+v", info)
				}
				if strings.Join(info.MissingScopes, ",") != "okr:okr.period:readonly,okr:okr" {
					t.Fatalf("缺失 scope 应去重保序，得到 %v", info.MissingScopes)
				}
				if !strings.Contains(info.Troubleshooter, "https://open.feishu.cn/search") {
					t.Fatalf("troubleshooter 丢失: %q", info.Troubleshooter)
				}
			},
		},
		{
			name: "字段校验与 details", status: 400, wantOK: true,
			body: `{"code":99992402,"msg":"field validation failed","error":{"field_violations":[{"field":"document_id","description":"the min len is 27"}],"details":[{"value":"end_time should be later than start_time"}]}}`,
			check: func(t *testing.T, info Info) {
				if len(info.FieldViolations) != 1 || info.FieldViolations[0].Field != "document_id" {
					t.Fatalf("field_violations 解析失败: %+v", info.FieldViolations)
				}
				if len(info.Details) != 1 {
					t.Fatalf("details 解析失败: %+v", info.Details)
				}
			},
		},
		{
			name: "body 无 log_id 时回退 X-Tt-Logid 头", status: 200, wantOK: true,
			header: http.Header{"X-Tt-Logid": []string{"HDRLOG"}},
			body:   `{"code":1254005,"msg":"not found"}`,
			check: func(t *testing.T, info Info) {
				if info.LogID != "HDRLOG" {
					t.Fatalf("应回退读取 X-Tt-Logid，得到 %q", info.LogID)
				}
			},
		},
		{name: "字符串形式的 code", status: 200, body: `{"code":"230001","msg":"param is invalid"}`, wantOK: true},
		{name: "code 0 不是错误", status: 200, body: `{"code":0,"msg":"success","data":{}}`},
		{name: "非 JSON", status: 502, body: `<html>bad gateway</html>`},
		{name: "空 body", status: 500, body: ``},
		{name: "无 code 字段", status: 200, body: `{"data":{"x":1}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info, ok := Parse(tc.status, tc.header, []byte(tc.body))
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v（info=%+v）", ok, tc.wantOK, info)
			}
			if ok && tc.check != nil {
				tc.check(t, info)
			}
		})
	}
}

func TestLinesSkipsValuesAlreadyInMessage(t *testing.T) {
	info, _ := Parse(400, nil, []byte(okrDenied))
	lines := strings.Join(info.Lines("调用失败: code=99991672, log_id=2026LOG"), "\n")
	if strings.Contains(lines, "log_id: 2026LOG") {
		t.Fatalf("错误文本已含 log_id 时不应重复输出:\n%s", lines)
	}
	if !strings.Contains(lines, "所需 scope（满足其一即可）: okr:okr.period:readonly, okr:okr") {
		t.Fatalf("应输出缺失 scope:\n%s", lines)
	}
}

func TestObserveResponseRecordsErrorEnvelope(t *testing.T) {
	Reset()
	defer Reset()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/err":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, okrDenied)
		case "/ok":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"code":0,"data":{}}`)
		case "/binary":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.WriteString(w, `{"code":12345,"msg":"looks like json but is a file"}`)
		case "/big":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"code":777,"msg":"%s"}`, strings.Repeat("x", maxObservedBody+10))
		}
	}))
	defer srv.Close()

	fetch := func(path string) string {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}
		ObserveResponse(resp)
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return string(b)
	}

	if got := fetch("/err"); got != okrDenied {
		t.Fatalf("旁路观察不得改变响应体，得到 %q", got)
	}
	info, ok := Lookup(func(i Info) bool { return i.Code == 99991672 })
	if !ok || info.LogID != "2026LOG" || info.HTTPStatus != 400 {
		t.Fatalf("应记录 HTTP 400 的错误信封，得到 ok=%v info=%+v", ok, info)
	}
	fetch("/ok")
	fetch("/binary")
	fetch("/big")
	if _, ok := Lookup(func(i Info) bool { return i.Code == 12345 || i.Code == 777 }); ok {
		t.Fatal("非 JSON 与超大响应不应被记录")
	}
}

func TestLookupNewestFirst(t *testing.T) {
	Reset()
	defer Reset()
	Record(Info{Code: 1, LogID: "old"})
	Record(Info{Code: 1, LogID: "new"})
	for i := 0; i < maxRecorded+5; i++ {
		Record(Info{Code: 2})
	}
	if _, ok := Lookup(func(i Info) bool { return i.Code == 1 }); ok {
		t.Fatal("超出容量的旧记录应被淘汰")
	}
	Record(Info{Code: 3, LogID: "a"})
	Record(Info{Code: 3, LogID: "b"})
	if info, _ := Lookup(func(i Info) bool { return i.Code == 3 }); info.LogID != "b" {
		t.Fatalf("应返回最近一条，得到 %q", info.LogID)
	}
}
