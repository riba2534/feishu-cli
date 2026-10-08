package client

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"
)

func mkTextElem(content string) *larkdocx.TextElement {
	c := content
	return &larkdocx.TextElement{TextRun: &larkdocx.TextRun{Content: &c}}
}

func TestDocWriteLimiter_AcquireBurst(t *testing.T) {
	ResetDocWriteLimiters()
	defer ResetDocWriteLimiters()

	docID := "test-doc-burst"
	ctx := context.Background()

	// 桶初始满（默认 burst=3），前 3 次 acquire 应该立即返回。
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := AcquireDocWriteSlot(ctx, docID); err != nil {
			t.Fatalf("acquire %d 失败: %v", i, err)
		}
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("burst 期间不应等待，但花了 %v", elapsed)
	}

	// 第 4 次必须等到下一个 token，约 1/3 秒（QPS=3）。
	start = time.Now()
	if err := AcquireDocWriteSlot(ctx, docID); err != nil {
		t.Fatalf("第 4 次 acquire 失败: %v", err)
	}
	wait := time.Since(start)
	// 下界 200ms（容忍调度抖动）；上界 500ms（防 limiter 误睡过久）。
	if wait < 200*time.Millisecond || wait > 500*time.Millisecond {
		t.Fatalf("第 4 次 acquire 等待时长 %v 不在 [200ms, 500ms] 区间", wait)
	}
}

func TestDocWriteLimiter_PerDocumentIsolation(t *testing.T) {
	ResetDocWriteLimiters()
	defer ResetDocWriteLimiters()

	ctx := context.Background()

	// 用尽 docA 的 burst 后，docB 应该不受影响。
	for i := 0; i < 3; i++ {
		if err := AcquireDocWriteSlot(ctx, "docA"); err != nil {
			t.Fatalf("docA acquire %d 失败: %v", i, err)
		}
	}
	start := time.Now()
	if err := AcquireDocWriteSlot(ctx, "docB"); err != nil {
		t.Fatalf("docB acquire 失败: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("docB 不应受 docA 影响，但等了 %v", elapsed)
	}
}

func TestDocWriteLimiter_EmptyDocIDIsNoop(t *testing.T) {
	ResetDocWriteLimiters()
	defer ResetDocWriteLimiters()

	// 空 documentID 应直接返回，不做任何节流。
	for i := 0; i < 100; i++ {
		if err := AcquireDocWriteSlot(context.Background(), ""); err != nil {
			t.Fatalf("空 docID acquire 失败: %v", err)
		}
	}
}

func TestDocWriteLimiter_ContextCanceled(t *testing.T) {
	ResetDocWriteLimiters()
	defer ResetDocWriteLimiters()

	docID := "test-ctx-cancel"
	bg := context.Background()
	// 把桶用完
	for i := 0; i < 3; i++ {
		if err := AcquireDocWriteSlot(bg, docID); err != nil {
			t.Fatalf("acquire %d 失败: %v", i, err)
		}
	}

	// 立刻取消的 ctx，下一次 acquire 应快速返回 ctx.Err()
	ctx, cancel := context.WithCancel(bg)
	cancel()
	start := time.Now()
	if err := AcquireDocWriteSlot(ctx, docID); err == nil {
		t.Fatal("已取消的 ctx 应该返回错误")
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("被取消的 acquire 应快速返回，但花了 %v", elapsed)
	}
}

func TestDocWriteLimiter_TokenRefill(t *testing.T) {
	ResetDocWriteLimiters()
	defer ResetDocWriteLimiters()

	docID := "test-refill"
	ctx := context.Background()

	// 用完桶
	for i := 0; i < 3; i++ {
		if err := AcquireDocWriteSlot(ctx, docID); err != nil {
			t.Fatalf("acquire %d 失败: %v", i, err)
		}
	}
	// 等 1 秒，应该有 3 个新 token
	time.Sleep(1100 * time.Millisecond)

	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := AcquireDocWriteSlot(ctx, docID); err != nil {
			t.Fatalf("refill 后 acquire %d 失败: %v", i, err)
		}
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("refill 后 3 次 acquire 不应等待，但花了 %v", elapsed)
	}
}

func TestSplitCellElements_SingleGroupNoBR(t *testing.T) {
	groups := splitCellElements([]*larkdocx.TextElement{mkTextElem("hello world")})
	if len(groups) != 1 {
		t.Fatalf("纯文本应返回 1 组，实际 %d", len(groups))
	}
	if groups[0].blockType != blockTypeText {
		t.Fatalf("纯文本应分类为 Text(2)，实际 %d", groups[0].blockType)
	}
}

func TestSplitCellElements_MultiGroupWithBR(t *testing.T) {
	groups := splitCellElements([]*larkdocx.TextElement{
		mkTextElem("first"),
		mkTextElem("\n"),
		mkTextElem("- bullet item"),
	})
	if len(groups) != 2 {
		t.Fatalf("含 \\n 元素应返回 2 组，实际 %d", len(groups))
	}
	if groups[1].blockType != blockTypeBullet {
		t.Fatalf("第二组以 '- ' 开头应分类为 Bullet(12)，实际 %d", groups[1].blockType)
	}
}

func TestSplitCellElements_NewlineMustBeStandalone(t *testing.T) {
	// content 中嵌入 \n 但不是单独元素 → 不应触发分组
	groups := splitCellElements([]*larkdocx.TextElement{mkTextElem("line1\nline2")})
	if len(groups) != 1 {
		t.Fatalf("嵌入式 \\n 不应分组，实际 %d 组", len(groups))
	}
}

func TestSplitCellElements_HeadingPrefix(t *testing.T) {
	groups := splitCellElements([]*larkdocx.TextElement{
		mkTextElem("intro"),
		mkTextElem("\n"),
		mkTextElem("## level2 heading"),
	})
	if len(groups) != 2 {
		t.Fatalf("应返回 2 组，实际 %d", len(groups))
	}
	if groups[1].blockType != 4 { // heading2 = 4
		t.Fatalf("'## ' 应分类为 heading2(4)，实际 %d", groups[1].blockType)
	}
	// 标题前缀应被剥离
	if got := *groups[1].elements[0].TextRun.Content; got != "level2 heading" {
		t.Fatalf("标题前缀应被剥离，实际 %q", got)
	}
}

func TestPartitionSingleCellTasks(t *testing.T) {
	tasks := []singleCellTask{
		{cellID: "c1", textBlockID: "tb1", index: 0},
		{cellID: "c2", textBlockID: "", index: 1},
		{cellID: "c3", textBlockID: "tb3", index: 2},
		{cellID: "c4", textBlockID: "", index: 3},
		{cellID: "c5", textBlockID: "tb5", index: 4},
	}
	batchable, fallback := partitionSingleCellTasks(tasks)
	if len(batchable) != 3 {
		t.Fatalf("有 textBlockID 的应入 batchable，期望 3 个，实际 %d", len(batchable))
	}
	if len(fallback) != 2 {
		t.Fatalf("缺 textBlockID 的应入 fallback，期望 2 个，实际 %d", len(fallback))
	}
	// 顺序保留
	if batchable[0].cellID != "c1" || batchable[1].cellID != "c3" || batchable[2].cellID != "c5" {
		t.Fatalf("batchable 顺序错误: %+v", batchable)
	}
	if fallback[0].cellID != "c2" || fallback[1].cellID != "c4" {
		t.Fatalf("fallback 顺序错误: %+v", fallback)
	}
}

func TestPartitionSingleCellTasks_Empty(t *testing.T) {
	batchable, fallback := partitionSingleCellTasks(nil)
	if len(batchable) != 0 || len(fallback) != 0 {
		t.Fatalf("空输入应返回空切片")
	}
}

func TestFillBatchSize_BoundaryDoesNotExceedAPILimit(t *testing.T) {
	// 飞书 batch_update 单次最多 200 个；本项目用 30 远低于上限。
	// 此测试只是钉死常量，避免不慎调高破坏 3 QPS 节流。
	if fillBatchSize > 200 {
		t.Fatalf("fillBatchSize=%d 超过飞书 API 上限 200", fillBatchSize)
	}
	if fillBatchSize <= 0 {
		t.Fatal("fillBatchSize 必须为正")
	}
}

// TestDocxLowLevelWritersAcquireDocWriteSlot 是元编程式守护测试：
// 扫描 internal/client/docx.go 的源码，断言飞书 docx 4 个底层写函数
// （CreateBlock / UpdateBlock / DeleteBlocks / BatchUpdateBlocks）的函数体
// 都包含 `AcquireDocWriteSlot(` 调用。下次有人新增写函数，记得也接入 limiter；
// 否则单文档 3 QPS 节流会被绕过（issue #159 review fix-3）。
func TestDocxLowLevelWritersAcquireDocWriteSlot(t *testing.T) {
	src, err := os.ReadFile("docx.go")
	if err != nil {
		t.Fatalf("read docx.go: %v", err)
	}

	// 必须经过 acquire 的函数列表。
	// 注意：上层 helper（如 InsertTableRow / AppendTableRows / fillCellSingleBlock）
	// 走这 4 个底层函数，自动被 limiter 覆盖；这里只盯死 4 个底层。
	required := []string{
		// CreateBlock 委托给 CreateBlockWithClientToken（携带幂等 client_token），限流在后者内完成
		"func CreateBlockWithClientToken(",
		"func UpdateBlock(",
		"func DeleteBlocks(",
		"func BatchUpdateBlocks(",
	}

	text := string(src)
	for _, sig := range required {
		idx := strings.Index(text, sig)
		if idx < 0 {
			t.Errorf("函数 %q 不存在于 docx.go", sig)
			continue
		}
		// 找函数体范围（从签名后第一个 `{` 到匹配的 `}`）
		braceStart := strings.Index(text[idx:], "{")
		if braceStart < 0 {
			t.Errorf("函数 %q 未找到 { ", sig)
			continue
		}
		body := extractBalancedBraceBody(text[idx+braceStart:])
		// 接受直接 AcquireDocWriteSlot 或经 acquireDocWriteSlotWithTimeout 封装——后者内部仍调
		// AcquireDocWriteSlot，但拿到 token 后立即 cancel context，避免超大文档导入时驻留大量 5min goroutine。
		if !strings.Contains(body, "AcquireDocWriteSlot(") && !strings.Contains(body, "acquireDocWriteSlotWithTimeout(") {
			t.Errorf("函数 %q 未调用文档写限流（AcquireDocWriteSlot / acquireDocWriteSlotWithTimeout）；新增写 API 必须先 acquire 文档写配额（issue #159）", sig)
		}
	}
}

// extractBalancedBraceBody 从以 `{` 开头的字符串提取到匹配 `}` 的子串（含 `}`）。
// 极简实现：跳过 `"..."` 与 “ `...` “ 字符串字面量，不处理注释里的 brace（够用即可）。
func extractBalancedBraceBody(s string) string {
	depth := 0
	inString := false
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			if c == '\\' && i+1 < len(s) {
				i++
				continue
			}
			if c == quote {
				inString = false
			}
			continue
		}
		switch c {
		case '"', '`', '\'':
			inString = true
			quote = c
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}
	return s
}
