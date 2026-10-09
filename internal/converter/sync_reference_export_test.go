package converter

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
	"github.com/riba2534/feishu-cli/internal/client"
)

func syncReferenceBlock(id, sourceDocumentID, sourceBlockID string) *larkdocx.Block {
	return &larkdocx.Block{
		BlockId:   strPtr(id),
		BlockType: intPtr(int(BlockTypeSyncReference)),
		ReferenceSynced: &larkdocx.ReferenceSynced{
			SourceDocumentId: strPtr(sourceDocumentID),
			SourceBlockId:    strPtr(sourceBlockID),
		},
	}
}

func sourceSyncBlocks(sourceBlockID, content string) []*larkdocx.Block {
	return []*larkdocx.Block{
		{
			BlockId:   strPtr(sourceBlockID),
			BlockType: intPtr(int(BlockTypeSyncSource)),
			Children:  []string{"source-text"},
		},
		createTextBlock("source-text", content),
	}
}

func TestSyncReferenceExpandsCrossDocumentSource(t *testing.T) {
	var calls int
	provider := func(documentID, blockID, userAccessToken string) ([]*larkdocx.Block, error) {
		calls++
		if documentID != "source-doc" || blockID != "source-block" || userAccessToken != "u-test" {
			t.Fatalf("provider 参数异常: document=%q block=%q token=%q", documentID, blockID, userAccessToken)
		}
		return sourceSyncBlocks("source-block", "跨文档同步内容"), nil
	}

	conv := NewBlockToMarkdown([]*larkdocx.Block{
		syncReferenceBlock("reference", "source-doc", "source-block"),
	}, ConvertOptions{UserAccessToken: "u-test", SyncBlockProvider: provider})
	markdown, err := conv.Convert()
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("provider 调用次数 = %d，期望 1", calls)
	}
	if !strings.Contains(markdown, "跨文档同步内容") {
		t.Fatalf("导出未包含同步内容:\n%s", markdown)
	}
	if strings.Contains(markdown, "同步块内容未展开") {
		t.Fatalf("成功展开不应输出降级占位:\n%s", markdown)
	}
}

func TestSyncReferenceFailureProducesPlaceholderAndDiagnostic(t *testing.T) {
	var diagnostics bytes.Buffer
	conv := NewBlockToMarkdown([]*larkdocx.Block{
		syncReferenceBlock("reference", "private-doc", "private-block"),
	}, ConvertOptions{
		SyncBlockProvider: func(string, string, string) ([]*larkdocx.Block, error) {
			return nil, errors.New("code=1770032 permission denied")
		},
	})
	conv.diagnostics = &diagnostics

	markdown, err := conv.Convert()
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	for _, want := range []string{"[!WARNING]", "同步块内容未展开", "source_document_id=private-doc", "source_block_id=private-block"} {
		if !strings.Contains(markdown, want) {
			t.Errorf("降级输出缺少 %q:\n%s", want, markdown)
		}
	}
	if got := diagnostics.String(); !strings.Contains(got, "permission denied") || !strings.Contains(got, "同步块展开失败") {
		t.Fatalf("诊断信息不完整: %q", got)
	}
}

func TestSyncReferenceMissingSourceMetadataIsNeverSilent(t *testing.T) {
	var diagnostics bytes.Buffer
	conv := NewBlockToMarkdown([]*larkdocx.Block{{
		BlockId:         strPtr("broken-reference"),
		BlockType:       intPtr(int(BlockTypeSyncReference)),
		ReferenceSynced: &larkdocx.ReferenceSynced{},
	}}, ConvertOptions{})
	conv.diagnostics = &diagnostics

	markdown, err := conv.Convert()
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	if !strings.Contains(markdown, "同步块内容未展开") || !strings.Contains(markdown, "source_document_id=未知") {
		t.Fatalf("缺少源信息时不得静默输出空内容:\n%s", markdown)
	}
	if !strings.Contains(diagnostics.String(), "缺少 source_document_id/source_block_id") {
		t.Fatalf("缺少源信息时应输出诊断: %q", diagnostics.String())
	}
}

func TestSyncReferenceCachesRepeatedSource(t *testing.T) {
	var calls int
	provider := func(string, string, string) ([]*larkdocx.Block, error) {
		calls++
		return sourceSyncBlocks("shared-source", "复用内容"), nil
	}
	conv := NewBlockToMarkdown([]*larkdocx.Block{
		syncReferenceBlock("reference-1", "source-doc", "shared-source"),
		syncReferenceBlock("reference-2", "source-doc", "shared-source"),
	}, ConvertOptions{SyncBlockProvider: provider})

	markdown, err := conv.Convert()
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("相同源块应只请求一次，实际 %d 次", calls)
	}
	if got := strings.Count(markdown, "复用内容"); got != 2 {
		t.Fatalf("两个引用都应输出内容，实际 %d 次:\n%s", got, markdown)
	}
}

func TestSyncReferenceStopsCycle(t *testing.T) {
	var calls int
	provider := func(string, string, string) ([]*larkdocx.Block, error) {
		calls++
		return []*larkdocx.Block{
			{
				BlockId:   strPtr("cycle-source"),
				BlockType: intPtr(int(BlockTypeSyncSource)),
				Children:  []string{"cycle-reference"},
			},
			syncReferenceBlock("cycle-reference", "cycle-doc", "cycle-source"),
		}, nil
	}
	var diagnostics bytes.Buffer
	conv := NewBlockToMarkdown([]*larkdocx.Block{
		syncReferenceBlock("outer-reference", "cycle-doc", "cycle-source"),
	}, ConvertOptions{SyncBlockProvider: provider})
	conv.diagnostics = &diagnostics

	markdown, err := conv.Convert()
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("循环引用不得再次请求源块，实际 %d 次", calls)
	}
	if !strings.Contains(markdown, "检测到循环引用") {
		t.Fatalf("循环引用应输出明确占位:\n%s", markdown)
	}
	if !strings.Contains(diagnostics.String(), "循环引用") {
		t.Fatalf("循环引用应输出诊断: %q", diagnostics.String())
	}
}

func TestSyncReferenceListContentKeepsOuterCommonMarkIndent(t *testing.T) {
	remoteBlocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("list-source"),
			BlockType: intPtr(int(BlockTypeSyncSource)),
			Children:  []string{"remote-ordered"},
		},
		{
			BlockId:   strPtr("remote-ordered"),
			BlockType: intPtr(int(BlockTypeOrdered)),
			Ordered: &larkdocx.Text{
				Elements: listText("remote item").Elements,
				Style:    &larkdocx.TextStyle{Sequence: strPtr("10")},
			},
			Children: []string{"remote-bullet"},
		},
		{
			BlockId:   strPtr("remote-bullet"),
			BlockType: intPtr(int(BlockTypeBullet)),
			Bullet:    listText("remote leaf"),
		},
	}
	destinationBlocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("outer-list"),
			BlockType: intPtr(int(BlockTypeOrdered)),
			Ordered: &larkdocx.Text{
				Elements: listText("outer item").Elements,
				Style:    &larkdocx.TextStyle{Sequence: strPtr("1")},
			},
			Children: []string{"reference"},
		},
		syncReferenceBlock("reference", "source-doc", "list-source"),
	}
	conv := NewBlockToMarkdown(destinationBlocks, ConvertOptions{
		SyncBlockProvider: func(string, string, string) ([]*larkdocx.Block, error) {
			return remoteBlocks, nil
		},
	})

	markdown, err := conv.Convert()
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	want := "1. outer item\n   10. remote item\n       - remote leaf\n"
	if markdown != want {
		t.Fatalf("同步块列表未保持外层 CommonMark 缩进:\n--- got ---\n%s--- want ---\n%s", markdown, want)
	}
}

func TestSyncReferenceMediaUsesSourceDocumentContext(t *testing.T) {
	sourceBlocks := []*larkdocx.Block{
		{
			BlockId:   strPtr("media-source"),
			BlockType: intPtr(int(BlockTypeSyncSource)),
			Children:  []string{"source-image", "source-video", "source-board"},
		},
		{
			BlockId:   strPtr("source-image"),
			BlockType: intPtr(int(BlockTypeImage)),
			Image:     &larkdocx.Image{Token: strPtr("image-token")},
		},
		{
			BlockId:   strPtr("source-video"),
			BlockType: intPtr(int(BlockTypeFile)),
			File:      &larkdocx.File{Token: strPtr("video-token"), Name: strPtr("clip.mp4")},
		},
		{
			BlockId:   strPtr("source-board"),
			BlockType: intPtr(int(BlockTypeBoard)),
			Board:     &larkdocx.Board{Token: strPtr("board-token")},
		},
	}
	conv := NewBlockToMarkdown([]*larkdocx.Block{
		syncReferenceBlock("reference", "source-doc", "media-source"),
	}, ConvertOptions{
		DocumentID:      "destination-doc",
		UserAccessToken: "u-test",
		DownloadImages:  true,
		AssetsDir:       t.TempDir(),
		SyncBlockProvider: func(string, string, string) ([]*larkdocx.Block, error) {
			return sourceBlocks, nil
		},
	})

	mediaDocuments := make(map[string]string)
	var boardToken, boardUserToken string
	conv.services = &blockToMarkdownServices{
		getMediaTempURL: func(token string, opts client.DownloadMediaOptions) (string, error) {
			mediaDocuments[token] = opts.DocToken
			return "https://example.com/" + token, nil
		},
		downloadFromURL: func(string, string) error { return nil },
		downloadMedia: func(string, string, client.DownloadMediaOptions) error {
			t.Fatal("临时 URL 下载成功后不应调用 SDK 下载兜底")
			return nil
		},
		getBoardImage: func(token, outputPath, userAccessToken string) (string, error) {
			boardToken = token
			boardUserToken = userAccessToken
			return outputPath + ".png", nil
		},
	}

	markdown, err := conv.Convert()
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	for _, token := range []string{"image-token", "video-token"} {
		if got := mediaDocuments[token]; got != "source-doc" {
			t.Errorf("%s 使用的 DocToken = %q，期望源文档 source-doc", token, got)
		}
	}
	if boardToken != "board-token" || boardUserToken != "u-test" {
		t.Errorf("画板下载上下文异常: token=%q user_token=%q", boardToken, boardUserToken)
	}
	for _, want := range []string{"image_1.png", "clip.mp4", "board_2.png"} {
		if !strings.Contains(markdown, want) {
			t.Errorf("媒体导出缺少 %q:\n%s", want, markdown)
		}
	}
}
