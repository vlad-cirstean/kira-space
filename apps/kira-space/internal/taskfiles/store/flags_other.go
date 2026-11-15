//go:build !unix

package store

import (
	"io/fs"
	"os"
)

const openFlags = os.O_RDONLY

func linkCount(fs.FileInfo) uint64 { return 1 }
