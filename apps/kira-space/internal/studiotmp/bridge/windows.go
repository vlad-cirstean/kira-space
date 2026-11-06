package bridge

import (
	"github.com/kirathecat/kira-studio/internal/windowsvc"
)

// WindowsService is Kira Studio's own binding-name shim over internal/windowsvc.Service — Wails'
// binding generator builds a call's FQN from the *registered* type's own package,
// promoted methods included, so embedding here keeps this app's binding names app-local
// while every method body lives once in internal/windowsvc.
type WindowsService struct {
	*windowsvc.Service
}
