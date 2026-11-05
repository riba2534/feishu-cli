package client

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

const unsupportedRichResp = `{"code":0,"msg":"ok","data":{"value_ranges":[{"range":"s1!B2:C4","values":[
	[[{"type":"text","text":{"text":"名称"}}],[{"type":"text","text":{"text":"优先级"}}]],
	[[{"type":"text","text":{"text":"任务一"}}],[{"type":"#UNSUPPORTED_TYPE"}]],
	[[{"type":"text","text":{"text":"任务二"}}],[{"type":"#UNSUPPORTED_TYPE"}]]
]}]}}`

func captureSheetsWarn(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	old := sheetsWarnWriter
	sheetsWarnWriter = buf
	t.Cleanup(func() { sheetsWarnWriter = old })
	return buf
}

// TestReadTable_BackfillUnsupportedCells 下拉框单元格（#UNSUPPORTED_TYPE）用 batch_get_plain 纯文本回填，不再读成 null。
func TestReadTable_BackfillUnsupportedCells(t *testing.T) {
	warn := captureSheetsWarn(t)
	rec := &sheetsRecorder{}
	var plainBody string
	newSheetsServer(t, rec, func(w http.ResponseWriter, r *http.Request, body []byte) {
		if strings.HasSuffix(r.URL.Path, "/batch_get_plain") {
			plainBody = string(body)
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"value_ranges":[{"range":"s1!B2:C4","values":[["名称","优先级"],["任务一","P1"],["任务二","P2"]]}]}}`)
			return
		}
		_, _ = io.WriteString(w, unsupportedRichResp)
	})
	res, err := ReadTable(context.Background(), "shtTok", "s1", "B2:C4", false, "u-test")
	if err != nil {
		t.Fatal(err)
	}
	sh := res.Sheets[0]
	if sh.Data[0][1] != "P1" || sh.Data[1][1] != "P2" {
		t.Fatalf("下拉框单元格应回填为纯文本，得到 %v", sh.Data)
	}
	if sh.Dtypes["优先级"] != "string" {
		t.Errorf("dtype = %q", sh.Dtypes["优先级"])
	}
	if !strings.Contains(plainBody, `"s1!B2:C4"`) {
		t.Errorf("回填应读取同一范围，请求体 %s", plainBody)
	}
	if warn.Len() != 0 {
		t.Errorf("回填成功不应告警: %s", warn.String())
	}
}

// TestReadTable_BackfillFailureWarns 回填失败时 stderr 列出单元格地址，读取本身仍成功（不改退出码）。
func TestReadTable_BackfillFailureWarns(t *testing.T) {
	warn := captureSheetsWarn(t)
	rec := &sheetsRecorder{}
	newSheetsServer(t, rec, func(w http.ResponseWriter, r *http.Request, body []byte) {
		if strings.HasSuffix(r.URL.Path, "/batch_get_plain") {
			_, _ = io.WriteString(w, `{"code":1310214,"msg":"no permission","data":{}}`)
			return
		}
		_, _ = io.WriteString(w, unsupportedRichResp)
	})
	res, err := ReadTable(context.Background(), "shtTok", "s1", "B2:C4", false, "u-test")
	if err != nil {
		t.Fatalf("回填失败不应让读取失败: %v", err)
	}
	if res.Sheets[0].Data[0][1] != nil {
		t.Errorf("回填失败时保持空值，得到 %v", res.Sheets[0].Data[0][1])
	}
	got := warn.String()
	if !strings.Contains(got, "C3, C4") || !strings.Contains(got, "回填失败") {
		t.Errorf("告警应列出单元格地址 C3, C4: %q", got)
	}
}

// TestReadTable_BackfillMissingCellWarns 纯文本结果缺少对应单元格时只告警缺失的那些。
func TestReadTable_BackfillMissingCellWarns(t *testing.T) {
	warn := captureSheetsWarn(t)
	old := readPlainForBackfill
	readPlainForBackfill = func(ctx context.Context, tok, sid string, ranges []string, uat ...string) ([]*CellRange, error) {
		return []*CellRange{{Range: ranges[0], Values: [][]any{{"名称", "优先级"}, {"任务一", "P1"}}}}, nil
	}
	t.Cleanup(func() { readPlainForBackfill = old })
	rec := &sheetsRecorder{}
	newSheetsServer(t, rec, func(w http.ResponseWriter, r *http.Request, body []byte) {
		_, _ = io.WriteString(w, unsupportedRichResp)
	})
	res, err := ReadTable(context.Background(), "shtTok", "s1", "B2:C4", false, "u-test")
	if err != nil {
		t.Fatal(err)
	}
	if res.Sheets[0].Data[0][1] != "P1" || res.Sheets[0].Data[1][1] != nil {
		t.Errorf("data = %v", res.Sheets[0].Data)
	}
	if got := warn.String(); !strings.Contains(got, "C4") || strings.Contains(got, "C3") {
		t.Errorf("只应告警缺失的 C4: %q", got)
	}
}

// TestReadTable_NoUnsupportedNoPlainRequest 没有 #UNSUPPORTED_TYPE 时不多发纯文本请求。
func TestReadTable_NoUnsupportedNoPlainRequest(t *testing.T) {
	rec := &sheetsRecorder{}
	plainCalls := 0
	newSheetsServer(t, rec, func(w http.ResponseWriter, r *http.Request, body []byte) {
		if strings.HasSuffix(r.URL.Path, "/batch_get_plain") {
			plainCalls++
		}
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"value_ranges":[{"range":"s1!A1:A2","values":[[[{"type":"text","text":{"text":"h"}}]],[[{"type":"text","text":{"text":"v"}}]]]}]}}`)
	})
	if _, err := ReadTable(context.Background(), "shtTok", "s1", "A1:A2", false, "u-test"); err != nil {
		t.Fatal(err)
	}
	if plainCalls != 0 {
		t.Errorf("无不支持类型时不应请求 batch_get_plain，实际 %d 次", plainCalls)
	}
}

func TestUnsupportedCellAddrs(t *testing.T) {
	grid := &CellRangeV3{Range: "", Values: [][][]*CellElement{
		{nil, {{Type: UnsupportedCellType}}},
		{{{Type: UnsupportedCellType}}, textCell("x")},
	}}
	_, addrs := UnsupportedCellAddrs(grid, "s1!AA10:AB11")
	if strings.Join(addrs, ",") != "AB10,AA11" {
		t.Errorf("addrs = %v", addrs)
	}
}

// 纯文本读取返回的范围起点与原范围不同时，按下标回填会错位：告警且不回填。
func TestReadTable_BackfillRangeMismatchSkips(t *testing.T) {
	warn := captureSheetsWarn(t)
	rec := &sheetsRecorder{}
	newSheetsServer(t, rec, func(w http.ResponseWriter, r *http.Request, body []byte) {
		if strings.HasSuffix(r.URL.Path, "/batch_get_plain") {
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"value_ranges":[{"range":"s1!B3:C4","values":[["任务一","P1"],["任务二","P2"]]}]}}`)
			return
		}
		_, _ = io.WriteString(w, unsupportedRichResp)
	})
	res, err := ReadTable(context.Background(), "shtTok", "s1", "B2:C4", false, "u-test")
	if err != nil {
		t.Fatal(err)
	}
	sh := res.Sheets[0]
	if sh.Data[0][1] == "任务二" || sh.Data[0][1] == "P2" {
		t.Fatalf("范围起点不同不得错位回填，得到 %v", sh.Data)
	}
	if !strings.Contains(warn.String(), "起点") || !strings.Contains(warn.String(), "C3") {
		t.Fatalf("应告警并列出单元格地址: %s", warn.String())
	}
}
