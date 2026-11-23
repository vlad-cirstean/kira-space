package porcelain

import (
	"fmt"
	"strconv"
)

// NameStatusArgs builds commit.detail's file-list spawn: the source of Kind/OriginalPath/Similarity.
// `-M -C` enables rename/copy detection. from == nil selects a root commit's own diff against the
// empty tree (`--root`), never a literal empty-tree sha.
func NameStatusArgs(from *string, to string) []string {
	args := []string{"diff-tree", "-r", "--no-commit-id", "--name-status", "-M", "-C", "-z"}
	return appendRevPair(args, from, to)
}

// RangeNameStatusArgs is NameStatusArgs without copy detection (-C): a PR range lists a copy as
// added, as GitHub does, and -C cost grows with added x modified files (P739).
func RangeNameStatusArgs(from *string, to string) []string {
	args := []string{"diff-tree", "-r", "--no-commit-id", "--name-status", "-M", "-z"}
	return appendRevPair(args, from, to)
}

func appendRevPair(args []string, from *string, to string) []string {
	if from == nil {
		return append(args, "--root", to)
	}
	return append(args, *from, to)
}

// FileChangeKind mirrors @kira/git-ipc's own FileChangeKind union verbatim.
type FileChangeKind string

const (
	FileAdded       FileChangeKind = "added"
	FileModified    FileChangeKind = "modified"
	FileDeleted     FileChangeKind = "deleted"
	FileRenamed     FileChangeKind = "renamed"
	FileCopied      FileChangeKind = "copied"
	FileTypeChanged FileChangeKind = "typeChanged"
	FileUnmerged    FileChangeKind = "unmerged"
)

// FileChange structurally matches @kira/git-ipc's own FileChange (D5): OriginalPath/Similarity are
// Go pointers with omitempty (the contract's `| undefined` fields), never present-and-null.
type FileChange struct {
	Kind         FileChangeKind `json:"kind"`
	Path         string         `json:"path"`
	OriginalPath *string        `json:"originalPath,omitempty"`
	Similarity   *int           `json:"similarity,omitempty"`
}

// NumstatEntry is one parsed `--numstat` row, keyed by its *new* path. Only stash list reads it, for
// record framing and file count; the line counts are validated but not kept.
type NumstatEntry struct {
	Path         string
	OriginalPath string // set for a rename/copy row (probe P1's empty-third-field framing)
}

// ParseNumstatRecords parses records (already NUL-split by RecordSplitter): a normal
// row is one record, tab-limited to 3 fields; a binary row's counts are the literal string "-";
// a rename/copy row's own third (path) field is empty, which consumes the next two records as
// originalPath then path — the record set ending mid-rename is reported as an error, not a panic.
func ParseNumstatRecords(records [][]byte) ([]NumstatEntry, error) {
	var out []NumstatEntry
	for i := 0; i < len(records); i++ {
		fields := SplitLimitedFields(records[i], '\t', 3)
		if len(fields) != 3 {
			return nil, fmt.Errorf("porcelain: numstat record %q has %d tab fields, want 3", records[i], len(fields))
		}
		addStr, delStr, path := string(fields[0]), string(fields[1]), string(fields[2])

		if path == "" {
			if i+2 >= len(records) {
				return nil, fmt.Errorf("porcelain: numstat record set ends mid-rename at index %d", i)
			}
			if err := checkCounts(addStr, delStr); err != nil {
				return nil, err
			}
			out = append(out, NumstatEntry{OriginalPath: string(records[i+1]), Path: string(records[i+2])})
			i += 2
			continue
		}

		if err := checkCounts(addStr, delStr); err != nil {
			return nil, err
		}
		out = append(out, NumstatEntry{Path: path})
	}
	return out, nil
}

func checkCounts(addStr, delStr string) error {
	if addStr == "-" && delStr == "-" {
		return nil
	}
	if _, err := strconv.Atoi(addStr); err != nil {
		return fmt.Errorf("porcelain: numstat additions %q: %w", addStr, err)
	}
	if _, err := strconv.Atoi(delStr); err != nil {
		return fmt.Errorf("porcelain: numstat deletions %q: %w", delStr, err)
	}
	return nil
}

// ParseNameStatusRecords parses records per `--name-status -M -C -z`'s own framing: the type
// letter (with an optional similarity score suffix for R/C) is its own record, followed by one
// path record (A/M/D/T/U) or two (R/C: originalPath, path). An unrecognised letter is an error.
func ParseNameStatusRecords(records [][]byte) ([]FileChange, error) {
	var out []FileChange
	for i := 0; i < len(records); i++ {
		code := string(records[i])
		if code == "" {
			return nil, fmt.Errorf("porcelain: empty name-status type record at index %d", i)
		}
		letter := code[0]
		switch letter {
		case 'A', 'M', 'D', 'T', 'U':
			if i+1 >= len(records) {
				return nil, fmt.Errorf("porcelain: name-status record set ends after %q at index %d", code, i)
			}
			out = append(out, FileChange{Kind: nameStatusKind(letter), Path: string(records[i+1])})
			i++
		case 'R', 'C':
			if i+2 >= len(records) {
				return nil, fmt.Errorf("porcelain: name-status record set ends mid-%c at index %d", letter, i)
			}
			score := 0
			if len(code) > 1 {
				s, err := strconv.Atoi(code[1:])
				if err != nil {
					return nil, fmt.Errorf("porcelain: name-status similarity %q: %w", code, err)
				}
				score = s
			}
			orig := string(records[i+1])
			out = append(out, FileChange{
				Kind: nameStatusKind(letter), OriginalPath: &orig, Path: string(records[i+2]), Similarity: &score,
			})
			i += 2
		default:
			return nil, fmt.Errorf("porcelain: unrecognised name-status letter %q", code)
		}
	}
	return out, nil
}

func nameStatusKind(letter byte) FileChangeKind {
	switch letter {
	case 'A':
		return FileAdded
	case 'M':
		return FileModified
	case 'D':
		return FileDeleted
	case 'T':
		return FileTypeChanged
	case 'U':
		return FileUnmerged
	case 'R':
		return FileRenamed
	case 'C':
		return FileCopied
	}
	return ""
}
