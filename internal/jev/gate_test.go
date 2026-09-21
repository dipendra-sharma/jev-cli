package jev

import "testing"

func noulAnswer(p float64) Answer { return Answer{Type: TypeNoul, Noul: &p} }
func choiceAnswer(c float64) Answer {
	return Answer{Type: TypeChoice, Choice: "billing", Confidence: &c}
}

func TestNoulConfidenceMeasuresDistanceFromUndecided(t *testing.T) {
	cases := map[string]struct {
		probability float64
		want        float64
	}{
		"certain yes":   {1.0, 1.0},
		"certain no":    {0.0, 1.0},
		"undecided":     {0.5, 0.0},
		"leaning yes":   {0.8, 0.6},
		"leaning no":    {0.2, 0.6},
		"confident yes": {0.98, 0.96},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := noulAnswer(tc.probability).ConfidenceValue()
			if diff := got - tc.want; diff > 0.0001 || diff < -0.0001 {
				t.Errorf("probability %v: want confidence %v, got %v", tc.probability, tc.want, got)
			}
		})
	}
}

func TestVerdictBands(t *testing.T) {
	thresholds := Thresholds{Low: 0.4, High: 0.8}
	cases := map[string]struct {
		confidence float64
		want       Verdict
	}{
		"above high":   {0.95, VerdictAccept},
		"exactly high": {0.8, VerdictAccept},
		"between":      {0.6, VerdictReview},
		"exactly low":  {0.4, VerdictReview},
		"below low":    {0.1, VerdictReject},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := thresholds.Apply(tc.confidence); got != tc.want {
				t.Errorf("confidence %v: want %v, got %v", tc.confidence, tc.want, got)
			}
		})
	}
}

func TestWorstVerdictGovernsAMultiQuestionAnswer(t *testing.T) {
	thresholds := Thresholds{Low: 0.4, High: 0.8}
	answers := map[string]Answer{
		"certain":   choiceAnswer(0.99),
		"unsure":    noulAnswer(0.55),
		"confident": noulAnswer(0.99),
	}

	if got := WorstVerdict(answers, thresholds); got != VerdictReject {
		t.Errorf("want one uncertain answer to reject the whole set, got %v", got)
	}
}

func TestThresholdsRejectInvertedBounds(t *testing.T) {
	if err := (Thresholds{Low: 0.9, High: 0.2}).Validate(); err == nil {
		t.Error("want an error when low exceeds high, got nil")
	}
	if err := (Thresholds{Low: 0.2, High: 0.9}).Validate(); err != nil {
		t.Errorf("want valid bounds accepted, got %v", err)
	}
	if err := (Thresholds{Low: -0.1, High: 0.9}).Validate(); err == nil {
		t.Error("want an error for a threshold outside 0 to 1, got nil")
	}
}

func TestLegendLabelRendersEveryShapeALevelCanTake(t *testing.T) {
	cases := map[string]struct {
		value any
		want  string
	}{
		"string":              {"Calm, just stating facts", "Calm, just stating facts"},
		"object with what":    {map[string]any{"what": "Cosmetic", "examples": []any{"a typo"}}, "Cosmetic"},
		"array of text":       {[]any{"Handle immediately", "Customer is leaving"}, "Handle immediately / Customer is leaving"},
		"absent":              {nil, ""},
		"object without what": {map[string]any{"detail": "x"}, `{"detail":"x"}`},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := LegendLabel(tc.value); got != tc.want {
				t.Errorf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestNearestLevelRoundsToTheClosestRung(t *testing.T) {
	score := 1.6
	answer := Answer{
		Type:   TypeScore,
		Score:  &score,
		Legend: map[string]any{"0": "Calm", "1": "Annoyed", "2": "Furious"},
	}

	key, label, ok := answer.NearestLevel()

	if !ok {
		t.Fatal("want a nearest level for a score answer")
	}
	if key != "2" || label != "Furious" {
		t.Errorf("want score 1.6 to round to level 2 Furious, got level %s %q", key, label)
	}
}

func TestChoiceProbabilitiesRankHighestFirst(t *testing.T) {
	answer := Answer{
		Type:          TypeChoice,
		Choice:        "technical",
		Probabilities: map[string]float64{"billing": 0.09, "technical": 0.81, "sales": 0.10},
	}

	ranked := answer.RankedProbabilities()

	if ranked[0].Key != "technical" || ranked[1].Key != "sales" || ranked[2].Key != "billing" {
		t.Errorf("want descending probability order, got %s, %s, %s", ranked[0].Key, ranked[1].Key, ranked[2].Key)
	}
}

func TestScoreProbabilitiesKeepLevelOrder(t *testing.T) {
	answer := Answer{
		Type:          TypeScore,
		Probabilities: map[string]float64{"0": 0.1, "1": 0.2, "2": 0.7},
	}

	ranked := answer.RankedProbabilities()

	if ranked[0].Key != "0" || ranked[1].Key != "1" || ranked[2].Key != "2" {
		t.Errorf("want levels in rubric order, got %s, %s, %s", ranked[0].Key, ranked[1].Key, ranked[2].Key)
	}
}
