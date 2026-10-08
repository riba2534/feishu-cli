// Package apidiag 解析飞书 OpenAPI 错误响应中的诊断字段（log_id、缺失 scope、字段校验等），
// 并在进程内记录最近的错误响应，供根命令在打印错误时附加诊断信息。
//
// 背景：飞书大量业务错误随 HTTP 400/403 下发，且 code/msg 之外的诊断信息
// （error.log_id、error.permission_violations、error.field_violations、troubleshooter）
// 在各命令"code=%d, msg=%s"式的错误格式化里被丢弃。本包在传输层旁路观察响应体，
// 不改变任何请求/响应语义，也不需要逐个改造调用点。
//
// 本包是叶子包（只依赖标准库），config 的受控 Transport 与 client 都可以引用。
package apidiag

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// FieldViolation 对应 error.field_violations[] 的一项。
type FieldViolation struct {
	Field       string `json:"field"`
	Value       string `json:"value"`
	Description string `json:"description"`
}

// Info 一次飞书 OpenAPI 错误响应的诊断信息。
type Info struct {
	HTTPStatus      int
	Code            int
	Msg             string
	LogID           string
	Troubleshooter  string
	MissingScopes   []string // error.permission_violations[].subject（去重保序；服务端语义为"满足其一即可"）
	FieldViolations []FieldViolation
	Details         []string // error.details[].value
}

type envelope struct {
	Code  json.RawMessage `json:"code"`
	Msg   string          `json:"msg"`
	LogID string          `json:"log_id"`
	Error *struct {
		Message              string           `json:"message"`
		LogID                string           `json:"log_id"`
		Troubleshooter       string           `json:"troubleshooter"`
		FieldViolations      []FieldViolation `json:"field_violations"`
		PermissionViolations []struct {
			Subject string `json:"subject"`
		} `json:"permission_violations"`
		Details []struct {
			Value string `json:"value"`
		} `json:"details"`
	} `json:"error"`
}

// Parse 从响应体解析飞书业务错误信封。只看 body，不看 HTTP 状态码：
// 只要 body 是带非零 code 的飞书 JSON 信封就返回 ok=true（HTTP 200/400/403/500 均可）。
// header 可为 nil；body 里没有 log_id 时回退读取 X-Tt-Logid / X-Request-Id 响应头。
func Parse(status int, header http.Header, body []byte) (Info, bool) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '{' {
		return Info{}, false
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return Info{}, false
	}
	code, ok := parseCode(env.Code)
	if !ok || code == 0 {
		return Info{}, false
	}
	info := Info{HTTPStatus: status, Code: code, Msg: env.Msg, LogID: env.LogID}
	if e := env.Error; e != nil {
		if info.LogID == "" {
			info.LogID = e.LogID
		}
		info.Troubleshooter = strings.TrimSpace(e.Troubleshooter)
		info.FieldViolations = e.FieldViolations
		seen := map[string]bool{}
		for _, pv := range e.PermissionViolations {
			if s := strings.TrimSpace(pv.Subject); s != "" && !seen[s] {
				seen[s] = true
				info.MissingScopes = append(info.MissingScopes, s)
			}
		}
		for _, d := range e.Details {
			if v := strings.TrimSpace(d.Value); v != "" {
				info.Details = append(info.Details, v)
			}
		}
	}
	if info.LogID == "" && header != nil {
		info.LogID = firstNonEmpty(header.Get("X-Tt-Logid"), header.Get("X-Request-Id"))
	}
	return info, true
}

// parseCode 兼容数字与数字字符串两种 code 形态。
func parseCode(raw json.RawMessage) (int, bool) {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// Lines 渲染诊断信息（不含 code/msg，调用方的错误主文本里已有）。
// skipIn 非空时，已出现在其中的值不再重复输出（例如错误文本已包含 log_id）。
func (i Info) Lines(skipIn string) []string {
	var lines []string
	add := func(value, line string) {
		if value == "" || (skipIn != "" && strings.Contains(skipIn, value)) {
			return
		}
		lines = append(lines, line)
	}
	if len(i.MissingScopes) > 0 {
		joined := strings.Join(i.MissingScopes, ", ")
		add(joined, "所需 scope（满足其一即可）: "+joined)
	}
	for _, fv := range i.FieldViolations {
		desc := fv.Description
		if fv.Value != "" {
			desc = fmt.Sprintf("%s（当前值: %s）", desc, fv.Value)
		}
		line := strings.TrimSpace(fv.Field + ": " + desc)
		add(line, "字段校验: "+line)
	}
	for _, d := range i.Details {
		add(d, "详情: "+d)
	}
	add(i.LogID, "log_id: "+i.LogID)
	add(i.Troubleshooter, i.Troubleshooter)
	return lines
}

// ---- 进程内最近错误响应记录 ----

const maxRecorded = 16

var (
	recordMu sync.Mutex
	recorded []Info
)

// Record 记录一次错误响应（保留最近 maxRecorded 条）。
func Record(info Info) {
	if info.Code == 0 {
		return
	}
	recordMu.Lock()
	defer recordMu.Unlock()
	recorded = append(recorded, info)
	if len(recorded) > maxRecorded {
		recorded = append([]Info(nil), recorded[len(recorded)-maxRecorded:]...)
	}
}

// Lookup 从最近到最早查找第一条满足 match 的错误记录。
func Lookup(match func(Info) bool) (Info, bool) {
	recordMu.Lock()
	defer recordMu.Unlock()
	for i := len(recorded) - 1; i >= 0; i-- {
		if match(recorded[i]) {
			return recorded[i], true
		}
	}
	return Info{}, false
}

// Reset 清空记录（测试用）。
func Reset() {
	recordMu.Lock()
	recorded = nil
	recordMu.Unlock()
}

// ---- 传输层旁路观察 ----

// maxObservedBody 只观察不超过该大小的 JSON 响应：错误信封都很小，
// 大响应（列表、导出）直接放行，不额外占内存。
const maxObservedBody = 64 << 10

// ObserveResponse 包装 resp.Body：调用方读完（或关闭）响应体后，
// 若内容是飞书错误信封则记录诊断信息。不缓冲、不改变读取语义；非 JSON 响应（文件下载等）不包装。
func ObserveResponse(resp *http.Response) {
	if resp == nil || resp.Body == nil || resp.Body == http.NoBody {
		return
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "json") {
		return
	}
	if resp.ContentLength > maxObservedBody {
		return
	}
	resp.Body = &observingBody{rc: resp.Body, status: resp.StatusCode, header: resp.Header}
}

type observingBody struct {
	rc       io.ReadCloser
	status   int
	header   http.Header
	buf      bytes.Buffer
	overflow bool
	once     sync.Once
}

func (b *observingBody) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	if n > 0 && !b.overflow {
		if b.buf.Len()+n > maxObservedBody {
			b.overflow = true
			b.buf = bytes.Buffer{}
		} else {
			b.buf.Write(p[:n])
		}
	}
	if err == io.EOF {
		b.finish()
	}
	return n, err
}

func (b *observingBody) Close() error {
	b.finish()
	return b.rc.Close()
}

func (b *observingBody) finish() {
	b.once.Do(func() {
		if b.overflow || b.buf.Len() == 0 {
			return
		}
		if info, ok := Parse(b.status, b.header, b.buf.Bytes()); ok {
			Record(info)
		}
		b.buf = bytes.Buffer{}
	})
}
