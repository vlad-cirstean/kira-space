package dbmcp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestBindHTTPRefusesFallbackOnPortConflict guards: a DefaultPort conflict must refuse to
// start, never silently fall back to an OS-assigned ephemeral port — every existing registration
// names DefaultPort, and a fallback would leave it pointing at whatever else is now listening
// there, with the live bearer token going to it on the next connection attempt.
func TestBindHTTPRefusesFallbackOnPortConflict(t *testing.T) {
	occupied, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", DefaultPort))
	if err != nil {
		t.Skipf("cannot occupy DefaultPort %d to set up this test's own precondition: %v", DefaultPort, err)
	}
	t.Cleanup(func() { _ = occupied.Close() })

	s := &Server{}
	err = s.bindHTTP()
	if err == nil {
		t.Fatalf("bindHTTP succeeded despite DefaultPort %d being taken — want a refusal, not a fallback bind", DefaultPort)
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("%d", DefaultPort)) {
		t.Fatalf("error = %q, want it to name the conflicting port", err.Error())
	}
	if s.listener != nil {
		t.Fatal("s.listener was set despite bindHTTP returning an error — no fallback listener must be left behind")
	}
}

// TestRequestCancellationReachesDetachedHandlerContext guards P168 Part 6 F2: go-sdk detaches the
// tool handler's context from the HTTP request, so a client that goes away must still cancel it.
func TestRequestCancellationReachesDetachedHandlerContext(t *testing.T) {
	handlerCtx := make(chan context.Context, 1)
	h := withRequestContext(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		detached := context.WithoutCancel(r.Context())
		ctx, cancel := bindRequestCancellation(detached)
		defer cancel()
		handlerCtx <- ctx
		<-ctx.Done()
	}))

	reqCtx, cancelReq := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/mcp", nil).WithContext(reqCtx))
		close(done)
	}()
	ctx := <-handlerCtx
	if ctx.Err() != nil {
		t.Fatal("handler context cancelled before the request ended")
	}
	cancelReq()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler context not cancelled after the request context ended")
	}
}
