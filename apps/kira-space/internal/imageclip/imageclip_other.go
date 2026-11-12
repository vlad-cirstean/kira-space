//go:build !(darwin && cgo)

package imageclip

import "errors"

// Windows: one clipboard block per format (CF_DIB, CF_DIBV5, PNG), so one image; several only as a
// CF_HDROP file list. Linux: X11 and Wayland offer one payload per MIME target, so one image/png;
// several only as text/uri-list file references.

// MultiImage reports false: no platform here holds several images.
func (c *Clipboard) MultiImage() bool { return false }

// WriteImages is unsupported here.
func (c *Clipboard) WriteImages([]string) error { return errors.ErrUnsupported }
