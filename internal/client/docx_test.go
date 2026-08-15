package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetAllBlockDescendantsUsesWithDescendantsAndPaginates(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if got, want := r.URL.Path, "/open-apis/docx/v1/documents/source-doc/blocks/source-block/children"; got != want {
			t.Errorf("请求路径 = %q，期望 %q", got, want)
		}
		if got := r.URL.Query().Get("with_descendants"); got != "true" {
			t.Errorf("with_descendants = %q，期望 true", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer u-test" {
			t.Errorf("Authorization = %q，期望 User Token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page_token") == "" {
			if _, err := fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"items":[{"block_id":"source-block","block_type":49,"children":["text-1"]}],"has_more":true,"page_token":"next-page"}}`); err != nil {
				t.Errorf("写入第一页响应失败: %v", err)
			}
			return
		}
		if got := r.URL.Query().Get("page_token"); got != "next-page" {
			t.Errorf("page_token = %q，期望 next-page", got)
		}
		if _, err := fmt.Fprint(w, `{"code":0,"msg":"ok","data":{"items":[{"block_id":"text-1","block_type":2}],"has_more":false}}`); err != nil {
			t.Errorf("写入第二页响应失败: %v", err)
		}
	}))
	defer server.Close()
	setupTestConfig(t, server.URL)

	blocks, err := GetAllBlockDescendants("source-doc", "source-block", "u-test")
	if err != nil {
		t.Fatalf("GetAllBlockDescendants() error = %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("请求次数 = %d，期望 2", requestCount)
	}
	if len(blocks) != 2 || StringVal(blocks[0].BlockId) != "source-block" || StringVal(blocks[1].BlockId) != "text-1" {
		t.Fatalf("分页结果异常: %#v", blocks)
	}
}

func TestAppendRowsLoop_ZeroCount(t *testing.T) {
	called := 0
	err := appendRowsLoop(0, nil, func() error {
		called++
		return nil
	})
	if err != nil {
		t.Fatalf("count=0 应返回 nil，得到 %v", err)
	}
	if called != 0 {
		t.Fatalf("count=0 不应调用 insertOne，实际调用 %d 次", called)
	}
}

func TestAppendRowsLoop_NegativeCount(t *testing.T) {
	called := 0
	err := appendRowsLoop(-3, nil, func() error {
		called++
		return nil
	})
	if err != nil {
		t.Fatalf("count<0 应返回 nil，得到 %v", err)
	}
	if called != 0 {
		t.Fatalf("count<0 不应调用 insertOne，实际调用 %d 次", called)
	}
}

func TestAppendRowsLoop_Success(t *testing.T) {
	called := 0
	err := appendRowsLoop(5, nil, func() error {
		called++
		return nil
	})
	if err != nil {
		t.Fatalf("期望成功，得到 %v", err)
	}
	if called != 5 {
		t.Fatalf("期望调用 5 次，实际 %d", called)
	}
}

func TestAppendRowsLoop_ErrorInMiddle(t *testing.T) {
	called := 0
	sentinel := errors.New("boom")
	err := appendRowsLoop(5, nil, func() error {
		called++
		if called == 3 {
			return sentinel
		}
		return nil
	})
	if err == nil {
		t.Fatal("期望返回错误")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("错误应包装原始 sentinel，实际 %v", err)
	}
	// 第 3 次失败，前 2 次成功 → 共调用 3 次
	if called != 3 {
		t.Fatalf("期望调用 3 次后中断，实际 %d", called)
	}
	// 错误消息应包含行号
	if want := "追加第 3 行失败"; !strings.Contains(err.Error(), want) {
		t.Fatalf("错误消息应包含 %q，实际 %q", want, err.Error())
	}
}

func TestAppendRowsLoop_ProgressFiresAfterEachRow(t *testing.T) {
	var progressCalls []struct{ appended, total int }
	err := appendRowsLoop(3, func(appended, total int) {
		progressCalls = append(progressCalls, struct{ appended, total int }{appended, total})
	}, func() error { return nil })
	if err != nil {
		t.Fatalf("期望成功，得到 %v", err)
	}
	if len(progressCalls) != 3 {
		t.Fatalf("期望 progress 回调 3 次，实际 %d", len(progressCalls))
	}
	for i, p := range progressCalls {
		if p.appended != i+1 || p.total != 3 {
			t.Fatalf("progress[%d] = (%d,%d), 期望 (%d,3)", i, p.appended, p.total, i+1)
		}
	}
}

func TestAppendRowsLoop_ProgressNotFiredAfterError(t *testing.T) {
	progressCount := 0
	boom := errors.New("boom")
	err := appendRowsLoop(5, func(int, int) { progressCount++ }, func() error {
		return boom
	})
	if err == nil {
		t.Fatal("期望返回错误")
	}
	// 第一次就错误 → progress 不应触发
	if progressCount != 0 {
		t.Fatalf("错误发生时不应回调 progress，实际 %d 次", progressCount)
	}
}

func TestAppendRowsLoop_NilProgressOK(t *testing.T) {
	err := appendRowsLoop(2, nil, func() error { return nil })
	if err != nil {
		t.Fatalf("nil progress 应正常，得到 %v", err)
	}
}
