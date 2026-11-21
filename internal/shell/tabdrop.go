package shell

import (
	"math"
	"os"
	"runtime"
	"strings"
)

// WindowGeom is one live window as the tab-drop hit-test sees it.
type WindowGeom struct {
	Key       string
	Order     int
	Bounds    WindowBounds
	Ephemeral bool
	// Hidden: not visible or minimised; never a drop target.
	Hidden bool
	// FocusSeq: higher means focused more recently.
	FocusSeq uint64
}

// DropInput is the renderer's pointerup, grab point and viewport size, in CSS px.
type DropInput struct{ ClientX, ClientY, GrabX, GrabY, InnerWidth, InnerHeight float64 }

// clientOrigin is the non-client inset of the frame: side borders are equal and the bottom border
// equals a side border, so the title bar is what is left of the height. Frame equal to viewport
// (macOS full-size content) gives 0, 0.
func clientOrigin(src WindowBounds, innerW, innerH float64) (x, y float64) {
	x = math.Max(0, (src.Width-innerW)/2)
	y = math.Max(0, src.Height-innerH-x)
	return x, y
}

// DropPoint is the screen point of the drop: source origin plus client inset plus client coords.
// Never PointerEvent.screenX: WebKit reports 0,0 under synthetic input and Bounds() shares one
// coordinate space with every target's Bounds().
func DropPoint(src WindowBounds, in DropInput) (x, y float64) {
	ox, oy := clientOrigin(src, in.InnerWidth, in.InnerHeight)
	return src.X + ox + in.ClientX, src.Y + oy + in.ClientY
}

func contains(b WindowBounds, x, y float64) bool {
	return x >= b.X && x < b.X+b.Width && y >= b.Y && y < b.Y+b.Height
}

// DropTarget picks the window under the point: inSource when the point lies inside src's own
// frame (a Windows title-bar drop); else the live, visible, non-ephemeral window other than src
// containing the point with the highest FocusSeq. ok is false on a miss.
func DropTarget(geoms []WindowGeom, src string, x, y float64) (key string, inSource, ok bool) {
	var best *WindowGeom
	for i := range geoms {
		g := &geoms[i]
		if g.Key == src {
			if contains(g.Bounds, x, y) {
				return src, true, false
			}
			continue
		}
		if g.Ephemeral || g.Hidden || !contains(g.Bounds, x, y) {
			continue
		}
		if best == nil || g.FocusSeq > best.FocusSeq {
			best = g
		}
	}
	if best == nil {
		return "", false, false
	}
	return best.Key, false, true
}

// TornBounds is a new window the size of src whose grab point sits under the drop point.
func TornBounds(src WindowBounds, in DropInput) WindowBounds {
	px, py := DropPoint(src, in)
	ox, oy := clientOrigin(src, in.InnerWidth, in.InnerHeight)
	return WindowBounds{X: px - ox - in.GrabX, Y: py - oy - in.GrabY, Width: src.Width, Height: src.Height}
}

// WindowPositionsKnown reports whether window positions are global: false on Linux Wayland unless
// GDK_BACKEND forces x11. isWayland nil uses IsWayland.
func WindowPositionsKnown(isWayland func() bool) bool {
	if runtime.GOOS != "linux" {
		return true
	}
	if strings.Contains(os.Getenv("GDK_BACKEND"), "x11") {
		return true
	}
	if isWayland == nil {
		isWayland = IsWayland
	}
	return !isWayland()
}
