package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/yeisme/pinax/internal/testkit/continuitydogfood"
)

func main() {
	cohortRun := flag.String("cohort-run", "", "Initial continuity dogfood run directory")
	followUpRun := flag.String("followup-run", "", "Seven-day continuity follow-up run directory")
	flag.Parse()

	result, err := continuitydogfood.AnalyzeDecision(continuitydogfood.DecisionConfig{
		CohortRun:   *cohortRun,
		FollowUpRun: *followUpRun,
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "continuity decision error: %v\n", err)
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stdout, "continuity decision evidence: %s\n", result.RunDir)
	_, _ = fmt.Fprintf(os.Stdout, "decision=%s maturity=%s follow_up_change=%s\n", result.Receipt.Decision, result.Receipt.Maturity, result.Receipt.FollowUpChange)
}
