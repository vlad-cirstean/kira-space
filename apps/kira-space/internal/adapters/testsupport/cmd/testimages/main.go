// Command testimages prints every pinned test image, one per line.
package main

import (
	"fmt"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/adapters/testsupport"
)

func main() {
	for _, image := range testsupport.Images() {
		fmt.Println(image)
	}
}
