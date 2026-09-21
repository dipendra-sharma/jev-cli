# Contributing

Bug reports, questions and pull requests are all welcome.

## Build and test

```bash
go build ./...
go vet ./...
go test ./...
```

The test suite uses a local HTTP test server. It needs no API key, makes no network
calls and costs nothing. It runs in about five seconds — the retry tests wait on real
timers.

To try a change against a live endpoint you need a key for one of the two providers:

```bash
export TYPESAFE_API_KEY="..."     # or OPENROUTER_API_KEY
go run ./cmd/jev models
```

## Pull requests

- One change per pull request, with the reason in the description.
- Add a test for any behaviour that could break silently. Retry, decoding and
  provider-selection paths all have tests already — follow the pattern in
  `internal/jev`.
- Run `gofmt -l .`, `go vet ./...` and `go test ./...` before pushing. The same three
  run in continuous integration.
- No third-party dependencies. This tool is standard library only, and that is a
  feature — it keeps `go install` instant and the supply chain empty.

## Commit messages

`<type>(<scope>): <description>`, for example `fix(jev): honour the Retry-After
header`. Put the why in the body, and link any source you relied on with a full URL
on its own line:

```
Ref: https://docs.typesafe.ai/sdk/python/api/retries
```

## Adding a provider

Providers live in `internal/jev/provider.go`. A new one is an entry in the
`providers` slice plus a function that decodes that service's model list. Everything
else — retries, gating, rendering — is shared. Order in the slice is the preference
order used when no `--provider` is given.
