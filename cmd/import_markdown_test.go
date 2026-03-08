package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateWorkerCount(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		wantErr bool
	}{
		{name: "positive", value: 1, wantErr: false},
		{name: "zero", value: 0, wantErr: true},
		{name: "negative", value: -1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWorkerCount("image-workers", tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateWorkerCount() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestResolveImageSourceLocal(t *testing.T) {
	baseDir := t.TempDir()
	imagePath := filepath.Join(baseDir, "local-image.png")
	if err := os.WriteFile(imagePath, []byte("png"), 0644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	localPath, fileName, cleanup, err := resolveImageSource("local-image.png", baseDir)
	if err != nil {
		t.Fatalf("resolveImageSource() error = %v", err)
	}
	defer cleanup()

	if localPath != imagePath {
		t.Fatalf("localPath = %q, want %q", localPath, imagePath)
	}
	if fileName != "local-image.png" {
		t.Fatalf("fileName = %q, want %q", fileName, "local-image.png")
	}
}

func TestResolveImageSourceHTTPURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("fake-png-data"))
	}))
	defer srv.Close()

	source := srv.URL + "/nested/logo.png?x=1"
	localPath, fileName, cleanup, err := resolveImageSource(source, "")
	if err != nil {
		t.Fatalf("resolveImageSource() error = %v", err)
	}

	if fileName != "logo.png" {
		t.Fatalf("fileName = %q, want %q", fileName, "logo.png")
	}
	if _, err := os.Stat(localPath); err != nil {
		t.Fatalf("downloaded file missing: %v", err)
	}

	cleanup()
	if _, err := os.Stat(localPath); !os.IsNotExist(err) {
		t.Fatalf("cleanup did not remove temp file, stat err = %v", err)
	}
}

func TestResolveImageSourceHTTPURLWithoutPathName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("fake-png-data"))
	}))
	defer srv.Close()

	localPath, fileName, cleanup, err := resolveImageSource(srv.URL, "")
	if err != nil {
		t.Fatalf("resolveImageSource() error = %v", err)
	}
	defer cleanup()

	if fileName != "image.png" {
		t.Fatalf("fileName = %q, want %q", fileName, "image.png")
	}
	if filepath.Ext(localPath) != ".png" {
		t.Fatalf("temp file ext = %q, want %q", filepath.Ext(localPath), ".png")
	}
}
