package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// TestBuildTypedCell_DateTimeKeepsTime table-get 输出的日期时间经 table-put 写回不丢时分秒
// （旧实现截掉 T 之后的部分，08:30 写回变成 00:00）。
func TestBuildTypedCell_DateTimeKeepsTime(t *testing.T) {
	col := TableColSpec{Name: "ts", Type: TableColTypeDate, Format: "yyyy-mm-dd"}
	cases := map[string]string{
		`"2024-01-15T08:30:00"`: "45306.354166666664",
		`"2024-01-15T12:00:00"`: "45306.5",
		`"2024-01-15"`:          "45306",
	}
	for raw, want := range cases {
		cell, err := BuildTypedCell(col, json.RawMessage(raw))
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if cell.Type != "value" || cell.Value == nil || cell.Value.Value != want {
			t.Errorf("BuildTypedCell(%s) = %+v, want value %s", raw, cell.Value, want)
		}
	}
}

// TestBuildTypedCell_BlankAndNumericStrings 数字/日期列空串写空单元格；数字列中恰为数字字面量的字符串按数字写。
func TestBuildTypedCell_BlankAndNumericStrings(t *testing.T) {
	num := TableColSpec{Name: "n", Type: TableColTypeNumber}
	date := TableColSpec{Name: "d", Type: TableColTypeDate, Format: "yyyy-mm-dd"}

	for _, c := range []struct {
		col TableColSpec
		raw string
	}{{num, `""`}, {num, `"  "`}, {date, `""`}, {date, `null`}} {
		cell, err := BuildTypedCell(c.col, json.RawMessage(c.raw))
		if err != nil {
			t.Fatalf("%s 列 %s 应写空单元格，得到错误: %v", c.col.Type, c.raw, err)
		}
		if cell.Type != "text" || cell.Text == nil || cell.Text.Text != "" {
			t.Errorf("%s 列 %s 应为空文本元素，得到 %+v", c.col.Type, c.raw, cell)
		}
	}

	cell, err := BuildTypedCell(num, json.RawMessage(`"100.5"`))
	if err != nil || cell.Type != "value" || cell.Value.Value != "100.5" {
		t.Errorf("数字字面量字符串应按数字写入: %+v %v", cell, err)
	}
	for _, bad := range []string{`"abc"`, `"1,234"`, `"null"`, `"NaN"`, `true`} {
		if _, err := BuildTypedCell(num, json.RawMessage(bad)); err == nil {
			t.Errorf("数字列 %s 应报错", bad)
		}
	}
}

// TestParseTablePutPayload_DetectsTimeOfDay date 列带时分秒时自动改用日期时间 formatter；
// 用户显式指定日期格式时尊重用户选择。
func TestParseTablePutPayload_DetectsTimeOfDay(t *testing.T) {
	raw := `{"sheets":[{"columns":["ts","d","d2"],"data":[["2024-01-15T08:30:00","2024-01-15","2024-01-15T09:00:00"],["2024-01-16T00:00:00","","2024-01-16"]],
		"dtypes":{"ts":"datetime64[ns]","d":"datetime64[ns]","d2":"datetime64[ns]"},"formats":{"d2":"yyyy-mm-dd"}}]}`
	p, err := ParseTablePutPayload([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	cols := p.Sheets[0].Columns
	if !cols[0].HasTime || cols[1].HasTime || !cols[2].HasTime || !cols[2].FormatExplicit {
		t.Fatalf("HasTime/FormatExplicit 推断错误: %+v", cols)
	}
	if got := FormatterForType(cols[0]); got != "yyyy/MM/dd HH:mm:ss" {
		t.Errorf("带时间的日期列 formatter = %q, want yyyy/MM/dd HH:mm:ss", got)
	}
	if got := FormatterForType(cols[1]); got != "yyyy/MM/dd" {
		t.Errorf("纯日期列 formatter = %q", got)
	}
	if got := FormatterForType(cols[2]); got != "yyyy/MM/dd" {
		t.Errorf("显式 yyyy-mm-dd 应保持纯日期 formatter，得到 %q", got)
	}
	if got := FormatterForType(TableColSpec{Type: TableColTypeDate, Format: TableDateTimeFormat, FormatExplicit: true}); got != "yyyy/MM/dd HH:mm:ss" {
		t.Errorf("table-get 输出的日期时间格式应映射为 V2 日期时间 formatter，得到 %q", got)
	}
}

// TestReadTable_DateTimeFormat table-get 对带时间的日期列输出 yyyy-mm-dd hh:mm:ss 格式与 ISO 日期时间值。
func TestReadTable_DateTimeFormat(t *testing.T) {
	rec := &sheetsRecorder{}
	newSheetsServer(t, rec, func(w http.ResponseWriter, r *http.Request, body []byte) {
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","data":{"value_ranges":[{"range":"s1!A1:B3","values":[
			[[{"type":"text","text":{"text":"ts"}}],[{"type":"text","text":{"text":"d"}}]],
			[[{"type":"date_time","date_time":{"date_time":"2024/01/15 08:30:00"}}],[{"type":"date_time","date_time":{"date_time":"2024/01/15"}}]],
			[[{"type":"date_time","date_time":{"date_time":"2024/02/20 23:59:59"}}],[{"type":"date_time","date_time":{"date_time":"2024/02/20"}}]]
		]}]}}`)
	})
	res, err := ReadTable(context.Background(), "shtTok", "s1", "A1:B3", false, "u-test")
	if err != nil {
		t.Fatal(err)
	}
	sh := res.Sheets[0]
	if sh.Formats["ts"] != TableDateTimeFormat || sh.Formats["d"] != "yyyy-mm-dd" {
		t.Errorf("formats = %v", sh.Formats)
	}
	if sh.Data[0][0] != "2024-01-15T08:30:00" || sh.Data[1][0] != "2024-02-20T23:59:59" || sh.Data[0][1] != "2024-01-15" {
		t.Errorf("data = %v", sh.Data)
	}

	// round-trip：table-get 输出直接喂给 table-put，序列号保留时间
	out, _ := json.Marshal(res)
	p, err := ParseTablePutPayload(out)
	if err != nil {
		t.Fatal(err)
	}
	cell, err := BuildTypedCell(p.Sheets[0].Columns[0], p.Sheets[0].Rows[0][0])
	if err != nil || cell.Value == nil || cell.Value.Value != "45306.354166666664" {
		t.Errorf("round-trip 序列号 = %+v %v", cell, err)
	}
	if got := FormatterForType(p.Sheets[0].Columns[0]); got != "yyyy/MM/dd HH:mm:ss" {
		t.Errorf("round-trip formatter = %q", got)
	}
}
