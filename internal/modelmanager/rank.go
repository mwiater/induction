package modelmanager

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

const maxPreferredProviders = 20

// SearchRanked performs bounded searches and stably promotes preferred authors.
func SearchRanked(ctx context.Context, client HubClient, query string, limit int, providers []string) ([]SearchResult, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("search limit must be between 1 and 100")
	}
	if len(providers) > maxPreferredProviders {
		providers = providers[:maxPreferredProviders]
	}
	type response struct {
		provider string
		values   []SearchResult
		err      error
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	out := make(chan response, len(providers)+1)
	var wg sync.WaitGroup
	search := func(provider string) {
		defer wg.Done()
		values, err := client.Search(ctx, query, limit, provider)
		out <- response{provider, values, err}
	}
	wg.Add(1)
	go search("")
	for _, provider := range providers {
		wg.Add(1)
		go search(provider)
	}
	go func() { wg.Wait(); close(out) }()
	groups := make(map[string][]SearchResult, len(providers)+1)
	for response := range out {
		if response.err != nil {
			cancel()
			return nil, response.err
		}
		groups[strings.ToLower(response.provider)] = response.values
	}
	seen := make(map[string]struct{})
	result := make([]SearchResult, 0, limit)
	appendGroup := func(values []SearchResult, expected string) {
		for _, item := range values {
			if expected != "" && !strings.EqualFold(item.Provider, expected) {
				continue
			}
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			result = append(result, item)
			if len(result) == limit {
				return
			}
		}
	}
	for _, provider := range providers {
		appendGroup(groups[strings.ToLower(provider)], provider)
		if len(result) == limit {
			return result, nil
		}
	}
	appendGroup(groups[""], "")
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
