package prowlarr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Indexer is a Prowlarr indexer as returned by GET /api/v1/indexer.
type Indexer struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	DefinitionName string `json:"definitionName"`
	Enable         bool   `json:"enable"`
	SupportsRss    bool   `json:"supportsRss"`
}

func listIndexers(ctx context.Context, baseURL, apiKey string) ([]Indexer, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/indexer", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Api-Key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prowlarr /api/v1/indexer returned %d", resp.StatusCode)
	}

	var indexers []Indexer
	if err := json.NewDecoder(resp.Body).Decode(&indexers); err != nil {
		return nil, err
	}

	return indexers, nil
}
