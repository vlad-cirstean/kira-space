package agentport

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/internal/fileservice"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

// BackupsArgs names the provider whose export backups to list.
type BackupsArgs struct {
	Provider string `json:"provider"`
}

// Backup is one backup an export took beside a native file.
type Backup struct {
	Role   string `json:"role"`
	Target string `json:"target"` // display path of the file it restores
	File   string `json:"file"`   // base name, the Restore key
	Taken  string `json:"taken"`  // RFC 3339 UTC, parsed from the stamp
	Size   int64  `json:"size"`
	// Restorable is false for ~/.claude.json; Reason says why.
	Restorable bool   `json:"restorable"`
	Reason     string `json:"reason"`
}

// RestoreArgs names one backup of a provider.
type RestoreArgs struct {
	Provider string `json:"provider"`
	File     string `json:"file"`
}

// backupPattern matches the backups of one target file: stamp, then the optional -N suffix.
func backupPattern(base string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(base) + `\.kira-backup-(\d{8}T\d{6}Z)(?:-(\d+))?$`)
}

type backupEntry struct {
	Backup
	n int
}

// listTarget lists the backups beside one target's real file, newest first.
func (s *Service) listTarget(t target) ([]backupEntry, error) {
	real, err := resolveTarget(t.Path)
	if err != nil {
		return nil, err
	}
	names, err := fileservice.Host().ReadDir(filepath.Dir(real))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	re := backupPattern(filepath.Base(real))
	var out []backupEntry
	for _, d := range names {
		m := re.FindStringSubmatch(d.Name)
		if m == nil {
			continue
		}
		st, err := fileservice.Host().Lstat(filepath.Join(filepath.Dir(real), d.Name))
		if err != nil || !st.Mode.IsRegular() {
			continue
		}
		at, err := time.Parse(backupStamp, m[1])
		if err != nil {
			continue
		}
		n := 0
		if m[2] != "" {
			if n, err = strconv.Atoi(m[2]); err != nil {
				continue
			}
		}
		b := Backup{Role: t.Role, Target: s.display(t.Path), File: d.Name, Taken: at.UTC().Format(time.RFC3339), Size: st.Size, Restorable: t.Role != roleMCP}
		if !b.Restorable {
			b.Reason = i18n.T("backend.core.agentPort.restore.claudeJSON.body", nil)
		}
		out = append(out, backupEntry{b, n})
	}
	slices.SortFunc(out, func(a, b backupEntry) int {
		if c := strings.Compare(b.Taken, a.Taken); c != 0 {
			return c
		}
		return b.n - a.n
	})
	return out, nil
}

// Backups lists the provider's export backups, newest first per file. Contents are never read.
func (s *Service) Backups(ctx context.Context, args BackupsArgs) ([]Backup, error) {
	files, _, ok := s.targets(ctx, args.Provider)
	if !ok || !s.has(args.Provider) {
		return nil, ipcerr.BadRequest(i18n.T("backend.core.agentPort.unknownProvider.body", map[string]string{"provider": args.Provider}))
	}
	out := []Backup{}
	for _, t := range files {
		list, err := s.listTarget(t)
		if err != nil {
			return nil, ipcerr.InternalErr(err)
		}
		for _, e := range list {
			out = append(out, e.Backup)
		}
	}
	return out, nil
}

func unknownBackup() error {
	return ipcerr.BadRequest(i18n.T("backend.core.agentPort.restore.unknown.body", nil))
}

// Restore writes a backup's bytes over its target file. The current file is backed up first, so a
// restore is itself undoable. ~/.claude.json is never written.
func (s *Service) Restore(ctx context.Context, args RestoreArgs) (PortResult, error) {
	files, _, ok := s.targets(ctx, args.Provider)
	if !ok || !s.has(args.Provider) {
		return PortResult{}, ipcerr.BadRequest(i18n.T("backend.core.agentPort.unknownProvider.body", map[string]string{"provider": args.Provider}))
	}
	var t target
	var real string
	for _, c := range files {
		r, err := resolveTarget(c.Path)
		if err != nil {
			return PortResult{}, ipcerr.InternalErr(err)
		}
		if backupPattern(filepath.Base(r)).MatchString(args.File) {
			t, real = c, r
			break
		}
	}
	if real == "" {
		return PortResult{}, unknownBackup()
	}
	if t.Role == roleMCP {
		return PortResult{}, ipcerr.BadRequest(i18n.T("backend.core.agentPort.restore.claudeJSON.body", nil))
	}
	src := filepath.Join(filepath.Dir(real), args.File)
	if st, err := fileservice.Host().Lstat(src); err != nil || !st.Mode.IsRegular() {
		return PortResult{}, unknownBackup()
	}
	limit, defMode := int64(maxSettingsFile), os.FileMode(0o600)
	if t.Role == roleContext {
		limit, defMode = maxContextFile, 0o644
	}
	data, _, _, err := readIfExists(src, limit)
	if err != nil {
		return PortResult{}, ipcerr.BadRequest(readProblem(src, err, limit))
	}
	cur, mode, exists, err := readIfExists(real, limit)
	if err != nil {
		return PortResult{}, ipcerr.BadRequest(readProblem(real, err, limit))
	}
	res := PortResult{Written: []string{}, Backups: []string{}, Servers: []ServerResult{}, Secrets: []string{}, Notes: []string{}}
	if exists && bytes.Equal(cur, data) {
		res.Notes = append(res.Notes, i18n.T("backend.core.agentPort.restore.same.body", map[string]string{"file": filepath.Base(real)}))
		return res, nil
	}
	if exists {
		b, err := backup(real, cur, mode, s.o.Now())
		if err != nil {
			return PortResult{}, ipcerr.InternalErr(err)
		}
		res.Backups = append(res.Backups, b)
	}
	if err := writeFile(real, data, defMode); err != nil {
		return res, ipcerr.InternalErr(err)
	}
	res.Written = append(res.Written, real)
	return res, nil
}
