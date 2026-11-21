package shell

import "testing"

func TestDropPoint(t *testing.T) {
	t.Run("macOS frame equals viewport", func(t *testing.T) {
		src := WindowBounds{X: 100, Y: 50, Width: 1000, Height: 800}
		x, y := DropPoint(src, DropInput{ClientX: -60, ClientY: 200, InnerWidth: 1000, InnerHeight: 800})
		if x != 40 || y != 250 {
			t.Fatalf("got %v,%v want 40,250", x, y)
		}
	})
	t.Run("Windows non-client inset", func(t *testing.T) {
		src := WindowBounds{X: 0, Y: 0, Width: 1016, Height: 839}
		x, y := DropPoint(src, DropInput{ClientX: 10, ClientY: 10, InnerWidth: 1000, InnerHeight: 800})
		if x != 18 || y != 41 {
			t.Fatalf("got %v,%v want 18,41", x, y)
		}
	})
	t.Run("viewport larger than frame clamps inset to zero", func(t *testing.T) {
		src := WindowBounds{X: 5, Y: 5, Width: 100, Height: 100}
		x, y := DropPoint(src, DropInput{InnerWidth: 120, InnerHeight: 120})
		if x != 5 || y != 5 {
			t.Fatalf("got %v,%v want 5,5", x, y)
		}
	})
	t.Run("half-open edges", func(t *testing.T) {
		geoms := []WindowGeom{
			{Key: "a", Bounds: WindowBounds{X: 0, Y: 0, Width: 1000, Height: 800}},
			{Key: "b", Bounds: WindowBounds{X: 1000, Y: 0, Width: 1000, Height: 800}},
		}
		if k, in, ok := DropTarget(geoms, "a", 1000, 10); !ok || in || k != "b" {
			t.Fatalf("right edge: got %q in=%v ok=%v, want b", k, in, ok)
		}
		if _, in, _ := DropTarget(geoms, "a", 999.9, 10); !in {
			t.Fatal("999.9 should be inside source")
		}
		if _, _, ok := DropTarget(geoms, "a", 2000, 10); ok {
			t.Fatal("2000 should miss")
		}
	})
	t.Run("most recently focused wins; ephemeral and hidden skipped", func(t *testing.T) {
		b := WindowBounds{X: 1200, Y: 0, Width: 800, Height: 600}
		geoms := []WindowGeom{
			{Key: "a", Bounds: WindowBounds{Width: 1000, Height: 800}},
			{Key: "b", Bounds: b, FocusSeq: 1},
			{Key: "c", Bounds: b, FocusSeq: 2},
			{Key: "e", Bounds: b, FocusSeq: 3, Ephemeral: true},
			{Key: "h", Bounds: b, FocusSeq: 4, Hidden: true},
		}
		if k, _, ok := DropTarget(geoms, "a", 1300, 100); !ok || k != "c" {
			t.Fatalf("got %q ok=%v want c", k, ok)
		}
	})
	t.Run("TornBounds keeps grab under point", func(t *testing.T) {
		src := WindowBounds{X: 0, Y: 0, Width: 1016, Height: 839}
		in := DropInput{ClientX: -60, ClientY: 200, GrabX: 300, GrabY: 20, InnerWidth: 1000, InnerHeight: 800}
		tb := TornBounds(src, in)
		px, py := DropPoint(src, in)
		ox, oy := clientOrigin(src, in.InnerWidth, in.InnerHeight)
		if tb.X+ox+in.GrabX != px || tb.Y+oy+in.GrabY != py || tb.Width != 1016 || tb.Height != 839 {
			t.Fatalf("torn %+v does not keep grab under %v,%v", tb, px, py)
		}
	})
}
