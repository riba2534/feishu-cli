package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/internal/converter"
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

func TestValidateMarkdownEncoding(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		wantErr bool
	}{
		{name: "valid utf8", content: []byte("# 标题\n内容"), wantErr: false},
		{name: "invalid utf8", content: []byte{0xff, 0xfe, 0xfd}, wantErr: true},
		{name: "replacement char is valid utf8", content: []byte("乱码�内容"), wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMarkdownEncoding(tt.content)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateMarkdownEncoding() error = %v, wantErr = %v", err, tt.wantErr)
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

func TestAppendVideoTasksIncludesNestedVideosInTreeOrder(t *testing.T) {
	topVideo := videoNode("top.mp4")
	grid := &converter.BlockNode{
		Block: blockWithType(converter.BlockTypeGrid),
		Children: []*converter.BlockNode{
			videoNode("nested-a.mp4"),
			{
				Block: blockWithType(converter.BlockTypeQuoteContainer),
				Children: []*converter.BlockNode{
					videoNode("nested-b.mp4"),
				},
			},
		},
	}

	tasks := appendVideoTasks(nil,
		[]*converter.BlockNode{topVideo, grid},
		[]string{"top-id", "grid-id"},
		map[int][]createdBlockNode{
			1: {
				{node: grid.Children[0], blockID: "nested-a-id"},
				{node: grid.Children[1], blockID: "quote-id"},
				{node: grid.Children[1].Children[0], blockID: "nested-b-id"},
			},
		},
		[]string{"./top.mp4", "./nested-a.mp4", "./nested-b.mp4"},
		"/tmp",
	)

	if len(tasks) != 3 {
		t.Fatalf("len(tasks) = %d, want 3", len(tasks))
	}
	wantIDs := []string{"top-id", "nested-a-id", "nested-b-id"}
	wantSources := []string{"./top.mp4", "./nested-a.mp4", "./nested-b.mp4"}
	for i := range tasks {
		if tasks[i].fileBlockID != wantIDs[i] || tasks[i].source != wantSources[i] {
			t.Fatalf("task[%d] = {id:%q source:%q}, want {id:%q source:%q}",
				i, tasks[i].fileBlockID, tasks[i].source, wantIDs[i], wantSources[i])
		}
	}
}

func TestAppendImageTasksIncludesNestedImagesInTreeOrder(t *testing.T) {
	topImage := blockNodeWithType(converter.BlockTypeImage)
	grid := &converter.BlockNode{
		Block: blockWithType(converter.BlockTypeGrid),
		Children: []*converter.BlockNode{
			blockNodeWithType(converter.BlockTypeImage),
			{Block: blockWithType(converter.BlockTypeText)},
		},
	}

	tasks := appendImageTasks(nil,
		[]*converter.BlockNode{topImage, grid},
		[]string{"top-id", "grid-id"},
		map[int][]createdBlockNode{
			1: {
				{node: grid.Children[0], blockID: "nested-image-id"},
				{node: grid.Children[1], blockID: "nested-text-id"},
			},
		},
		[]string{"./top.png", "./nested.png"},
		"/tmp",
	)

	if len(tasks) != 2 {
		t.Fatalf("len(tasks) = %d, want 2", len(tasks))
	}
	wantIDs := []string{"top-id", "nested-image-id"}
	wantSources := []string{"./top.png", "./nested.png"}
	for i := range tasks {
		if tasks[i].imageBlockID != wantIDs[i] || tasks[i].source != wantSources[i] {
			t.Fatalf("task[%d] = {id:%q source:%q}, want {id:%q source:%q}",
				i, tasks[i].imageBlockID, tasks[i].source, wantIDs[i], wantSources[i])
		}
	}
}

func TestAppendImageTasksSkipsTokenImages(t *testing.T) {
	tokenImage := imageNode("img_existing")
	localImage := blockNodeWithType(converter.BlockTypeImage)

	tasks := appendImageTasks(nil,
		[]*converter.BlockNode{tokenImage, localImage},
		[]string{"existing-id", "local-id"},
		nil,
		[]string{"./local.png"},
		"/tmp",
	)

	if len(tasks) != 1 {
		t.Fatalf("len(tasks) = %d, want 1", len(tasks))
	}
	if tasks[0].imageBlockID != "local-id" || tasks[0].source != "./local.png" {
		t.Fatalf("task = {id:%q source:%q}, want local image binding", tasks[0].imageBlockID, tasks[0].source)
	}
}

// TestProcessVideoTaskUsesMultipartOverUploadAllLimit 超过 20MB 的视频改走分片上传（此前直接拒绝）。
func TestProcessVideoTaskUsesMultipartOverUploadAllLimit(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok","tenant_access_token":"t-test","expire":7200}`))
			return
		}
		paths = append(paths, r.URL.Path)
		// 让 prepare 失败即可证明走了分片通道，且不会回落到 upload_all
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":1061002,"msg":"params error"}`))
	}))
	defer server.Close()
	initDocUpdateTestConfig(t, server.URL)

	baseDir := t.TempDir()
	videoPath := filepath.Join(baseDir, "large.mp4")
	f, err := os.Create(videoPath)
	if err != nil {
		t.Fatalf("create video: %v", err)
	}
	if err := f.Truncate(20*1024*1024 + 1); err != nil {
		_ = f.Close()
		t.Fatalf("truncate video: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close video: %v", err)
	}

	result := processVideoTask("doc-token", videoTask{
		index:       1,
		fileBlockID: "block-id",
		source:      "large.mp4",
		basePath:    baseDir,
	}, false, "")

	if result.success {
		t.Fatal("prepare 失败时不应成功")
	}
	if len(paths) == 0 || paths[0] != "/open-apis/drive/v1/medias/upload_prepare" {
		t.Fatalf(">20MB 视频应先调用 upload_prepare，实际请求: %v", paths)
	}
	for _, p := range paths {
		if strings.HasSuffix(p, "/upload_all") {
			t.Fatalf(">20MB 视频不应走 upload_all: %v", paths)
		}
	}
}

func videoNode(name string) *converter.BlockNode {
	blockType := int(converter.BlockTypeFile)
	return &converter.BlockNode{
		Block: &larkdocx.Block{
			BlockType: &blockType,
			File:      &larkdocx.File{Name: &name},
		},
	}
}

func blockNodeWithType(blockType converter.BlockType) *converter.BlockNode {
	return &converter.BlockNode{Block: blockWithType(blockType)}
}

func blockWithType(blockType converter.BlockType) *larkdocx.Block {
	bt := int(blockType)
	return &larkdocx.Block{BlockType: &bt}
}

func imageNode(token string) *converter.BlockNode {
	blockType := int(converter.BlockTypeImage)
	return &converter.BlockNode{
		Block: &larkdocx.Block{
			BlockType: &blockType,
			Image: &larkdocx.Image{
				Token: &token,
			},
		},
	}
}
