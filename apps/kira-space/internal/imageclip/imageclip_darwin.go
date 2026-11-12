//go:build darwin && cgo

package imageclip

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit -framework Foundation

#import <AppKit/AppKit.h>
#include <stdlib.h>

static int kira_can_write_images(void) {
	@autoreleasepool {
		return ([NSPasteboard generalPasteboard] != nil && [NSPasteboardItem class] != nil) ? 1 : 0;
	}
}

// kira_write_images writes one pasteboard item per path: public.png data plus a public.file-url
// pointing at the file. name NULL means the general pasteboard. All items are built before the
// pasteboard is cleared, so a failed read leaves it untouched.
// Returns 0 ok, 1 file unreadable, 2 pasteboard refused the write.
static int kira_write_images(const char *name, const char **paths, int n) {
	@autoreleasepool {
		NSPasteboard *pb = name == NULL
			? [NSPasteboard generalPasteboard]
			: [NSPasteboard pasteboardWithName:[NSString stringWithUTF8String:name]];
		NSMutableArray<NSPasteboardItem *> *items = [NSMutableArray arrayWithCapacity:n];
		for (int i = 0; i < n; i++) {
			NSString *p = [NSString stringWithUTF8String:paths[i]];
			NSData *data = [NSData dataWithContentsOfFile:p];
			if (data == nil) {
				return 1;
			}
			NSPasteboardItem *item = [[NSPasteboardItem alloc] init];
			[item setData:data forType:NSPasteboardTypePNG];
			[item setString:[[NSURL fileURLWithPath:p] absoluteString] forType:NSPasteboardTypeFileURL];
			[items addObject:item];
		}
		[pb clearContents];
		return [pb writeObjects:items] ? 0 : 2;
	}
}
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"
)

var (
	errUnreadable = errors.New("imageclip: screenshot file unreadable")
	errRefused    = errors.New("imageclip: pasteboard refused the write")
)

var (
	capOnce sync.Once
	capOK   bool
)

// MultiImage reports whether NSPasteboard is available. Probed once, on the main thread.
func (c *Clipboard) MultiImage() bool {
	capOnce.Do(func() {
		c.invoke(func() { capOK = C.kira_can_write_images() == 1 })
	})
	return capOK
}

// WriteImages puts one item per PNG path on the general pasteboard, in order.
func (c *Clipboard) WriteImages(paths []string) error {
	return c.writeImagesTo("", paths)
}

// writeImagesTo writes to the named pasteboard; "" is the general one.
func (c *Clipboard) writeImagesTo(name string, paths []string) error {
	if len(paths) == 0 {
		return errors.New("imageclip: no images")
	}
	cpaths := make([]*C.char, len(paths))
	for i, p := range paths {
		cpaths[i] = C.CString(p)
		defer C.free(unsafe.Pointer(cpaths[i]))
	}
	var cname *C.char
	if name != "" {
		cname = C.CString(name)
		defer C.free(unsafe.Pointer(cname))
	}
	var rc C.int
	c.invoke(func() {
		rc = C.kira_write_images(cname, (**C.char)(unsafe.Pointer(&cpaths[0])), C.int(len(paths)))
	})
	switch rc {
	case 0:
		return nil
	case 1:
		return errUnreadable
	default:
		return fmt.Errorf("%w (code %d)", errRefused, int(rc))
	}
}
