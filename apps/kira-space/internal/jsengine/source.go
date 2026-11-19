package jsengine

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"

	"github.com/dop251/goja"
	js_ast "github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"
)

// The script is a function body; the wrapper adds exactly wrapperLines lines before it.
const (
	wrapperPrefix = "(function () {\"use strict\";\n"
	wrapperSuffix = "\n})"
	wrapperLines  = 1
)

// srcInfo describes the caller's source for position mapping.
type srcInfo struct {
	lines   int // line count
	lastCol int // column just past the last line
}

func newSrcInfo(source string) srcInfo {
	lines := strings.Count(source, "\n") + 1
	last := source[strings.LastIndexByte(source, '\n')+1:]
	return srcInfo{lines: lines, lastCol: utf8.RuneCountInString(last) + 1}
}

// SourceHash is the lowercase hex SHA-256 of the exact source bytes, no normalisation. Callers bind
// an approval to it the way scriptruns binds Preview.Hash.
func SourceHash(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:])
}

// Check validates source and parses it without running it. Nil means the source is fine. It does
// not compile: goja folds constant expressions at compile time (`2n ** (2n ** 30n)` allocates
// gigabytes), so compiling is only safe in the worker, where Exec does it under the ceilings.
// Early errors raised by the compiler (rare) therefore surface from Exec only.
func Check(source string) *Error {
	_, err := parseSource(source)
	return err
}

func parseSource(source string) (*js_ast.Program, *Error) {
	if e := validateSource(source); e != nil {
		return nil, e
	}
	ast, err := goja.Parse(scriptName, wrapperPrefix+source+wrapperSuffix, parser.WithDisableSourceMaps)
	if err != nil {
		return nil, fromGoja(err, newSrcInfo(source), nil, func(v goja.Value) string { return v.String() })
	}
	return ast, nil
}

// compileSource parses and compiles. Worker only, see Check.
func compileSource(source string) (*goja.Program, *Error) {
	ast, e := parseSource(source)
	if e != nil {
		return nil, e
	}
	prg, err := goja.CompileAST(ast, true)
	if err != nil {
		return nil, fromGoja(err, newSrcInfo(source), nil, func(v goja.Value) string { return v.String() })
	}
	return prg, nil
}

func isBidi(r rune) bool { return r >= 0x202A && r <= 0x202E || r >= 0x2066 && r <= 0x2069 }

// validateSource rejects blank, oversized and malformed sources, naming the first offending
// character. Bidirectional controls anywhere are refused (Trojan Source): the approval card must
// show what runs.
func validateSource(source string) *Error {
	if strings.TrimSpace(source) == "" {
		return newError(KindSource, "script is empty")
	}
	if len(source) > MaxSourceBytes {
		return newError(KindSource, "script is %d bytes; the limit is %d", len(source), MaxSourceBytes)
	}
	line, col := 1, 1
	for i := 0; i < len(source); {
		r, n := utf8.DecodeRuneInString(source[i:])
		var msg string
		switch {
		case r == utf8.RuneError && n == 1:
			msg = "script is not valid UTF-8"
		case r == 0:
			msg = "script contains a NUL character"
		case isBidi(r):
			msg = "script contains a bidirectional control character"
		}
		if msg != "" {
			e := newError(KindSource, "%s", msg)
			e.Line, e.Column = line, col
			return e
		}
		i += n
		if r == '\n' {
			line, col = line+1, 1
		} else {
			col++
		}
	}
	return nil
}
