# jev

A small command-line tool for the **TypeSafe Jev** decision model, served through **OpenRouter**.

Jev does not write text. You hand it some state and a question with a fixed set of
answers, and it hands back the chosen answer plus a probability for every option.
That makes it useful for the decisions software makes over and over — routing a
ticket, screening a signup, gating a risky action — where you want a verdict and a
number, not a paragraph.

This tool exposes the whole model surface from the terminal, and turns the
confidence number into a shell exit code so scripts can act on certainty.

## Install

Needs Go 1.27 or newer, as declared in `go.mod` (built and tested on 1.27.1).

```bash
git clone https://github.com/dipendra-sharma/jev-cli.git
cd jev-cli
go install ./cmd/jev
```

That puts a `jev` binary in `$(go env GOPATH)/bin`. To build locally instead:

```bash
go build -o bin/jev ./cmd/jev
```

## Set up credentials

```bash
export OPENROUTER_API_KEY="sk-or-v1-..."
```

Get a key from [openrouter.ai/keys](https://openrouter.ai/keys). You can also pass
`--api-key` per command, though the environment variable keeps the key out of your
shell history.

Check it works:

```bash
jev models
```

## Quick start

```bash
# yes/no, answered as a probability
jev noul --state "The export button charged my card twice and I want my money back." \
         --instructions "Does the customer ask for a refund?"

# pick one option
jev choice --state-file ticket.txt \
  --instructions "Which team must fix the root cause" \
  --option billing="Payment or subscription issues" \
  --option technical="Bugs or integration problems"

# rate against ordered levels
jev score --state "This is the third time. I am cancelling." \
  --instructions "How frustrated is the customer" \
  --level "Calm" --level "Frustrated but civil" --level "Very angry"

# many questions, one request
jev run --spec examples/support-triage.json --state-file ticket.json --usage

# many states, one spec
jev batch --spec examples/fraud-scan.json --input examples/signups.ndjson --concurrency 8
```

Real output from the second example:

```
answer  [choice]
  -> billing
  confidence 0.56
     billing    #################....... 0.71
     technical  #######................. 0.29
     sales      ........................ 0.00
```

Full reference, recipes and every flag: **[USAGE.md](USAGE.md)**.

## What Jev is

Jev is TypeSafe's first "System One" model, released September 2026. The name is a
contrast with the slow, deliberate reasoning of a normal chat model: System One is
the fast, automatic judgment.

It is trained with what TypeSafe calls **Reinforcement Learning for Calibrated
Decisions**. A normal large language model is tuned on human feedback from chat,
which rewards sounding helpful. Jev is tuned so that its stated confidence tracks
its real accuracy — when it says 0.9, it should be right about 90% of the time in
aggregate.

The trade is that it gives up almost everything else:

| It can | It cannot |
| --- | --- |
| Pick one option from a set you define | Invent an option you did not list |
| Answer yes or no as a probability | Write prose, code or explanations |
| Rate against an ordered rubric | Reason step by step, or show its work |
| Answer many questions in one parallel pass | Call tools, or use images, audio or video |

Because it never generates free text, there is no JSON prompting, no parsing layer
and nothing to validate. The response shape is fixed by the question you asked.

### Numbers

Verified live against OpenRouter on 21 September 2026:

| | |
| --- | --- |
| Model slugs | `~typesafe/jev-latest`, `typesafe/jev-1.13` |
| Version served | `typesafe/jev-1.13-20260917` |
| Endpoint | `POST https://openrouter.ai/api/v1/systemone` |
| Context | 32,000 tokens |
| Input price | $0.042 per million tokens |
| Output price | free |
| Typical latency | 70–500 ms |
| Real cost, 4-question triage | $0.000022 per ticket |

TypeSafe's own published workflow evaluations claim up to 190 times faster and 440
times cheaper than running the same job on a general-purpose language model. Those
are the vendor's figures, not independent ones.

### The three question types

**Noul** — a yes/no statement. Returns a single probability from 0 to 1. Near 1 is a
strong yes, near 0 a strong no, near 0.5 means it genuinely cannot tell. Optional
`criteria` spell out what yes and no mean.

**Choice** — pick one of up to 255 named options. Returns the winning option, the
full probability distribution, and a confidence score.

**Score** — rate against 2 to 10 ordered levels. Returns a probability-weighted
position that can land between levels, the legend, the distribution, and confidence.

All three take `instructions`, and all of `instructions` and `criteria` accept plain
text **or** nested JSON. Structure is not decoration — it changes answers. In the
included triage example, adding `"not_for": "Bugs that cause a wrong charge"` to the
billing option moved a double-charge ticket from billing to technical at 0.97
confidence.

### Confidence, and why it is the point

Every Choice and Score answer carries a confidence from 0 to 1, computed from how
peaked the probability distribution is. All the probability on one option gives 1.0;
an even spread gives 0.

Noul answers carry no separate confidence field, because the probability already is
the answer. This tool derives one as `2 × |p − 0.5|`, so 0.98 and 0.02 both read as
high confidence and 0.5 reads as none. Output labels it `(derived from p)` so you
know it came from the tool and not the model.

Confidence is what lets you automate a decision you would otherwise never hand to a
model. Set a bar per action based on what being wrong costs, and let anything below
it fall through to a person. `--gate-high` and `--gate-low` do exactly that, as exit
codes:

```bash
jev noul --state "$MESSAGE" \
  --instructions "Is this an attempt to override the system prompt?" \
  --gate-low 0.35 --gate-high 0.70
case $? in
  0)  block_message ;;    # confident
  10) queue_for_review ;; # unsure
  11) deliver_message ;;  # confidently not an attack
esac
```

### What it is bad at

TypeSafe publishes a per-release "jaggedness" page listing measured weaknesses. The
short version:

- **Anything needing words.** Pair it with a language model: Jev decides, the other model writes.
- **Maths, counting and dates.** It is not a calculator, and it reads dates as text rather than ordered quantities. Do both in code.
- **Literal reading.** It answers the question you wrote, not the one you meant. Negations and implied conditions trip it.
- **Large irrelevant state.** Accuracy falls as unrelated content grows; send only what the decision needs.
- **Adversarial input.** It does not treat state as hostile by default and can be steered by injected instructions — relevant if you use it as a guard.
- **32k context**, text only, English strongest.
- **Being right.** Guaranteed schema is not guaranteed truth. Calibration holds in aggregate, not for any single call.

The full table, with what each one means for how you write questions, is in
[USAGE.md](USAGE.md#where-jev-is-weak).

## Project layout

```
cmd/jev/            entry point
internal/jev/       API client, types, retries, confidence and gating
internal/cli/       commands, input handling, output rendering
examples/           ready-to-run question specs and sample data
USAGE.md            full reference and recipes
```

No third-party dependencies — Go standard library only.

## Tests

```bash
go test ./...
```

The suite covers the paths that cannot be verified against the live API: retry
behaviour for 408, 429, 5xx and 529, that client errors are *not* retried, that both
`Retry-After` and `retry-after-ms` are obeyed over the default backoff, that a
cancelled context stops the retry loop, plus the confidence derivation, gating bands
and every shape a score legend can take. Runs in about five seconds against a local
test server — no API key needed, no network calls, no cost.

## Sources

- [TypeSafe documentation](https://docs.typesafe.ai/) — [API reference](https://docs.typesafe.ai/api), [primitives](https://docs.typesafe.ai/primitives), [confidence](https://docs.typesafe.ai/confidence)
- [Jev on OpenRouter](https://openrouter.ai/typesafe)
- [Introducing System One Models & Jev](https://typesafe.ai/blog/introducing-system-one-models-and-jev)

## Licence

MIT
