package jev

import (
	"fmt"
	"net/http"
	"time"
)

const (
	TypeNoul   = "noul"
	TypeChoice = "choice"
	TypeScore  = "score"

	MaxChoiceOptions = 255
	MinScoreLevels   = 2
	MaxScoreLevels   = 10

	ModelLatest  = "~typesafe/jev-latest"
	ModelPinned  = "typesafe/jev-1.13"
	ContextLimit = 32000
)

type NoulCriteria struct {
	True  any `json:"true,omitempty"`
	False any `json:"false,omitempty"`
}

type Question struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions,omitempty"`
	Criteria     any    `json:"criteria,omitempty"`
}

type Request struct {
	Model     string              `json:"model"`
	State     any                 `json:"state"`
	Questions map[string]Question `json:"questions"`
}

type Answer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Legend        map[string]any     `json:"legend,omitempty"`
}

type Usage struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	Cost         float64 `json:"cost"`
}

type Response struct {
	Model    string            `json:"model"`
	Provider string            `json:"provider"`
	ID       string            `json:"id"`
	Answers  map[string]Answer `json:"answers"`
	Usage    Usage             `json:"usage"`
}

type Model struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextLength int    `json:"context_length"`
	Pricing       struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
	Architecture struct {
		Modality         string   `json:"modality"`
		OutputModalities []string `json:"output_modalities"`
	} `json:"architecture"`
}

type APIError struct {
	StatusCode int
	Message    string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	switch e.StatusCode {
	case 401:
		return fmt.Sprintf("unauthorized (401): %s — check OPENROUTER_API_KEY", e.Message)
	case 402:
		return fmt.Sprintf("payment required (402): %s — the OpenRouter account is out of credit", e.Message)
	case 422:
		return fmt.Sprintf("invalid request (422): %s", e.Message)
	case 429:
		return fmt.Sprintf("rate limited (429): %s", e.Message)
	case 529:
		return fmt.Sprintf("provider overloaded (529): %s", e.Message)
	default:
		return fmt.Sprintf("openrouter returned %d: %s", e.StatusCode, e.Message)
	}
}

func (e *APIError) Retryable() bool {
	switch {
	case e.StatusCode == http.StatusRequestTimeout, e.StatusCode == http.StatusTooManyRequests:
		return true
	case e.StatusCode >= 500 && e.StatusCode < 600:
		return true
	default:
		return false
	}
}

func NewNoul(instructions any, whenTrue, whenFalse any) Question {
	q := Question{Type: TypeNoul, Instructions: instructions}
	if whenTrue != nil || whenFalse != nil {
		q.Criteria = NoulCriteria{True: whenTrue, False: whenFalse}
	}
	return q
}

func NewChoice(instructions any, options map[string]any) (Question, error) {
	if len(options) == 0 {
		return Question{}, fmt.Errorf("a choice question needs at least one option")
	}
	if len(options) > MaxChoiceOptions {
		return Question{}, fmt.Errorf("a choice question allows at most %d options, got %d", MaxChoiceOptions, len(options))
	}
	return Question{Type: TypeChoice, Instructions: instructions, Criteria: options}, nil
}

func NewScore(instructions any, levels []any) (Question, error) {
	if len(levels) < MinScoreLevels || len(levels) > MaxScoreLevels {
		return Question{}, fmt.Errorf("a score question needs between %d and %d levels, got %d", MinScoreLevels, MaxScoreLevels, len(levels))
	}
	return Question{Type: TypeScore, Instructions: instructions, Criteria: levels}, nil
}
