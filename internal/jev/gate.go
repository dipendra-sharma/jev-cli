package jev

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type Verdict string

const (
	VerdictAccept Verdict = "accept"
	VerdictReview Verdict = "review"
	VerdictReject Verdict = "reject"
)

type Thresholds struct {
	Low  float64
	High float64
}

func (t Thresholds) Enabled() bool {
	return t.Low > 0 || t.High > 0
}

func (t Thresholds) Validate() error {
	for name, v := range map[string]float64{"--gate-low": t.Low, "--gate-high": t.High} {
		if v < 0 || v > 1 {
			return fmt.Errorf("%s must be between 0 and 1, got %v", name, v)
		}
	}
	if t.Low > t.High {
		return fmt.Errorf("--gate-low (%v) cannot exceed --gate-high (%v)", t.Low, t.High)
	}
	return nil
}

func (t Thresholds) Apply(confidence float64) Verdict {
	switch {
	case confidence >= t.High:
		return VerdictAccept
	case confidence >= t.Low:
		return VerdictReview
	default:
		return VerdictReject
	}
}

func (a Answer) ConfidenceValue() float64 {
	if a.Type == TypeNoul {
		if a.Noul == nil {
			return 0
		}
		return math.Abs(*a.Noul-0.5) * 2
	}
	if a.Confidence == nil {
		return 0
	}
	return *a.Confidence
}

func (a Answer) ConfidenceIsDerived() bool {
	return a.Type == TypeNoul
}

func (a Answer) Verdict(t Thresholds) Verdict {
	return t.Apply(a.ConfidenceValue())
}

func (a Answer) NearestLevel() (string, string, bool) {
	if a.Type != TypeScore || a.Score == nil {
		return "", "", false
	}
	key := strconv.Itoa(int(math.Round(*a.Score)))
	return key, LegendLabel(a.Legend[key]), true
}

func LegendLabel(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case map[string]any:
		if what, ok := typed["what"].(string); ok {
			return what
		}
	case []any:
		if joined, ok := joinStrings(typed); ok {
			return joined
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(encoded)
}

func (a Answer) RankedProbabilities() []ProbabilityEntry {
	entries := make([]ProbabilityEntry, 0, len(a.Probabilities))
	for key, p := range a.Probabilities {
		entries = append(entries, ProbabilityEntry{Key: key, Label: LegendLabel(a.Legend[key]), Probability: p})
	}
	if a.Type == TypeScore {
		sort.Slice(entries, func(i, j int) bool { return numericKey(entries[i].Key) < numericKey(entries[j].Key) })
		return entries
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Probability != entries[j].Probability {
			return entries[i].Probability > entries[j].Probability
		}
		return entries[i].Key < entries[j].Key
	})
	return entries
}

func joinStrings(values []any) (string, bool) {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			return "", false
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, " / "), len(parts) > 0
}

type ProbabilityEntry struct {
	Key         string
	Label       string
	Probability float64
}

func numericKey(key string) int {
	n, err := strconv.Atoi(key)
	if err != nil {
		return math.MaxInt
	}
	return n
}

func WorstVerdict(answers map[string]Answer, t Thresholds) Verdict {
	worst := VerdictAccept
	rank := map[Verdict]int{VerdictAccept: 0, VerdictReview: 1, VerdictReject: 2}
	for _, a := range answers {
		if v := a.Verdict(t); rank[v] > rank[worst] {
			worst = v
		}
	}
	return worst
}
