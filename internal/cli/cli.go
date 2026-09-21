package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/dipendra-sharma/jev-cli/internal/jev"
)

const Version = "0.1.0"

const (
	ExitOK       = 0
	ExitError    = 1
	ExitUsage    = 2
	ExitReview   = 10
	ExitRejected = 11
)

type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

type options struct {
	model     string
	baseURL   string
	apiKey    string
	timeout   time.Duration
	retries   int
	jsonOut   bool
	showUsage bool
	gateLow   float64
	gateHigh  float64
	state     string
	stateFile string
}

func (o *options) register(fs *flag.FlagSet, withState bool) {
	fs.StringVar(&o.model, "model", jev.ModelLatest, "OpenRouter model slug")
	fs.StringVar(&o.baseURL, "base-url", jev.DefaultBaseURL, "OpenRouter API base URL")
	fs.StringVar(&o.apiKey, "api-key", "", "API key (defaults to $OPENROUTER_API_KEY)")
	fs.DurationVar(&o.timeout, "timeout", 60*time.Second, "request timeout")
	fs.IntVar(&o.retries, "retries", 3, "retries on 429, 529 and 5xx responses")
	fs.BoolVar(&o.jsonOut, "json", false, "print the raw JSON response")
	fs.BoolVar(&o.showUsage, "usage", false, "print token usage and cost")
	fs.Float64Var(&o.gateLow, "gate-low", 0, "confidence below this exits 11 (reject)")
	fs.Float64Var(&o.gateHigh, "gate-high", 0, "confidence below this but at or above --gate-low exits 10 (review)")
	if withState {
		fs.StringVar(&o.state, "state", "", "the content to evaluate, as text or JSON")
		fs.StringVar(&o.stateFile, "state-file", "", "read state from a file, or - for stdin")
	}
}

func (o *options) thresholds() jev.Thresholds {
	return jev.Thresholds{Low: o.gateLow, High: o.gateHigh}
}

func (o *options) client() (*jev.Client, error) {
	key := o.apiKey
	if key == "" {
		key = os.Getenv("OPENROUTER_API_KEY")
	}
	if key == "" {
		return nil, usageError{errors.New("no API key: set OPENROUTER_API_KEY or pass --api-key")}
	}
	if o.retries < 0 {
		return nil, usageError{fmt.Errorf("--retries cannot be negative")}
	}
	if err := o.thresholds().Validate(); err != nil {
		return nil, usageError{err}
	}
	return jev.NewClient(o.baseURL, key, o.retries, o.timeout), nil
}

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printHelp(stdout)
		return ExitUsage
	}

	command, rest := args[0], args[1:]
	ctx := context.Background()

	var err error
	var verdict jev.Verdict = jev.VerdictAccept

	switch command {
	case "noul":
		verdict, err = runNoul(ctx, rest, stdout)
	case "choice":
		verdict, err = runChoice(ctx, rest, stdout)
	case "score":
		verdict, err = runScore(ctx, rest, stdout)
	case "run":
		verdict, err = runSpec(ctx, rest, stdout)
	case "batch":
		err = runBatch(ctx, rest, stdout)
	case "models":
		err = runModels(ctx, rest, stdout)
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "jev %s\n", Version)
		return ExitOK
	case "help", "--help", "-h":
		printHelp(stdout)
		return ExitOK
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n", command)
		printHelp(stderr)
		return ExitUsage
	}

	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ExitOK
		}
		fmt.Fprintf(stderr, "jev: %v\n", err)
		if _, ok := errors.AsType[usageError](err); ok {
			return ExitUsage
		}
		return ExitError
	}

	switch verdict {
	case jev.VerdictReview:
		return ExitReview
	case jev.VerdictReject:
		return ExitRejected
	default:
		return ExitOK
	}
}

func printHelp(w io.Writer) {
	fmt.Fprint(w, `jev - ask the TypeSafe Jev decision model a typed question, through OpenRouter.

Jev does not write text. You give it state plus questions with fixed answers,
and it returns the chosen answer with a probability for every option.

USAGE
  jev <command> [flags]

COMMANDS
  noul     ask one yes/no question, answered as a probability
  choice   pick one option from a set you define
  score    rate the state against an ordered list of levels
  run      send a JSON spec with any number of questions in one request
  batch    run one spec over many states from newline-delimited JSON
  models   list the decision models OpenRouter currently serves
  version  print the version

STATE
  Every question needs state. Supply it with --state, --state-file, or by
  piping it on stdin. Text stays text; a JSON object or array is sent as
  structure, which Jev reads natively.

CONFIDENCE GATING
  --gate-high and --gate-low turn a confidence score into an exit code, so
  shell scripts can act on certainty instead of parsing output:

    exit 0   confidence >= --gate-high     act automatically
    exit 10  --gate-low <= c < --gate-high  send to a human
    exit 11  confidence < --gate-low        do not act
    exit 1   request failed
    exit 2   bad usage

EXAMPLES
  jev noul --state "I want my money back" --instructions "Is this a refund request?"

  jev choice --state-file ticket.txt \
    --instructions "Which team must fix the root cause" \
    --option billing="Payment or subscription issues" \
    --option technical="Bugs or integration problems"

  jev score --state "This is completely unacceptable" \
    --instructions "How frustrated is the customer" \
    --level "Calm" --level "Annoyed" --level "Furious"

  jev run --spec support.json --state-file ticket.json --usage

  cat signups.ndjson | jev batch --spec fraud.json --concurrency 8

Run 'jev <command> -h' for the flags of one command.
`)
}
