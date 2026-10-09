package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/riba2534/feishu-cli/internal/auth"
	"github.com/riba2534/feishu-cli/internal/config"
)

func TestAuthLogout_RemovesBakWhenMainMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FEISHU_APP_ID", "cli_app")
	t.Setenv("FEISHU_APP_SECRET", "secret_x")
	config.SetBotFlagCredentials("", "")
	dir := filepath.Join(home, ".feishu-cli")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(auth.TokenStore{AccessToken: "from-bak", AppID: "cli_app"})
	if err := os.WriteFile(filepath.Join(dir, "token.json.bak"), raw, 0600); err != nil {
		t.Fatal(err)
	}

	_, _, err := runCLI(t, "auth", "logout", "--no-revoke")
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	got, err := auth.LoadTokenFrom(filepath.Join(dir, "token.json"))
	if err != nil || got != nil {
		t.Fatalf("logout 后不得从 .bak 复活: %+v err=%v", got, err)
	}
}
