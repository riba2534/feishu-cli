package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestCreateFileVersionNumericStatus 服务端创建成功时 status 返回数字（文档写 string），
// 不能把已成功的创建报成失败（用户重试会建出重复版本）。
func TestCreateFileVersionNumericStatus(t *testing.T) {
	var method, path, auth string
	var body map[string]any
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		method, path, auth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"msg":"success","data":{"name":"v1.0","version":"fnJfyX","parent_token":"doxcnTest","owner_id":"ou_x","creator_id":"ou_x","create_time":"1700000000","update_time":1700000001,"status":0,"obj_type":"docx","parent_type":"docx"}}`)
	})
	defer cleanup()

	v, err := CreateFileVersion("doxcnTest", "docx", "v1.0", "u-test")
	if err != nil {
		t.Fatalf("数字 status 的成功响应不应报错: %v", err)
	}
	if method != http.MethodPost || path != "/open-apis/drive/v1/files/doxcnTest/versions" || auth != "Bearer u-test" {
		t.Fatalf("请求不对: %s %s auth=%q", method, path, auth)
	}
	if body["name"] != "v1.0" || body["obj_type"] != "docx" {
		t.Fatalf("请求体不对: %v", body)
	}
	if v.Name != "v1.0" || v.Version != "fnJfyX" || v.Status != "0" || v.UpdateTime != "1700000001" || v.ObjType != "docx" {
		t.Fatalf("解析结果不对: %+v", v)
	}
}

func TestCreateFileVersionStringStatusAndAPIError(t *testing.T) {
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "bad") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"code":1061002,"msg":"params error."}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"name":"v2","version":"ver2","status":"1"}}`)
	})
	defer cleanup()

	v, err := CreateFileVersion("doxcnOK", "docx", "v2", "u-test")
	if err != nil || v.Status != "1" || v.Version != "ver2" {
		t.Fatalf("字符串 status 解析失败: %+v, %v", v, err)
	}
	_, err = CreateFileVersion("doxcnbad", "docx", "v2", "u-test")
	if err == nil || !HasAPICode(err, 1061002) || !strings.Contains(err.Error(), "创建文件版本") {
		t.Fatalf("业务错误应保留错误码与动作: %v", err)
	}
}

func TestListAndGetFileVersionNumericStatus(t *testing.T) {
	var queries []string
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Path+"?"+r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/versions") {
			_, _ = fmt.Fprint(w, `{"code":0,"data":{"has_more":true,"page_token":"pt2","items":[{"name":"a","version":"v1","status":0},{"name":"b","version":"v2","status":"2"}]}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"name":"a","version":"v1","creator_id":"ou_c","status":0}}`)
	})
	defer cleanup()

	list, next, more, err := ListFileVersions("doxcnL", "docx", 20, "pt1", "u-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Status != "0" || list[1].Status != "2" || next != "pt2" || !more {
		t.Fatalf("list 解析不对: %+v next=%q more=%v", list, next, more)
	}
	got, err := GetFileVersion("doxcnL", "v1", "docx", "u-test")
	if err != nil || got.Status != "0" || got.CreatorID != "ou_c" {
		t.Fatalf("get 解析不对: %+v, %v", got, err)
	}
	want := []string{
		"/open-apis/drive/v1/files/doxcnL/versions?obj_type=docx&page_size=20&page_token=pt1",
		"/open-apis/drive/v1/files/doxcnL/versions/v1?obj_type=docx",
	}
	if strings.Join(queries, "\n") != strings.Join(want, "\n") {
		t.Fatalf("请求路径 = %v, want %v", queries, want)
	}
}

func TestFlexStringUnmarshal(t *testing.T) {
	var got struct {
		A, B, C, D, E flexString
	}
	if err := json.Unmarshal([]byte(`{"A":"x","B":0,"C":12.5,"D":null,"E":true}`), &got); err != nil {
		t.Fatal(err)
	}
	if got.A != "x" || got.B != "0" || got.C != "12.5" || got.D != "" || got.E != "true" {
		t.Fatalf("flexString = %+v", got)
	}
	var bad struct{ A flexString }
	if err := json.Unmarshal([]byte(`{"A":{"x":1}}`), &bad); err == nil {
		t.Fatal("对象不应被当作字符串")
	}
}
