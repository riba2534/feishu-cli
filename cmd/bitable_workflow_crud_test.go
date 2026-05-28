package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestWorkflowCreateDryRunPath(t *testing.T) {
	initTestConfig(t)
	out, err := captureRunE(t, bitableWorkflowCreateCmd, map[string]string{
		"base-token": "bascn1", "config": `{"title":"W","steps":[]}`, "dry-run": "true",
	})
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if !strings.Contains(out, `"method": "POST"`) ||
		!strings.Contains(out, "/open-apis/base/v3/bases/bascn1/workflows") {
		t.Errorf("workflow create 路径/方法不对: %s", out)
	}
	if strings.Contains(out, "bitable/v1") {
		t.Errorf("workflow create 不应走 bitable/v1: %s", out)
	}
	if !strings.Contains(out, `"title": "W"`) {
		t.Errorf("workflow create body 应含 title: %s", out)
	}
}

func TestWorkflowGetDryRunPath(t *testing.T) {
	initTestConfig(t)
	out, err := captureRunE(t, bitableWorkflowGetCmd, map[string]string{
		"base-token": "bascn1", "workflow-id": "wkfABC", "user-id-type": "open_id", "dry-run": "true",
	})
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if !strings.Contains(out, `"method": "GET"`) ||
		!strings.Contains(out, "/open-apis/base/v3/bases/bascn1/workflows/wkfABC") {
		t.Errorf("workflow get 路径/方法不对: %s", out)
	}
	if !strings.Contains(out, `"user_id_type": "open_id"`) {
		t.Errorf("workflow get 应带 user_id_type query: %s", out)
	}
}

func TestWorkflowUpdateDryRunPathIsPUT(t *testing.T) {
	initTestConfig(t)
	out, err := captureRunE(t, bitableWorkflowUpdateCmd, map[string]string{
		"base-token": "bascn1", "workflow-id": "wkfABC", "config": `{"title":"New"}`, "dry-run": "true",
	})
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	// update 是 PUT（整体替换），不是 PATCH
	if !strings.Contains(out, `"method": "PUT"`) {
		t.Errorf("workflow update 应为 PUT: %s", out)
	}
	if !strings.Contains(out, "/open-apis/base/v3/bases/bascn1/workflows/wkfABC") {
		t.Errorf("workflow update 路径不对: %s", out)
	}
}

func TestWorkflowGetRequiresID(t *testing.T) {
	initTestConfig(t)
	// 用独立命令实例，避免共享全局命令的 flag 跨用例残留。
	cmd := &cobra.Command{Use: "get", RunE: bitableWorkflowGetCmd.RunE}
	addBitableWriteFlags(cmd)
	cmd.Flags().String("workflow-id", "", "")
	cmd.Flags().String("user-id-type", "", "")

	_, err := captureRunE(t, cmd, map[string]string{
		"base-token": "bascn1", "dry-run": "true",
	})
	if err == nil {
		t.Error("缺 --workflow-id 应报错")
	}
}
