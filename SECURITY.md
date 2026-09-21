# Security

## Reporting a vulnerability

Please report security problems privately, not as a public issue. Use
[GitHub's private vulnerability reporting](https://github.com/dipendra-sharma/jev-cli/security/advisories/new)
on this repository.

Include what you did, what happened, and what you expected. You will get a reply
within a few days.

## Scope

This is a command-line client. The realistic risks are around credential handling:

- The tool reads your key from `TYPESAFE_API_KEY`, `OPENROUTER_API_KEY` or
  `--api-key`, sends it only as a `Bearer` header to the selected provider, and never
  writes it to output or to disk.
- `--base-url` sends your key to whatever host you name. Point it only at a host you
  trust.
- Error messages from the provider are printed as returned, truncated to 400
  characters. If a provider ever echoed a credential back in an error, it would be
  printed — no such behaviour is known.

Anything the model itself gets wrong is not a vulnerability in this tool. Jev does not
treat the state you send as hostile, so text you screen with it can attempt to steer
the answer. See the guard caveat in [USAGE.md](USAGE.md#recipes).
