package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/v2/internal/converter"
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

// TestCollectMediaTasksFollowsMediaRefsInTreeOrder 资源补齐任务按 MediaRef（块指针）收集，
// 覆盖顶层与嵌套节点，附件取 View 块的 children[0] 作为上传目标，画板取建块响应里的新画板 token。
func TestCollectMediaTasksFollowsMediaRefsInTreeOrder(t *testing.T) {
	topImage := blockNodeWithType(converter.BlockTypeImage)
	nestedImage := blockNodeWithType(converter.BlockTypeImage)
	plainImage := blockNodeWithType(converter.BlockTypeImage) // 无 MediaRef：不应产生任务
	video := blockNodeWithType(converter.BlockTypeFile)
	file := blockNodeWithType(converter.BlockTypeFile)
	board := blockNodeWithType(converter.BlockTypeBoard)

	refs := map[*larkdocx.Block]*converter.MediaRef{
		topImage.Block:    {Kind: converter.MediaKindImage, Source: "./top.png"},
		nestedImage.Block: {Kind: converter.MediaKindImage, Token: "imgTok", Width: 120, Height: 80, Align: 2},
		video.Block:       {Kind: converter.MediaKindFile, Source: "./demo.mp4", Name: "demo.mp4", Video: true},
		file.Block:        {Kind: converter.MediaKindFile, Token: "fileTok", Name: "a.pdf"},
		board.Block:       {Kind: converter.MediaKindWhiteboard, Token: "srcBoard"},
	}
	viewType := int(converter.BlockTypeView)
	boardTok := "newBoard"
	created := []createdBlockNode{
		{node: topImage, blockID: "top-img", parentID: "doc"},
		{node: plainImage, blockID: "plain-img", parentID: "doc"},
		{node: nestedImage, blockID: "nested-img", parentID: "col-1"},
		{node: video, blockID: "view-1", parentID: "doc", created: &larkdocx.Block{BlockType: &viewType, Children: []string{"file-1"}}},
		{node: file, blockID: "view-2", parentID: "quote-1", created: &larkdocx.Block{BlockType: &viewType, Children: []string{"file-2"}}},
		{node: board, blockID: "board-blk", parentID: "doc", created: &larkdocx.Block{Board: &larkdocx.Board{Token: &boardTok}}},
	}

	var set mediaTaskSet
	collectMediaTasks(&set, refs, created, "/base")

	if len(set.images) != 2 {
		t.Fatalf("images = %d, want 2: %#v", len(set.images), set.images)
	}
	if set.images[0].imageBlockID != "top-img" || set.images[0].source != "./top.png" || set.images[0].reuseToken != "" {
		t.Fatalf("image[0] = %#v", set.images[0])
	}
	img := set.images[1]
	if img.imageBlockID != "nested-img" || img.parentID != "col-1" || img.source != "feishu://media/imgTok" ||
		img.reuseToken != "imgTok" || img.width != 120 || img.height != 80 || img.align != 2 {
		t.Fatalf("token 图片任务应复用素材并保留显示属性: %#v", img)
	}

	if len(set.files) != 2 || set.videoCount() != 1 {
		t.Fatalf("files = %#v", set.files)
	}
	if v := set.files[0]; v.fileBlockID != "file-1" || v.viewBlockID != "view-1" || !v.video || v.source != "./demo.mp4" || v.name != "demo.mp4" {
		t.Fatalf("视频任务应上传到 View 的子 File 块: %#v", v)
	}
	if f := set.files[1]; f.fileBlockID != "file-2" || f.parentID != "quote-1" || f.video || f.source != "feishu://media/fileTok" || f.failureKind() != "file" {
		t.Fatalf("附件任务异常: %#v", f)
	}

	if len(set.boards) != 1 || set.boards[0].whiteboardID != "newBoard" || set.boards[0].sourceToken != "srcBoard" || set.boards[0].blockID != "board-blk" {
		t.Fatalf("画板复制任务异常: %#v", set.boards)
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
