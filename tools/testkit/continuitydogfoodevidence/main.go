package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/yeisme/pinax/tools/testkit/continuitydogfood"
)

func main() {
	vault := flag.String("vault", "", "Isolated copy of a real Pinax vault")
	pinaxPath := flag.String("pinax", "./dist/pinax", "Path to the Pinax binary")
	limit := flag.Int("limit", 10, "Number of independent real task samples")
	flag.Parse()

	result, err := continuitydogfood.Run(context.Background(), continuitydogfood.Config{
		PinaxPath: *pinaxPath,
		VaultPath: *vault,
		Limit:     *limit,
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "continuity dogfood error: %v\n", err)
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stdout, "continuity dogfood evidence: %s\n", result.RunDir)
	if result.Summary.Status != "success" {
		os.Exit(1)
	}
}
