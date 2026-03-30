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

func TestIsGridDivOpen(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{`<div style="display: flex; gap: 16px;">`, true},
		{`<div style="display:flex;gap:16px;">`, true},
		{`<DIV STYLE="DISPLAY: FLEX;">`, true},
		{`<div style="flex: 40;">`, false},
		{`<div class="other">`, false},
		{`<p>text</p>`, false},
		{`just text`, false},
	}
	for _, tt := range tests {
		got := isGridDivOpen(tt.input)
		if got != tt.want {
			t.Errorf("isGridDivOpen(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseGridColumns(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
		checks  func(t *testing.T, cols []gridColumn)
	}{
		{
			name: "basic 2 columns with ratios",
			input: `<div style="display: flex; gap: 16px;">
<div style="flex: 40;">

Left content

</div>
<div style="flex: 60;">

Right content

</div>
</div>`,
			wantLen: 2,
			checks: func(t *testing.T, cols []gridColumn) {
				if cols[0].widthRatio != 40 {
					t.Errorf("col[0].widthRatio = %d, want 40", cols[0].widthRatio)
				}
				if cols[0].content != "Left content" {
					t.Errorf("col[0].content = %q, want %q", cols[0].content, "Left content")
				}
				if cols[1].widthRatio != 60 {
					t.Errorf("col[1].widthRatio = %d, want 60", cols[1].widthRatio)
				}
				if cols[1].content != "Right content" {
					t.Errorf("col[1].content = %q, want %q", cols[1].content, "Right content")
				}
			},
		},
		{
			name: "columns without explicit ratio default to 1",
			input: `<div style="display: flex; gap: 16px;">
<div style="flex: 1;">

Content A

</div>
<div style="flex: 1;">

Content B

</div>
</div>`,
			wantLen: 2,
			checks: func(t *testing.T, cols []gridColumn) {
				if cols[0].widthRatio != 1 {
					t.Errorf("col[0].widthRatio = %d, want 1", cols[0].widthRatio)
				}
				if cols[1].widthRatio != 1 {
					t.Errorf("col[1].widthRatio = %d, want 1", cols[1].widthRatio)
				}
			},
		},
		{
			name: "3 columns",
			input: `<div style="display: flex; gap: 16px;">
<div style="flex: 33;">

Col 1

</div>
<div style="flex: 34;">

Col 2

</div>
<div style="flex: 33;">

Col 3

</div>
</div>`,
			wantLen: 3,
			checks: func(t *testing.T, cols []gridColumn) {
				if cols[0].widthRatio != 33 || cols[1].widthRatio != 34 || cols[2].widthRatio != 33 {
					t.Errorf("ratios = %d/%d/%d", cols[0].widthRatio, cols[1].widthRatio, cols[2].widthRatio)
				}
			},
		},
		{
			name:    "empty div",
			input:   `<div style="display: flex;"></div>`,
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cols := parseGridColumns(tt.input)
			if len(cols) != tt.wantLen {
				t.Fatalf("parseGridColumns() returned %d columns, want %d", len(cols), tt.wantLen)
			}
			if tt.checks != nil {
				tt.checks(t, cols)
			}
		})
	}
}

func TestParseMarkdownSegmentsGrid(t *testing.T) {
	input := `# Title

Some text

<div style="display: flex; gap: 16px;">
<div style="flex: 40;">

Left column

</div>
<div style="flex: 60;">

Right column

</div>
</div>

More text after grid
`
	segs := parseMarkdownSegments(input)

	var kinds []string
	for _, s := range segs {
		kinds = append(kinds, s.kind)
	}

	hasGrid := false
	for _, s := range segs {
		if s.kind == "grid" {
			hasGrid = true
			if !isGridDivOpen(s.content[:50]) {
				t.Errorf("grid segment should start with grid div open tag")
			}
		}
	}
	if !hasGrid {
		t.Fatalf("expected a grid segment, got kinds: %v", kinds)
	}
}
