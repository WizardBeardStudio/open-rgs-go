package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wizardbeardstudio/open-rgs-go/internal/platform/evidence"
)

func main() {
	mode := flag.String("mode", "strict", "validation mode: strict|json")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/verifysummary [--mode=strict|json] <summary.json>")
		os.Exit(2)
	}
	summaryPath := flag.Arg(0)

	if err := evidence.ValidateSummaryArtifact(summaryPath, *mode); err != nil {
		fmt.Fprintf(os.Stderr, "invalid verify summary: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("verify summary validation passed")
}
