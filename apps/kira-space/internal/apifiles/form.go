package apifiles

import (
	"fmt"
	"strings"

	"github.com/kirathecat/kira-studio/internal/configfs"
)

// FileDetail is one collection or environment file as the variables dialog edits it: the raw text and
// the hash a save is built on.
type FileDetail struct {
	File     string       `json:"file"`
	Kind     string       `json:"kind"`
	Path     string       `json:"path"`
	Text     string       `json:"text"`
	Hash     string       `json:"hash"`
	Problem  *ProblemView `json:"problem"`
	LastGood bool         `json:"lastGood"`
}

const (
	kindCollection  = "collection"
	kindEnvironment = "environment"
)

// kindOf picks the file kind by suffix; anything else is not an api file.
func kindOf(file string) (string, error) {
	switch {
	case strings.HasSuffix(file, CollectionExt):
		return kindCollection, nil
	case strings.HasSuffix(file, EnvironmentExt):
		return kindEnvironment, nil
	}
	return "", fmt.Errorf("%w: file name %q", configfs.ErrInvalid, file)
}

// FileText answers one file's text and state.
func (s *Store) FileText(file string) (FileDetail, error) {
	kind, err := kindOf(file)
	if err != nil {
		return FileDetail{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.detailLocked(kind, file)
}

func (s *Store) detailLocked(kind, file string) (FileDetail, error) {
	d := FileDetail{File: file, Kind: kind}
	var (
		src []byte
		err error
	)
	if kind == kindCollection {
		e, ok := s.collections.Get(file)
		if !ok {
			return FileDetail{}, fmt.Errorf("%w: %s", ErrNotFound, file)
		}
		d.Path, d.Problem, d.LastGood = e.Path, problemView(e.Problem), e.Problem != nil && e.Value != nil
		src, d.Hash, err = s.collections.Read(file)
	} else {
		e, ok := s.environments.Get(file)
		if !ok {
			return FileDetail{}, fmt.Errorf("%w: %s", ErrNotFound, file)
		}
		d.Path, d.Problem, d.LastGood = e.Path, problemView(e.Problem), e.Problem != nil && e.Value != nil
		src, d.Hash, err = s.environments.Read(file)
	}
	if err != nil {
		return FileDetail{}, err
	}
	d.Text = string(src)
	return d, nil
}

// SaveFile writes text as the file's content, valid or not; the last good content keeps serving while
// the file is broken. baseHash must match the file's current text, else configfs.ErrConflict. The
// save goes through the same load path as a file edited elsewhere, so the rows follow and the change
// event fires.
func (s *Store) SaveFile(file, baseHash, text string) (FileDetail, error) {
	kind, err := kindOf(file)
	if err != nil {
		return FileDetail{}, err
	}
	s.mu.Lock()
	s.pend = &Changes{}
	d, err := s.saveLocked(kind, file, baseHash, text)
	ch := *s.pend
	s.mu.Unlock()
	s.emit(ch)
	return d, err
}

func (s *Store) saveLocked(kind, file, baseHash, text string) (FileDetail, error) {
	var problem *configfs.Problem
	if kind == kindCollection {
		if _, ok := s.collections.Get(file); !ok {
			return FileDetail{}, fmt.Errorf("%w: %s", ErrNotFound, file)
		}
		e, err := s.collections.Save(file, baseHash, []byte(text))
		if err != nil {
			return FileDetail{}, err
		}
		problem = e.Problem
	} else {
		if _, ok := s.environments.Get(file); !ok {
			return FileDetail{}, fmt.Errorf("%w: %s", ErrNotFound, file)
		}
		e, err := s.environments.Save(file, baseHash, []byte(text))
		if err != nil {
			return FileDetail{}, err
		}
		problem = e.Problem
	}
	s.noteProblems1(file, problem)
	return s.detailLocked(kind, file)
}

// Form applies the form's edits to the draft text as YAML node edits and decodes the result with the
// rules of the file's kind. Stateless: nothing is written; the dialog saves the text with SaveFile.
func (s *Store) Form(file, text string, edits []configfs.Edit) (configfs.FormResult, error) {
	kind, err := kindOf(file)
	if err != nil {
		return configfs.FormResult{}, err
	}
	if kind == kindCollection {
		return configfs.Form(s.dec.collection, file, []byte(text), edits), nil
	}
	return configfs.Form(s.dec.environment, file, []byte(text), edits), nil
}
