package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ", ") }

func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func resolveState(literal, file string) (any, error) {
	raw, err := readStateBytes(literal, file)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, fmt.Errorf("state is empty: pass --state, --state-file, or pipe it on stdin")
	}
	return decodeStructuredOrText(trimmed), nil
}

func readStateBytes(literal, file string) ([]byte, error) {
	switch {
	case literal != "" && file != "":
		return nil, fmt.Errorf("pass either --state or --state-file, not both")
	case literal != "":
		return []byte(literal), nil
	case file == "-":
		return io.ReadAll(os.Stdin)
	case file != "":
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("reading state file: %w", err)
		}
		return raw, nil
	case stdinIsPiped():
		return io.ReadAll(os.Stdin)
	default:
		return nil, fmt.Errorf("no state given: pass --state, --state-file, or pipe it on stdin")
	}
}

func stdinIsPiped() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice == 0
}

func decodeStructuredOrText(s string) any {
	if !strings.HasPrefix(s, "{") && !strings.HasPrefix(s, "[") {
		return s
	}
	var structured any
	if err := json.Unmarshal([]byte(s), &structured); err != nil {
		return s
	}
	return structured
}

func parseKeyValue(pair string) (string, any, error) {
	name, description, found := strings.Cut(pair, "=")
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil, fmt.Errorf("option %q needs a name before the '='", pair)
	}
	if !found || strings.TrimSpace(description) == "" {
		return name, nil, nil
	}
	return name, decodeStructuredOrText(strings.TrimSpace(description)), nil
}

func parseOptions(pairs []string) (map[string]any, error) {
	options := make(map[string]any, len(pairs))
	for _, pair := range pairs {
		name, description, err := parseKeyValue(pair)
		if err != nil {
			return nil, err
		}
		if _, exists := options[name]; exists {
			return nil, fmt.Errorf("option %q given twice", name)
		}
		options[name] = description
	}
	return options, nil
}

func parseLevels(levels []string) []any {
	parsed := make([]any, 0, len(levels))
	for _, level := range levels {
		parsed = append(parsed, decodeStructuredOrText(strings.TrimSpace(level)))
	}
	return parsed
}

func loadSpec(path string) (map[string]any, error) {
	var raw []byte
	var err error
	if path == "-" {
		raw, err = io.ReadAll(os.Stdin)
	} else {
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("reading spec: %w", err)
	}
	spec := map[string]any{}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, fmt.Errorf("parsing spec %s: %w", path, err)
	}
	if _, ok := spec["questions"]; !ok {
		return nil, fmt.Errorf("spec %s has no \"questions\" field", path)
	}
	return spec, nil
}
