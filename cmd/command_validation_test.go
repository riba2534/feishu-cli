package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestValidateSendReceiveIDType(t *testing.T) {
	for _, valid := range []string{"email", "open_id", "user_id", "union_id", "chat_id", "thread_id"} {
		if err := validateSendReceiveIDType(valid); err != nil {
			t.Fatalf("validateSendReceiveIDType(%q) unexpected error: %v", valid, err)
		}
	}
	if err := validateSendReceiveIDType("bad"); err == nil || !strings.Contains(err.Error(), "--receive-id-type") {
		t.Fatalf("expected receive-id-type error, got %v", err)
	}
}

func TestValidateSendMessageType(t *testing.T) {
	for _, valid := range []string{"text", "post", "image", "file", "audio", "media", "sticker", "interactive", "share_chat", "share_user"} {
		if err := validateSendMessageType(valid); err != nil {
			t.Fatalf("validateSendMessageType(%q) unexpected error: %v", valid, err)
		}
	}
	if err := validateSendMessageType("bad"); err == nil || !strings.Contains(err.Error(), "--msg-type") {
		t.Fatalf("expected msg-type error, got %v", err)
	}
}

func TestValidateFilterViewID(t *testing.T) {
	for _, valid := range []string{"", "aB12345678"} {
		if err := validateFilterViewID(valid); err != nil {
			t.Fatalf("validateFilterViewID(%q) unexpected error: %v", valid, err)
		}
	}
	for _, invalid := range []string{"short", "12345678901", "abcde1234_"} {
		if err := validateFilterViewID(invalid); err == nil {
			t.Fatalf("validateFilterViewID(%q) expected error", invalid)
		}
	}
}

func TestReadSheetSpreadsheetTokenAlias(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("token", "", "")
	cmd.Flags().String("spreadsheet-token", "", "")
	if err := cmd.Flags().Set("spreadsheet-token", "sht_xxx"); err != nil {
		t.Fatal(err)
	}
	got, err := readSheetSpreadsheetToken(cmd)
	if err != nil {
		t.Fatalf("readSheetSpreadsheetToken() error = %v", err)
	}
	if got != "sht_xxx" {
		t.Fatalf("token = %q, want sht_xxx", got)
	}
	if err := cmd.Flags().Set("token", "sht_other"); err != nil {
		t.Fatal(err)
	}
	if _, err := readSheetSpreadsheetToken(cmd); err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestResolveMarkdownFileFlagAlias(t *testing.T) {
	got, err := resolveMarkdownFileFlag("", "a.md")
	if err != nil {
		t.Fatalf("resolveMarkdownFileFlag() error = %v", err)
	}
	if got != "a.md" {
		t.Fatalf("file = %q, want a.md", got)
	}
	if _, err := resolveMarkdownFileFlag("a.md", "b.md"); err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestValidateMsgFlagMessageID(t *testing.T) {
	if err := validateMsgFlagMessageID("om_xxx"); err != nil {
		t.Fatalf("validateMsgFlagMessageID() unexpected error: %v", err)
	}
	for _, invalid := range []string{"", "omt_xxx", "oc_xxx"} {
		if err := validateMsgFlagMessageID(invalid); err == nil {
			t.Fatalf("validateMsgFlagMessageID(%q) expected error", invalid)
		}
	}
}
