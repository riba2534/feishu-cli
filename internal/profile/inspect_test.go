package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadConfigFields(t *testing.T) {
	dir := t.TempDir()
	got, err := ReadConfigFields(dir)
	if err != nil {
		t.Fatalf("missing file: %v", err)
	}
	if got.AppID != "" || got.AppSecret != "" {
		t.Errorf("empty dir should yield zero fields, got %+v", got)
	}

	content := "app_id: cli_from_file\napp_secret: secret_xxx\nbase_url: https://open.larksuite.com/\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = ReadConfigFields(dir)
	if err != nil {
		t.Fatalf("ReadConfigFields: %v", err)
	}
	if got.AppID != "cli_from_file" {
		t.Errorf("AppID = %q", got.AppID)
	}
	if got.AppSecret != "secret_xxx" {
		t.Errorf("AppSecret should be present")
	}
	if got.BaseURL != "https://open.larksuite.com" {
		t.Errorf("BaseURL = %q (want trimmed slash)", got.BaseURL)
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("FEISHU_APP_ID", "cli_x")
	t.Setenv("FEISHU_APP_SECRET", "sec")
	t.Setenv(EnvVar, "work")
	got := EnvOverrides()
	if !got.AppID || !got.AppSecret || !got.Profile {
		t.Errorf("EnvOverrides = %+v, want all true", got)
	}
	t.Setenv("FEISHU_APP_ID", "")
	t.Setenv("FEISHU_APP_SECRET", "")
	t.Setenv(EnvVar, "")
	got = EnvOverrides()
	if got.AppID || got.AppSecret || got.Profile {
		t.Errorf("EnvOverrides = %+v, want all false", got)
	}
}
