package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/dipendra-sharma/jev-cli/internal/jev"
)

const maxBatchLineBytes = 4 << 20

type batchResult struct {
	Line    int                   `json:"line"`
	Verdict jev.Verdict           `json:"verdict,omitempty"`
	Model   string                `json:"model,omitempty"`
	Answers map[string]jev.Answer `json:"answers,omitempty"`
	Usage   *jev.Usage            `json:"usage,omitempty"`
	Error   string                `json:"error,omitempty"`
}

func runBatch(ctx context.Context, args []string, out io.Writer) error {
	fs := newFlagSet("batch")
	var opts options
	opts.register(fs, false)
	specPath := fs.String("spec", "", "JSON file holding the questions to ask of every state")
	inputPath := fs.String("input", "-", "newline-delimited states, one per line, or - for stdin")
	concurrency := fs.Int("concurrency", 4, "how many requests to keep in flight")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *specPath == "" {
		return usageError{fmt.Errorf("--spec is required")}
	}
	if *concurrency < 1 {
		return usageError{fmt.Errorf("--concurrency must be at least 1")}
	}

	spec, err := loadSpec(*specPath)
	if err != nil {
		return usageError{err}
	}
	questions, ok := spec["questions"]
	if !ok {
		return usageError{fmt.Errorf("spec %s has no \"questions\" field", *specPath)}
	}

	states, err := readBatchStates(*inputPath)
	if err != nil {
		return err
	}
	if len(states) == 0 {
		return usageError{fmt.Errorf("no states to process")}
	}

	client, err := opts.client()
	if err != nil {
		return err
	}

	results := dispatchBatch(ctx, client, &opts, questions, states, *concurrency)
	return writeBatchResults(out, results, opts.showUsage)
}

func readBatchStates(path string) ([]any, error) {
	source := os.Stdin
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("reading batch input: %w", err)
		}
		defer file.Close()
		source = file
	}

	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 0, 64<<10), maxBatchLineBytes)
	var states []any
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		states = append(states, decodeStructuredOrText(line))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading batch input: %w", err)
	}
	return states, nil
}

func dispatchBatch(ctx context.Context, client *jev.Client, opts *options, questions any, states []any, concurrency int) []batchResult {
	results := make([]batchResult, len(states))
	slots := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, state := range states {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()

			body := map[string]any{"model": opts.model, "state": state, "questions": questions}
			resp, err := client.Decide(ctx, body)
			if err != nil {
				results[i] = batchResult{Line: i + 1, Error: err.Error()}
				return
			}
			usage := resp.Usage
			results[i] = batchResult{
				Line:    i + 1,
				Verdict: jev.WorstVerdict(resp.Answers, opts.thresholds()),
				Model:   resp.Model,
				Answers: resp.Answers,
				Usage:   &usage,
			}
		})
	}
	wg.Wait()
	return results
}

func writeBatchResults(out io.Writer, results []batchResult, showUsage bool) error {
	enc := json.NewEncoder(out)
	var totalCost float64
	var totalIn, totalOut, failed int

	for _, result := range results {
		if err := enc.Encode(result); err != nil {
			return fmt.Errorf("writing results: %w", err)
		}
		if result.Error != "" {
			failed++
			continue
		}
		if result.Usage != nil {
			totalCost += result.Usage.Cost
			totalIn += result.Usage.InputTokens
			totalOut += result.Usage.OutputTokens
		}
	}

	if showUsage {
		fmt.Fprintf(os.Stderr, "%d states · %d failed · in %d tok · out %d tok · $%.6f\n",
			len(results), failed, totalIn, totalOut, totalCost)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d states failed", failed, len(results))
	}
	return nil
}
