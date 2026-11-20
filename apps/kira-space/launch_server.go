//go:build server

package main

import "github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"

// The server build's host is not the viewer's machine: reveal and open would act on the wrong one.
func newEditorLauncher() bridge.EditorLauncher { return nil }

func newTaskRevealer(bridge.TaskRevealer) bridge.TaskRevealer { return nil }
