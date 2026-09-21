package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/dipendra-sharma/jev-cli/internal/jev"
)

const barWidth = 24

func renderJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func renderRaw(w io.Writer, raw json.RawMessage) error {
	var pretty any
	if err := json.Unmarshal(raw, &pretty); err != nil {
		_, err := w.Write(raw)
		return err
	}
	return renderJSON(w, pretty)
}

func renderResponse(w io.Writer, resp *jev.Response, thresholds jev.Thresholds, showUsage bool) {
	for _, name := range sortedKeys(resp.Answers) {
		renderAnswer(w, name, resp.Answers[name], thresholds)
	}
	if showUsage {
		fmt.Fprintf(w, "\n%s  in %d tok · out %d tok · $%.8f · %s\n",
			resp.Model, resp.Usage.InputTokens, resp.Usage.OutputTokens, resp.Usage.Cost, resp.ID)
	}
}

func renderAnswer(w io.Writer, name string, a jev.Answer, thresholds jev.Thresholds) {
	fmt.Fprintf(w, "%s  [%s]\n", name, a.Type)
	fmt.Fprintf(w, "  -> %s\n", headline(a))
	fmt.Fprintf(w, "  %s\n", confidenceLine(a, thresholds))

	entries := a.RankedProbabilities()
	if len(entries) == 0 {
		fmt.Fprintln(w)
		return
	}
	labelWidth := 0
	for _, e := range entries {
		if n := len(entryLabel(e)); n > labelWidth {
			labelWidth = n
		}
	}
	for _, e := range entries {
		fmt.Fprintf(w, "     %-*s  %s %.2f\n", labelWidth, entryLabel(e), bar(e.Probability), e.Probability)
	}
	fmt.Fprintln(w)
}

func headline(a jev.Answer) string {
	switch a.Type {
	case jev.TypeNoul:
		if a.Noul == nil {
			return "no answer"
		}
		return fmt.Sprintf("%s  (p=%.2f)", yesNo(*a.Noul), *a.Noul)
	case jev.TypeChoice:
		return a.Choice
	case jev.TypeScore:
		if a.Score == nil {
			return "no answer"
		}
		if key, label, ok := a.NearestLevel(); ok && label != "" {
			return fmt.Sprintf("%.2f  (nearest level %s: %s)", *a.Score, key, label)
		}
		return fmt.Sprintf("%.2f", *a.Score)
	default:
		return "unknown answer type"
	}
}

func yesNo(p float64) string {
	switch {
	case p > 0.5:
		return "yes"
	case p < 0.5:
		return "no"
	default:
		return "undecided"
	}
}

func confidenceLine(a jev.Answer, thresholds jev.Thresholds) string {
	confidence := a.ConfidenceValue()
	suffix := ""
	if a.ConfidenceIsDerived() {
		suffix = " (derived from p)"
	}
	line := fmt.Sprintf("confidence %.2f%s", confidence, suffix)
	if thresholds.Enabled() {
		line += fmt.Sprintf("  ->  %s", strings.ToUpper(string(a.Verdict(thresholds))))
	}
	return line
}

func entryLabel(e jev.ProbabilityEntry) string {
	if e.Label == "" {
		return e.Key
	}
	return fmt.Sprintf("%s %s", e.Key, e.Label)
}

func bar(p float64) string {
	filled := max(min(int(p*barWidth+0.5), barWidth), 0)
	return strings.Repeat("#", filled) + strings.Repeat(".", barWidth-filled)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func renderModels(w io.Writer, models []jev.Model) {
	if len(models) == 0 {
		fmt.Fprintln(w, "no decision models available")
		return
	}
	for _, m := range models {
		fmt.Fprintf(w, "%s\n", m.ID)
		fmt.Fprintf(w, "  %s · %s · context %d\n", m.Name, m.Architecture.Modality, m.ContextLength)
		fmt.Fprintf(w, "  input %s/token · output %s/token\n\n", m.Pricing.Prompt, m.Pricing.Completion)
	}
}
