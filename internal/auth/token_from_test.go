package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTokenFrom(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	got, err := LoadTokenFrom(path)
	if err != nil || got != nil {
		t.Fatalf("missing file: got=%v err=%v", got, err)
	}
	if err := os.WriteFile(path, []byte(`{"access_token":"u-from-path","expires_at":"2099-01-01T00:00:00Z"}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = LoadTokenFrom(path)
	if err != nil {
		t.Fatalf("LoadTokenFrom: %v", err)
	}
	if got == nil || got.AccessToken != "u-from-path" {
		t.Errorf("token = %+v", got)
	}
}
