// g1measure is gate G1's instrument: sums RSS across the app's whole process set —
// this app's own executables plus, on darwin, only the native-webview helper processes macOS
// itself attributes to one of them (see internal/metrics.AppProcessSet — a plain substring match
// over "com.apple.WebKit" also matches every other running app's idle webview helpers, confirmed
// on a real machine: it inflated a real 215 MB reading to a reported 300 MB) — over 10 samples 1s
// apart, reporting the min, the same methodology the removed tests/e2e/memory.spec.ts used,
// so the numbers are directly comparable.
//
// Not part of the shipped app — a throwaway measurement tool, invoked manually against a running
// kira-studio-shell process for the G1 report.
package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/kirathecat/kira-studio/internal/metrics"
	"github.com/shirou/gopsutil/v4/process"
)

func main() {
	var anchorFlag, helperFlag string
	var samples int
	var intervalSec int
	// "Kira Studio" is this app's own shipping executable name (apps/kira-studio/Taskfile.yml's
	// APP_NAME) — P116 H4 moved the shared AppProcessSet/HelperNeedles machinery to repo-root
	// internal/metrics, which no longer carries a Studio-specific anchor default of its own.
	flag.StringVar(&anchorFlag, "anchor", "Kira Studio",
		"comma-separated executable-path substrings identifying this app's own executable (the Go binary)")
	flag.StringVar(&helperFlag, "helper", strings.Join(metrics.HelperNeedles, ","),
		"comma-separated executable-path substrings identifying native-webview helper processes — filtered to this app's own on darwin, see AppProcessSet")
	flag.IntVar(&samples, "samples", 10, "number of samples")
	flag.IntVar(&intervalSec, "interval", 1, "seconds between samples")
	flag.Parse()

	anchors := splitNonEmpty(anchorFlag, ',')
	if len(anchors) == 0 {
		log.Fatal("g1measure: -anchor must name at least one substring")
	}
	helpers := splitNonEmpty(helperFlag, ',')

	var minRSS uint64 = ^uint64(0)
	var minSet []procInfo

	for i := 0; i < samples; i++ {
		pids, err := metrics.AppProcessSet(anchors, helpers)
		if err != nil {
			log.Fatalf("g1measure: %v", err)
		}
		set, total := describe(pids)
		fmt.Printf("sample %2d: %d processes, %d bytes (%.1f MB)\n", i+1, len(set), total, float64(total)/1024/1024)
		if total < minRSS {
			minRSS = total
			minSet = set
		}
		if i < samples-1 {
			time.Sleep(time.Duration(intervalSec) * time.Second)
		}
	}

	fmt.Printf("\nmin of %d samples: %d bytes (%.1f MB)\n", samples, minRSS, float64(minRSS)/1024/1024)
	fmt.Println("process set at min sample:")
	for _, p := range minSet {
		fmt.Printf("  pid=%-7d rss=%8d KB  %s\n", p.pid, p.rssKB, p.exe)
	}
}

type procInfo struct {
	pid   int32
	rssKB uint64
	exe   string
}

func describe(pids []int32) ([]procInfo, uint64) {
	out := make([]procInfo, 0, len(pids))
	var total uint64
	for _, pid := range pids {
		p, err := process.NewProcess(pid)
		if err != nil {
			continue
		}
		exe, _ := p.Exe()
		mi, err := p.MemoryInfo()
		if err != nil || mi == nil {
			continue
		}
		out = append(out, procInfo{pid: pid, rssKB: mi.RSS / 1024, exe: exe})
		total += mi.RSS
	}
	return out, total
}

func splitNonEmpty(s string, sep rune) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == sep {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
