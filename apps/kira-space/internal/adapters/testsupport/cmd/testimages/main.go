// Command testimages prints pinned test images, one per line: every image with no arguments,
// else the image of each kind named (testimages [kind...]). Unknown kind exits 2.
package main

import (
	"fmt"
	"os"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
)

func main() {
	if len(os.Args) == 1 {
		for _, image := range testsupport.Images() {
			fmt.Println(image)
		}
		return
	}
	for _, kind := range os.Args[1:] {
		image, ok := testsupport.ImageOf(kind)
		if !ok {
			fmt.Fprintf(os.Stderr, "testimages: unknown kind %q\n", kind)
			os.Exit(2)
		}
		fmt.Println(image)
	}
}
