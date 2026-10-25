//go:build ios && cgo

package daemon

/*
#include <errno.h>
#include <stdint.h>

// reboot3 hands the request to launchd, which stops every service, unmounts
// the volumes and halts the kernel. The flags are those `launchctl reboot halt`
// passes, as vphoned's vphoned_power.m uses: a full reboot with RB_HALT.
// <sys/reboot.h> is not in the iOS SDK.
extern int reboot3(uint64_t flags, ...);

static int goiosvm_halt(void) {
	errno = 0;
	if (reboot3(0x8000000000000000ULL | 0x08, 0) == 0) return 0;
	return errno ? errno : EIO;
}
*/
import "C"

import (
	"fmt"
	"syscall"
)

// Halt shuts the guest down through launchd.
func Halt() error {
	if status := C.goiosvm_halt(); status != 0 {
		return fmt.Errorf("reboot3 halt: %w", syscall.Errno(status))
	}
	return nil
}
