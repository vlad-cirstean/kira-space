package clickhouse

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-studio/internal/adapters"
)

// ClickHouse's own numeric result codes (src/Common/ErrorCodes.cpp@master) — errors.ts's fifteen-
// code table, ported verbatim; only the extraction of the code itself changes (B12).
const (
	notImplemented     = "48"
	unknownDatabase    = "81"
	timeoutExceeded    = "159"
	readonlyCode       = "164"
	unknownUser        = "192"
	wrongPassword      = "193"
	requiredPassword   = "194"
	ipAddressNotAllow  = "195"
	socketTimeout      = "209"
	networkError       = "210"
	tableIsReadOnly    = "242"
	databaseAccessDeny = "291"
	queryWasCancelled  = "394"
	accessDenied       = "497"
	authenticationFail = "516"
)

// codeMessageRE is B12's second and third extraction sites: the body's own leading
// "Code: N. DB::Exception: ..." envelope, both on a non-2xx response and inside a mid-stream
// __exception__ trailer.
var codeMessageRE = regexp.MustCompile(`Code:\s*(\d+)\.\s*(.*)`)

// extractCodeFromBody is B12's second/third extraction site (the first, the
// X-ClickHouse-Exception-Code header, is read by the caller before falling back to this).
func extractCodeFromBody(body string) (code, message string) {
	trimmed := strings.TrimSpace(body)
	if m := codeMessageRE.FindStringSubmatch(trimmed); m != nil {
		return m[1], strings.TrimSpace(m[2])
	}
	return "", trimmed
}

// mapCode is errors.ts's own mapError, minus the AdapterError passthrough (nothing in this Go
// package ever wraps one) and re-pointed at a (code, message) pair the caller already extracted
// rather than a driver error object — dispatched on the numeric code, never on message text.
func mapCode(code, message string) error {
	switch code {
	case unknownDatabase:
		return adapters.New(adapters.CodeNotFound, message, nil)
	case timeoutExceeded, socketTimeout:
		return adapters.New(adapters.CodeTimeout, message, nil)
	case readonlyCode:
		return adapters.New(adapters.CodeUnsupported, message, adapters.RefusedWrite(nil))
	case tableIsReadOnly:
		return adapters.New(adapters.CodeUnsupported, message, nil)
	case unknownUser, wrongPassword, requiredPassword, authenticationFail, ipAddressNotAllow,
		databaseAccessDeny, accessDenied:
		return adapters.New(adapters.CodeAuth, message, nil)
	case queryWasCancelled:
		return adapters.New(adapters.CodeCancelled, message, nil)
	case notImplemented:
		return adapters.New(adapters.CodeQuery, message, nil)
	case networkError:
		return adapters.New(adapters.CodeConnect, message, nil)
	default:
		return adapters.New(adapters.CodeQuery, message, nil)
	}
}

// mapTransportError is errors.ts's own network-level fallback, re-derived against Go's own error
// types rather than Node's errno strings (ECONNREFUSED/ENOTFOUND/EHOSTUNREACH/ETIMEDOUT) — the same
// re-derivation postgres/errors.go and mysqlfamily/errors.go already did for their own drivers.
func mapTransportError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return adapters.New(adapters.CodeTimeout, err.Error(), err)
	}
	ne := adapters.ClassifyNetError(err)
	if ne.NetTimeout {
		return adapters.New(adapters.CodeTimeout, err.Error(), err)
	}
	if ne.DNS {
		return adapters.New(adapters.CodeConnect, err.Error(), err)
	}
	if ne.OpError != nil {
		if errors.Is(ne.OpError.Err, os.ErrDeadlineExceeded) {
			return adapters.New(adapters.CodeTimeout, err.Error(), err)
		}
		return adapters.New(adapters.CodeConnect, err.Error(), err)
	}
	return adapters.New(adapters.CodeQuery, err.Error(), err)
}

// mapHTTPError is B12's first and second extraction sites: the X-ClickHouse-Exception-Code header
// when present (a header, present on a failed request), else the body's own leading "Code: N."
// prefix.
func mapHTTPError(headerCode string, body string) error {
	if headerCode != "" {
		if _, err := strconv.Atoi(headerCode); err == nil {
			_, message := extractCodeFromBody(body)
			return mapCode(headerCode, message)
		}
	}
	code, message := extractCodeFromBody(body)
	return mapCode(code, message)
}

// mapExceptionTrailer is B12's third extraction site: a 200-OK response that streamed real rows
// and then failed mid-stream appends a `__exception__` trailer whose own body carries the same
// "Code: N. DB::Exception: ..." envelope the non-2xx case does (CH-1's own measured layout).
func mapExceptionTrailer(trailer string) error {
	code, message := extractCodeFromBody(trailer)
	return mapCode(code, message)
}
