package bridge

import "github.com/kirathecat/kira-studio/internal/shell"

// Browser is the OS-browser seam LinkService needs.
type Browser interface {
	OpenURL(url string) error
}

// LinkService opens a URL in the OS browser for renderer features that link to external pages
// (the Docker module's image registry button). It validates only the URL shape: a well-formed
// http(s) URL, never `javascript:`, `file:` or another locally dangerous scheme.
type LinkService struct {
	Browser Browser
}

// LinkOpenExternalArgs is OpenExternal's request.
type LinkOpenExternalArgs struct {
	URL string `json:"url"`
}

// OpenExternal opens args.URL in the OS browser, refusing anything that is not a well-formed
// http(s) URL with a non-empty host.
func (s *LinkService) OpenExternal(args LinkOpenExternalArgs) error {
	return shell.OpenExternalURL(s.Browser, args.URL)
}
