package config

import (
	"os"
	"testing"
)

func TestFlagCredentialsBeatEnv(t *testing.T) {
	resetConfig()
	t.Setenv("HOME", t.TempDir())
	os.Setenv("FEISHU_APP_ID", "cli_env")
	os.Setenv("FEISHU_APP_SECRET", "sec_env")
	t.Cleanup(func() {
		os.Unsetenv("FEISHU_APP_ID")
		os.Unsetenv("FEISHU_APP_SECRET")
		resetConfig()
	})

	SetBotFlagCredentials("cli_flag", "sec_flag")
	if err := Init(""); err != nil {
		t.Fatal(err)
	}
	c := Get()
	if c.AppID != "cli_flag" {
		t.Errorf("AppID = %q, want cli_flag", c.AppID)
	}
	if c.AppSecret != "sec_flag" {
		t.Errorf("AppSecret = %q, want sec_flag", c.AppSecret)
	}
}

func TestFlagCredentialsPartial(t *testing.T) {
	resetConfig()
	t.Setenv("HOME", t.TempDir())
	os.Setenv("FEISHU_APP_ID", "cli_env")
	os.Setenv("FEISHU_APP_SECRET", "sec_env")
	t.Cleanup(func() {
		os.Unsetenv("FEISHU_APP_ID")
		os.Unsetenv("FEISHU_APP_SECRET")
		resetConfig()
	})

	SetBotFlagCredentials("cli_flag", "")
	if err := Init(""); err != nil {
		t.Fatal(err)
	}
	c := Get()
	if c.AppID != "cli_flag" {
		t.Errorf("AppID = %q, want cli_flag", c.AppID)
	}
	if c.AppSecret != "sec_env" {
		t.Errorf("AppSecret = %q, want env secret when flag omitted", c.AppSecret)
	}
}
