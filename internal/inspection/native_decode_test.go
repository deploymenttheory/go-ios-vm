package inspection

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestNativeWorkerDeadlineKillsAndReapsStalledDecoder(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "worker.pid")
	start := time.Now()
	_, err := decodeNativeLZFSEWithBudget(
		context.Background(), []byte("bvx2"), TreeLimit,
		500*time.Millisecond, "stall:"+pidFile,
	)
	if !errors.Is(err, errNativeDecodeTimeout) {
		t.Fatalf("stalled decode: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("worker exceeded cancellation bound: %s", elapsed)
	}
	assertWorkerReaped(t, pidFile)
}

func TestNativeWorkerCancellationRemainsDistinctFromDeadline(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "worker.pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := decodeNativeLZFSEWithBudget(
			ctx, []byte("bvx2"), TreeLimit, 5*time.Second, "stall:"+pidFile,
		)
		result <- err
	}()
	deadline := time.After(3 * time.Second)
	for {
		if _, err := os.Stat(pidFile); err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("worker never started")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) || errors.Is(err, errNativeDecodeTimeout) {
			t.Fatalf("cancellation collapsed into timeout: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not stop native worker")
	}
	assertWorkerReaped(t, pidFile)
}

func TestNativeWorkerResourceFailureIsPartialReason(t *testing.T) {
	_, err := decodeNativeLZFSEWithBudget(
		context.Background(), []byte("bvx2"), TreeLimit,
		time.Second, "resource",
	)
	if !errors.Is(err, errNativeWorkerResource) ||
		errors.Is(err, errNativeDecodeTimeout) || errors.Is(err, context.Canceled) {
		t.Fatalf("resource failure lost identity: %v", err)
	}
}

func assertWorkerReaped(t *testing.T, pidFile string) {
	t.Helper()
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("native worker %d remains after return: %v", pid, err)
	}
}
