package proof

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// One hostile file per rule: the message names the element or attribute and the line.
func TestCheckSVGHostile(t *testing.T) {
	cases := map[string]string{
		"script":          "line 3: SVG element <script> is not allowed",
		"foreignobject":   "line 3: SVG element <foreignObject> is not allowed",
		"iframe":          "line 3: SVG element <iframe> is not allowed",
		"object":          "line 3: SVG element <object> is not allowed",
		"embed":           "line 3: SVG element <embed> is not allowed",
		"animate":         "line 3: SVG element <animate> is not allowed",
		"set":             "line 3: SVG element <set> is not allowed",
		"onload":          "line 1: attribute onload is not allowed on <svg>",
		"javascript-href": "line 2: attribute href on <a> holds javascript:",
		"external-href":   "line 2: href points outside the file on <image>",
		"use-xlink":       "line 3: href points outside the file on <use>",
		"nested-svg-data": "line 2: href points outside the file on <image>",
		"import":          "line 2: CSS loads from outside: @import and @keyframes are not allowed",
		"style-url":       "line 2: CSS loads from outside in style on <rect>",
		"doctype-entity":  "line 2: DOCTYPE or entity declarations are not allowed",
		"malformed":       "not well-formed",
		"no-size":         "line 1: size missing",
		"too-wide":        "side or area out of range: svg is 16385x10",
		"no-namespace":    "line 1: the root element must be <svg xmlns=",
	}
	for name, want := range cases {
		raw, err := os.ReadFile(filepath.Join("testdata", "svg", name+".svg"))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := CheckSVG(raw); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", name, err, want)
		}
	}
}

func TestCheckSVGAllowed(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "svg", "allowed.svg"))
	if err != nil {
		t.Fatal(err)
	}
	w, h, err := CheckSVG(raw)
	if err != nil || w != 120 || h != 80 {
		t.Fatalf("allowed.svg = %dx%d, %v; want 120x80", w, h, err)
	}
}

// Size rules that a fixture file cannot hold: the byte cap and the nesting depth.
func TestCheckSVGLimits(t *testing.T) {
	over := append([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1">`), bytes.Repeat([]byte(" "), MaxSVGBytes)...)
	if _, _, err := CheckSVG(append(over, "</svg>"...)); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Errorf("over 2 MiB: err = %v", err)
	}
	deep := `<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1">` + strings.Repeat("<g>", 70) + strings.Repeat("</g>", 70) + "</svg>"
	if _, _, err := CheckSVG([]byte(deep)); err == nil || !strings.Contains(err.Error(), "deeper than 64 levels") {
		t.Errorf("depth over 64: err = %v", err)
	}
}
