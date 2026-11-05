package event

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestLooksLikeWSConnected(t *testing.T) {
	if !looksLikeWSConnected([]interface{}{"connected to wss://open.feishu.cn/ws"}) {
		t.Fatal("Dial 成功后的 connected to 日志应视为握手就绪")
	}
	if looksLikeWSConnected([]interface{}{"disconnected to wss://open.feishu.cn/ws"}) {
		t.Fatal("disconnected 不得误判为握手就绪")
	}
	if looksLikeWSConnected([]interface{}{"connect failed, err: timeout"}) {
		t.Fatal("失败日志不应视为握手")
	}
}

func TestReadyNotEmittedBeforeHandshake(t *testing.T) {
	var ready concurrentBuffer
	started := make(chan struct{})
	r := NewRuntime(ConsumeOptions{
		AppID:     "cli_test",
		AppSecret: "secret",
		EventKey:  "im.message.receive_v1",
		ErrOut:    io.Discard,
		ReadyOut:  &ready,
		StartWS: func(ctx context.Context, onHandshake func()) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := r.Run(ctx)
		errCh <- err
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("StartWS 未被调用")
	}

	time.Sleep(50 * time.Millisecond)
	if r.readyOnce.Load() || strings.Contains(ready.String(), "[event] ready") {
		t.Fatalf("握手前不得发 ready，实际: %q", ready.String())
	}

	cancel()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("Run 未在 cancel 后退出")
	}
	if r.readyOnce.Load() || strings.Contains(ready.String(), "[event] ready") {
		t.Fatalf("握手从未发生时仍不得发 ready，实际: %q", ready.String())
	}
}

func TestReadyEmittedAfterHandshake(t *testing.T) {
	var ready concurrentBuffer
	r := NewRuntime(ConsumeOptions{
		AppID:     "cli_test",
		AppSecret: "secret",
		EventKey:  "im.message.receive_v1",
		ErrOut:    io.Discard,
		ReadyOut:  &ready,
		StartWS: func(ctx context.Context, onHandshake func()) error {
			onHandshake()
			<-ctx.Done()
			return ctx.Err()
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		_, err := r.Run(ctx)
		errCh <- err
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(ready.String(), "[event] ready event_key=im.message.receive_v1") {
			cancel()
			<-errCh
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("握手后应发 ready，实际: %q", ready.String())
}

func TestReadyNotEmittedIfPreConsumeFails(t *testing.T) {
	var ready concurrentBuffer
	var startWSCalled atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"code":99991679,"msg":"scope 不足"}`))
	}))
	defer srv.Close()

	r := NewRuntime(ConsumeOptions{
		AppID:           "cli_test",
		AppSecret:       "secret",
		EventKey:        "vc.meeting.participant_meeting_started_v1",
		BaseURL:         srv.URL,
		UserAccessToken: "u-test",
		ErrOut:          io.Discard,
		ReadyOut:        &ready,
		StartWS: func(ctx context.Context, onHandshake func()) error {
			startWSCalled.Store(true)
			onHandshake()
			return nil
		},
	})
	reason, err := r.Run(context.Background())
	if err == nil || reason != "error" {
		t.Fatalf("pre-consume 失败应返回 error，实际 reason=%s err=%v", reason, err)
	}
	if startWSCalled.Load() {
		t.Fatal("pre-consume 失败后不得启动 WebSocket")
	}
	if r.readyOnce.Load() || strings.Contains(ready.String(), "[event] ready") {
		t.Fatalf("pre-consume 失败不得发 ready，实际: %q", ready.String())
	}
}

func TestReadyAfterPreConsumeAndHandshake(t *testing.T) {
	var ready concurrentBuffer
	var subscribed atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isForeignProbe(w, r) {
			return
		}
		if r.URL.Path != "/open-apis/vc/v1/meetings/subscription" && r.URL.Path != "/open-apis/vc/v1/meetings/unsubscription" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Path == "/open-apis/vc/v1/meetings/subscription" {
			subscribed.Store(true)
		}
		w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer srv.Close()

	r := NewRuntime(ConsumeOptions{
		AppID:           "cli_test",
		AppSecret:       "secret",
		EventKey:        "vc.meeting.participant_meeting_started_v1",
		BaseURL:         srv.URL,
		UserAccessToken: "u-test",
		ErrOut:          io.Discard,
		ReadyOut:        &ready,
		StartWS: func(ctx context.Context, onHandshake func()) error {
			if !subscribed.Load() {
				t.Error("握手前必须先完成 User pre-consume")
			}
			onHandshake()
			<-ctx.Done()
			return ctx.Err()
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := r.Run(ctx)
		errCh <- err
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(ready.String(), "[event] ready event_key=vc.meeting.participant_meeting_started_v1") {
			if !subscribed.Load() {
				t.Fatal("ready 发出时 pre-consume 必须已完成")
			}
			cancel()
			<-errCh
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("pre-consume+握手后应发 ready，实际: %q subscribed=%v", ready.String(), subscribed.Load())
}
