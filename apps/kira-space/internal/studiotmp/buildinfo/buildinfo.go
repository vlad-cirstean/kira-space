// Package buildinfo carries the one fact a build knows about itself that the source cannot: which
// version it is. Everything else about the app is in the tree; this is stamped onto it.
package buildinfo

// Version is the app's version string, and the single place it is defined.
//
// The literal here is the *development* value. A real build overwrites it at link time from the
// version in the repository-root package.json — `apps/kira-studio/build/darwin/Taskfile.yml`'s
// BUILD_FLAGS passes `-ldflags "-X …/internal/buildinfo.Version=<version>"` — and the release
// workflow writes the git tag into that package.json before packaging, so a tagged build reports
// the tag. `go run`/`go test`, which link no such flag, report this literal instead.
//
// The same value is stamped into the bundle's Info.plist (`create:app:bundle`), so what the app
// says about itself and what macOS says about it cannot drift apart.
//
// It is deliberately a var, not a const: `-X` can only write to a string variable.
var Version = "0.0.0-dev"
