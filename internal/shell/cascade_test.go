package shell

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestCascadeSized(t *testing.T) {
	rect := func(x, y, w, h int) application.Rect { return application.Rect{X: x, Y: y, Width: w, Height: h} }
	tests := []struct {
		name string
		from application.Rect
		work *application.Rect
		want WindowBounds
	}{
		{"large work area", rect(100, 100, 1024, 640), ptr(rect(0, 0, 2560, 1415)), WindowBounds{124, 124, 1152, 720}},
		{"proof size overflows where source fits", rect(400, 200, 1024, 640), ptr(rect(0, 25, 1440, 875)), WindowBounds{0, 25, 1152, 720}},
		{"small work area clamps and wraps", rect(50, 0, 1024, 640), ptr(rect(0, 0, 1100, 700)), WindowBounds{0, 0, 1060, 660}},
		{"nil work area", rect(100, 100, 1024, 640), nil, WindowBounds{124, 124, 1152, 720}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cascadeSized(tt.from, tt.work, 1152, 720)
			if *got != tt.want {
				t.Fatalf("got %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }
