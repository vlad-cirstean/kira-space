package grpcclient

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/sync/singleflight"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

// SourceMode names one of the two descriptor sources (D4).
type SourceMode string

const (
	SourceReflection SourceMode = "reflection"
	SourceProto      SourceMode = "proto"
)

// MetaPair is one gRPC metadata name/value pair — bridge/grpc.go hands this straight across the
// wire both ways (GrpcCallArgs.Metadata in, CallResult.Header/Trailer out), so it carries JSON
// tags directly rather than a second bridge-owned type, the same way HttpSendArgs reuses
// httpclient.Header verbatim.
type MetaPair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// TLSConfig is D6's dial-time TLS decision. InsecureSkipVerify is deliberately not a field here —
// P2 D4's "verification is always on, with no per-request opt-out" (§0.2), and this package never
// constructs a tls.Config that would bypass it.
type TLSConfig struct {
	Enabled    bool   `json:"enabled"`
	CAFile     string `json:"caFile"`
	ServerName string `json:"serverName"`
}

// Source names one descriptor source (D4) — either a live server's reflection, or a supplied
// .proto file. Target/TLS/Metadata are meaningful only for SourceReflection; ProtoPath/ImportPaths
// only for SourceProto.
type Source struct {
	Mode SourceMode

	Target   string
	TLS      TLSConfig
	Metadata []MetaPair

	ProtoPath   string
	ImportPaths []string
}

// Schema is the projection the renderer browses (D4) — never a descriptor, never a registry: the
// renderer must not parse a wire protocol (§0.3), and a full descriptor set is megabytes.
type Schema struct {
	Services []Service `json:"services"`
	Mode     string    `json:"mode"`
	Warnings []string  `json:"warnings"`
}

type Service struct {
	Name string `json:"name"` // short, for display
	// FullName is the package-qualified name a Call addresses.
	FullName string   `json:"fullName"`
	Methods  []Method `json:"methods"`
}

type Method struct {
	Name            string `json:"name"`
	FullName        string `json:"fullName"`
	ClientStreaming bool   `json:"clientStreaming"`
	ServerStreaming bool   `json:"serverStreaming"`
	InputType       string `json:"inputType"`
	OutputType      string `json:"outputType"`
	// RequestTemplate is the empty-instance JSON of the input message — EmitUnpopulated +
	// Multiline — the "fill this in" template the message editor seeds with. Deliberately not
	// recursive into nested message fields (a self-referential message would not terminate); a
	// nested message field renders as {} and the user fills it in.
	RequestTemplate string `json:"requestTemplate"`
}

// CallResult is the terminal outcome of a call (call.go's Unary/ServerStream) — a non-OK
// gRPC status lives HERE, not in the returned error. Messages carries every message this call
// produced, up to maxStoredMessages: exactly one for Unary ("a unary call is the same pane
// with exactly one entry"), up to that cap for ServerStream (which also streams each one to its
// own onMessage callback as it arrives, uncapped — this field exists for recordGrpcHistory's own
// persistence input and a caller that missed live events, not as the live view's own source).
// MessageCount/MessageBytes are always the true, uncapped totals regardless of len(Messages) —
// what an elision check (repos/grpc_history.go) must compare against, not this slice's length.
type CallResult struct {
	Code          int32      `json:"code"`
	CodeName      string     `json:"codeName"`
	StatusMessage string     `json:"statusMessage"`
	ElapsedMs     int64      `json:"elapsedMs"`
	Header        []MetaPair `json:"header"`
	Trailer       []MetaPair `json:"trailer"`
	MessageCount  int        `json:"messageCount"`
	MessageBytes  int        `json:"messageBytes"`
	Messages      []Message  `json:"messages,omitempty"`
}

// Message is one message of a call — a wire size (the WireLength, approximated here by the encoded
// message length since this package installs no stats.Handler) and an arrival offset from the
// call's own start (these are real and are the single most informative thing about a stream).
type Message struct {
	Seq       int    `json:"seq"`
	JSON      string `json:"json"`
	WireBytes int    `json:"wireBytes"`
	OffsetMs  int64  `json:"offsetMs"`
}

// resolved is one Source's linked descriptors, cached for the life of the process. files is
// this Source's OWN private *protoregistry.Files — never protoregistry.GlobalFiles anywhere in
// this package (a duplicate file path there PANICS; two users' .proto files both declaring
// the same package is an ordinary situation for this feature, and a private registry returns an
// error instead).
type resolved struct {
	files    *protoregistry.Files
	mode     string
	warnings []string
}

// maxCachedDescriptors bounds descriptorCache by entry count — a plain LRU, evicting
// the least-recently-used Source's descriptors once the cache holds more than this many. Each
// entry can be a multi-megabyte descriptor registry (a large .proto, or a server with a big API
// surface), so an unbounded map here was a real, unbounded leak, compounded by every dynamic
// metadata value (e.g. {{$guid}}, an ordinary feature) previously minting its own cache key —
// see cacheKey's own comment below.
//
// A plain entry count doesn't cap the resource (memory) — 32 entries of a large API surface's
// registry could still retain hundreds of MB for the life of the process. Rebounded here by an
// approximate byte budget instead (maxCachedDescriptorBytes below), tracked via each entry's own
// descriptor-set size at insert time (approxDescriptorBytes) — chosen over lowering the count
// because a fixed count can never adapt to how large any one Source's descriptor set is: a
// byte budget evicts exactly when it should, whether that is one huge registry or many small ones.
//
// A var, not a const: descriptor_cache_test.go temporarily lowers it to exercise eviction against
// a real, small compiled descriptor set rather than needing to allocate 64+ MiB of registries in a
// test to cross the production default.
var maxCachedDescriptorBytes int64 = 64 * 1024 * 1024 // 64 MiB

// descriptorCache is the in-memory-only cache, keyed by the Source's own fields — which for
// reflection includes the resolved target. The key is a SHA-256 digest, never the
// fields themselves: this map is unexported, has no accessor that returns a key, is never
// serialised and never logged, which is what keeps a secret out of it even though the fields it
// is derived from can contain one. map[string]*list.Element + container/list is this codebase's
// own established budgeted-cache shape (internal/enginecache.ByteLru) — not reused directly:
// that type's EntryMeta is shaped for the DB-adapter query cache (ConnectionID/Path/Label) and
// grpcclient imports nothing else in this module, a property worth keeping for a package this
// self-contained.
var descriptorCache = struct {
	mu         sync.Mutex
	byKey      map[string]*list.Element // Value is *descriptorCacheEntry
	order      *list.List               // front = least recently used, back = most recently used
	totalBytes int64
	// generation is the staleness guard, keyed the same as byKey but never pruned when an
	// entry is evicted or invalidated — a key's counter only ever goes up. InvalidateCache bumps a
	// key's counter; resolveSource captures it right before starting a resolution and passes it to
	// descriptorCachePut, which skips the Put outright when the counter has since moved. Without
	// this, a resolution already in flight when Reload calls InvalidateCache would finish after the
	// invalidation and Put the stale descriptors right back.
	generation map[string]int64
}{byKey: map[string]*list.Element{}, order: list.New(), generation: map[string]int64{}}

// resolveGroup collapses concurrent resolveSource calls for the same key into one underlying
// resolution (F11) — without this, two Calls (or a Call racing a Describe) against the same
// not-yet-cached Source each pay their own reflection round trip or .proto parse.
var resolveGroup singleflight.Group

type descriptorCacheEntry struct {
	key   string
	r     *resolved
	bytes int64
}

// approxDescriptorBytes sums the encoded size of every file registered in files — an
// approximation (it re-marshals each FileDescriptorProto rather than tracking whatever the wire
// size once was), cheap enough to pay once per Put and never per Get. A nil files (an empty
// *resolved{}, as this package's own tests construct) ranges over nothing and costs 0, exactly
// like protoregistry.Files' own nil-receiver-safe RangeFiles.
func approxDescriptorBytes(files *protoregistry.Files) int64 {
	var total int64
	files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		total += int64(proto.Size(protodesc.ToFileDescriptorProto(fd)))
		return true
	})
	return total
}

// cacheKey excludes metadata *values* on purpose: reflection's own resolved metadata
// used to be hashed in full, so a dynamic value substituted fresh per call (e.g. {{$guid}} in an
// auth header) minted a brand-new key on every single call, guaranteeing a cache miss — defeating
// the documented "a Call following a Describe on the same Source costs no second reflection round
// trip" property for any Source using one. Metadata *names* stay in the key (a plausible, cheap
// signal that two calls address genuinely different schemas, e.g. a tenant-routing header), only
// the value each name resolved to is dropped.
//
// Mode-aware, hashing only the fields the Source doc above says are
// meaningful for that mode. Target/TLS/Metadata are reflection-only, ProtoPath/ImportPaths are
// proto-only — hashing both regardless of mode meant Describe (bridge/grpc.go's
// resolveGrpcSource, which sends proto mode's target unresolved and metadata as nil) and Call
// (resolveGrpcCallSource, which sends the real resolved target and metadata rows) landed on
// different keys for the very same proto Source whenever the target has a {{ref}} or there is any
// metadata row — the normal case. InvalidateCache is only ever called from Describe, so Call's
// entry was never invalidated by Reload in proto mode. Restricting the hash to the
// fields that mode uses makes Describe's and Call's keys identical by construction.
func cacheKey(src Source) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00", src.Mode)
	switch src.Mode {
	case SourceProto:
		fmt.Fprintf(h, "%s\x00", src.ProtoPath)
		for _, p := range src.ImportPaths {
			fmt.Fprintf(h, "%s\x00", p)
		}
	default: // SourceReflection
		fmt.Fprintf(h, "%s\x00%t\x00%s\x00%s\x00", src.Target, src.TLS.Enabled, src.TLS.CAFile, src.TLS.ServerName)
		for _, m := range src.Metadata {
			fmt.Fprintf(h, "%s\x00", m.Name)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// descriptorCacheGet/Put/evictLocked assume descriptorCache.mu is already held by the caller —
// kept as separate functions only so resolveSource's own hit/miss shape stays readable.
func descriptorCacheGet(key string) (*resolved, bool) {
	descriptorCache.mu.Lock()
	defer descriptorCache.mu.Unlock()
	el, ok := descriptorCache.byKey[key]
	if !ok {
		return nil, false
	}
	descriptorCache.order.MoveToBack(el)
	return el.Value.(*descriptorCacheEntry).r, true
}

// descriptorCacheGeneration reads key's current invalidation counter (F11) — resolveSource calls
// this right before starting a resolution and passes the result back to descriptorCachePut, which
// only stores the result when the counter is still the same.
func descriptorCacheGeneration(key string) int64 {
	descriptorCache.mu.Lock()
	defer descriptorCache.mu.Unlock()
	return descriptorCache.generation[key]
}

func descriptorCachePut(key string, gen int64, r *resolved) {
	size := approxDescriptorBytes(r.files)

	descriptorCache.mu.Lock()
	defer descriptorCache.mu.Unlock()
	// F11: key was invalidated (Reload) after this resolution started — the descriptors just
	// fetched are already stale, so drop them instead of overwriting whatever a later resolution
	// (or a cache miss) will produce.
	if descriptorCache.generation[key] != gen {
		return
	}
	if el, ok := descriptorCache.byKey[key]; ok {
		entry := el.Value.(*descriptorCacheEntry)
		descriptorCache.totalBytes += size - entry.bytes
		entry.r, entry.bytes = r, size
		descriptorCache.order.MoveToBack(el)
	} else {
		el := descriptorCache.order.PushBack(&descriptorCacheEntry{key: key, r: r, bytes: size})
		descriptorCache.byKey[key] = el
		descriptorCache.totalBytes += size
	}
	// order.Len() > 1: always keep at least the entry just inserted, even alone over budget — a
	// single Source whose own descriptor set exceeds maxCachedDescriptorBytes should still get the
	// documented "a Call following a Describe costs no second round trip" property, not be evicted
	// by its own Put before anything can ever hit it.
	for descriptorCache.totalBytes > maxCachedDescriptorBytes && descriptorCache.order.Len() > 1 {
		oldest := descriptorCache.order.Front()
		entry := oldest.Value.(*descriptorCacheEntry)
		descriptorCache.order.Remove(oldest)
		delete(descriptorCache.byKey, entry.key)
		descriptorCache.totalBytes -= entry.bytes
	}
}

func resolveSource(ctx context.Context, src Source) (*resolved, error) {
	key := cacheKey(src)

	if r, ok := descriptorCacheGet(key); ok {
		return r, nil
	}

	// singleflight collapses concurrent resolveSource calls sharing key into the one call
	// below, so two Calls (or a Call racing a Describe) against the same not-yet-cached Source
	// only ever pay for one reflection round trip or .proto parse.
	//
	// The shared work runs detached from any one caller's ctx: the leader pressing Stop must not
	// fail every other waiter. It stays bounded (reflection and .proto timeouts); each caller
	// still returns on its own ctx.
	workCtx := context.WithoutCancel(ctx)
	ch := resolveGroup.DoChan(key, func() (any, error) {
		// Re-check: another goroutine may have already resolved and cached key while this one
		// waited to become the singleflight leader.
		if r, ok := descriptorCacheGet(key); ok {
			return r, nil
		}
		// Captured right before resolving starts, so a concurrent InvalidateCache anywhere in the
		// resolution's lifetime — including one that lands after this call becomes leader but
		// before it finishes — is visible to descriptorCachePut below.
		gen := descriptorCacheGeneration(key)

		var r *resolved
		var err error
		switch src.Mode {
		case SourceReflection:
			r, err = resolveReflection(workCtx, src)
		case SourceProto:
			r, err = resolveProto(workCtx, src)
		default:
			return nil, BadRequest(fmt.Sprintf("unrecognised descriptor source %q", src.Mode))
		}
		if err != nil {
			return nil, err
		}

		descriptorCachePut(key, gen, r)
		return r, nil
	})
	select {
	case <-ctx.Done():
		return nil, Cancelled("request was cancelled")
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		return res.Val.(*resolved), nil
	}
}

// InvalidateCache drops one Source's cached descriptors — the UI's explicit Reload action (D4).
// Descriptors are never invalidated by a timer: a cached-but-stale schema silently disagreeing
// with a live server is a worse failure than one refetch.
func InvalidateCache(src Source) {
	key := cacheKey(src)
	descriptorCache.mu.Lock()
	defer descriptorCache.mu.Unlock()
	// F11: bumped even though the generation map is never pruned — a resolution already in flight
	// for key captured the pre-bump value and must not Put its (now stale) result.
	descriptorCache.generation[key]++
	if el, ok := descriptorCache.byKey[key]; ok {
		descriptorCache.order.Remove(el)
		delete(descriptorCache.byKey, key)
		descriptorCache.totalBytes -= el.Value.(*descriptorCacheEntry).bytes
	}
}

// Describe resolves src's services and methods (D3) — from server reflection or a compiled
// .proto, transparently: both sources produce plain protoreflect.FileDescriptors (F1, F2), so
// there is one projection for both.
func Describe(ctx context.Context, src Source) (Schema, error) {
	r, err := resolveSource(ctx, src)
	if err != nil {
		return Schema{}, err
	}
	return projectSchema(r), nil
}

// registerTransitive registers fd and every file it (transitively) imports into reg — never
// protoregistry.GlobalFiles — skipping a file already registered under this path. This is
// what makes the recursive reflection fallback and a .proto's well-known-type imports
// both land in the one registry a Source's Schema is projected from.
func registerTransitive(reg *protoregistry.Files, fd protoreflect.FileDescriptor) error {
	if _, err := reg.FindFileByPath(fd.Path()); err == nil {
		return nil
	}
	imports := fd.Imports()
	for i := 0; i < imports.Len(); i++ {
		if err := registerTransitive(reg, imports.Get(i).FileDescriptor); err != nil {
			return err
		}
	}
	if err := reg.RegisterFile(fd); err != nil {
		// F14: a private *protoregistry.Files (never GlobalFiles) returns an error here rather
		// than panicking — turned into a legible E_BAD_REQUEST rather than propagated raw.
		return SchemaError(fmt.Sprintf("registering %s: %s", fd.Path(), err))
	}
	return nil
}

// projectSchema walks every file in r.files and every service in it — the inverse of
// registerTransitive: a well-known-type import (timestamp.proto, …) declares no service and so
// contributes nothing here.
func projectSchema(r *resolved) Schema {
	schema := Schema{Mode: r.mode, Warnings: r.warnings, Services: []Service{}}
	r.files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		services := fd.Services()
		for i := 0; i < services.Len(); i++ {
			schema.Services = append(schema.Services, projectService(services.Get(i)))
		}
		return true
	})
	return schema
}

func projectService(sd protoreflect.ServiceDescriptor) Service {
	methods := sd.Methods()
	svc := Service{Name: string(sd.Name()), FullName: string(sd.FullName()), Methods: make([]Method, 0, methods.Len())}
	for i := 0; i < methods.Len(); i++ {
		md := methods.Get(i)
		svc.Methods = append(svc.Methods, Method{
			Name:            string(md.Name()),
			FullName:        string(md.FullName()),
			ClientStreaming: md.IsStreamingClient(),
			ServerStreaming: md.IsStreamingServer(),
			InputType:       string(md.Input().FullName()),
			OutputType:      string(md.Output().FullName()),
			RequestTemplate: requestTemplate(md.Input()),
		})
	}
	return svc
}

// requestTemplate is D4's own one-liner: the empty-instance JSON of the input message,
// EmitUnpopulated + Multiline — costs one protojson.Marshal over a fresh dynamicpb.NewMessage, and
// is deliberately not recursive past one level (a self-referential message would not terminate).
func requestTemplate(input protoreflect.MessageDescriptor) string {
	msg := dynamicpb.NewMessage(input)
	opts := protojson.MarshalOptions{EmitUnpopulated: true, Multiline: true, Indent: "  "}
	b, err := opts.Marshal(msg)
	if err != nil {
		return "{}"
	}
	// protojson randomises its whitespace per build; re-indent for stable text.
	var out bytes.Buffer
	if err := json.Indent(&out, b, "", "  "); err != nil {
		return "{}"
	}
	return out.String()
}

func splitFullMethod(fullMethod string) (service protoreflect.FullName, method protoreflect.Name, err error) {
	trimmed := strings.TrimPrefix(fullMethod, "/")
	idx := strings.LastIndex(trimmed, "/")
	if idx < 0 || idx == len(trimmed)-1 {
		return "", "", BadRequest(`method must be of the form "pkg.Service/Method"`)
	}
	return protoreflect.FullName(trimmed[:idx]), protoreflect.Name(trimmed[idx+1:]), nil
}

// resolveMethod finds the method descriptor for a "/pkg.Service/Method"-shaped full method (the
// same string conn.Invoke/NewStream take), through the same cached resolution Describe uses — so
// a Call that follows a Describe on the same Source costs no second reflection round trip.
func resolveMethod(ctx context.Context, src Source, fullMethod string) (protoreflect.MethodDescriptor, error) {
	svcName, methodName, err := splitFullMethod(fullMethod)
	if err != nil {
		return nil, err
	}
	r, err := resolveSource(ctx, src)
	if err != nil {
		return nil, err
	}
	desc, err := r.files.FindDescriptorByName(svcName)
	if err != nil {
		return nil, SchemaError(fmt.Sprintf("service %s not found in the resolved schema: %s", svcName, err))
	}
	svc, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, SchemaError(fmt.Sprintf("%s is not a service", svcName))
	}
	method := svc.Methods().ByName(methodName)
	if method == nil {
		return nil, SchemaError(fmt.Sprintf("method %s not found on service %s", methodName, svcName))
	}
	return method, nil
}
