package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/yeisme/pinax/internal/testkit/continuitydogfood"
)

func main() {
	vault := flag.String("vault", "", "Preserved isolated vault used by the initial cohort")
	cohortRun := flag.String("cohort-run", "", "Initial dogfood run directory")
	pinaxPath := flag.String("pinax", "./dist/pinax", "Path to the Pinax binary")
	flag.Parse()

	result, err := continuitydogfood.RunFollowUp(context.Background(), continuitydogfood.FollowUpConfig{
		PinaxPath: *pinaxPath,
		VaultPath: *vault,
		CohortRun: *cohortRun,
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "continuity follow-up error: %v\n", err)
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stdout, "continuity follow-up evidence: %s\n", result.RunDir)
	if result.Summary.Status != "success" {
		os.Exit(1)
	}
}
