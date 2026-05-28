package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestUpdateFloatImageOffsetZero 验证 offset-x/offset-y 用 *float64 指针表达「是否更新」：
// 传 0 指针时 PATCH body 必须出现 "offset_x":0（哨兵 bug 修复，0 是合法偏移值）。
func TestUpdateFloatImageOffsetZero(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"float_image":{"float_image_id":"fi1"}}}`)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	zero := 0.0
	img := FloatImage{Range: "sht1!B2:B2"}
	_, err := UpdateFloatImage(context.Background(), "shtcn1", "sht1", "fi1",
		&img, &zero, nil, "u-test")
	if err != nil {
		t.Fatalf("UpdateFloatImage error: %v", err)
	}
	if !strings.Contains(gotBody, `"offset_x":0`) {
		t.Errorf("PATCH body 缺 offset_x:0（哨兵 bug 未修复）: %s", gotBody)
	}
	// offset_y 传 nil 时不应出现在 body
	if strings.Contains(gotBody, "offset_y") {
		t.Errorf("offset_y 未传不应出现在 body: %s", gotBody)
	}
}

// TestUpdateFloatImageOffsetNilOmitted 验证 offset-x/offset-y 都传 nil 时 body 不含 offset 字段。
func TestUpdateFloatImageOffsetNilOmitted(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"float_image":{"float_image_id":"fi1"}}}`)
	}))
	defer srv.Close()
	setupTestConfig(t, srv.URL)

	img := FloatImage{Width: 200, Height: 150}
	_, err := UpdateFloatImage(context.Background(), "shtcn1", "sht1", "fi1", &img, nil, nil, "u-test")
	if err != nil {
		t.Fatalf("UpdateFloatImage error: %v", err)
	}
	if strings.Contains(gotBody, "offset_x") || strings.Contains(gotBody, "offset_y") {
		t.Errorf("offset 均未传不应出现在 body: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"width":200`) {
		t.Errorf("width 哨兵 (>0) 应写入 body: %s", gotBody)
	}
}
