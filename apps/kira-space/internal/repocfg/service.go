package repocfg

import (
	"errors"
	"os"
	"unicode/utf8"

	"github.com/kirathecat/kira-studio/internal/configfs"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// template is what the editor offers for a repo with no file: the path and the sections, commented out.
const template = `# nickname: ""
# integration_branches: []
# environments:
#   - name: staging
#     deployed_sha_script: curl -fsS https://staging.example.com/version
#   - name: prod
#     deployed_sha_run: { script: Prod sha }
# worktree:
#   prepare_script: bun install
#   # or a Run script: prepare_run: { script: Install deps, params: { mode: ci } }
#   prepare_timeout: 15m
#   base_path: ""
# graph:
#   page_size: 5000
#   scope: all
# stash:
#   show_in_graph: true
#   include_untracked: false
# review:
#   base_candidates: [main, master]
# pull:
#   strategy: auto
# git_host:
#   provider: auto
# checkout:
#   auto_stash: true
`

// Service is the bound surface of the repo file editor. The app's bridge type embeds it so binding
// names stay app-local.
type Service struct {
	Store *Store
	// CodeRepos maps a repo id (a code repo's repo_id) to its code repo id.
	CodeRepos func() map[string]string
}

// ViewProblem is why a file is invalid.
type ViewProblem struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// FileItem is one file of the folder.
type FileItem struct {
	File     string       `json:"file"`
	Path     string       `json:"path"`
	Problem  *ViewProblem `json:"problem"`
	LastGood bool         `json:"lastGood"`
	// Repo is the code repo the file configures; "" when no registered repo has the path.
	Repo string `json:"repo"`
}

// ListView is every file of the folder.
type ListView struct {
	Dir   string     `json:"dir"`
	Files []FileItem `json:"files"`
}

// FileArgs names the file to open: by file name when set, else the file naming Path.
type FileArgs struct {
	Path string `json:"path"`
	File string `json:"file"`
}

// SaveArgs is one save. File "" creates a file for Path; otherwise BaseHash is the hash of the text
// the editor loaded.
type SaveArgs struct {
	Path     string `json:"path"`
	File     string `json:"file"`
	Text     string `json:"text"`
	BaseHash string `json:"baseHash"`
}

// FileView is the editor's state of one file.
type FileView struct {
	File     string       `json:"file"`
	FilePath string       `json:"filePath"`
	Path     string       `json:"path"`
	Text     string       `json:"text"`
	Hash     string       `json:"hash"`
	Exists   bool         `json:"exists"`
	Problem  *ViewProblem `json:"problem"`
	LastGood bool         `json:"lastGood"`
	Editable bool         `json:"editable"`
}

func viewProblem(p *configfs.Problem) *ViewProblem {
	if p == nil {
		return nil
	}
	return &ViewProblem{Line: p.Line, Message: p.Message}
}

// List returns every file.
func (s *Service) List() (ListView, error) {
	codeRepos := map[string]string{}
	if s.CodeRepos != nil {
		codeRepos = s.CodeRepos()
	}
	v := ListView{Dir: s.Store.Dir(), Files: []FileItem{}}
	for _, f := range s.Store.Files() {
		item := FileItem{File: f.File, Path: f.Path, Problem: viewProblem(f.Problem), LastGood: f.LastGood}
		if f.Path != "" && !f.Duplicate {
			item.Repo = codeRepos[f.Path]
		}
		v.Files = append(v.Files, item)
	}
	return v, nil
}

// File returns one file's text, hash and state; a repo with no file gets a template to create one from.
func (s *Service) File(args FileArgs) (FileView, error) {
	file := args.File
	if file == "" {
		file = s.Store.FileFor(args.Path)
	}
	return s.Store.view(file, args.Path)
}

// SaveFile writes the file, valid or not: the last good config stays in use while it is broken.
func (s *Service) SaveFile(args SaveArgs) (FileView, error) {
	file, err := s.Store.saveText(args)
	if err != nil {
		switch {
		case errors.Is(err, configfs.ErrConflict):
			return FileView{}, ipcerr.T(ipcerr.CodeConflict, "backend.space.repoFiles.changed.body", nil)
		case errors.Is(err, os.ErrNotExist):
			return FileView{}, ipcerr.T(ipcerr.CodeNotFound, "backend.space.repoFiles.gone.body", nil)
		case errors.Is(err, configfs.ErrInvalid):
			return FileView{}, ipcerr.BadRequest(err.Error())
		}
		return FileView{}, ipcerr.Wrap(err)
	}
	return s.Store.view(file, args.Path)
}

func (s *Store) view(file, path string) (FileView, error) {
	key := Key(path)
	v := FileView{File: file, Path: key, Editable: true}
	if file == "" {
		if path == "" {
			return v, nil
		}
		head, err := configfs.RenderYAML(File{Path: key}, header)
		if err != nil {
			return FileView{}, ipcerr.Wrap(err)
		}
		v.Text = string(head) + template
		return v, nil
	}
	e, ok := s.folder.Get(file)
	if !ok {
		return FileView{}, ipcerr.T(ipcerr.CodeNotFound, "backend.space.repoFiles.gone.body", nil)
	}
	v.FilePath, v.Exists = e.Path, true
	v.Problem, v.LastGood = viewProblem(e.Problem), e.Problem != nil && e.Value != nil
	for _, f := range s.Files() {
		if f.File == file {
			v.Problem = viewProblem(f.Problem)
			if f.Path != "" {
				v.Path = f.Path
			}
		}
	}
	raw, hash, err := s.folder.Read(file)
	if err != nil || !utf8.Valid(raw) {
		v.Editable = false
		return v, nil
	}
	v.Text, v.Hash = string(raw), hash
	return v, nil
}

// saveText writes args.Text and reloads. It returns the file name written.
func (s *Store) saveText(args SaveArgs) (string, error) {
	s.op.Lock()
	first := s.load("")
	file, err := s.write(args)
	var l loaded
	if err == nil {
		l = s.load("")
	}
	s.op.Unlock()
	s.finish(first)
	s.finish(l)
	return file, err
}

func (s *Store) write(args SaveArgs) (string, error) {
	if args.File == "" {
		if args.Path == "" {
			return "", ipcerr.BadRequest(i18n.T("backend.space.repoFiles.pathRequired.body", nil))
		}
		if s.FileFor(args.Path) != "" {
			return "", ipcerr.T(ipcerr.CodeConflict, "backend.space.repoFiles.pathTaken.body", nil)
		}
		name, err := s.freeName(slug(Key(args.Path)))
		if err != nil {
			return "", err
		}
		_, err = s.folder.Save(name, "", []byte(args.Text))
		return name, err
	}
	if args.BaseHash == "" {
		return "", ipcerr.BadRequest(i18n.T("backend.space.repoFiles.hashRequired.body", nil))
	}
	if _, _, err := s.folder.Read(args.File); err != nil {
		return "", err
	}
	_, err := s.folder.Save(args.File, args.BaseHash, []byte(args.Text))
	return args.File, err
}
