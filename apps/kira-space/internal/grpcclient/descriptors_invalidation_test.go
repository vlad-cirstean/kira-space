package grpcclient

import (
	"container/list"
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// TestDescriptorCachePut_SkipsStaleGenerationAfterInvalidate is the staleness guard: a
// resolution already in flight when InvalidateCache runs must not Put its (now stale) result back
// into the cache. descriptorCachePut takes the generation resolveSource captured before it started
// resolving; InvalidateCache bumps the key's generation, so a Put carrying the pre-bump value must
// be skipped outright.
func TestDescriptorCachePut_SkipsStaleGenerationAfterInvalidate(t *testing.T) {
	descriptorCache.mu.Lock()
	descriptorCache.byKey = map[string]*list.Element{}
	descriptorCache.order.Init()
	descriptorCache.totalBytes = 0
	descriptorCache.generation = map[string]int64{}
	descriptorCache.mu.Unlock()

	src := Source{Mode: SourceProto, ProtoPath: "/tmp/stale.proto"}
	key := cacheKey(src)

	// The generation a resolution starting "now" would capture.
	gen := descriptorCacheGeneration(key)

	// Reload happens while that resolution is still in flight.
	InvalidateCache(src)

	// The in-flight resolution finishes and tries to Put its result under the pre-invalidation
	// generation it captured — must be dropped, not resurrected into the cache.
	descriptorCachePut(key, gen, &resolved{})
	if _, ok := descriptorCacheGet(key); ok {
		t.Fatal("descriptorCachePut stored a result under a generation InvalidateCache had already bumped past")
	}

	// A Put that captures the CURRENT generation (a resolution started after the invalidation)
	// must still succeed normally.
	descriptorCachePut(key, descriptorCacheGeneration(key), &resolved{})
	if _, ok := descriptorCacheGet(key); !ok {
		t.Fatal("descriptorCachePut with the current generation was dropped, want stored")
	}
}

// TestResolveSource_ConcurrentCallsCollapseIntoOneReflectionResolution: concurrent resolveSource
// calls for the same not-yet-cached Source must collapse into one
// underlying resolution (golang.org/x/sync/singleflight), not one reflection round trip per
// caller. Counted at the transport level (a grpc.StreamInterceptor around the real reflection
// service) rather than by mocking resolveReflection, so the assertion holds for the actual
// production call path.
func TestResolveSource_ConcurrentCallsCollapseIntoOneReflectionResolution(t *testing.T) {
	protoPath := writeProto(t, "echo.proto", echoProtoSource)
	svc := compileService(t, protoPath)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}

	var calls int32
	srv := grpc.NewServer(grpc.StreamInterceptor(func(
		srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler,
	) error {
		if info.FullMethod == "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo" {
			atomic.AddInt32(&calls, 1)
		}
		return handler(srv, ss)
	}))
	registerEcho(srv, svc, &echoImpl{})
	registerGlobalForReflection(t, svc.ParentFile())
	reflection.Register(srv)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	waitEchoServerReady(t, lis.Addr().String())
	// waitEchoServerReady's own readiness probe issues a real ServerReflectionInfo round trip —
	// discount it so the count below reflects only the concurrent resolveSource calls under test.
	atomic.StoreInt32(&calls, 0)

	src := Source{Mode: SourceReflection, Target: lis.Addr().String()}
	descriptorCache.mu.Lock()
	delete(descriptorCache.byKey, cacheKey(src))
	descriptorCache.mu.Unlock()

	const n = 20
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := resolveSource(context.Background(), src)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("resolveSource: %v", err)
		}
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("ServerReflectionInfo was called %d times for %d concurrent resolveSource calls on the same uncached Source, want exactly 1 (singleflight should collapse them)", got, n)
	}
}
