# jev — usage guide

Everything the tool does, with working examples. Every command and output below was
run against a live endpoint — the official TypeSafe API, OpenRouter, or both.

- [Setup](#setup)
- [Providers](#providers)
- [How a request is shaped](#how-a-request-is-shaped)
- [Giving it state](#giving-it-state)
- [Commands](#commands)
  - [noul](#noul--yesno)
  - [choice](#choice--pick-one-option)
  - [score](#score--rate-against-levels)
  - [run](#run--many-questions-at-once)
  - [batch](#batch--many-states-at-once)
  - [models](#models)
- [Flags shared by every command](#flags-shared-by-every-command)
- [Structured instructions and criteria](#structured-instructions-and-criteria)
- [Confidence and gating](#confidence-and-gating)
- [Exit codes](#exit-codes)
- [Errors and retries](#errors-and-retries)
- [Recipes](#recipes)
- [Cost](#cost)
- [Troubleshooting](#troubleshooting)

---

## Setup

```bash
go install github.com/dipendra-sharma/jev-cli/cmd/jev@latest
export TYPESAFE_API_KEY="..."   # or OPENROUTER_API_KEY, see Providers below
jev models
```

`jev models` is the cheapest check that your key works — it is a free metadata call.

Put the export in `~/.zshrc` to make it permanent. Never paste the key into a command
you will keep in shell history; use the environment variable.

---

## Providers

The same model is reachable two ways, and this tool speaks both.

| | `typesafe` | `openrouter` |
| --- | --- | --- |
| Endpoint | `https://api.typesafe.ai/v1/systemone` | `https://openrouter.ai/api/v1/systemone` |
| Key from | `$TYPESAFE_API_KEY` | `$OPENROUTER_API_KEY` |
| Get a key | [typesafe.ai](https://typesafe.ai) | [openrouter.ai/keys](https://openrouter.ai/keys) |
| Latest alias | `jev-latest` | `~typesafe/jev-latest` |
| Pinned version | `jev-1.13.0` | `typesafe/jev-1.13` |
| Reports cost and a request id | no | yes |

With no `--provider`, the tool uses **typesafe** when `$TYPESAFE_API_KEY` is set,
otherwise **openrouter** when `$OPENROUTER_API_KEY` is set. Name one explicitly to
override:

```bash
jev noul --provider openrouter --state "..." --instructions "..."
```

Model ids differ between the two, so `--model` is only portable within one provider.
Leave `--model` off and each provider gets its own latest alias.

Answers are the same model either way. Pick OpenRouter if you already meter spend
there or want the per-request cost in `--usage`; pick the official API for one less
hop.

---

## How a request is shaped

Every call has two halves.

**State** is the thing being judged: a ticket, a message, a signup record, a page of
text. One state per request.

**Questions** are what you want decided about it. Each question has a name you
choose, a type, instructions, and the answers it is allowed to give. You can ask up
to as many questions as fit the context window, and they are all answered in one
parallel pass — they do not see each other.

The tool sends this to `POST /systemone` on whichever provider is selected —
`https://api.typesafe.ai/v1` or `https://openrouter.ai/api/v1`:

```json
{
  "model": "jev-latest",
  "state": "The export button charged my card twice.",
  "questions": {
    "department": {
      "type": "choice",
      "instructions": "Which team must fix the root cause",
      "criteria": {
        "billing": "Payment or subscription issues",
        "technical": "Bugs or integration problems"
      }
    }
  }
}
```

and gets back (this one from OpenRouter, which adds `cost`, `id` and `provider`):

```json
{
  "model": "typesafe/jev-1.13-20260917",
  "answers": {
    "department": {
      "type": "choice",
      "choice": "billing",
      "probabilities": { "billing": 0.71, "technical": 0.29 },
      "confidence": 0.56
    }
  },
  "usage": { "input_tokens": 289, "output_tokens": 20, "cost": 0.00001214 },
  "id": "gen-dec-...",
  "provider": "TypeSafe"
}
```

The single-question commands (`noul`, `choice`, `score`) build that body for you and
name the question `answer`. The `run` and `batch` commands let you write it yourself.

---

## Complete schema reference

Every field the API accepts and returns. All of it is reachable through `jev run`
and `jev batch`; the single-question commands cover the common subset.

### JSON content

Four fields take what TypeSafe calls JSON content — **string**, **object**, **array**,
or **null**. Those are `state`, `instructions`, and every value inside `criteria`.
Structure is read natively, so pass records and rubrics as they already are rather
than flattening them into a sentence.

### Request

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `model` | string | yes | `jev-latest` or `jev-1.13.0` on the official API, `~typesafe/jev-latest` or `typesafe/jev-1.13` on OpenRouter; the tool sets it from `--model`, defaulting to the provider's latest alias |
| `state` | JSON content | yes | the one thing being judged |
| `questions` | map of name to Question | yes | names are yours; answers come back under the same names |

### Question

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `type` | `"noul"` \| `"choice"` \| `"score"` | yes | anything else is rejected with a 400 |
| `instructions` | JSON content | yes in practice | the question being asked |
| `criteria` | varies by type | see below | the answers it may give |

### Criteria by type

| Type | Criteria shape | Limits |
| --- | --- | --- |
| `noul` | object with optional `true` and `false`, each JSON content | optional; omit it entirely if the statement is unambiguous |
| `choice` | map of option name to JSON content **or `null`** | 1 to 255 options |
| `score` | ordered array of JSON content, lowest first | 2 to 10 levels, numbered from 0 |

A `null` choice description sends the option name alone, for when the name says
everything. On the command line that is `--option unclear=`.

### Response

| Field | Type | Notes |
| --- | --- | --- |
| `model` | string | the exact version that answered, e.g. `typesafe/jev-1.13-20260917` |
| `answers` | map of name to Answer | keyed by your question names |
| `usage.input_tokens` | integer | |
| `usage.output_tokens` | integer | billed at zero |
| `usage.cost` | number | OpenRouter only, in dollars; the official API omits it |
| `id` | string | OpenRouter only; quote it in support requests |
| `provider` | string | OpenRouter only; always `TypeSafe` |

### Answer, by type

| Type | Fields |
| --- | --- |
| `noul` | `type`, `noul` — one probability from 0 to 1. No confidence field |
| `choice` | `type`, `choice`, `probabilities` (per option, sums to 1), `confidence` |
| `score` | `type`, `score` (weighted position), `legend` (level number to description), `probabilities` (per level), `confidence` |

`score` is `Σ(level × probability)`, so it lands between levels when the model is
split. `legend` echoes back whatever shape you sent as a level — string, object or
array — so this tool renders an object's `what` field, joins an array with ` / `, and
falls back to compact JSON for anything else.

### Limits and pricing

| | |
| --- | --- |
| Context | 64,000 tokens per request, of which 32,000 for `state` plus the longest question; OpenRouter advertises 32,000 |
| Choice options | 255 maximum |
| Score levels | 2 to 10 |
| Input | $0.042 per million tokens |
| Output | free |
| Rate limit | 250,000 tokens per second, 1,200 requests per minute |
| Latency | 70–500 ms typical |
| Input modality | text only — no image, audio or video |

### Errors

| Status | Meaning | Retried? |
| --- | --- | --- |
| 400 / 422 | malformed request | no |
| 401 | bad or missing key | no |
| 402 | account out of credit | no |
| 429 | rate limited | yes, honouring `Retry-After` |
| 529 | provider overloaded | yes |
| 5xx | server error | yes |

### Coverage

Every row above was exercised against the live endpoint. Runnable examples:

| Shape | Example file |
| --- | --- |
| Plain strings, all three types | `examples/support-triage.json` |
| Object criteria with `what`, `not_for`, `examples` | `examples/support-triage.json` |
| Object score levels with `examples` | `examples/severity-rubric.json` |
| Object instructions | `examples/structured-instructions.json` |
| Array state, array instructions, array criteria, null option | `examples/all-shapes.json` |
| Batch over many states | `examples/fraud-scan.json` with `examples/signups.ndjson` |

```bash
jev run --spec examples/all-shapes.json --usage
```

---

## Giving it state

Three ways, in order of precedence:

```bash
jev noul --state "some text"           # inline
jev noul --state-file ticket.json      # from a file
echo "some text" | jev noul ...        # piped on stdin
jev noul --state-file - < ticket.json  # explicit stdin
```

Passing both `--state` and `--state-file` is an error.

**Text stays text. JSON becomes structure.** If what you pass starts with `{` or `[`
and parses as JSON, it is sent as real JSON and Jev reads the field names. Otherwise
it is sent as a plain string. So this:

```bash
echo '{"email":"bot@mailinator.com","signups_today":88,"prompts_run":0}' \
  | jev noul --instructions "Is this signup a bot?"
```

gives the model labelled fields rather than a sentence. Use structure whenever you
already have it — you do not need to write the record out as prose.

For `run`, the spec file may carry its own `state`, in which case you can leave the
flags off entirely. A `--state` or `--state-file` flag overrides what is in the spec.

---

## Commands

### `noul` — yes/no

Returns one probability from 0 to 1. Near 1 is a strong yes, near 0 a strong no,
near 0.5 means it cannot tell.

| Flag | Meaning |
| --- | --- |
| `--instructions` | the yes/no question (required) |
| `--true` | what a yes answer means (optional) |
| `--false` | what a no answer means (optional) |

```bash
jev noul \
  --state "The export button charged my card twice and I want my money back immediately." \
  --instructions "Does the customer ask for a refund?" \
  --true  "Explicitly asks for a refund or reversal" \
  --false "Reports a problem without asking for money back" \
  --usage
```

```
answer  [noul]
  -> yes  (p=0.98)
  confidence 0.96 (derived from p)

typesafe/jev-1.13-20260917  in 289 tok · out 20 tok · $0.00001214 · gen-dec-...
```

The `--true` and `--false` descriptions are worth writing whenever the boundary is at
all subtle. They are the cheapest accuracy you will buy.

---

### `choice` — pick one option

Returns the winning option, the full distribution, and a confidence score. Up to 255
options; the tool rejects more before spending a request.

| Flag | Meaning |
| --- | --- |
| `--instructions` | the question (required) |
| `--option name=description` | one option, repeatable |

```bash
jev choice \
  --state "The export button charged my card twice and my invoice is wrong." \
  --instructions "Which team must fix the root cause" \
  --option billing="Payment or subscription issues" \
  --option technical="Bugs or integration problems" \
  --option sales="Pricing or account questions"
```

```
answer  [choice]
  -> billing
  confidence 0.56
     billing    #################....... 0.71
     technical  #######................. 0.29
     sales      ........................ 0.00
```

The description is optional when the name speaks for itself — `--option billing=`
sends the name alone. Both the name and the description are visible to the model, so
name options meaningfully.

A description that is valid JSON is sent as structure. See
[structured instructions and criteria](#structured-instructions-and-criteria).

---

### `score` — rate against levels

Returns a probability-weighted position across your levels. Between 2 and 10 levels,
listed lowest first; the tool checks the count before spending a request.

| Flag | Meaning |
| --- | --- |
| `--instructions` | what the levels measure (required) |
| `--level "text"` | one level, lowest first, repeatable |

```bash
jev score \
  --state "This is the third time. Absolutely unacceptable, I am cancelling." \
  --instructions "How frustrated is the customer" \
  --level "Calm, just stating facts" \
  --level "Frustrated but civil" \
  --level "Very angry, threatening to leave"
```

```
answer  [score]
  -> 2.00  (nearest level 2: Very angry, threatening to leave)
  confidence 0.99
     0 Calm, just stating facts          ........................ 0.00
     1 Frustrated but civil              ........................ 0.00
     2 Very angry, threatening to leave  ######################## 1.00
```

The score is `Σ(level × probability)`, so it can land between levels. A 0.69 means
the weight sits mostly on level 1 but not entirely. Levels are numbered from 0 in the
order you list them.

---

### `run` — many questions at once

Send a JSON spec with any number of questions. They are answered in one request, in
parallel, each in isolation. This is the command that reaches the whole API — anything
the model accepts, you can write in the spec.

| Flag | Meaning |
| --- | --- |
| `--spec` | JSON file with `questions`, and optionally `state` (required) |

```bash
jev run --spec examples/support-triage.json \
  --state "The export button double charged my credit card so this month's invoice is wrong. Please reverse it." \
  --usage
```

```
department  [choice]
  -> technical
  confidence 0.95
     technical  #######################. 0.97
     billing    #....................... 0.03
     sales      ........................ 0.00

frustration  [score]
  -> 0.69  (nearest level 1: Frustrated but civil)
  confidence 0.53
     0 Calm, just stating facts                         #######................. 0.31
     1 Frustrated but civil                             #################....... 0.69
     2 Very angry, strong language or threats to leave  ........................ 0.00

needs_human  [noul]
  -> yes  (p=0.80)
  confidence 0.60 (derived from p)

refund_requested  [noul]
  -> yes  (p=0.97)
  confidence 0.94 (derived from p)

typesafe/jev-1.13-20260917  in 524 tok · out 89 tok · $0.00002201 · gen-dec-...
```

Four decisions about one ticket, one round trip, $0.000022.

Note it chose **technical**, not billing, for a double-charge complaint. That is the
structured `not_for` hint in the spec doing its job — see the next section.

Spec files are passed through untouched apart from `model` and `state`, so anything
the API supports works, including nested objects and `null` option descriptions.

---

### `batch` — many states at once

Run one spec over many states. Input is newline-delimited: one state per line, each
either JSON or plain text. Output is newline-delimited JSON, one result per input
line, in input order.

| Flag | Meaning |
| --- | --- |
| `--spec` | JSON file with the `questions` to ask of every state (required) |
| `--input` | file of states, one per line; default `-` for stdin |
| `--concurrency` | requests in flight at once; default 4 |

```bash
jev batch --spec examples/fraud-scan.json \
          --input examples/signups.ndjson \
          --concurrency 3 --gate-low 0.4 --gate-high 0.8 --usage
```

```json
{"line":1,"verdict":"review","model":"typesafe/jev-1.13-20260917","answers":{"fraud":{"type":"noul","noul":0.18},"value":{"type":"choice","choice":"enterprise_lead","confidence":0.96,"probabilities":{"enterprise_lead":0.97,"no_value":0.03,"normal_user":0}}},"usage":{"input_tokens":441,"output_tokens":61,"cost":0.000018522}}
{"line":2,"verdict":"accept","model":"typesafe/jev-1.13-20260917","answers":{"fraud":{"type":"noul","noul":0.94},"value":{"type":"choice","choice":"no_value","confidence":1,"probabilities":{"enterprise_lead":0,"no_value":1,"normal_user":0}}},"usage":{"input_tokens":445,"output_tokens":60,"cost":0.00001869}}
{"line":3,"verdict":"reject","model":"typesafe/jev-1.13-20260917","answers":{"fraud":{"type":"noul","noul":0.33},"value":{"type":"choice","choice":"normal_user","confidence":0.96,"probabilities":{"enterprise_lead":0,"no_value":0.02,"normal_user":0.98}}},"usage":{"input_tokens":442,"output_tokens":60,"cost":0.000018564}}
```

with a summary on stderr, so it does not pollute the data stream:

```
3 states · 0 failed · in 1328 tok · out 181 tok · $0.000056
```

Pipe the output straight into `jq`:

```bash
jev batch --spec examples/fraud-scan.json --input signups.ndjson \
  | jq -r 'select(.answers.fraud.noul > 0.8) | .line'
```

A line that fails gets `{"line":N,"error":"..."}` and the run continues. The command
exits 1 at the end if any line failed, so a partial failure is never silent.

The `verdict` on each line is the **worst** verdict across all that line's answers —
one uncertain question marks the whole record for review. That is deliberate: a gate
should be conservative.

All states are read into memory before dispatch. Fine for thousands of lines; if you
have millions, split the file.

---

### `models`

Lists the decision models the selected provider currently serves.

```bash
jev models --provider typesafe
```

```
jev-latest
  The latest iteration of TypeSafe's System One Model: Jev
  released 2026-09-10

jev-preview
  A preview version of `jev-latest`: should be better in most ways
  released 2026-09-10
```

On OpenRouter, decision models are hidden from the default model list, so the tool
queries `?output_modalities=decisions` for you:

```bash
jev models --provider openrouter
```

```
~typesafe/jev-latest
  TypeSafe: Jev Latest
  context 32000 · input 0.000000042/token · output 0/token

typesafe/jev-1.13
  TypeSafe: Jev 1.13
  context 32000 · input 0.000000042/token · output 0/token
```

Use the `-latest` alias to always follow the newest release, or pin the versioned id
if you want answers to stay stable as TypeSafe ships updates.

---

## Flags shared by every command

| Flag | Default | Meaning |
| --- | --- | --- |
| `--provider` | whichever key is set, `typesafe` first | `typesafe` or `openrouter` |
| `--model` | the provider's latest alias | model id |
| `--api-key` | the provider's key variable | credentials |
| `--base-url` | the provider's endpoint | for a proxy or gateway |
| `--timeout` | `60s` | per-request timeout |
| `--retries` | `3` | retries on 429, 529 and 5xx |
| `--json` | off | print the raw JSON response instead of the table |
| `--usage` | off | print tokens and cost |
| `--gate-low` | `0` | confidence below this exits 11 |
| `--gate-high` | `0` | confidence below this but at or above `--gate-low` exits 10 |
| `--state` | — | inline state (not on `models` or `batch`) |
| `--state-file` | — | state from a file, or `-` for stdin |

`--json` gives you the untouched response body, pretty-printed:

```bash
jev noul --state "I want a refund" --instructions "Is this a refund request?" --json
```

```json
{
  "answers": { "answer": { "noul": 0.99, "type": "noul" } },
  "id": "gen-dec-...",
  "model": "typesafe/jev-1.13-20260917",
  "provider": "TypeSafe",
  "usage": { "cost": 0.000011592, "input_tokens": 276, "output_tokens": 20 }
}
```

Gating still sets the exit code in `--json` mode, so you can have both the data and
the verdict.

---

## Structured instructions and criteria

`instructions` and every `criteria` value accept plain text **or** nested JSON. This
is the main lever you have for accuracy, because you cannot fine-tune Jev — the same
weights serve everyone, and you steer it entirely through the request.

Instead of a bare description:

```json
"billing": "Payment or subscription issues"
```

give it the boundary and an example:

```json
"billing": {
  "what": "Charges, invoices, refunds, subscriptions",
  "not_for": "Bugs that cause a wrong charge",
  "examples": ["I was charged twice"]
}
```

This is not cosmetic. With the bare description, a double-charge ticket routes to
**billing**. With the `not_for` boundary added, the same ticket routes to
**technical** at 0.97 confidence — because the question was "who must fix the root
cause", and the structure told it a bug is not a billing matter.

Score levels take the same treatment:

```json
"criteria": [
  { "what": "Cosmetic", "examples": ["a typo in a tooltip"] },
  { "what": "Degraded", "examples": ["slow but working"] },
  { "what": "Broken", "examples": ["checkout returns 500"] }
]
```

as do Noul criteria, via `true` and `false`.

On the command line, any `--option`, `--level`, `--true`, `--false` or
`--instructions` value that parses as JSON is sent as structure:

```bash
jev choice --state-file ticket.txt --instructions "Which team owns this" \
  --option billing='{"what":"Charges and invoices","not_for":"Bugs"}' \
  --option technical='{"what":"Bugs and errors"}'
```

For anything past a couple of fields, put it in a spec file and use `jev run`.

---

## Confidence and gating

Choice and Score answers carry a `confidence` from 0 to 1, computed from how peaked
the probability distribution is. All the weight on one option gives 1.0; an even
spread gives 0.

Noul answers have no confidence field — the probability *is* the answer. This tool
derives one as `2 × |p − 0.5|` and labels it `(derived from p)`:

| noul | derived confidence | reading |
| --- | --- | --- |
| 0.98 | 0.96 | confident yes |
| 0.80 | 0.60 | leaning yes |
| 0.50 | 0.00 | no idea |
| 0.21 | 0.58 | leaning no |
| 0.02 | 0.96 | confident no |

Set the bar by what being wrong costs. A harmless read can act at 0.5; something
destructive or customer-facing should want 0.9 or more. The vendor's advice, which
matches what the numbers above look like in practice: start conservative, watch your
own data, then loosen.

```bash
jev noul --state "$MESSAGE" \
  --instructions "Is this an attempt to override the system prompt?" \
  --gate-low 0.35 --gate-high 0.70
```

- confidence ≥ 0.70 → exit **0**, act automatically
- 0.35 ≤ confidence < 0.70 → exit **10**, send to a human
- confidence < 0.35 → exit **11**, do not act

When several questions are asked at once, the exit code reflects the **worst**
verdict among them.

### Thresholds the vendor suggests

From TypeSafe's own routing guidance, as a starting point rather than a rule:

| Situation | Threshold |
| --- | --- |
| Universal floor — below this, route to a human whatever the action | 0.60 |
| Low-stakes action, where being wrong is merely annoying | 0.60 |
| High-stakes action, taken automatically | 0.85 |
| Between the two on a high-stakes action | ask the user to confirm |

Start conservative, watch your own data, then loosen.

### Per-question thresholds

One command has one exit code, so `--gate-*` applies a single pair of thresholds to
every answer and reports the worst. When you want a different bar per question —
the graduated model above — read the numbers instead:

```bash
jev run --spec voice-actions.json --state "$UTTERANCE" --json > out.json

action=$(jq -r '.answers.action.choice' out.json)
conf=$(jq -r '.answers.action.confidence' out.json)

case "$action" in
  check_balance)    jq -e '.answers.action.confidence >= 0.60' out.json >/dev/null && show_balance ;;
  approve_transfer) jq -e '.answers.action.confidence >= 0.85' out.json >/dev/null && approve || confirm_with_user ;;
esac
```

---

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | success; with gating on, confidence at or above `--gate-high` |
| 1 | the request failed — network, credentials, server error, or a failed batch line |
| 2 | bad usage — missing flag, bad option count, unreadable spec |
| 10 | gated: confidence in the review band |
| 11 | gated: confidence below `--gate-low` |

Codes 10 and 11 only ever appear when you set a gate, so a script that ignores gating
sees only the normal 0/1/2.

```bash
jev run --spec guard.json --state "$MSG" --gate-low 0.35 --gate-high 0.7 >/dev/null
case $? in
  0)  echo "act" ;;
  10) echo "review" ;;
  11) echo "hold" ;;
  *)  echo "failed"; exit 1 ;;
esac
```

---

## Errors and retries

Errors are reported on stderr as a single line, prefixed `jev:`.

| Status | What it means | Retried |
| --- | --- | --- |
| 400 / 422 | your request is malformed — a bad question type, a missing field | no |
| 401 | the key is wrong or missing | no |
| 402 | the account is out of credit | no |
| 403 | the key lacks permission | no |
| 408 | the server timed out receiving the request | yes |
| 429 | rate limited | yes |
| 5xx, including 529 overloaded | server-side failure | yes |

Retries use exponential backoff with jitter, starting at 400 ms and capping at 8 s.
When the server sends `Retry-After` (seconds or an HTTP date) or `retry-after-ms`,
that wait is obeyed instead, capped at 30 s so a misbehaving header cannot hang the
command. Everything else fails immediately — retrying a malformed request just
wastes time and money. Control it with `--retries` (`0` disables).

This matches the official SDK's policy, which also retries 408, 429 and 5xx and
honours both header spellings.

Validation the tool does locally, before spending a request: more than 255 choice
options, fewer than 2 or more than 10 score levels, a gate-low above gate-high, a
negative retry count, a missing or unparseable spec file.

```
jev: a score question needs between 2 and 10 levels, got 1
jev: --gate-low (0.9) cannot exceed --gate-high (0.2)
jev: unauthorized (401): User not found. — check TYPESAFE_API_KEY
```

The 401 message names the key variable of the provider it actually called, so it
tells you which of the two keys is wrong.

---

## Recipes

### Guard a chat endpoint against prompt injection

```bash
echo "$USER_MESSAGE" | jev run --spec examples/jailbreak-guard.json \
  --gate-low 0.35 --gate-high 0.70 --json
```

The included spec asks two questions at once — whether it is an attack, and how
aggressive — so one call gives you both the block decision and a severity for your
logs.

> **Read this before shipping that one.** TypeSafe documents that Jev "does not treat
> state as hostile by default" and "can be steered by injected instructions or
> misleading framing". The text you are screening is the same text Jev reads, so a
> sufficiently clever injection can aim at the guard itself. Use it as one layer with
> a conservative threshold, not as your only defence.

### Composite scoring — many dimensions, your own weights

Ask each dimension as its own score question in one call, then combine them in code
with weights you control. Jev never sees the weights, so you can retune the formula
without touching the model.

```bash
jev run --spec candidate-rubric.json --state-file cv.json --json > scores.json

jq '
  (.answers.python_depth.score / 4)    as $py  |
  (.answers.team_leadership.score / 4) as $lead|
  (.answers.system_design.score / 4)   as $arch|
  { individual_contributor: (0.40*$py + 0.10*$lead + 0.40*$arch),
    engineering_manager:    (0.15*$py + 0.40*$lead + 0.20*$arch) }
' scores.json
```

Because questions run in parallel, adding dimensions costs almost nothing in time.

### Score a batch of signups for fraud and value

```bash
jev batch --spec examples/fraud-scan.json --input signups.ndjson --concurrency 8 \
  | jq -r 'select(.answers.fraud.noul > 0.85) | .line' > to_ban.txt
```

Three signups cost $0.000056. Scanning a few hundred a day costs cents.

### Triage support tickets into a queue

```bash
for f in tickets/*.json; do
  jev run --spec examples/support-triage.json --state-file "$f" --json \
    | jq --arg f "$f" '{file:$f, team:.answers.department.choice, conf:.answers.department.confidence}'
done
```

### Filter a stream and keep only what clears a bar

```bash
cat posts.ndjson | jev batch --spec launch-filter.json \
  | jq -c 'select(.answers.is_launch.noul > 0.8 and .answers.organic.noul > 0.7)'
```

### Classify many pages fast

Because output tokens are free and latency is 70–500 ms, work that was never worth
doing with a general model becomes routine. Feed one page per line and raise
`--concurrency`:

```bash
find site/ -name '*.md' -exec sh -c 'jq -Rs . < "$1"' _ {} \; \
  | jev batch --spec page-classify.json --concurrency 16
```

---

## Cost

Input is $0.042 per million tokens; output is free. Measured from real runs:

| Job | Tokens in | Cost |
| --- | --- | --- |
| One yes/no question | 289 | $0.0000121 |
| Four-question ticket triage | 524 | $0.0000220 |
| Three signups, two questions each | 1,328 | $0.0000560 |

Roughly: **a thousand two-question records costs about two cents.** Use `--usage` to
see the real figure for your own prompts; the state dominates, so trimming what you
send matters more than trimming questions.

---

## Where Jev is weak

TypeSafe publishes a "jaggedness" page for each release — the things the model is
measurably bad at. None of these are tool limitations; no flag fixes them. Design
around them.

| Weakness | What it means for your questions |
| --- | --- |
| **Literal reading** | It "answers the question you wrote, not the one you meant." Scoping words, negations and implied conditions trip it. Write the question plainly and positively. |
| **Maths and counting** | "Jev is not a calculator." Counting error grows with the number of items, and it cannot reliably tell whether two numbers are close. Do arithmetic in code and ask Jev only the judgment. |
| **Dates and times** | It reads dates as text, not as ordered quantities. Ordering, durations and time windows are unreliable, worse with mixed formats. Compare dates in code. |
| **Indirection** | Double negatives, properties of properties, and multi-hop reasoning all lower accuracy. Flatten what you ask. |
| **Large irrelevant state** | Accuracy falls as unrelated content grows — it acts as a distractor. Send only the fields the decision needs. |
| **Adversarial content** | It does not treat state as hostile by default and can be steered by injected instructions. See the guard caveat above. |
| **Contradictory instructions** | It gets confused when `instructions` and `criteria` pull in different directions. Keep them consistent. |
| **Structural invariants** | Related questions are answered independently, so their probabilities will not satisfy mathematical identities you might expect. Do not assume two questions are inverses of each other. |
| **Generation** | Not trained to produce text. Chaining it to build prose is slow and ineffective — pair it with a language model instead. |

**Language:** English is the primary training language. Other languages, including
Chinese, Japanese and Korean scripts, are accepted but currently less accurate.

**Calibration is an aggregate property.** A 0.9 means roughly nine in ten are right
across many calls. It does not mean any particular answer is right. Guaranteed schema
is not guaranteed truth.

---

## Troubleshooting

**`no API key for typesafe`** — set `TYPESAFE_API_KEY`, or set `OPENROUTER_API_KEY`
and the tool will use OpenRouter, or pass `--api-key`.

**`unknown provider "..."`** — `--provider` takes `typesafe` or `openrouter`.

**`unauthorized (401)`** — the key is wrong, revoked, or from a different account.
The message names the variable to check. Verify with `jev models`.

**`openrouter returned 400 ... Invalid discriminator value`** — a question in your
spec has a `type` that is not `noul`, `choice` or `score`. The same failure on the
official API reads `typesafe returned 400 ...`.

**`spec ... has no "questions" field`** — the spec must be an object with a
`questions` key; the question names inside it are yours to choose.

**Nothing happens, then it times out** — you probably piped nothing into stdin. The
tool waits on stdin when no `--state` or `--state-file` is given. Pass state
explicitly.

**The answer is confidently wrong** — that is the model, not the tool. Add
`--true`/`--false` or richer option descriptions with `not_for` boundaries and
examples; that is the whole adaptation surface Jev gives you. Then raise your gate.

**Everything comes back near 0.5** — the question is ambiguous or the state does not
contain the answer. Check by sending the same state with `--json` and reading the
distribution: a flat spread means it genuinely cannot tell from what you gave it.
