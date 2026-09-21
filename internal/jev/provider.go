package jev

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const (
	ProviderTypeSafe   = "typesafe"
	ProviderOpenRouter = "openrouter"
)

type Provider struct {
	Name        string
	BaseURL     string
	KeyEnv      string
	LatestModel string
	PinnedModel string
	ModelsPath  string
	parseModels func(json.RawMessage) ([]Model, error)
}

var providers = []Provider{
	{
		Name:        ProviderTypeSafe,
		BaseURL:     "https://api.typesafe.ai/v1",
		KeyEnv:      "TYPESAFE_API_KEY",
		LatestModel: "jev-latest",
		PinnedModel: "jev-1.13.0",
		ModelsPath:  "/models",
		parseModels: parseTypeSafeModels,
	},
	{
		Name:        ProviderOpenRouter,
		BaseURL:     "https://openrouter.ai/api/v1",
		KeyEnv:      "OPENROUTER_API_KEY",
		LatestModel: "~typesafe/jev-latest",
		PinnedModel: "typesafe/jev-1.13",
		ModelsPath:  "/models?output_modalities=decisions",
		parseModels: parseOpenRouterModels,
	},
}

func Providers() []Provider { return slices.Clone(providers) }

func ProviderNames() []string {
	names := make([]string, 0, len(providers))
	for _, p := range providers {
		names = append(names, p.Name)
	}
	return names
}

func ProviderByName(name string) (Provider, error) {
	for _, p := range providers {
		if strings.EqualFold(name, p.Name) {
			return p, nil
		}
	}
	return Provider{}, fmt.Errorf("unknown provider %q, want one of: %s", name, strings.Join(ProviderNames(), ", "))
}

func SelectProvider(name string, lookupEnv func(string) (string, bool)) (Provider, error) {
	if name != "" {
		return ProviderByName(name)
	}
	for _, p := range providers {
		if key, ok := lookupEnv(p.KeyEnv); ok && key != "" {
			return p, nil
		}
	}
	return providers[0], nil
}

func (p Provider) APIKey(explicit string, lookupEnv func(string) (string, bool)) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if key, ok := lookupEnv(p.KeyEnv); ok && key != "" {
		return key, nil
	}
	return "", fmt.Errorf("no API key for %s: set %s or pass --api-key", p.Name, p.KeyEnv)
}

func (p Provider) Model(requested string) string {
	if requested != "" {
		return requested
	}
	return p.LatestModel
}

func parseTypeSafeModels(raw json.RawMessage) ([]Model, error) {
	var payload struct {
		Models []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			ReleaseDate string `json:"release_date"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	models := make([]Model, 0, len(payload.Models))
	for _, m := range payload.Models {
		models = append(models, Model{
			ID:          m.Name,
			Description: m.Description,
			ReleaseDate: m.ReleaseDate,
		})
	}
	return models, nil
}

func parseOpenRouterModels(raw json.RawMessage) ([]Model, error) {
	var payload struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ContextLength int    `json:"context_length"`
			Pricing       struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	models := make([]Model, 0, len(payload.Data))
	for _, m := range payload.Data {
		models = append(models, Model{
			ID:            m.ID,
			Description:   m.Name,
			ContextLength: m.ContextLength,
			InputPrice:    m.Pricing.Prompt,
			OutputPrice:   m.Pricing.Completion,
		})
	}
	return models, nil
}
