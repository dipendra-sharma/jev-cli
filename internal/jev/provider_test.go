package jev

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func envLookup(pairs map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := pairs[name]
		return value, ok
	}
}

func TestSelectProviderFollowsTheKeysThatAreSet(t *testing.T) {
	cases := map[string]struct {
		requested string
		env       map[string]string
		want      string
	}{
		"both keys set prefers the official API": {
			env:  map[string]string{"TYPESAFE_API_KEY": "t", "OPENROUTER_API_KEY": "o"},
			want: ProviderTypeSafe,
		},
		"only the openrouter key falls back to openrouter": {
			env:  map[string]string{"OPENROUTER_API_KEY": "o"},
			want: ProviderOpenRouter,
		},
		"no key at all still picks the official API so the error names it": {
			env:  map[string]string{},
			want: ProviderTypeSafe,
		},
		"an empty key counts as unset": {
			env:  map[string]string{"TYPESAFE_API_KEY": "", "OPENROUTER_API_KEY": "o"},
			want: ProviderOpenRouter,
		},
		"an explicit name beats the environment": {
			requested: ProviderOpenRouter,
			env:       map[string]string{"TYPESAFE_API_KEY": "t"},
			want:      ProviderOpenRouter,
		},
		"an explicit name is case insensitive": {
			requested: "TypeSafe",
			env:       map[string]string{"OPENROUTER_API_KEY": "o"},
			want:      ProviderTypeSafe,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			provider, err := SelectProvider(tc.requested, envLookup(tc.env))

			if err != nil {
				t.Fatalf("want provider %s, got error: %v", tc.want, err)
			}
			if provider.Name != tc.want {
				t.Errorf("want provider %s, got %s", tc.want, provider.Name)
			}
		})
	}
}

func TestSelectProviderRejectsAnUnknownName(t *testing.T) {
	_, err := SelectProvider("anthropic", envLookup(nil))

	if err == nil {
		t.Fatal("want an error for an unknown provider, got nil")
	}
	for _, name := range ProviderNames() {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("want the error to list %q as a valid provider, got %q", name, err)
		}
	}
}

func TestAPIKeyPrefersTheFlagThenTheProvidersEnvironmentVariable(t *testing.T) {
	provider, _ := ProviderByName(ProviderTypeSafe)

	fromFlag, err := provider.APIKey("from-flag", envLookup(map[string]string{"TYPESAFE_API_KEY": "from-env"}))
	if err != nil || fromFlag != "from-flag" {
		t.Errorf("want the explicit key to win, got %q (%v)", fromFlag, err)
	}

	fromEnv, err := provider.APIKey("", envLookup(map[string]string{"TYPESAFE_API_KEY": "from-env"}))
	if err != nil || fromEnv != "from-env" {
		t.Errorf("want the environment key, got %q (%v)", fromEnv, err)
	}

	_, err = provider.APIKey("", envLookup(nil))
	if err == nil || !strings.Contains(err.Error(), "TYPESAFE_API_KEY") {
		t.Errorf("want a missing-key error naming TYPESAFE_API_KEY, got %v", err)
	}
}

func TestModelDefaultsToTheProvidersLatestAlias(t *testing.T) {
	for _, provider := range Providers() {
		t.Run(provider.Name, func(t *testing.T) {
			if got := provider.Model(""); got != provider.LatestModel {
				t.Errorf("want the latest alias %q, got %q", provider.LatestModel, got)
			}
			if got := provider.Model("jev-1.13.0"); got != "jev-1.13.0" {
				t.Errorf("want the requested model to be kept, got %q", got)
			}
		})
	}
}

func TestDecisionModelsDecodesEachProvidersListShape(t *testing.T) {
	cases := map[string]struct {
		provider string
		body     string
		wantPath string
		want     Model
	}{
		ProviderTypeSafe: {
			provider: ProviderTypeSafe,
			body:     `{"models":[{"name":"jev-latest","description":"Alias for jev-1.13.0","release_date":"2026-09-17"}]}`,
			wantPath: "/models",
			want:     Model{ID: "jev-latest", Description: "Alias for jev-1.13.0", ReleaseDate: "2026-09-17"},
		},
		ProviderOpenRouter: {
			provider: ProviderOpenRouter,
			body:     `{"data":[{"id":"typesafe/jev-1.13","name":"TypeSafe: Jev 1.13","context_length":32000,"pricing":{"prompt":"0.000000042","completion":"0"}}]}`,
			wantPath: "/models?output_modalities=decisions",
			want: Model{
				ID:            "typesafe/jev-1.13",
				Description:   "TypeSafe: Jev 1.13",
				ContextLength: 32000,
				InputPrice:    "0.000000042",
				OutputPrice:   "0",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var requested string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requested = r.URL.RequestURI()
				w.Write([]byte(tc.body))
			}))
			t.Cleanup(server.Close)
			provider, err := ProviderByName(tc.provider)
			if err != nil {
				t.Fatalf("unknown provider in the test table: %v", err)
			}
			client := NewClient(provider, server.URL, "key", 0, 5*time.Second)

			models, err := client.DecisionModels(context.Background())

			if err != nil {
				t.Fatalf("want a decoded model list, got error: %v", err)
			}
			if requested != tc.wantPath {
				t.Errorf("want a request to %s, got %s", tc.wantPath, requested)
			}
			if len(models) != 1 {
				t.Fatalf("want 1 model, got %d", len(models))
			}
			if models[0] != tc.want {
				t.Errorf("want %+v, got %+v", tc.want, models[0])
			}
		})
	}
}

func TestUnauthorizedErrorNamesTheProvidersKeyVariable(t *testing.T) {
	for _, provider := range Providers() {
		t.Run(provider.Name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":{"message":"bad key"}}`))
			}))
			t.Cleanup(server.Close)
			client := NewClient(provider, server.URL, "key", 0, 5*time.Second)

			_, err := client.Decide(context.Background(), Request{})

			if err == nil || !strings.Contains(err.Error(), provider.KeyEnv) {
				t.Errorf("want the 401 to tell the user to check %s, got %v", provider.KeyEnv, err)
			}
		})
	}
}

func TestBaseURLDefaultsToTheProvidersEndpoint(t *testing.T) {
	for _, provider := range Providers() {
		t.Run(provider.Name, func(t *testing.T) {
			client := NewClient(provider, "", "key", 0, 5*time.Second)

			if client.BaseURL != provider.BaseURL {
				t.Errorf("want %s, got %s", provider.BaseURL, client.BaseURL)
			}
		})
	}
}
