// Package buildinfo carries the one fact a build knows about itself that the source cannot: which
// version it is. Everything else about the app is in the tree; this is stamped onto it.
//
// Kira Space's own copy of apps/kira-space/internal/buildinfo — deliberately not
// hoisted to repo-root internal/, since it is genuinely just this one Version var with no shared
// logic worth sharing (see internal/kirapaths and internal/sqlitex for the packages that were).
package buildinfo

// Version is this app's version string, and the single place it is defined.
//
// The literal here is the *development* value. A real build overwrites it at link time from the
// version in build/config.yml's info.version — apps/kira-space/build/darwin/Taskfile.yml's
// BUILD_FLAGS passes `-ldflags "-X …/internal/buildinfo.Version=<version>"`.
//
// It is deliberately a var, not a const: `-X` can only write to a string variable.
var Version = "0.0.0-dev"
