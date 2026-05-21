package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/riba2534/feishu-cli/internal/client"
)

// TestOKRCycleListCmdRegistered 验证 okr cycle list 子命令注册
func TestOKRCycleListCmdRegistered(t *testing.T) {
	if okrCycleListCmd.Use != "list" {
		t.Fatalf("okrCycleListCmd.Use = %q, want list", okrCycleListCmd.Use)
	}
	if okrCycleListCmd.Short == "" {
		t.Fatal("okrCycleListCmd.Short should not be empty")
	}
	if !strings.Contains(okrCycleListCmd.Long, "OKR") {
		t.Fatalf("okrCycleListCmd.Long should mention OKR, got %q", okrCycleListCmd.Long)
	}
	found := false
	for _, sub := range okrCycleCmd.Commands() {
		if sub == okrCycleListCmd {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("okrCycleListCmd should be registered as child of okrCycleCmd")
	}
}

// TestOKRCycleListFlags 校验 output flag
func TestOKRCycleListFlags(t *testing.T) {
	output := okrCycleListCmd.Flags().Lookup("output")
	if output == nil {
		t.Fatal("--output flag missing")
	}
	if okrCycleListCmd.Flags().ShorthandLookup("o") == nil {
		t.Fatal("--output should have -o shorthand")
	}
}

// TestValidateUserIDType 校验 OKR user-id-type 取值校验
func TestValidateUserIDType(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"open_id", false},
		{"union_id", false},
		{"user_id", false},
		{"employee_id", true},
		{"", true},
		{"random_str", true},
	}
	for _, tt := range tests {
		err := validateUserIDType(tt.input)
		if (err != nil) != tt.wantErr {
			t.Fatalf("validateUserIDType(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
	}
}

// TestOKRCycleJSONMarshal 校验 client.OKRCycle 结构体 JSON marshal 字段名一致
func TestOKRCycleJSONMarshal(t *testing.T) {
	c := client.OKRCycle{
		ID:          "p1",
		ZhName:      "Q2",
		EnName:      "Q2",
		StartTime:   "2026-04-01",
		EndTime:     "2026-06-30",
		CycleStatus: "normal",
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("json.Marshal err: %v", err)
	}
	s := string(data)
	for _, want := range []string{`"id":"p1"`, `"zh_name":"Q2"`, `"start_time":"2026-04-01"`, `"cycle_status":"normal"`} {
		if !strings.Contains(s, want) {
			t.Errorf("marshal output missing %q\n got: %s", want, s)
		}
	}
}
