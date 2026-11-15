package proof

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// SVG limits. The validator rejects, never sanitizes: a file that needs cleaning is the agent's to fix.
const (
	maxSVGElements = 50_000
	maxSVGDepth    = 64
	svgNS          = "http://www.w3.org/2000/svg"
)

var svgBadElements = map[string]bool{
	"script": true, "foreignobject": true, "iframe": true, "object": true, "embed": true, "audio": true, "video": true,
	"animate": true, "animatemotion": true, "animatetransform": true, "set": true,
}

var (
	cssURL     = regexp.MustCompile(`(?i)url\(\s*([^)]*)\)`)
	dataImage  = regexp.MustCompile(`^data:image/(png|jpeg|webp);base64,([A-Za-z0-9+/=\s]*)$`)
	svgLength  = regexp.MustCompile(`^\s*([0-9]*\.?[0-9]+)\s*(px)?\s*$`)
	svgPresent = map[string]bool{"fill": true, "stroke": true, "filter": true, "clip-path": true, "mask": true, "marker-start": true, "marker-mid": true, "marker-end": true, "style": true}
)

// svgError names the offending element or attribute and its line.
func svgError(line int, format string, args ...any) error {
	return invalid("line %d: "+format, append([]any{line}, args...)...)
}

// CheckSVG validates a SVG file and returns its intrinsic size.
func CheckSVG(raw []byte) (width, height int, err error) {
	if len(raw) > MaxSVGBytes {
		return 0, 0, invalid("too large: %d KiB, the limit is %d KiB for a svg file", len(raw)>>10, MaxSVGBytes>>10)
	}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.Strict = true
	dec.Entity = nil
	var depth, elements int
	var root *xml.StartElement
	var rootLine int
	var inStyle bool
	for {
		off := dec.InputOffset()
		tok, terr := dec.RawToken()
		if errors.Is(terr, io.EOF) {
			break
		}
		line := 1 + bytes.Count(raw[:min(int(off), len(raw))], []byte("\n"))
		if terr != nil {
			return 0, 0, invalid("not well-formed: %v", terr)
		}
		switch t := tok.(type) {
		case xml.Directive:
			return 0, 0, svgError(line, "DOCTYPE or entity declarations are not allowed")
		case xml.StartElement:
			elements++
			depth++
			if elements > maxSVGElements || depth > maxSVGDepth {
				return 0, 0, invalid("too large: more than %d elements or deeper than %d levels", maxSVGElements, maxSVGDepth)
			}
			if root == nil {
				cp := t
				root, rootLine = &cp, line
				if strings.ToLower(t.Name.Local) != "svg" || xmlnsOf(t) != svgNS {
					return 0, 0, svgError(line, "the root element must be <svg xmlns=\"%s\">", svgNS)
				}
			}
			name := strings.ToLower(t.Name.Local)
			if svgBadElements[name] {
				return 0, 0, svgError(line, "SVG element <%s> is not allowed", t.Name.Local)
			}
			inStyle = name == "style"
			if err := checkSVGAttrs(line, t); err != nil {
				return 0, 0, err
			}
		case xml.EndElement:
			depth--
			inStyle = false
		case xml.CharData:
			if inStyle {
				if err := checkCSS(string(t)); err != nil {
					return 0, 0, svgError(line, "CSS loads from outside: %v", err)
				}
			}
		}
	}
	if root == nil {
		return 0, 0, invalid("not well-formed: no root element")
	}
	w, h, ok := svgSize(*root)
	if !ok {
		return 0, 0, svgError(rootLine, "size missing: give <svg> width and height (px or unitless) or a viewBox")
	}
	if w < 1 || h < 1 || w > maxImageSide || h > maxImageSide || w*h > maxImagePixels {
		return 0, 0, invalid("side or area out of range: svg is %dx%d; each side must be 1-%d and the area at most %d megapixels", w, h, maxImageSide, maxImagePixels/1_000_000)
	}
	return w, h, nil
}

func xmlnsOf(t xml.StartElement) string {
	for _, a := range t.Attr {
		if a.Name.Local == "xmlns" && a.Name.Space == "" {
			return a.Value
		}
	}
	return ""
}

func checkSVGAttrs(line int, t xml.StartElement) error {
	for _, a := range t.Attr {
		local := strings.ToLower(a.Name.Local)
		if strings.HasPrefix(local, "on") {
			return svgError(line, "attribute %s is not allowed on <%s>", a.Name.Local, t.Name.Local)
		}
		if strings.Contains(strings.ToLower(a.Value), "javascript:") {
			return svgError(line, "attribute %s on <%s> holds javascript:, which is not allowed", a.Name.Local, t.Name.Local)
		}
		if local == "href" {
			if err := checkHref(a.Value); err != nil {
				return svgError(line, "href points outside the file on <%s>: %v", t.Name.Local, err)
			}
			continue
		}
		if svgPresent[local] {
			if err := checkCSS(a.Value); err != nil {
				return svgError(line, "CSS loads from outside in %s on <%s>: %v", a.Name.Local, t.Name.Local, err)
			}
		}
	}
	return nil
}

// checkHref allows a #fragment or an embedded raster data URL, which passes the raster checks.
func checkHref(v string) error {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "#") {
		return nil
	}
	m := dataImage.FindStringSubmatch(v)
	if m == nil {
		return errors.New("only #fragment or data:image/png|jpeg|webp;base64 are allowed")
	}
	data, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(m[2]), ""))
	if err != nil {
		return errors.New("the embedded image is not base64")
	}
	if _, err := CheckImage("embedded."+map[string]string{"png": "png", "jpeg": "jpg", "webp": "webp"}[m[1]], data); err != nil {
		return errors.New("the embedded image is not valid: " + err.Error())
	}
	return nil
}

// checkCSS refuses @import, @keyframes and url() to anything but a #fragment or an allowed data image.
func checkCSS(css string) error {
	low := strings.ToLower(css)
	if strings.Contains(low, "@import") || strings.Contains(low, "@keyframes") {
		return errors.New("@import and @keyframes are not allowed")
	}
	for _, m := range cssURL.FindAllStringSubmatch(css, -1) {
		arg := strings.Trim(strings.TrimSpace(m[1]), `"'`)
		if arg == "" || strings.HasPrefix(arg, "#") {
			continue
		}
		if err := checkHref(arg); err != nil {
			return errors.New("url(" + truncate(arg, 40) + ") is not a #fragment or an embedded image")
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func svgSize(t xml.StartElement) (w, h int, ok bool) {
	var ws, hs, vb string
	for _, a := range t.Attr {
		switch strings.ToLower(a.Name.Local) {
		case "width":
			ws = a.Value
		case "height":
			hs = a.Value
		case "viewbox":
			vb = a.Value
		}
	}
	if wv, ok1 := svgLen(ws); ok1 {
		if hv, ok2 := svgLen(hs); ok2 {
			return wv, hv, true
		}
	}
	f := strings.FieldsFunc(vb, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' || r == '\n' })
	if len(f) == 4 {
		wv, e1 := strconv.ParseFloat(f[2], 64)
		hv, e2 := strconv.ParseFloat(f[3], 64)
		if e1 == nil && e2 == nil && wv > 0 && hv > 0 && wv < 1e9 && hv < 1e9 {
			return int(wv + 0.5), int(hv + 0.5), true
		}
	}
	return 0, 0, false
}

func svgLen(s string) (int, bool) {
	m := svgLength.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil || v <= 0 || v > 1e9 {
		return 0, false
	}
	return int(v + 0.5), true
}
