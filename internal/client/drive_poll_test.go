package client

import (
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func shortenPolls(t *testing.T) {
	t.Helper()
	oldMoveI, oldMoveN := DriveMovePollInterval, DriveMoveMaxAttempts
	oldExpI, oldImpI := DriveExportPollInterval, DriveImportPollInterval
	DriveMovePollInterval, DriveMoveMaxAttempts = time.Millisecond, 4
	DriveExportPollInterval, DriveImportPollInterval = time.Millisecond, time.Millisecond
	t.Cleanup(func() {
		DriveMovePollInterval, DriveMoveMaxAttempts = oldMoveI, oldMoveN
		DriveExportPollInterval, DriveImportPollInterval = oldExpI, oldImpI
	})
}

// P0：删除任务返回 "fail" 时轮询立即以失败结束（旧实现只认 "failed"，会一直轮询直到超时并显示进行中）。
func TestWaitDriveTaskCheck_FailIsTerminal(t *testing.T) {
	shortenPolls(t)
	var calls int32
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"status":"fail"}}`)
	})
	defer cleanup()

	st, timedOut, err := WaitDriveTaskCheckWithBound("task1", "u-x")
	if err == nil || timedOut {
		t.Fatalf("fail 应为失败终态: st=%+v timedOut=%v err=%v", st, timedOut, err)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("失败终态后不应继续轮询，calls=%d", n)
	}
}

// P1：首次查询失败（瞬时错误）后继续轮询，拿到成功即返回。
func TestWaitDriveTaskCheck_TransientErrorIgnored(t *testing.T) {
	shortenPolls(t)
	var calls int32
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(w, `{"code":1061001,"msg":"internal"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"status":"success"}}`)
	})
	defer cleanup()

	st, timedOut, err := WaitDriveTaskCheckWithBound("task1", "u-x")
	if err != nil || timedOut || !st.Ready() {
		t.Fatalf("瞬时错误后应继续轮询成功: st=%+v timedOut=%v err=%v", st, timedOut, err)
	}
}

// P1：遇限流立即停止并返回 DrivePollError（RateLimited），调用方据此输出续查命令。
func TestWaitDriveExport_RateLimitStopsImmediately(t *testing.T) {
	shortenPolls(t)
	var calls int32
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"code":99991400,"msg":"request trigger frequency limit"}`)
	})
	defer cleanup()

	_, _, err := WaitDriveExportWithBound("ticket1", "doc1", "u-x")
	var pe *DrivePollError
	if !errors.As(err, &pe) || !pe.RateLimited {
		t.Fatalf("want rate-limited DrivePollError, got %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("限流后不应继续轮询，calls=%d", n)
	}
}

// 所有查询都失败：返回 AllFailed，而不是伪装成"超时进行中"。
func TestWaitDriveImport_AllPollsFailed(t *testing.T) {
	shortenPolls(t)
	old := DriveImportMaxAttempts
	DriveImportMaxAttempts = 3
	defer func() { DriveImportMaxAttempts = old }()
	_, cleanup := stubFeishuServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprint(w, `{"code":1061001,"msg":"bad gateway"}`)
	})
	defer cleanup()

	_, timedOut, err := WaitDriveImportWithBound("ticket1", "u-x")
	var pe *DrivePollError
	if timedOut || !errors.As(err, &pe) || !pe.AllFailed {
		t.Fatalf("want AllFailed DrivePollError, got timedOut=%v err=%v", timedOut, err)
	}
}
