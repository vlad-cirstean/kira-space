package configfs

import "path/filepath"

// RootName is the config subfolder of an app home; it can later be a git repo.
const RootName = "config"

// Root is <home>/config.
func Root(home string) string { return filepath.Join(home, RootName) }

// ModuleDir is <home>/config/<module>, the one config-root resolver.
func ModuleDir(home, module string) string { return filepath.Join(Root(home), module) }
