package client

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestDtypeToTypeFormat(t *testing.T) {
	cases := []struct {
		dtype      string
		wantType   TableColType
		wantFormat string
	}{
		{"", TableColTypeString, "@"},
		{"object", TableColTypeString, "@"},
		{"string", TableColTypeString, "@"},
		{"int64", TableColTypeNumber, ""},
		{"Int64", TableColTypeNumber, ""}, // nullable pandas
		{"uint8", TableColTypeNumber, ""},
		{"float64", TableColTypeNumber, ""},
		{"complex128", TableColTypeNumber, ""},
		{"bool", TableColTypeBool, ""},
		{"boolean", TableColTypeBool, ""},
		{"datetime64[ns]", TableColTypeDate, "yyyy-mm-dd"},
		{"datetime64[ns, UTC]", TableColTypeDate, "yyyy-mm-dd"},
	}
	for _, c := range cases {
		gotType, gotFormat := dtypeToTypeFormat(c.dtype)
		if gotType != c.wantType || gotFormat != c.wantFormat {
			t.Errorf("dtypeToTypeFormat(%q) = (%q,%q), want (%q,%q)", c.dtype, gotType, gotFormat, c.wantType, c.wantFormat)
		}
	}
}

func TestIsoDateToSerial(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"2024-01-15", 45306}, // 官方验证值
		{"1899-12-30", 0},
		{"1900-01-01", 2},
		// 时分秒保留为小数部分（旧实现截掉 T 之后的部分，table-get → table-put 丢时间）
		{"2024-01-15T08:30:00", 45306 + 8.5/24},
		{"2024-01-15T12:00:00", 45306.5},
		{"2024-01-15 12:00:00", 45306.5}, // 空格分隔同样接受
		{"2024-01-15T12:00", 45306.5},    // 省略秒
		{"2024-01-15T23:59:59", 45306 + 86399.0/86400},
		{"2024-01-15T00:00:00.000+08:00", 45306}, // 带时区：保留墙上时间
		{"2024-01-15T06:00:00+08:00", 45306.25},
		{"2024-01-15T06:00:00Z", 45306.25},
	}
	for _, c := range cases {
		got, err := isoDateToSerial(c.in)
		if err != nil {
			t.Errorf("isoDateToSerial(%q) 意外错误: %v", c.in, err)
			continue
		}
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("isoDateToSerial(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	if _, err := isoDateToSerial("not-a-date"); err == nil {
		t.Error("非法日期应报错")
	}
	if _, err := isoDateToSerial("2024/01/15"); err == nil {
		t.Error("非 ISO 格式应报错")
	}
}

func TestParseTablePutPayload_OK(t *testing.T) {
	raw := `{"sheets":[{"name":"s1","columns":["id","amt","dt","flag"],"data":[["A",1.5,"2024-01-15",true]],"dtypes":{"amt":"float64","dt":"datetime64[ns]","flag":"bool"},"formats":{"amt":"#,##0.00"}}]}`
	p, err := ParseTablePutPayload([]byte(raw))
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(p.Sheets) != 1 {
		t.Fatalf("期望 1 sheet，得到 %d", len(p.Sheets))
	}
	s := p.Sheets[0]
	if s.Name != "s1" {
		t.Errorf("name = %q", s.Name)
	}
	if len(s.Columns) != 4 {
		t.Fatalf("期望 4 列，得到 %d", len(s.Columns))
	}
	// 列 1 amt：number + format 覆盖为 #,##0.00
	if s.Columns[1].Type != TableColTypeNumber || s.Columns[1].Format != "#,##0.00" {
		t.Errorf("amt 列规格错误: %+v", s.Columns[1])
	}
	// 列 2 dt：date + yyyy-mm-dd
	if s.Columns[2].Type != TableColTypeDate || s.Columns[2].Format != "yyyy-mm-dd" {
		t.Errorf("dt 列规格错误: %+v", s.Columns[2])
	}
	// 列 0 id：无 dtype → string + @
	if s.Columns[0].Type != TableColTypeString || s.Columns[0].Format != "@" {
		t.Errorf("id 列规格错误: %+v", s.Columns[0])
	}
}

func TestParseTablePutPayload_Errors(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		expect string
	}{
		{"空 sheets", `{"sheets":[]}`, "至少需要一个"},
		{"重复列名", `{"sheets":[{"columns":["a","a"],"data":[]}]}`, "重复列名"},
		{"列数不一致", `{"sheets":[{"columns":["a","b"],"data":[[1]]}]}`, "列数"},
		{"dtypes 引用不存在列", `{"sheets":[{"columns":["a"],"data":[],"dtypes":{"x":"int"}}]}`, "不存在的列"},
		{"多余顶层内容", `{"sheets":[]} trailing`, "多余"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseTablePutPayload([]byte(c.raw))
			if err == nil {
				t.Fatalf("期望报错含 %q", c.expect)
			}
			if !strings.Contains(err.Error(), c.expect) {
				t.Errorf("错误 %q 不含 %q", err.Error(), c.expect)
			}
		})
	}
}

func TestBuildTypedCell(t *testing.T) {
	// string 列：数字串保文本，走 text 元素（不是 value），不带 cell_styles
	cell, err := BuildTypedCell(TableColSpec{Name: "id", Type: TableColTypeString, Format: "@"}, json.RawMessage(`"007"`))
	if err != nil {
		t.Fatalf("string 列 007 失败: %v", err)
	}
	if cell.Type != "text" || cell.Text == nil || cell.Text.Text != "007" {
		t.Errorf("string 列应是 text 元素且值为 007，得到 %+v", cell)
	}
	if cell.Value != nil {
		t.Errorf("string 列不应使用 value 元素（会被服务端拒收）")
	}

	// F4：string 列拿未加引号的大整数，须保精度（不经 float64 → 无科学计数法）
	cell, _ = BuildTypedCell(TableColSpec{Name: "id", Type: TableColTypeString, Format: "@"}, json.RawMessage(`9007199254740993`))
	if cell.Text == nil || cell.Text.Text != "9007199254740993" {
		t.Errorf("string 列大整数精度丢失: %+v", cell.Text)
	}

	// F4：string 列拿 JSON object，须保留原始 JSON 文本而非 Go 语法 map[...]
	cell, _ = BuildTypedCell(TableColSpec{Name: "j", Type: TableColTypeString, Format: "@"}, json.RawMessage(`{"a":1}`))
	if cell.Text == nil || cell.Text.Text != `{"a":1}` {
		t.Errorf("string 列 object 应保留 JSON 文本，得到 %q", cell.Text.Text)
	}

	// number 列：保精度大整数，走 value 元素
	cell, _ = BuildTypedCell(TableColSpec{Name: "n", Type: TableColTypeNumber}, json.RawMessage(`9007199254740993`))
	if cell.Type != "value" || cell.Value == nil || cell.Value.Value != "9007199254740993" {
		t.Errorf("number 大整数精度丢失或元素类型错误: %+v", cell)
	}

	// date 列：写 Excel 序列号，走 value 元素，不带 cell_styles
	cell, _ = BuildTypedCell(TableColSpec{Name: "d", Type: TableColTypeDate, Format: "yyyy-mm-dd"}, json.RawMessage(`"2024-01-15"`))
	if cell.Type != "value" || cell.Value == nil || cell.Value.Value != "45306" {
		t.Errorf("date 序列号错误: %+v", cell)
	}

	// bool 列：走 text 元素（不是 value）
	cell, _ = BuildTypedCell(TableColSpec{Name: "b", Type: TableColTypeBool}, json.RawMessage(`true`))
	if cell.Type != "text" || cell.Text == nil || cell.Text.Text != "TRUE" {
		t.Errorf("bool true → text TRUE 错误: %+v", cell)
	}

	// F6：null → 空文本元素（不是 nil，否则命令层会写空数组导致整批 500）
	cell, _ = BuildTypedCell(TableColSpec{Name: "x", Type: TableColTypeString, Format: "@"}, json.RawMessage(`null`))
	if cell == nil || cell.Type != "text" || cell.Text == nil || cell.Text.Text != "" {
		t.Errorf("null 应返回空文本元素，得到 %+v", cell)
	}

	// number 列传字符串报错
	_, err = BuildTypedCell(TableColSpec{Name: "n", Type: TableColTypeNumber}, json.RawMessage(`"abc"`))
	if err == nil {
		t.Error("number 列传非数字应报错")
	}
}

func TestFormatterForType(t *testing.T) {
	cases := []struct {
		name string
		col  TableColSpec
		want string
	}{
		{"date 默认 → yyyy/MM/dd", TableColSpec{Type: TableColTypeDate, Format: "yyyy-mm-dd"}, "yyyy/MM/dd"},
		{"date 空 Format → yyyy/MM/dd", TableColSpec{Type: TableColTypeDate, Format: ""}, "yyyy/MM/dd"},
		{"date 自定义格式透传", TableColSpec{Type: TableColTypeDate, Format: "yyyy年m月d日"}, "yyyy年m月d日"},
		{"string @ 透传", TableColSpec{Type: TableColTypeString, Format: "@"}, "@"},
		{"number 自定义透传", TableColSpec{Type: TableColTypeNumber, Format: "#,##0.00"}, "#,##0.00"},
		{"number 无格式 → 空", TableColSpec{Type: TableColTypeNumber, Format: ""}, ""},
		{"bool 无格式 → 空", TableColSpec{Type: TableColTypeBool, Format: ""}, ""},
	}
	for _, c := range cases {
		if got := FormatterForType(c.col); got != c.want {
			t.Errorf("%s: FormatterForType = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestIsNumericDtype_ExcludesInterval(t *testing.T) {
	// F5：interval 以 "int" 开头但不是数值列，须按文本（string）处理
	for _, dtype := range []string{"interval", "interval[int64, right]", "Interval[float64]"} {
		if typ, _ := dtypeToTypeFormat(dtype); typ != TableColTypeString {
			t.Errorf("dtypeToTypeFormat(%q) = %q, want string（interval 不是数值列）", dtype, typ)
		}
	}
	// 真数值 dtype 仍判 number
	for _, dtype := range []string{"int64", "uint8", "float64", "complex128"} {
		if typ, _ := dtypeToTypeFormat(dtype); typ != TableColTypeNumber {
			t.Errorf("dtypeToTypeFormat(%q) = %q, want number", dtype, typ)
		}
	}
}
