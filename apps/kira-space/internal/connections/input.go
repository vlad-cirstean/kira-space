package connections

import (
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/connfiles"
	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
	"github.com/kirathecat/kira-studio/internal/ipcerr"
)

// Input is what Create takes: the connection's config plus the secret a file never holds. It lives
// here, not in internal/storage/model, so that package never carries a password.
type Input struct {
	model.ConnectionFields
	Password *string `json:"password"`
}

// Validate runs the file rules on the file Create would write.
func (in Input) Validate() error {
	if in.Mode == "uri" {
		if in.URI == nil || strings.TrimSpace(*in.URI) == "" {
			return errURIRequired()
		}
		// A password with a raw '/', '?' or '#' defeats findAuthority's delimiter scan, so foldPassword
		// would misplace it. Reject rather than corrupt it.
		if uriHasAmbiguousPassword(*in.URI) {
			return ipcerr.T(ipcerr.CodeBadRequest, "backend.studio.connection.uriPasswordAmbiguous.body", nil)
		}
	}
	if err := in.file("check").Validate(); err != nil {
		return ipcerr.T(ipcerr.CodeBadRequest, err.Key, err.Args)
	}
	return nil
}

// file builds the file Create writes; the secret refs are named for stem.
func (in Input) file(stem string) connfiles.File {
	f := connfiles.FileOf(in.ConnectionFields, nil, model.EmptyVisibility(), "", "", "")
	if in.Mode == "uri" {
		f.URI = connfiles.RefOf(stem + ".uri")
	} else if in.Password != nil && *in.Password != "" {
		f.Password = connfiles.RefOf(stem + ".password")
	}
	return f
}

// normalized drops what the mode does not own: fields mode never carries a URI, URI mode derives its
// options from the URI in Go, so a stale draft leftover cannot reach a file. An unset McpReview mode or
// McpMaskedMode becomes inherit.
func (in Input) normalized() Input {
	if in.Mode == "uri" {
		in.Options = map[string]any{}
	} else {
		in.URI = nil
	}
	for _, v := range []*string{&in.McpReview.Read, &in.McpReview.Write, &in.McpReview.Ddl, &in.McpReview.Masked} {
		if *v == "" {
			*v = "inherit"
		}
	}
	if in.McpMaskedMode == "" {
		in.McpMaskedMode = "inherit"
	}
	return in
}

// copySuffix is the suffix of Duplicate's generated name.
const copySuffix = " copy"

// duplicateName is Duplicate's name generator: name + copySuffix, capped at connfiles.NameMax UTF-16
// units. The base name is truncated, never the suffix, at a rune boundary so it is never invalid UTF-8.
func duplicateName(name string) string {
	if model.UTF16Len(name)+len(copySuffix) <= connfiles.NameMax {
		return name + copySuffix
	}
	base, _ := model.TruncateUTF16(name, connfiles.NameMax-len(copySuffix))
	return base + copySuffix
}
