// Package taskfiles is the core of a task's Details content on disk (P643): which path is which
// kind, who may change what, the per-kind caps, the README, and the submit pipeline that walks a task
// folder, validates, diffs against the SQLite cache and applies or restores. Content semantics of a
// kind (parse, diff, cache write, render) plug in through Handler.
package taskfiles

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Kind names what a path under a task folder is.
type Kind string

// Kinds. KindIgnored is every path that is not Details content.
const (
	KindIgnored    Kind = ""
	KindReadme     Kind = "readme"
	KindTask       Kind = "task"
	KindState      Kind = "state"
	KindLinks      Kind = "links"
	KindNote       Kind = "note"
	KindDraft      Kind = "draft"
	KindHandover   Kind = "handover"
	KindProof      Kind = "proof"
	KindProofData  Kind = "proofdata"
	KindProofImage Kind = "proofimage"
)

// Class is the result of classifying one path.
type Class struct {
	Kind Kind
	// Step is the step id of a draft file.
	Step string
	// Slug is the proof folder of a proof, proof data or proof image file.
	Slug string
}

// ReadOnlyFiles are the Kira-written files an agent cannot change: every other write is restored.
var ReadOnlyFiles = []string{"README.md", "task.yaml", "state.yaml"}

// Fixed file names.
const (
	ReadmeFile = "README.md"
	TaskFile   = "task.yaml"
	StateFile  = "state.yaml"
	LinksFile  = "links.yaml"
	ProofFile  = "proof.md"
)

var (
	noteRe     = regexp.MustCompile(`^notes/[A-Za-z0-9][A-Za-z0-9._-]*\.md$`)
	draftRe    = regexp.MustCompile(`^handovers/draft-([A-Za-z0-9][A-Za-z0-9_-]*)\.md$`)
	handoverRe = regexp.MustCompile(`^handovers/[0-9]{4}-[A-Za-z0-9][A-Za-z0-9_-]*-to-[A-Za-z0-9][A-Za-z0-9_-]*\.md$`)
	slugRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	dataRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}\.json$`)
	imageRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.(png|jpg|jpeg|webp|svg)$`)
)

// Classify maps a task-folder-relative slash path to its kind. A hidden segment, an unknown name and
// anything deeper than proofs/<slug>/ is KindIgnored: not Details, left alone.
func Classify(rel string) Class {
	if rel == "" || strings.HasPrefix(rel, "/") || path.Clean(rel) != rel {
		return Class{}
	}
	for _, seg := range strings.Split(rel, "/") {
		if strings.HasPrefix(seg, ".") {
			return Class{}
		}
	}
	switch rel {
	case ReadmeFile:
		return Class{Kind: KindReadme}
	case TaskFile:
		return Class{Kind: KindTask}
	case StateFile:
		return Class{Kind: KindState}
	case LinksFile:
		return Class{Kind: KindLinks}
	}
	switch {
	case noteRe.MatchString(rel):
		return Class{Kind: KindNote}
	case draftRe.MatchString(rel):
		return Class{Kind: KindDraft, Step: draftRe.FindStringSubmatch(rel)[1]}
	case handoverRe.MatchString(rel):
		return Class{Kind: KindHandover}
	}
	parts := strings.Split(rel, "/")
	if len(parts) == 3 && parts[0] == "proofs" && slugRe.MatchString(parts[1]) && len(parts[1]) <= MaxSlugLen {
		switch name := parts[2]; {
		case name == ProofFile:
			return Class{Kind: KindProof, Slug: parts[1]}
		case dataRe.MatchString(name):
			return Class{Kind: KindProofData, Slug: parts[1]}
		case imageRe.MatchString(strings.ToLower(name)):
			return Class{Kind: KindProofImage, Slug: parts[1]}
		}
	}
	return Class{}
}

// MaxSlugLen is the longest proof folder name.
const MaxSlugLen = 40

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// SuggestSlug turns a folder name into a valid slug.
func SuggestSlug(name string) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(s) > MaxSlugLen {
		s = strings.Trim(s[:MaxSlugLen], "-")
	}
	if s == "" {
		s = "proof"
	}
	return s
}

// ProofFolderHint explains a path under proofs/ that Classify ignores: a folder name that breaks the
// slug rule, a file deeper than proofs/<slug>/, a file outside any folder or a name no proof file may
// have. It is "" for every other path, valid proof files included.
func ProofFolderHint(rel string) string {
	trimmed := strings.TrimSuffix(rel, "/")
	if !strings.HasPrefix(trimmed, "proofs/") || path.Clean(trimmed) != trimmed || Classify(trimmed).Kind != KindIgnored && Classify(trimmed).Slug != "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	for _, seg := range parts {
		if strings.HasPrefix(seg, ".") {
			return ""
		}
	}
	if !slugRe.MatchString(parts[1]) || len(parts[1]) > MaxSlugLen {
		return fmt.Sprintf("proofs/%s/: folder name must be a-z, 0-9 and - (at most %d characters); rename it to proofs/%s/.", parts[1], MaxSlugLen, SuggestSlug(parts[1]))
	}
	switch {
	case len(parts) == 2 && !strings.HasSuffix(rel, "/"):
		return fmt.Sprintf("%s: a proof is a folder. Write proofs/<slug>/proof.md.", rel)
	case len(parts) > 3:
		return fmt.Sprintf("%s: too deep. Proof files go directly in proofs/%s/.", trimmed, parts[1])
	case len(parts) == 3 && Classify(trimmed).Kind == KindIgnored:
		return fmt.Sprintf("%s: not a proof file. Use proof.md, <id>.json (a-z, 0-9, -), or an image: png, jpg, webp or svg.", trimmed)
	}
	return ""
}

// hintPath is the path a folder hint is reported on: the folder for a bad name, else the path itself.
func hintPath(rel string) string {
	parts := strings.Split(strings.TrimSuffix(rel, "/"), "/")
	if len(parts) >= 2 && (!slugRe.MatchString(parts[1]) || len(parts[1]) > MaxSlugLen) {
		return "proofs/" + parts[1] + "/"
	}
	return rel
}

// KiraOwned reports whether only Kira writes the kind: any other write is restored.
func (k Kind) KiraOwned() bool { return k == KindReadme || k == KindTask || k == KindState }

// Per-file caps, enforced before a file is read whole.
const (
	MaxHandoverBytes   = 16 << 10
	MaxNoteBytes       = 1 << 20
	MaxLinksBytes      = 256 << 10
	MaxProofBytes      = 8 << 20
	MaxProofDataBytes  = 4 << 20
	MaxRasterBytes     = 10 << 20
	MaxSVGBytes        = 2 << 20
	MaxKiraFileBytes   = 1 << 20
	MaxFilesPerTask    = 500
	MaxBytesPerTask    = 64 << 20
	MaxBytesPerProof   = 32 << 20
	MaxDepth           = 3
	InlineStepMaxBytes = 64 << 10
)

// MaxBytes is the read cap of one file of kind; rel picks SVG from the raster images.
func MaxBytes(k Kind, rel string) int64 {
	switch k {
	case KindDraft, KindHandover:
		return MaxHandoverBytes
	case KindNote:
		return MaxNoteBytes
	case KindLinks:
		return MaxLinksBytes
	case KindProof:
		return MaxProofBytes
	case KindProofData:
		return MaxProofDataBytes
	case KindProofImage:
		if strings.HasSuffix(strings.ToLower(rel), ".svg") {
			return MaxSVGBytes
		}
		return MaxRasterBytes
	case KindReadme, KindTask, KindState:
		return MaxKiraFileBytes
	}
	return 0
}
