package releaseline

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
)

// IndexHeader is the first line of a release index. One line per eligible release follows:
//
//	v0.3.3 kira-space-macos-arm64.dmg 43912345 sha256:<64 lowercase hex>
const IndexHeader = "# kira-release-index 1"

// maxIndexBytes bounds a read; a larger file is rejected, never truncated.
const maxIndexBytes = 1 << 20

var (
	digestRe = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	sizeRe   = regexp.MustCompile(`^[1-9][0-9]*$`)
)

// DecodeReleases accepts an array of pages (`gh api --paginate --slurp`), a flat array of
// releases, or a mix.
func DecodeReleases(data []byte) ([]Release, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("decode releases: %w", err)
	}
	var out []Release
	for _, it := range items {
		if bytes.HasPrefix(bytes.TrimSpace(it), []byte("[")) {
			var page []Release
			if err := json.Unmarshal(it, &page); err != nil {
				return nil, fmt.Errorf("decode releases page: %w", err)
			}
			out = append(out, page...)
			continue
		}
		var r Release
		if err := json.Unmarshal(it, &r); err != nil {
			return nil, fmt.Errorf("decode release: %w", err)
		}
		out = append(out, r)
	}
	return out, nil
}

// FormatIndex builds the index text. Candidates are eligible releases with a plain version tag,
// plus the draft tagged publish (the release being published right now, still a draft while the
// index is built). Every candidate needs a sha256 digest on asset; one without fails the build
// rather than vanishing from the list.
func FormatIndex(rels []Release, asset, publish string) (string, error) {
	type row struct {
		tag    string
		size   int64
		digest string
	}
	var rows []row
	seen := map[string]bool{}
	for _, r := range rels {
		if _, ok := Version(r.Tag); !ok {
			continue
		}
		pending := r.Tag == publish && r.Draft && !r.Prerelease && HasAsset(r, asset)
		if !pending && !Eligible(r, asset) {
			continue
		}
		if seen[r.Tag] {
			return "", fmt.Errorf("duplicate release %s", r.Tag)
		}
		seen[r.Tag] = true
		for _, a := range r.Assets {
			if a.Name != asset || a.State != "uploaded" || a.Size <= 0 {
				continue
			}
			if !digestRe.MatchString(a.Digest) {
				return "", fmt.Errorf("release %s: asset %s has no sha256 digest (got %q)", r.Tag, asset, a.Digest)
			}
			rows = append(rows, row{r.Tag, a.Size, a.Digest})
			break
		}
	}
	if publish != "" && !seen[publish] {
		return "", fmt.Errorf("release %s to publish not found as an eligible draft or release", publish)
	}
	if len(rows) == 0 {
		return "", errors.New("no eligible release")
	}
	slices.SortFunc(rows, func(a, b row) int { return semver.Compare(a.tag, b.tag) })
	var b strings.Builder
	b.WriteString(IndexHeader + "\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "%s %s %d %s\n", r.tag, asset, r.size, r.digest)
	}
	return b.String(), nil
}

// ParseIndex is the strict inverse of FormatIndex. Any malformed line rejects the whole file, so a
// damaged index never drives an install. Order is not trusted: NextStep computes its own.
func ParseIndex(r io.Reader) ([]Release, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxIndexBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxIndexBytes {
		return nil, fmt.Errorf("release index larger than %d bytes", maxIndexBytes)
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	if !sc.Scan() || sc.Text() != IndexHeader {
		return nil, errors.New("release index: missing or unknown header")
	}
	var out []Release
	seen := map[string]bool{}
	for n := 2; sc.Scan(); n++ {
		f := strings.Split(sc.Text(), " ")
		if len(f) != 4 {
			return nil, fmt.Errorf("release index line %d: want 4 fields, got %d", n, len(f))
		}
		tag, name, size, digest := f[0], f[1], f[2], f[3]
		if _, ok := Version(tag); !ok {
			return nil, fmt.Errorf("release index line %d: bad tag %q", n, tag)
		}
		if name == "" {
			return nil, fmt.Errorf("release index line %d: empty asset name", n)
		}
		if !sizeRe.MatchString(size) {
			return nil, fmt.Errorf("release index line %d: bad size %q", n, size)
		}
		sz, err := strconv.ParseInt(size, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("release index line %d: bad size %q", n, size)
		}
		if !digestRe.MatchString(digest) {
			return nil, fmt.Errorf("release index line %d: bad digest", n)
		}
		if seen[tag] {
			return nil, fmt.Errorf("release index line %d: duplicate tag %s", n, tag)
		}
		seen[tag] = true
		out = append(out, Release{Tag: tag, Assets: []Asset{{Name: name, State: "uploaded", Size: sz, Digest: digest}}})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("release index: no releases")
	}
	return out, nil
}
