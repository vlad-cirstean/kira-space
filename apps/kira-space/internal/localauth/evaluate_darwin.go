//go:build darwin && cgo

// This file wraps LocalAuthentication.framework's LAContext over cgo. It is mechanical
// and logic-free by design — Authorize's whole decision table lives in localauth.go, which a
// plain `go test` on Linux already covers — and it cannot be compiled or exercised outside a
// real macOS toolchain (CGO_ENABLED=1 GOOS=darwin go build fails here with "clang: error:
// unsupported option '-arch'"). Read it for structure, not for a compiler's blessing here;
// §6.3 of docs/HISTORY.md is what a human must still run on a real Mac to confirm it.
package localauth

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework LocalAuthentication -framework Foundation

#import <LocalAuthentication/LocalAuthentication.h>
#include <dispatch/dispatch.h>
#include <stdlib.h>

// kira_can_evaluate is the synchronous availability probe (LAContext.canEvaluatePolicy:error:),
// called from Go's Available() before Authorize ever attempts a real prompt — so a Mac with no
// Touch ID sensor and no login password (or one in a Screen Sharing session) never sees an
// evaluatePolicy attempt at all. Returns 1 if LAPolicyDeviceOwnerAuthentication can be evaluated,
// 0 otherwise.
static int kira_can_evaluate(void) {
	LAContext *context = [[LAContext alloc] init];
	NSError *error = nil;
	return [context canEvaluatePolicy:LAPolicyDeviceOwnerAuthentication error:&error] ? 1 : 0;
}

// kira_authorize wraps the asynchronous -[LAContext evaluatePolicy:localizedReason:reply:] behind
// one blocking call. LAPolicyDeviceOwnerAuthentication (not …WithBiometrics) is Touch ID *or*
// Apple Watch *or* the account password, whichever the machine and the moment allow — the system
// presents the password field itself, which is the fallback P14's SPEC row asks for (D4).
//
// The wait is a real dispatch_semaphore_t, signalled from inside the reply block once it fires —
// not a poll. This exists so the Go side is a single synchronous call with no callback lifetime
// to manage: a Go closure handed to this block could still be invoked long after whichever Go
// call created it has already returned, which is not a shape this package's caller can safely
// hold onto. The caller (evaluate(), below) must never be reached through
// application.InvokeSync — evaluatePolicy's own system sheet is itself modal on the main thread,
// so dispatching this call there would deadlock the UI against its own prompt.
//
// Returns 0 granted, 1 cancelled/denied, 2 unavailable, 3 timed out (the reply never arrived
// within timeoutSeconds — evaluatePolicy's own contract is to always eventually reply, so this is
// a defensive bound against a wedged call, not an outcome expected in normal use).
static int kira_authorize(const char *reason, double timeoutSeconds) {
	LAContext *context = [[LAContext alloc] init];

	NSError *availError = nil;
	if (![context canEvaluatePolicy:LAPolicyDeviceOwnerAuthentication error:&availError]) {
		return 2;
	}

	__block int result = 3;
	dispatch_semaphore_t sem = dispatch_semaphore_create(0);
	NSString *nsReason = [NSString stringWithUTF8String:reason];

	[context evaluatePolicy:LAPolicyDeviceOwnerAuthentication
	         localizedReason:nsReason
	                   reply:^(BOOL success, NSError *error) {
		result = success ? 0 : 1;
		dispatch_semaphore_signal(sem);
	}];

	dispatch_time_t deadline = dispatch_time(DISPATCH_TIME_NOW, (int64_t)(timeoutSeconds * NSEC_PER_SEC));
	if (dispatch_semaphore_wait(sem, deadline) != 0) {
		return 3;
	}
	return result;
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// authorizeTimeoutSeconds bounds kira_authorize's wait for evaluatePolicy's reply block —
// generous, since a real Touch ID/password prompt is paced by the person answering it, not by the
// OS: this exists only so a wedged call can never hang the calling goroutine forever, not to rush
// the user through it.
const authorizeTimeoutSeconds = 120

// Evaluate is this platform's EvaluateFunc.
func Evaluate(reason string) (Outcome, error) {
	cReason := C.CString(reason)
	defer C.free(unsafe.Pointer(cReason))

	switch C.kira_authorize(cReason, C.double(authorizeTimeoutSeconds)) {
	case 0:
		return Granted, nil
	case 1:
		return Cancelled, nil
	case 2:
		return Unavailable, nil
	default:
		return Unavailable, errors.New("localauth: the local authentication prompt did not answer within the timeout")
	}
}

// Available is this platform's AvailableFunc.
func Available() bool {
	return C.kira_can_evaluate() != 0
}
