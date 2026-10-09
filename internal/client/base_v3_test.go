package client

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestBaseV3CallParsesBusinessErrorBeforeHTTPStatus 飞书业务错误随 HTTP 400 下发时，
// 必须先解析业务信封：错误保留 code=N 与 data.error.hint，且能 AsAPIError 取到结构化诊断。
func TestBaseV3CallParsesBusinessErrorBeforeHTTPStatus(t *testing.T) {
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":800010701,"msg":"invalid_request","data":{"error":{"hint":"Array must contain at most 200 element(s)","path":"record_id_list"}}}`)
	})
	defer cleanup()

	_, err := BaseV3Call("POST", BaseV3Path("bases", "b1", "tables", "t1", "records", "batch_delete"), nil, map[string]any{}, "u-test")
	if err == nil {
		t.Fatal("HTTP 400 + code!=0 应返回错误")
	}
	if !HasAPICode(err, 800010701) {
		t.Errorf("错误应携带业务码 800010701: %v", err)
	}
	if !strings.Contains(err.Error(), "at most 200") || !strings.Contains(err.Error(), "record_id_list") {
		t.Errorf("错误应透出 hint 与字段路径: %v", err)
	}
	if strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("业务错误不应退化成 HTTP 状态码文案: %v", err)
	}
	apiErr, ok := AsAPIError(err)
	if !ok || apiErr.Code != 800010701 || apiErr.HTTPStatus != http.StatusBadRequest {
		t.Errorf("应能 AsAPIError 取到 code/HTTP 状态，got ok=%v %+v", ok, apiErr)
	}
}

// TestBaseV3CallHTTPErrorWithoutEnvelope 非飞书信封的 HTTP 错误仍按状态码报错。
func TestBaseV3CallHTTPErrorWithoutEnvelope(t *testing.T) {
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `<html>bad gateway</html>`)
	})
	defer cleanup()

	_, err := BaseV3Call("GET", BaseV3Path("bases", "b1"), nil, nil, "u-test")
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("非 JSON 的 HTTP 502 应报 HTTP 502，got %v", err)
	}
}

// TestBaseV3CallAnyReturnsArrayData 视图 group/sort/visible_fields 的 data 是数组：
// BaseV3CallAny 必须解包返回数组本身，而不是 {"code","data","msg"} 信封。
func TestBaseV3CallAnyReturnsArrayData(t *testing.T) {
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"","data":[{"field":"状态","desc":false}]}`)
	})
	defer cleanup()

	data, err := BaseV3CallAny("GET", BaseV3Path("bases", "b1", "tables", "t1", "views", "v1", "group"), nil, nil, "u-test")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	arr, ok := data.([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("应返回 data 数组本身，got %T %v", data, data)
	}
	if m, _ := arr[0].(map[string]any); m["field"] != "状态" {
		t.Errorf("数组元素不对: %v", arr[0])
	}
}

// TestBaseV3CallKeepsJSONLikeStrings 普通接口里以 { 开头的字符串（如用户起的名称/描述）必须原样保留，
// 不能被当成二次序列化的 JSON 改写（旧实现对所有接口都尝试反序列化）。
func TestBaseV3CallKeepsJSONLikeStrings(t *testing.T) {
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"data":{"name":"{\"a\":1}","description":"{plain}"}}`)
	})
	defer cleanup()

	data, err := BaseV3Call("GET", BaseV3Path("bases", "b1", "tables", "t1"), nil, nil, "u-test")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got, ok := data["name"].(string); !ok || got != `{"a":1}` {
		t.Errorf("name 应保持原始字符串，got %T %v", data["name"], data["name"])
	}
}

// TestBaseV3CallRetriesMethodLimited 800004135（按接口方法限流，请求未执行）有限重试后成功。
func TestBaseV3CallRetriesMethodLimited(t *testing.T) {
	var calls int32
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if atomic.AddInt32(&calls, 1) == 1 {
			_, _ = io.WriteString(w, `{"code":800004135,"msg":"the method：OpenAPIUpdateViewSort limited"}`)
			return
		}
		_, _ = io.WriteString(w, `{"code":0,"data":{"ok":true}}`)
	})
	defer cleanup()
	orig := baseV3RateLimitBackoff
	baseV3RateLimitBackoff = []time.Duration{0, 0}
	defer func() { baseV3RateLimitBackoff = orig }()

	data, err := BaseV3Call("PUT", BaseV3Path("bases", "b1", "tables", "t1", "views", "v1", "sort"), nil, map[string]any{}, "u-test")
	if err != nil {
		t.Fatalf("限流后重试应成功: %v", err)
	}
	if data["ok"] != true || atomic.LoadInt32(&calls) != 2 {
		t.Errorf("应重试 1 次后成功，calls=%d data=%v", calls, data)
	}
}

// TestBaseV3CallMethodLimitedGivesUp 重试次数用完后返回限流错误，不无限循环。
func TestBaseV3CallMethodLimitedGivesUp(t *testing.T) {
	var calls int32
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":800004135,"msg":"limited"}`)
	})
	defer cleanup()
	orig := baseV3RateLimitBackoff
	baseV3RateLimitBackoff = []time.Duration{0, 0}
	defer func() { baseV3RateLimitBackoff = orig }()

	_, err := BaseV3Call("GET", BaseV3Path("bases", "b1"), nil, nil, "u-test")
	if !HasAPICode(err, 800004135) {
		t.Fatalf("应返回限流错误，got %v", err)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Errorf("1 次原始请求 + 2 次重试 = 3，got %d", calls)
	}
}

func TestUnwrapBaseRoleData(t *testing.T) {
	t.Run("二次序列化的内层 data 被解开", func(t *testing.T) {
		got, err := UnwrapBaseRoleData(map[string]any{"data": `{"role_id":"rol1","role_name":"R"}`})
		if err != nil {
			t.Fatal(err)
		}
		m, _ := got.(map[string]any)
		if m["role_id"] != "rol1" {
			t.Errorf("got %v", got)
		}
	})
	t.Run("角色列表每项字符串也被解开", func(t *testing.T) {
		got, err := UnwrapBaseRoleData(map[string]any{"data": `{"base_roles":["{\"role_id\":\"rol1\"}","{\"role_id\":\"rol2\"}"],"total":2}`})
		if err != nil {
			t.Fatal(err)
		}
		roles := got.(map[string]any)["base_roles"].([]any)
		if r, ok := roles[1].(map[string]any); !ok || r["role_id"] != "rol2" {
			t.Errorf("base_roles 项应解码为对象: %v", roles)
		}
	})
	t.Run("内层 code 非 0 报错", func(t *testing.T) {
		_, err := UnwrapBaseRoleData(map[string]any{"code": json.Number("1254302"), "message": "role not found"})
		if err == nil || !HasAPICode(err, 1254302) || !strings.Contains(err.Error(), "role not found") {
			t.Fatalf("外层 code=0 但内层失败必须报错，got %v", err)
		}
	})
	t.Run("内层 JSON 里的 code 非 0 报错", func(t *testing.T) {
		_, err := UnwrapBaseRoleData(map[string]any{"data": `{"code":2,"message":"bad role"}`})
		if err == nil || !HasAPICode(err, 2) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("空内层视为空对象", func(t *testing.T) {
		got, err := UnwrapBaseRoleData(map[string]any{"data": ""})
		if err != nil {
			t.Fatal(err)
		}
		if m, ok := got.(map[string]any); !ok || len(m) != 0 {
			t.Errorf("got %v", got)
		}
	})
	t.Run("非 JSON 字符串原样保留", func(t *testing.T) {
		in := map[string]any{"data": "{not json"}
		got, err := UnwrapBaseRoleData(in)
		if err != nil {
			t.Fatal(err)
		}
		if m := got.(map[string]any); m["data"] != "{not json" {
			t.Errorf("got %v", got)
		}
	})
}
