package bench

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type Report struct {
	TotalScenarios  int                      `json:"total_scenarios"`
	TotalPassed     int                      `json:"total_passed"`
	OverallScore    float64                  `json:"overall_score"`    // 0-100
	OverallAccuracy float64                  `json:"overall_accuracy"` // 0-100
	AvgLatencyMs    int64                    `json:"avg_latency_ms"`
	P95LatencyMs    int64                    `json:"p95_latency_ms"`
	AbilityScores   map[string]AbilityReport `json:"ability_scores"`
	Failures        []FailureDetail          `json:"failures,omitempty"`
}

type AbilityReport struct {
	Ability    string  `json:"ability"`
	Total      int     `json:"total"`
	Passed     int     `json:"passed"`
	Score      float64 `json:"score"`          // avg score 0-100
	Accuracy   float64 `json:"accuracy"`       // pass_rate 0-100
	AvgLatency int64   `json:"avg_latency_ms"`
}

type FailureDetail struct {
	ScenarioID string   `json:"scenario_id"`
	Ability    string   `json:"ability"`
	Score      float64  `json:"score"`
	Missed     []string `json:"missed_terms"`
	FalsePos   []string `json:"false_positives"`
}

func GenerateReport(results []Result) Report {
	r := Report{AbilityScores: make(map[string]AbilityReport)}
	if len(results) == 0 {
		return r
	}
	r.TotalScenarios = len(results)
	type group struct {
		totalScore   float64
		totalLatency int64
		total, passed int
	}
	groups := make(map[string]*group)
	var totalScore float64
	var totalLatency int64
	latencies := make([]int64, 0, len(results))
	for _, res := range results {
		if res.Passed {
			r.TotalPassed++
		}
		totalScore += res.Score
		totalLatency += res.LatencyMs
		latencies = append(latencies, res.LatencyMs)
		g, ok := groups[res.Ability]
		if !ok {
			g = &group{}
			groups[res.Ability] = g
		}
		g.total++
		g.totalScore += res.Score
		g.totalLatency += res.LatencyMs
		if res.Passed {
			g.passed++
		}
		if res.Score < 0.8 {
			r.Failures = append(r.Failures, FailureDetail{
				ScenarioID: res.ScenarioID, Ability: res.Ability,
				Score: res.Score, Missed: res.MissedTerms, FalsePos: res.FalsePositives,
			})
		}
	}
	n := float64(r.TotalScenarios)
	r.OverallScore = (totalScore / n) * 100
	r.OverallAccuracy = (float64(r.TotalPassed) / n) * 100
	r.AvgLatencyMs = totalLatency / int64(r.TotalScenarios)
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	r.P95LatencyMs = percentile(latencies, 95)
	for ability, g := range groups {
		r.AbilityScores[ability] = AbilityReport{
			Ability: ability, Total: g.total, Passed: g.passed,
			Score:      (g.totalScore / float64(g.total)) * 100,
			Accuracy:   (float64(g.passed) / float64(g.total)) * 100,
			AvgLatency: g.totalLatency / int64(g.total),
		}
	}
	return r
}

func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := (p * len(sorted)) / 100
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

func (r Report) PrintMarkdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# memorX Benchmark Report\n\n## Overall\n| Metric | Value |\n|--------|-------|\n"+
		"| Scenarios | %d |\n| Passed | %d |\n| Overall Score | %.1f%% |\n| Accuracy | %.1f%% |\n"+
		"| Avg Latency | %dms |\n| P95 Latency | %dms |\n\n",
		r.TotalScenarios, r.TotalPassed, r.OverallScore, r.OverallAccuracy, r.AvgLatencyMs, r.P95LatencyMs)
	b.WriteString("## Ability Breakdown\n| Ability | Score | Accuracy | Scenarios |\n|---------|-------|----------|----------|\n")
	abilities := make([]string, 0, len(r.AbilityScores))
	for a := range r.AbilityScores {
		abilities = append(abilities, a)
	}
	sort.Strings(abilities)
	for _, a := range abilities {
		ar := r.AbilityScores[a]
		fmt.Fprintf(&b, "| %s | %.1f%% | %.1f%% | %d |\n", ar.Ability, ar.Score, ar.Accuracy, ar.Total)
	}
	b.WriteByte('\n')
	if len(r.Failures) > 0 {
		b.WriteString("## Failures\n")
		for _, f := range r.Failures {
			fmt.Fprintf(&b, "- %s: %s (%.2f)", f.ScenarioID, f.Ability, f.Score)
			if len(f.Missed) > 0 {
				fmt.Fprintf(&b, " — missed: %q", f.Missed)
			}
			if len(f.FalsePos) > 0 {
				fmt.Fprintf(&b, " — false positives: %q", f.FalsePos)
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (r Report) PrintJSON() string {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(data)
}
