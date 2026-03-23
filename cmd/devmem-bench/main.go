package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/arbazkhan971/memorx/bench"
)

func main() {
	ability := flag.String("ability", "", "Run only scenarios for this ability (e.g., session-continuity)")
	format := flag.String("format", "markdown", "Output format: markdown or json")
	verbose := flag.Bool("v", false, "Verbose: show each scenario result")
	scenario := flag.String("scenario", "", "Run a single scenario by ID (e.g., sc-001)")
	flag.Parse()

	scenarios := bench.AllScenarios()

	// Filter
	if *ability != "" {
		var filtered []bench.Scenario
		for _, s := range scenarios {
			if s.Ability == *ability {
				filtered = append(filtered, s)
			}
		}
		scenarios = filtered
	}
	if *scenario != "" {
		var filtered []bench.Scenario
		for _, s := range scenarios {
			if s.ID == *scenario {
				filtered = append(filtered, s)
			}
		}
		scenarios = filtered
	}

	if len(scenarios) == 0 {
		fmt.Fprintln(os.Stderr, "No scenarios matched filters")
		os.Exit(1)
	}

	// Create evaluator with temp DB
	tmpDir, _ := os.MkdirTemp("", "memorx-bench-*")
	defer os.RemoveAll(tmpDir)

	// Need to init a git repo in tmpDir for memorx to work
	// exec.Command("git", "init", tmpDir).Run()

	eval, err := bench.NewEvaluator(tmpDir+"/memory.db", tmpDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create evaluator: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Running %d scenarios...\n", len(scenarios))
	start := time.Now()

	results := eval.RunAll(scenarios)

	elapsed := time.Since(start)
	fmt.Fprintf(os.Stderr, "Completed in %s\n\n", elapsed.Round(time.Millisecond))

	if *verbose {
		for _, r := range results {
			status := "PASS"
			if !r.Passed {
				status = "FAIL"
			}
			fmt.Fprintf(os.Stderr, "[%s] %s (%.0f%%) %dms\n", status, r.ScenarioID, r.Score*100, r.LatencyMs)
			if len(r.MissedTerms) > 0 {
				fmt.Fprintf(os.Stderr, "  missed: %s\n", strings.Join(r.MissedTerms, ", "))
			}
		}
		fmt.Fprintln(os.Stderr)
	}

	report := bench.GenerateReport(results)

	if *format == "json" {
		fmt.Println(report.PrintJSON())
	} else {
		fmt.Println(report.PrintMarkdown())
	}
}
