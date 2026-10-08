package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUpdateDimensionAndMoveDimension_Bodies 更新行列属性（1 起始两端包含、支持 User Token）与 v3 移动行列（0 起始）。
func TestUpdateDimensionAndMoveDimension_Bodies(t *testing.T) {
	rec := &sheetsRecorder{}
	newSheetsServer(t, rec, nil)

	hidden := false
	size := 40
	if err := UpdateDimension(context.Background(), "shtTok", "s1", "ROWS", 2, 3, &hidden, &size, "u-test"); err != nil {
		t.Fatal(err)
	}
	req := rec.last()
	if req.Method != http.MethodPut || !strings.HasSuffix(req.Path, "/dimension_range") {
		t.Fatalf("请求 = %s %s", req.Method, req.Path)
	}
	var body map[string]map[string]any
	_ = json.Unmarshal(req.Body, &body)
	if body["dimension"]["startIndex"] != float64(2) || body["dimension"]["endIndex"] != float64(3) {
		t.Errorf("dimension = %v", body["dimension"])
	}
	if body["dimensionProperties"]["visible"] != false || body["dimensionProperties"]["fixedSize"] != float64(40) {
		t.Errorf("dimensionProperties = %v", body["dimensionProperties"])
	}
	if err := UpdateDimension(context.Background(), "shtTok", "s1", "ROWS", 0, 3, &hidden, nil, "u-test"); err == nil {
		t.Error("startIndex=0 应报错")
	}

	if err := MoveDimension(context.Background(), "shtTok", "s1", "ROWS", 1, 2, 4, "u-test"); err != nil {
		t.Fatal(err)
	}
	req = rec.last()
	if req.Method != http.MethodPost || req.Path != "/open-apis/sheets/v3/spreadsheets/shtTok/sheets/s1/move_dimension" {
		t.Fatalf("请求 = %s %s", req.Method, req.Path)
	}
	var mv struct {
		Source struct {
			Major string `json:"major_dimension"`
			Start int    `json:"start_index"`
			End   int    `json:"end_index"`
		} `json:"source"`
		Dest int `json:"destination_index"`
	}
	_ = json.Unmarshal(req.Body, &mv)
	if mv.Source.Major != "ROWS" || mv.Source.Start != 1 || mv.Source.End != 2 || mv.Dest != 4 {
		t.Errorf("move body = %+v", mv)
	}
}

// TestUpdateSheetProperties_ExplicitZeroAndFalse 取消隐藏 / 取消冻结 / 移到第一位必须真的发出 false / 0。
func TestUpdateSheetProperties_ExplicitZeroAndFalse(t *testing.T) {
	rec := &sheetsRecorder{}
	newSheetsServer(t, rec, func(w http.ResponseWriter, r *http.Request, body []byte) {
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"replies":[]}}`)
	})
	zero := 0
	f := false
	title := "新名字"
	if err := UpdateSheetProperties(context.Background(), "shtTok", &SheetPropertiesUpdate{
		SheetID: "s1", Title: &title, Index: &zero, Hidden: &f, FrozenRowCount: &zero, FrozenColCount: &zero,
	}, "u-test"); err != nil {
		t.Fatal(err)
	}
	raw := string(rec.last().Body)
	for _, want := range []string{`"updateSheet"`, `"sheetId":"s1"`, `"title":"新名字"`, `"index":0`, `"hidden":false`, `"frozenRowCount":0`, `"frozenColCount":0`} {
		if !strings.Contains(raw, want) {
			t.Errorf("请求体缺少 %s: %s", want, raw)
		}
	}
	if err := UpdateSheetProperties(context.Background(), "shtTok", &SheetPropertiesUpdate{SheetID: "s1"}); err == nil {
		t.Error("没有任何属性时应报错")
	}
}

// TestUploadSheetImageMediaAuto_Multipart >20MB 走 medias 分片上传，parent_type/parent_node 正确。
func TestUploadSheetImageMediaAuto_Multipart(t *testing.T) {
	payload := []byte(strings.Repeat("x", 25))
	path := filepath.Join(t.TempDir(), "big.png")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	orig := maxSingleUploadSize
	maxSingleUploadSize = len(payload) - 1
	defer func() { maxSingleUploadSize = orig }()

	rec := &sheetsRecorder{}
	newSheetsServer(t, rec, func(w http.ResponseWriter, r *http.Request, body []byte) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/medias/upload_prepare"):
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"upload_id":"up1","block_size":10,"block_num":3}}`)
		case strings.HasSuffix(r.URL.Path, "/medias/upload_part"):
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{}}`)
		case strings.HasSuffix(r.URL.Path, "/medias/upload_finish"):
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"file_token":"ftok"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"code":404,"msg":"unexpected"}`)
		}
	})
	tok, err := UploadSheetImageMediaAuto(path, "shtTok", "big.png", "u-test")
	if err != nil || tok != "ftok" {
		t.Fatalf("token=%q err=%v", tok, err)
	}
	var paths []string
	for _, r := range rec.reqs {
		paths = append(paths, r.Path[strings.LastIndex(r.Path, "/")+1:])
	}
	if strings.Join(paths, ",") != "upload_prepare,upload_part,upload_part,upload_part,upload_finish" {
		t.Errorf("请求序列 = %v", paths)
	}
	var prep map[string]any
	_ = json.Unmarshal(rec.reqs[0].Body, &prep)
	if prep["parent_type"] != "sheet_image" || prep["parent_node"] != "shtTok" || prep["size"] != float64(len(payload)) {
		t.Errorf("upload_prepare body = %v", prep)
	}
}
