package event

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func subscribeTestDef(url string) KeyDefinition {
	return KeyDefinition{
		Key:            "approval.instance.status_changed_v4",
		SubscribePath:  "/subscribe",
		SubscribeTypes: []string{"INVOLVED_APPROVAL"},
	}
}

func newSubscribeRuntime(baseURL string) *Runtime {
	return NewRuntime(ConsumeOptions{
		AppID:           "cli_test",
		AppSecret:       "secret",
		EventKey:        "approval.instance.status_changed_v4",
		BaseURL:         baseURL,
		UserAccessToken: "u-test",
		ErrOut:          io.Discard,
	})
}

func TestRegisterSubscriptionsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isForeignProbe(w, r) {
			return
		}
		if r.URL.Path != "/subscribe" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer u-test" {
			t.Errorf("Authorization = %s", got)
		}
		w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer srv.Close()

	r := newSubscribeRuntime(srv.URL)
	if err := r.registerSubscriptions(context.Background(), subscribeTestDef(srv.URL)); err != nil {
		t.Fatalf("应成功，实际: %v", err)
	}
}

func TestRegisterSubscriptionsFailClosedOnHTMLError(t *testing.T) {
	// 网关 5xx + HTML 错误页：必须判定失败（此前吞 Unmarshal 错误会假成功）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("<html>502 Bad Gateway</html>"))
	}))
	defer srv.Close()

	r := newSubscribeRuntime(srv.URL)
	err := r.registerSubscriptions(context.Background(), subscribeTestDef(srv.URL))
	if err == nil {
		t.Fatal("HTTP 502 + HTML 响应应判定注册失败，实际返回 nil（假成功）")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("错误应包含状态码，实际: %v", err)
	}
}

func TestRegisterSubscriptionsFailClosedOnBadJSON(t *testing.T) {
	// HTTP 200 但响应体非 JSON：同样失败
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	r := newSubscribeRuntime(srv.URL)
	if err := r.registerSubscriptions(context.Background(), subscribeTestDef(srv.URL)); err == nil {
		t.Fatal("非 JSON 响应应判定注册失败")
	}
}

func TestRegisterSubscriptionsBizError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":99991679,"msg":"scope 不足"}`))
	}))
	defer srv.Close()

	r := newSubscribeRuntime(srv.URL)
	err := r.registerSubscriptions(context.Background(), subscribeTestDef(srv.URL))
	if err == nil || !strings.Contains(err.Error(), "99991679") {
		t.Fatalf("业务错误码应透出，实际: %v", err)
	}
}

func TestRegisterSubscriptionsCtxCancel(t *testing.T) {
	// 端点挂起：ctx 取消必须能中断（此前 DefaultClient 无超时且未绑 ctx 会永久阻塞）。
	// handler 用有界 sleep 模拟挂起（阻塞到连接关闭会让 srv.Close 等待，反而卡测试）。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	r := newSubscribeRuntime(srv.URL)
	start := time.Now()
	err := r.registerSubscriptions(ctx, subscribeTestDef(srv.URL))
	if err == nil {
		t.Fatal("挂起端点应超时报错")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("应在 ctx 超时后立即返回，实际耗时 %v", elapsed)
	}
}

func waitRuntimeReady(t *testing.T, ready *concurrentBuffer, key string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(ready.String(), "[event] ready event_key="+key) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待 ready 超时，实际: %q", ready.String())
}

func startVCConsumer(t *testing.T, srvURL string, bus *Bus, pid int, ready *concurrentBuffer) (context.CancelFunc, <-chan error) {
	t.Helper()
	r := NewRuntime(ConsumeOptions{
		AppID:           "cli_test",
		AppSecret:       "secret",
		EventKey:        "vc.meeting.participant_meeting_started_v1",
		BaseURL:         srvURL,
		UserAccessToken: "u-test",
		ErrOut:          io.Discard,
		ReadyOut:        ready,
		Bus:             bus,
		ConsumerPID:     pid,
		StartWS: func(ctx context.Context, onHandshake func()) error {
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
	return cancel, errCh
}

func TestSequentialConsumersSubscribeEachUnsubscribeOnLast(t *testing.T) {
	stubConsumerAlive(t)
	bus := setupBus(t)

	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isForeignProbe(w, r) {
			return
		}
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer srv.Close()

	var ready1, ready2 concurrentBuffer
	cancel1, done1 := startVCConsumer(t, srv.URL, bus, 501, &ready1)
	waitRuntimeReady(t, &ready1, "vc.meeting.participant_meeting_started_v1")

	cancel2, done2 := startVCConsumer(t, srv.URL, bus, 502, &ready2)
	waitRuntimeReady(t, &ready2, "vc.meeting.participant_meeting_started_v1")

	mu.Lock()
	subCount := countPath(paths, "POST /open-apis/vc/v1/meetings/subscription")
	unsubCount := countPath(paths, "POST /open-apis/vc/v1/meetings/unsubscription")
	mu.Unlock()
	if subCount != 2 {
		t.Fatalf("每个 consumer 都必须幂等 subscribe，subscribe=%d paths=%v", subCount, paths)
	}
	if unsubCount != 0 {
		t.Fatalf("两人还在跑时不得 unsubscribe，unsub=%d", unsubCount)
	}

	cancel1()
	select {
	case <-done1:
	case <-time.After(2 * time.Second):
		t.Fatal("consumer1 未退出")
	}
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	unsubCount = countPath(paths, "POST /open-apis/vc/v1/meetings/unsubscription")
	mu.Unlock()
	if unsubCount != 0 {
		t.Fatalf("先退出者不得注销同伴仍在用的订阅，unsub=%d paths=%v", unsubCount, paths)
	}

	cancel2()
	select {
	case <-done2:
	case <-time.After(2 * time.Second):
		t.Fatal("consumer2 未退出")
	}
	mu.Lock()
	subCount = countPath(paths, "POST /open-apis/vc/v1/meetings/subscription")
	unsubCount = countPath(paths, "POST /open-apis/vc/v1/meetings/unsubscription")
	mu.Unlock()
	if subCount != 2 || unsubCount != 1 {
		t.Fatalf("每人 subscribe、最后一人退出才 unsubscribe，subscribe=%d unsub=%d paths=%v", subCount, unsubCount, paths)
	}
}

func TestConcurrentConsumersSubscribeEachUnsubscribeOnce(t *testing.T) {
	stubConsumerAlive(t)
	bus := setupBus(t)

	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isForeignProbe(w, r) {
			return
		}
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer srv.Close()

	var ready1, ready2 concurrentBuffer
	start := make(chan struct{})
	errCh := make(chan error, 2)
	cancels := make([]context.CancelFunc, 2)
	for i, pid := range []int{601, 602} {
		ready := &ready1
		if i == 1 {
			ready = &ready2
		}
		r := NewRuntime(ConsumeOptions{
			AppID:           "cli_test",
			AppSecret:       "secret",
			EventKey:        "vc.meeting.participant_meeting_started_v1",
			BaseURL:         srv.URL,
			UserAccessToken: "u-test",
			ErrOut:          io.Discard,
			ReadyOut:        ready,
			Bus:             bus,
			ConsumerPID:     pid,
			StartWS: func(ctx context.Context, onHandshake func()) error {
				onHandshake()
				<-ctx.Done()
				return ctx.Err()
			},
		})
		ctx, cancel := context.WithCancel(context.Background())
		cancels[i] = cancel
		go func() {
			<-start
			_, err := r.Run(ctx)
			errCh <- err
		}()
	}
	close(start)
	waitRuntimeReady(t, &ready1, "vc.meeting.participant_meeting_started_v1")
	waitRuntimeReady(t, &ready2, "vc.meeting.participant_meeting_started_v1")

	mu.Lock()
	subCount := countPath(paths, "POST /open-apis/vc/v1/meetings/subscription")
	mu.Unlock()
	if subCount != 2 {
		t.Fatalf("并发启动时每个 consumer 都必须 subscribe，subscribe=%d paths=%v", subCount, paths)
	}

	cancels[0]()
	cancels[1]()
	for i := 0; i < 2; i++ {
		select {
		case <-errCh:
		case <-time.After(2 * time.Second):
			t.Fatal("并发 consumer 未退出")
		}
	}
	mu.Lock()
	unsubCount := countPath(paths, "POST /open-apis/vc/v1/meetings/unsubscription")
	mu.Unlock()
	if unsubCount != 1 {
		t.Fatalf("并发退出只能 unsubscribe 一次，unsub=%d paths=%v", unsubCount, paths)
	}
}

func assertNeverReady(t *testing.T, ready *concurrentBuffer, when string) {
	t.Helper()
	if strings.Contains(ready.String(), "[event] ready") {
		t.Fatalf("%s 时不得 ready，实际: %q", when, ready.String())
	}
}

func TestFirstSubscribeBlocksSecondStillSubscribesBeforeReady(t *testing.T) {
	stubConsumerAlive(t)
	bus := setupBus(t)

	firstSubscribeStarted := make(chan struct{})
	blockFirst, unblockFirst := context.WithCancel(context.Background())
	t.Cleanup(unblockFirst)

	var mu sync.Mutex
	var paths []string
	var subSeq int
	var firstStartWS, secondStartWS bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isForeignProbe(w, r) {
			return
		}
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		isSub := r.URL.Path == "/open-apis/vc/v1/meetings/subscription"
		n := 0
		if isSub {
			subSeq++
			n = subSeq
		}
		mu.Unlock()
		if isSub && n == 1 {
			close(firstSubscribeStarted)
			<-blockFirst.Done()
		}
		w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer srv.Close()

	var ready1, ready2 concurrentBuffer
	cancel1, done1 := startVCConsumerWithStartWS(t, srv.URL, bus, 701, &ready1, func() {
		mu.Lock()
		firstStartWS = true
		mu.Unlock()
	})
	defer cancel1()
	select {
	case <-firstSubscribeStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first subscribe 未发出")
	}
	assertNeverReady(t, &ready1, "first subscribe 仍阻塞")
	mu.Lock()
	if firstStartWS {
		mu.Unlock()
		t.Fatal("subscribe 未完成前不得启动 WebSocket")
	}
	mu.Unlock()

	cancel2, done2 := startVCConsumerWithStartWS(t, srv.URL, bus, 702, &ready2, func() {
		mu.Lock()
		secondStartWS = true
		mu.Unlock()
	})
	defer cancel2()
	waitRuntimeReady(t, &ready2, "vc.meeting.participant_meeting_started_v1")
	assertNeverReady(t, &ready1, "second 已 ready 但 first subscribe 仍阻塞")
	mu.Lock()
	subCount := countPath(paths, "POST /open-apis/vc/v1/meetings/subscription")
	gotSecondWS := secondStartWS
	gotFirstWS := firstStartWS
	mu.Unlock()
	if subCount < 2 {
		t.Fatalf("second 必须自己 subscribe，不得因 first 阻塞而跳过，subscribe=%d paths=%v", subCount, paths)
	}
	if !gotSecondWS {
		t.Fatal("second 自己 subscribe 成功后才应握手")
	}
	if gotFirstWS {
		t.Fatal("first subscribe 仍阻塞时不得握手")
	}

	unblockFirst()
	waitRuntimeReady(t, &ready1, "vc.meeting.participant_meeting_started_v1")

	cancel1()
	cancel2()
	select {
	case <-done1:
	case <-time.After(2 * time.Second):
		t.Fatal("consumer1 未退出")
	}
	select {
	case <-done2:
	case <-time.After(2 * time.Second):
		t.Fatal("consumer2 未退出")
	}
	mu.Lock()
	unsubCount := countPath(paths, "POST /open-apis/vc/v1/meetings/unsubscription")
	mu.Unlock()
	if unsubCount != 1 {
		t.Fatalf("last-consumer 才 unsubscribe，unsub=%d paths=%v", unsubCount, paths)
	}
}

func TestFirstSubscribeFailsSecondStillSubscribesBeforeReady(t *testing.T) {
	stubConsumerAlive(t)
	bus := setupBus(t)

	firstSubscribeStarted := make(chan struct{})
	failGate, releaseFail := context.WithCancel(context.Background())
	t.Cleanup(releaseFail)

	var mu sync.Mutex
	var paths []string
	var subSeq int
	var firstStartWS bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isForeignProbe(w, r) {
			return
		}
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		isSub := r.URL.Path == "/open-apis/vc/v1/meetings/subscription"
		n := 0
		if isSub {
			subSeq++
			n = subSeq
		}
		mu.Unlock()
		if isSub && n == 1 {
			close(firstSubscribeStarted)
			<-failGate.Done()
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"code":1,"msg":"subscribe fail"}`))
			return
		}
		w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer srv.Close()

	var ready1, ready2 concurrentBuffer
	_, done1 := startVCConsumerWithStartWS(t, srv.URL, bus, 801, &ready1, func() {
		mu.Lock()
		firstStartWS = true
		mu.Unlock()
	})
	select {
	case <-firstSubscribeStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first subscribe 未发出")
	}

	cancel2, done2 := startVCConsumerWithStartWS(t, srv.URL, bus, 802, &ready2, nil)
	defer cancel2()
	waitRuntimeReady(t, &ready2, "vc.meeting.participant_meeting_started_v1")
	assertNeverReady(t, &ready1, "first subscribe 尚未失败返回、second 已 ready")

	releaseFail()
	select {
	case err := <-done1:
		if err == nil {
			t.Fatal("first subscribe 失败应返回 error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first subscribe 失败后 Run 未返回")
	}
	assertNeverReady(t, &ready1, "first subscribe 失败")
	mu.Lock()
	subCount := countPath(paths, "POST /open-apis/vc/v1/meetings/subscription")
	gotFirstWS := firstStartWS
	mu.Unlock()
	if subCount < 2 {
		t.Fatalf("second 必须自己 subscribe，不得因 first 失败而跳过，subscribe=%d paths=%v", subCount, paths)
	}
	if gotFirstWS {
		t.Fatal("first subscribe 失败后不得启动 WebSocket / ready")
	}
	if !strings.Contains(ready2.String(), "[event] ready event_key=vc.meeting.participant_meeting_started_v1") {
		t.Fatalf("second 自己 subscribe 成功后应 ready，实际: %q", ready2.String())
	}

	cancel2()
	select {
	case <-done2:
	case <-time.After(2 * time.Second):
		t.Fatal("consumer2 未退出")
	}
	mu.Lock()
	unsubCount := countPath(paths, "POST /open-apis/vc/v1/meetings/unsubscription")
	mu.Unlock()
	if unsubCount != 1 {
		t.Fatalf("成功订阅的 last-consumer 才 unsubscribe，unsub=%d paths=%v", unsubCount, paths)
	}
}

func startVCConsumerWithStartWS(t *testing.T, srvURL string, bus *Bus, pid int, ready *concurrentBuffer, onStartWS func()) (context.CancelFunc, <-chan error) {
	t.Helper()
	r := NewRuntime(ConsumeOptions{
		AppID:           "cli_test",
		AppSecret:       "secret",
		EventKey:        "vc.meeting.participant_meeting_started_v1",
		BaseURL:         srvURL,
		UserAccessToken: "u-test",
		ErrOut:          io.Discard,
		ReadyOut:        ready,
		Bus:             bus,
		ConsumerPID:     pid,
		StartWS: func(ctx context.Context, onHandshake func()) error {
			if onStartWS != nil {
				onStartWS()
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
	return cancel, errCh
}

func countPath(paths []string, want string) int {
	n := 0
	for _, p := range paths {
		if p == want {
			n++
		}
	}
	return n
}

func TestUnregisterSubscriptionsUses5sTimeout(t *testing.T) {
	if unsubscribeHTTPTimeout != 5*time.Second {
		t.Fatalf("unsubscribeHTTPTimeout = %s, want 5s", unsubscribeHTTPTimeout)
	}
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isForeignProbe(w, r) {
			return
		}
		close(started)
		select {
		case <-r.Context().Done():
		case <-time.After(20 * time.Second):
		}
	}))
	defer srv.Close()

	r := NewRuntime(ConsumeOptions{
		BaseURL:         srv.URL,
		UserAccessToken: "u-test",
		ErrOut:          io.Discard,
	})
	begin := time.Now()
	r.unregisterSubscriptions(KeyDefinition{
		Key:                "vc.meeting.participant_meeting_started_v1",
		EventType:          "vc.meeting.participant_meeting_started_v1",
		UnsubscribePath:    "/open-apis/vc/v1/meetings/unsubscription",
		SubscribeEventType: true,
	})
	elapsed := time.Since(begin)
	select {
	case <-started:
	default:
		t.Fatal("注销请求未发出")
	}
	if elapsed < 4*time.Second || elapsed > 7*time.Second {
		t.Fatalf("cleanup elapsed = %s, want ~5s", elapsed)
	}
}

func TestRegisterSubscriptionsRequiresUserToken(t *testing.T) {
	r := NewRuntime(ConsumeOptions{EventKey: "x", BaseURL: "http://127.0.0.1:1", ErrOut: io.Discard})
	err := r.registerSubscriptions(context.Background(), subscribeTestDef(""))
	if err == nil || !strings.Contains(err.Error(), "auth login") {
		t.Fatalf("缺 User Token 应报错并提示登录，实际: %v", err)
	}
}

func TestSubscriptionRequestBodiesApprovalVsVC(t *testing.T) {
	approval := KeyDefinition{
		EventType:      "approval.instance.status_changed_v4",
		SubscribeTypes: []string{"INVOLVED_APPROVAL", "MANAGED_APPROVAL"},
	}
	got := subscriptionRequestBodies(approval)
	if len(got) != 2 || got[0]["subscription_type"] != "INVOLVED_APPROVAL" || got[1]["subscription_type"] != "MANAGED_APPROVAL" {
		t.Fatalf("审批 body = %#v", got)
	}
	if _, ok := got[0]["event_type"]; ok {
		t.Fatal("审批 body 不应带 event_type")
	}

	vc := KeyDefinition{
		EventType:          "vc.meeting.participant_meeting_started_v1",
		SubscribeEventType: true,
		SubscribeTypes:     []string{"should-be-ignored"},
	}
	got = subscriptionRequestBodies(vc)
	if len(got) != 1 || got[0]["event_type"] != "vc.meeting.participant_meeting_started_v1" {
		t.Fatalf("VC body = %#v, want event_type=participant_meeting_started", got)
	}
	if _, ok := got[0]["subscription_type"]; ok {
		t.Fatal("VC body 不应带 subscription_type")
	}
}

func TestRegisterSubscriptionsVCEventTypePathAndBody(t *testing.T) {
	var gotPath, gotBody, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isForeignProbe(w, r) {
			return
		}
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		buf := new(strings.Builder)
		_, _ = io.Copy(buf, r.Body)
		gotBody = buf.String()
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
	})
	def := KeyDefinition{
		Key:                "vc.meeting.participant_meeting_started_v1",
		EventType:          "vc.meeting.participant_meeting_started_v1",
		SubscribePath:      "/open-apis/vc/v1/meetings/subscription",
		SubscribeEventType: true,
		Scopes:             []string{"vc:meeting.meetingevent:read"},
	}
	if err := r.registerSubscriptions(context.Background(), def); err != nil {
		t.Fatalf("VC 订阅应成功，实际: %v", err)
	}
	if gotPath != "/open-apis/vc/v1/meetings/subscription" {
		t.Errorf("path = %s", gotPath)
	}
	if gotAuth != "Bearer u-test" {
		t.Errorf("Authorization = %s", gotAuth)
	}
	if !strings.Contains(gotBody, `"event_type":"vc.meeting.participant_meeting_started_v1"`) {
		t.Errorf("body = %s, want event_type", gotBody)
	}
	if strings.Contains(gotBody, "subscription_type") {
		t.Errorf("VC body 不应含 subscription_type: %s", gotBody)
	}
}
