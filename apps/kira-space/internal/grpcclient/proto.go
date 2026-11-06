package grpcclient

import (
	"context"
	"path/filepath"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// resolveProto is the second descriptor source: protocompile over a supplied .proto file
// plus its import paths — no protoc binary anywhere (this app ships as a signed, sandboxed .app
// and cannot assume a toolchain on the user's machine). WithStandardImports resolves the
// well-known types (timestamp/duration/struct/empty/any/wrappers) from the protobuf module's own
// embedded copies, so a .proto that imports one compiles with nothing installed.
func resolveProto(ctx context.Context, src Source) (*resolved, error) {
	if src.ProtoPath == "" {
		return nil, BadRequest("a .proto file is required")
	}

	ctx, cancel := context.WithTimeout(ctx, defaultReflectionTimeout)
	defer cancel()

	// D4: import paths default to the chosen file's own directory, and the user can add more —
	// the file's own directory always goes first so its own basename is always resolvable.
	dir := filepath.Dir(src.ProtoPath)
	paths := []string{dir}
	for _, p := range src.ImportPaths {
		if p != "" && p != dir {
			paths = append(paths, p)
		}
	}
	base := filepath.Base(src.ProtoPath)

	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{ImportPaths: paths}),
	}
	files, err := compiler.Compile(ctx, base)
	if err != nil {
		// F1: protocompile's own error is already "file:line:col: message" and the first error
		// rather than a cascade — directly renderable (D17), passed through verbatim.
		return nil, SchemaError(err.Error())
	}

	reg := new(protoregistry.Files)
	for _, f := range files {
		if err := registerTransitive(reg, f); err != nil {
			return nil, err
		}
	}
	return &resolved{files: reg, mode: string(SourceProto)}, nil
}
