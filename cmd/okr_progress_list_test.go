package cmd

import (
	"testing"
)

// TestOKRProgressListCmdRegistered 验证 okr progress list 子命令注册
func TestOKRProgressListCmdRegistered(t *testing.T) {
	if okrProgressListCmd.Use != "list" {
		t.Fatalf("okrProgressListCmd.Use = %q, want list", okrProgressListCmd.Use)
	}
	if okrProgressListCmd.Short == "" {
		t.Fatal("okrProgressListCmd.Short should not be empty")
	}
	// 应注册到 okrProgressCmd 下
	found := false
	for _, sub := range okrProgressCmd.Commands() {
		if sub == okrProgressListCmd {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("okrProgressListCmd should be registered as child of okrProgressCmd")
	}
}

// TestOKRProgressListFlags 校验关键 flag 注册
func TestOKRProgressListFlags(t *testing.T) {
	for _, want := range []string{"objective-id", "key-result-id", "user-id-type", "output"} {
		if okrProgressListCmd.Flags().Lookup(want) == nil {
			t.Errorf("--%s flag missing", want)
		}
	}
	userIDType := okrProgressListCmd.Flags().Lookup("user-id-type")
	if userIDType == nil || userIDType.DefValue != "open_id" {
		t.Fatalf("--user-id-type default = %v, want open_id", userIDType)
	}
}

// TestOKRProgressListUserIDTypeValidationViaPickTarget 复用 pickOKRTarget 验空校验
func TestOKRProgressListUserIDTypeValidationViaPickTarget(t *testing.T) {
	// pickOKRTarget 是 list/create 共用，再做一次冒烟保证 list 路径也走通
	if _, _, err := pickOKRTarget("", ""); err == nil {
		t.Fatal("pickOKRTarget should error when both empty (list flow)")
	}
	if _, _, err := pickOKRTarget("obj1", ""); err != nil {
		t.Fatalf("pickOKRTarget should succeed for obj1, got %v", err)
	}
}
