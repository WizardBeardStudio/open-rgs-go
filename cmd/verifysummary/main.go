package main

import (
	"fmt"
	"os"

	"github.com/wizardbeardstudio/open-rgs-go/internal/platform/evidence"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/verifysummary <summary.json>")
		os.Exit(2)
	}

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read summary: %v\n", err)
		os.Exit(1)
	}

	if err := evidence.ValidateSummaryJSON(data); err != nil {
		fmt.Fprintf(os.Stderr, "invalid verify summary: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("verify summary validation passed")
}
