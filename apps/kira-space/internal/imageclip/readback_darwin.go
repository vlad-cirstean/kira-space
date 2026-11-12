//go:build darwin && cgo && imagecliptest

package imageclip

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit -framework Foundation

#import <AppKit/AppKit.h>
#include <stdlib.h>
#include <string.h>

static NSPasteboard *kira_named(const char *name) {
	return [NSPasteboard pasteboardWithName:[NSString stringWithUTF8String:name]];
}

static int kira_item_count(const char *name) {
	@autoreleasepool {
		return (int)[[kira_named(name) pasteboardItems] count];
	}
}

// kira_item_png copies item i's public.png bytes into a malloc'd buffer; NULL when missing.
static void *kira_item_png(const char *name, int i, long *len) {
	@autoreleasepool {
		NSData *d = [[kira_named(name) pasteboardItems][i] dataForType:NSPasteboardTypePNG];
		if (d == nil) {
			return NULL;
		}
		*len = (long)[d length];
		void *buf = malloc([d length]);
		memcpy(buf, [d bytes], [d length]);
		return buf;
	}
}

// kira_item_url returns item i's public.file-url as a malloc'd C string; NULL when missing.
static char *kira_item_url(const char *name, int i) {
	@autoreleasepool {
		NSString *s = [[kira_named(name) pasteboardItems][i] stringForType:NSPasteboardTypeFileURL];
		return s == nil ? NULL : strdup([s UTF8String]);
	}
}

static void kira_release(const char *name) {
	@autoreleasepool {
		[kira_named(name) releaseGlobally];
	}
}
*/
import "C"

import "unsafe"

// Readback helpers for imageclip_darwin_test.go; cgo is not allowed in _test.go files.

type pasteItem struct {
	png []byte
	url string
}

func readPasteboard(name string) []pasteItem {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	n := int(C.kira_item_count(cname))
	out := make([]pasteItem, n)
	for i := range n {
		var l C.long
		if p := C.kira_item_png(cname, C.int(i), &l); p != nil {
			out[i].png = C.GoBytes(p, C.int(l))
			C.free(p)
		}
		if u := C.kira_item_url(cname, C.int(i)); u != nil {
			out[i].url = C.GoString(u)
			C.free(unsafe.Pointer(u))
		}
	}
	return out
}

func releasePasteboard(name string) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	C.kira_release(cname)
}
