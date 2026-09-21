package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/dipendra-sharma/jev-cli/internal/jev"
)

const singleQuestionKey = "answer"

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "jev %s [flags]\n\nFLAGS\n", name)
		fs.PrintDefaults()
	}
	return fs
}

func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return usageError{fmt.Errorf("unexpected argument %q", fs.Arg(0))}
	}
	return nil
}

func runNoul(ctx context.Context, args []string, out io.Writer) (jev.Verdict, error) {
	fs := newFlagSet("noul")
	var opts options
	opts.register(fs, true)
	instructions := fs.String("instructions", "", "the yes/no question to answer")
	whenTrue := fs.String("true", "", "what a yes answer means")
	whenFalse := fs.String("false", "", "what a no answer means")
	if err := parseFlags(fs, args); err != nil {
		return jev.VerdictAccept, err
	}
	if *instructions == "" {
		return jev.VerdictAccept, usageError{fmt.Errorf("--instructions is required")}
	}

	question := jev.NewNoul(
		decodeStructuredOrText(*instructions),
		optionalText(*whenTrue),
		optionalText(*whenFalse),
	)
	return askOne(ctx, &opts, question, out)
}

func runChoice(ctx context.Context, args []string, out io.Writer) (jev.Verdict, error) {
	fs := newFlagSet("choice")
	var opts options
	opts.register(fs, true)
	instructions := fs.String("instructions", "", "the question to answer")
	var optionPairs stringList
	fs.Var(&optionPairs, "option", "an option as name=description, repeatable (description optional)")
	if err := parseFlags(fs, args); err != nil {
		return jev.VerdictAccept, err
	}
	if *instructions == "" {
		return jev.VerdictAccept, usageError{fmt.Errorf("--instructions is required")}
	}

	choices, err := parseOptions(optionPairs)
	if err != nil {
		return jev.VerdictAccept, usageError{err}
	}
	question, err := jev.NewChoice(decodeStructuredOrText(*instructions), choices)
	if err != nil {
		return jev.VerdictAccept, usageError{err}
	}
	return askOne(ctx, &opts, question, out)
}

func runScore(ctx context.Context, args []string, out io.Writer) (jev.Verdict, error) {
	fs := newFlagSet("score")
	var opts options
	opts.register(fs, true)
	instructions := fs.String("instructions", "", "what the levels measure")
	var levels stringList
	fs.Var(&levels, "level", "one level, lowest first, repeatable (2 to 10 levels)")
	if err := parseFlags(fs, args); err != nil {
		return jev.VerdictAccept, err
	}
	if *instructions == "" {
		return jev.VerdictAccept, usageError{fmt.Errorf("--instructions is required")}
	}

	question, err := jev.NewScore(decodeStructuredOrText(*instructions), parseLevels(levels))
	if err != nil {
		return jev.VerdictAccept, usageError{err}
	}
	return askOne(ctx, &opts, question, out)
}

func askOne(ctx context.Context, opts *options, question jev.Question, out io.Writer) (jev.Verdict, error) {
	state, err := resolveState(opts.state, opts.stateFile)
	if err != nil {
		return jev.VerdictAccept, usageError{err}
	}
	client, err := opts.client()
	if err != nil {
		return jev.VerdictAccept, err
	}
	request := jev.Request{
		Model:     opts.model,
		State:     state,
		Questions: map[string]jev.Question{singleQuestionKey: question},
	}
	return send(ctx, client, opts, request, out)
}

func runSpec(ctx context.Context, args []string, out io.Writer) (jev.Verdict, error) {
	fs := newFlagSet("run")
	var opts options
	opts.register(fs, true)
	specPath := fs.String("spec", "", "JSON file holding questions, and optionally state")
	if err := parseFlags(fs, args); err != nil {
		return jev.VerdictAccept, err
	}
	if *specPath == "" {
		return jev.VerdictAccept, usageError{fmt.Errorf("--spec is required")}
	}

	spec, err := loadSpec(*specPath)
	if err != nil {
		return jev.VerdictAccept, usageError{err}
	}
	if opts.state != "" || opts.stateFile != "" || spec["state"] == nil {
		state, err := resolveState(opts.state, opts.stateFile)
		if err != nil {
			return jev.VerdictAccept, usageError{err}
		}
		spec["state"] = state
	}
	spec["model"] = opts.model

	client, err := opts.client()
	if err != nil {
		return jev.VerdictAccept, err
	}
	return send(ctx, client, &opts, spec, out)
}

func send(ctx context.Context, client *jev.Client, opts *options, body any, out io.Writer) (jev.Verdict, error) {
	if opts.jsonOut {
		raw, err := client.DecideRaw(ctx, body)
		if err != nil {
			return jev.VerdictAccept, err
		}
		if err := renderRaw(out, raw); err != nil {
			return jev.VerdictAccept, err
		}
		var parsed jev.Response
		if json.Unmarshal(raw, &parsed) == nil {
			return jev.WorstVerdict(parsed.Answers, opts.thresholds()), nil
		}
		return jev.VerdictAccept, nil
	}

	resp, err := client.Decide(ctx, body)
	if err != nil {
		return jev.VerdictAccept, err
	}
	renderResponse(out, resp, opts.thresholds(), opts.showUsage)
	return jev.WorstVerdict(resp.Answers, opts.thresholds()), nil
}

func runModels(ctx context.Context, args []string, out io.Writer) error {
	fs := newFlagSet("models")
	var opts options
	opts.register(fs, false)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	client, err := opts.client()
	if err != nil {
		return err
	}
	models, err := client.DecisionModels(ctx)
	if err != nil {
		return err
	}
	if opts.jsonOut {
		return renderJSON(out, models)
	}
	renderModels(out, models)
	return nil
}

func optionalText(s string) any {
	if s == "" {
		return nil
	}
	return decodeStructuredOrText(s)
}
