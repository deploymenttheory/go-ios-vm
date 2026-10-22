package inspection

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	lzfse "github.com/blacktop/lzfse-cgo"
)

const (
	nativeWorkerArg       = "--go-ios-vm-native-lzfse-worker"
	nativeWorkerLimitEnv  = "GOIOSVM_NATIVE_DECODE_LIMIT"
	nativeWorkerTestEnv   = "GOIOSVM_NATIVE_TEST_MODE"
	nativeDecodeTimeout   = 30 * time.Second
	nativeWorkerWaitDelay = 2 * time.Second
	nativeAddressLimit    = 2 << 30
	nativeResidentLimit   = 512 << 20
	nativeCPULimitSeconds = 31
	nativeRSSPollInterval = 10 * time.Millisecond
)

var (
	errNativeDecodeTimeout  = errors.New("native LZFSE decode timed out")
	errNativeWorkerResource = errors.New("native LZFSE worker resource limit or process failure")
	errNativeWorkerStart    = errors.New("native LZFSE worker could not start")
)

// A process boundary lets CommandContext kill a decoder while it is inside C.
// Only the decoded bytes cross back into the inspection process; no firmware
// source or temporary file is written by the worker.
func decodeNativeLZFSE(ctx context.Context, compressed []byte, limit int) ([]byte, error) {
	testMode := ""
	if strings.HasSuffix(filepath.Base(os.Args[0]), ".test") {
		testMode = os.Getenv(nativeWorkerTestEnv)
	}
	return decodeNativeLZFSEWithBudget(ctx, compressed, limit, nativeDecodeTimeout, testMode)
}

func decodeNativeLZFSEWithBudget(
	ctx context.Context, compressed []byte, limit int, timeout time.Duration, testMode string,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("native decode: %w", err)
	}
	if limit < 1 || limit > KernelLimit || len(compressed) > KernelLimit || timeout <= 0 {
		return nil, errNativeWorkerResource
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("%w: executable: %w", errNativeWorkerStart, err)
	}
	deadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(deadline, executable, nativeWorkerArg)
	cmd.WaitDelay = nativeWorkerWaitDelay
	cmd.Env = []string{nativeWorkerLimitEnv + "=" + strconv.Itoa(limit)}
	if testMode != "" && strings.HasSuffix(filepath.Base(executable), ".test") {
		cmd.Env = append(cmd.Env, nativeWorkerTestEnv+"="+testMode)
	}
	cmd.Stdin = bytes.NewReader(compressed)
	var output limitedWorkerOutput
	output.max = limit + 1
	cmd.Stdout = &output
	// Worker diagnostics are intentionally not copied into coverage or CLI output.
	cmd.Stderr = io.Discard
	err = cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("native decode: %w", ctx.Err())
	}
	if errors.Is(deadline.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("%w: %s", errNativeDecodeTimeout, timeout)
	}
	if output.exceeded || output.Len() > limit {
		return nil, fmt.Errorf("%w: limit %d bytes", errLZFSEExceedsOutputBudget, limit)
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			switch exit.ExitCode() {
			case 10:
				return nil, errLZFSEDecodeFailed
			case 11:
				return nil, fmt.Errorf("%w: limit %d bytes", errLZFSEExceedsOutputBudget, limit)
			}
		}
		return nil, fmt.Errorf("%w: %w", errNativeWorkerResource, err)
	}
	return output.Bytes(), nil
}

type limitedWorkerOutput struct {
	bytes.Buffer
	max      int
	exceeded bool
}

func (w *limitedWorkerOutput) Write(p []byte) (int, error) {
	if len(p) > w.max-w.Len() {
		w.exceeded = true
		return 0, errLZFSEExceedsOutputBudget
	}
	n, err := w.Buffer.Write(p)
	if err != nil {
		return n, fmt.Errorf("native output buffer: %w", err)
	}
	return n, nil
}

// init is used instead of a CLI subcommand so package tests and the installed
// CLI exercise the same worker executable and resource limits.
func init() {
	if len(os.Args) == 2 && os.Args[1] == nativeWorkerArg {
		os.Exit(runNativeWorker())
	}
}

func runNativeWorker() int {
	limit, err := strconv.Atoi(os.Getenv(nativeWorkerLimitEnv))
	if err != nil || limit < 1 || limit > KernelLimit {
		return 12
	}
	if err := boundNativeMemory(); err != nil {
		return 12
	}
	// CPU is an independent guard for a decoder that burns CPU rather than
	// waiting for the parent's wall-clock timeout.
	if err := syscall.Setrlimit(syscall.RLIMIT_CPU, &syscall.Rlimit{
		Cur: nativeCPULimitSeconds, Max: nativeCPULimitSeconds,
	}); err != nil {
		return 12
	}
	if strings.HasSuffix(filepath.Base(os.Args[0]), ".test") {
		mode := os.Getenv(nativeWorkerTestEnv)
		switch {
		case strings.HasPrefix(mode, "stall:"):
			// #nosec G703 -- reachable only in a Go test binary; the test owns this path.
			_ = os.WriteFile(strings.TrimPrefix(mode, "stall:"),
				[]byte(strconv.Itoa(os.Getpid())), 0o600)
			time.Sleep(time.Minute)
		case mode == "resource":
			return 12
		}
	}
	input, err := io.ReadAll(io.LimitReader(os.Stdin, KernelLimit+1))
	if err != nil || len(input) > KernelLimit {
		return 12
	}
	decoded := make([]byte, limit+1)
	n := lzfse.DecodeBufferInto(input, decoded)
	if n == 0 {
		return 10
	}
	if n > limit {
		return 11
	}
	if _, err := os.Stdout.Write(decoded[:n]); err != nil {
		return 12
	}
	return 0
}

func boundNativeMemory() error {
	if runtime.GOOS == "linux" && !nativeRaceEnabled {
		// The Go 1.27 runtime reserves more than 1 GiB of virtual address
		// space before the decoder allocates a byte. Cap virtual mappings
		// separately from the worker's measured resident budget. Race
		// instrumentation reserves much larger virtual ranges; race test
		// binaries keep the resident watchdog but cannot use RLIMIT_AS.
		if err := syscall.Setrlimit(syscall.RLIMIT_AS, &syscall.Rlimit{
			Cur: nativeAddressLimit, Max: nativeAddressLimit,
		}); err != nil {
			return fmt.Errorf("native address limit: %w", err)
		}
	} else if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return errNativeWorkerResource
	}
	// Darwin rejects RLIMIT_AS and RLIMIT_DATA with EINVAL. A dedicated Go
	// thread samples peak RSS on both supported hosts while decoding runs in C.
	go func() {
		for {
			var usage syscall.Rusage
			if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
				os.Exit(12)
			}
			peakBytes := usage.Maxrss
			if runtime.GOOS == "linux" {
				peakBytes *= 1024
			}
			if peakBytes > nativeResidentLimit {
				os.Exit(12)
			}
			time.Sleep(nativeRSSPollInterval)
		}
	}()
	return nil
}
