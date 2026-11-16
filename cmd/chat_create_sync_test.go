package cmd

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestChatCreateSyncOfflineDryRun(t *testing.T) {
	command := &cobra.Command{}
	values := map[string]string{"name": "同步测试", "from-chat-id": "123456789", "last-messages": "3", "message-ids": "", "idempotency-key": "test", "dry-run": "true", "web-session-file": filepath.Join(t.TempDir(), "absent.json")}
	for _, name := range []string{"name", "from-chat-id", "message-ids", "idempotency-key", "web-session-file"} {
		command.Flags().String(name, "", "")
	}
	command.Flags().Int("last-messages", 0, "")
	command.Flags().Bool("dry-run", false, "")
	for name, value := range values {
		mustSetFlag(t, command, name, value)
	}
	output := captureStdout(t, func() {
		if err := chatCreateSyncCmd.RunE(command, nil); err != nil {
			t.Fatal(err)
		}
	})
	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil || result["dry_run"] != true {
		t.Fatalf("dry-run = %q, %v", output, err)
	}
	mustSetFlag(t, command, "message-ids", "111111111")
	if err := chatCreateSyncCmd.RunE(command, nil); err == nil || exitCodeFor(err) != 2 {
		t.Fatalf("invalid selection = %v", err)
	}
}
