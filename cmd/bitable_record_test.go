package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// newSearchTestCmd 构造一个注册了 record search 全部 flag 的命令，供测试 buildRecordSearchBody。
func newSearchTestCmd(t *testing.T) *cobra.Command {
	t.Helper()
	c := &cobra.Command{Use: "search"}
	c.Flags().String("config", "", "")
	c.Flags().String("config-file", "", "")
	c.Flags().String("keyword", "", "")
	c.Flags().StringSlice("search-field", nil, "")
	c.Flags().String("filter-json", "", "")
	c.Flags().String("sort-json", "", "")
	c.Flags().String("view-id", "", "")
	c.Flags().Int("offset", 0, "")
	c.Flags().Int("limit", 10, "")
	return c
}

func TestBuildRecordSearchBody_KeywordMode(t *testing.T) {
	c := newSearchTestCmd(t)
	_ = c.Flags().Set("keyword", "测试")
	_ = c.Flags().Set("search-field", "名称")
	body, err := buildRecordSearchBody(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := body.(map[string]any)
	if m["keyword"] != "测试" {
		t.Errorf("keyword = %v, want 测试", m["keyword"])
	}
	sf, _ := m["search_fields"].([]string)
	if len(sf) != 1 || sf[0] != "名称" {
		t.Errorf("search_fields = %v, want [名称]", m["search_fields"])
	}
	if m["limit"].(int) != 10 {
		t.Errorf("limit = %v, want 10", m["limit"])
	}
}

// TestBuildRecordSearchBody_PureFilterMode 验证 issue #170 的核心场景：
// 纯结构化过滤（无 keyword）也能用，不必硬塞 keyword。
func TestBuildRecordSearchBody_PureFilterMode(t *testing.T) {
	c := newSearchTestCmd(t)
	_ = c.Flags().Set("filter-json", `{"logic":"and","conditions":[["名称","==","测试"]]}`)
	body, err := buildRecordSearchBody(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := body.(map[string]any)
	if _, ok := m["filter"]; !ok {
		t.Errorf("filter missing in body: %v", m)
	}
	if _, hasKw := m["keyword"]; hasKw {
		t.Errorf("keyword should not be set in pure filter mode")
	}
}

func TestBuildRecordSearchBody_SortAndPaging(t *testing.T) {
	c := newSearchTestCmd(t)
	_ = c.Flags().Set("keyword", "x")
	_ = c.Flags().Set("search-field", "f1,f2")
	_ = c.Flags().Set("sort-json", `[{"field":"名称","desc":true}]`)
	_ = c.Flags().Set("offset", "20")
	_ = c.Flags().Set("limit", "50")
	body, err := buildRecordSearchBody(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := body.(map[string]any)
	if m["offset"].(int) != 20 {
		t.Errorf("offset = %v, want 20", m["offset"])
	}
	if m["limit"].(int) != 50 {
		t.Errorf("limit = %v, want 50", m["limit"])
	}
	if _, ok := m["sort"]; !ok {
		t.Errorf("sort missing: %v", m)
	}
	sf, _ := m["search_fields"].([]string)
	if len(sf) != 2 {
		t.Errorf("search_fields = %v, want 2 items", m["search_fields"])
	}
}

func TestBuildRecordSearchBody_ConfigPassthrough(t *testing.T) {
	c := newSearchTestCmd(t)
	_ = c.Flags().Set("config", `{"keyword":"x","search_fields":["f"]}`)
	body, err := buildRecordSearchBody(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m := body.(map[string]any)
	if m["keyword"] != "x" {
		t.Errorf("config passthrough failed: %v", m)
	}
}

// TestBuildRecordSearchBody_FieldsMisuseGuidance 验证 issue #170 的根因修复：
// 用户误传 upsert 的 {"fields":{...}} 格式时，给出明确指引而非裸 800010701。
func TestBuildRecordSearchBody_FieldsMisuseGuidance(t *testing.T) {
	c := newSearchTestCmd(t)
	_ = c.Flags().Set("config", `{"fields":{"名称":"测试"}}`)
	_, err := buildRecordSearchBody(c)
	if err == nil {
		t.Fatal("expect error for fields misuse, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"upsert", "keyword"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error msg missing %q: %s", want, msg)
		}
	}
}

func TestBuildRecordSearchBody_ConfigAndConvenienceMutex(t *testing.T) {
	c := newSearchTestCmd(t)
	_ = c.Flags().Set("config", `{"keyword":"x"}`)
	_ = c.Flags().Set("keyword", "y")
	_, err := buildRecordSearchBody(c)
	if err == nil {
		t.Fatal("expect mutex error, got nil")
	}
}

func TestBuildRecordSearchBody_NoInput(t *testing.T) {
	c := newSearchTestCmd(t)
	_, err := buildRecordSearchBody(c)
	if err == nil {
		t.Fatal("expect error when no input, got nil")
	}
}

func TestBuildRecordSearchBody_KeywordWithoutSearchField(t *testing.T) {
	c := newSearchTestCmd(t)
	_ = c.Flags().Set("keyword", "x")
	_, err := buildRecordSearchBody(c)
	if err == nil {
		t.Fatal("expect error when keyword without search-field, got nil")
	}
}

func TestBuildRecordSearchBody_LimitOutOfRange(t *testing.T) {
	c := newSearchTestCmd(t)
	_ = c.Flags().Set("filter-json", `{"logic":"and","conditions":[]}`)
	_ = c.Flags().Set("limit", "300")
	_, err := buildRecordSearchBody(c)
	if err == nil {
		t.Fatal("expect error for limit>200, got nil")
	}
}
