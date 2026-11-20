//go:build !server

package main

import (
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/bridge"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/editorlaunch"
)

func newEditorLauncher() bridge.EditorLauncher { return editorlaunch.VSCode{} }

func newTaskRevealer(opener bridge.TaskRevealer) bridge.TaskRevealer { return opener }
