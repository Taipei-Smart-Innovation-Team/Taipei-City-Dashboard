package models

import (
	"TaipeiCityDashboardBE/global"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// cityKnowledgeQueryRequest extends the base query with optional payload filter
type cityKnowledgeQueryRequest struct {
	Query          []float32     `json:"query"`
	Limit          int           `json:"limit"`
	ScoreThreshold float32       `json:"score_threshold,omitempty"`
	WithPayload    bool          `json:"with_payload"`
	Filter         *qdrantFilter `json:"filter,omitempty"`
}

type qdrantFilter struct {
	Must []qdrantCondition `json:"must,omitempty"`
}

type qdrantCondition struct {
	Key   string       `json:"key"`
	Match qdrantMatch  `json:"match"`
}

type qdrantMatch struct {
	Value string `json:"value"`
}

// QueryCityKnowledge performs semantic search on the city_knowledge collection.
// topic is optional; if provided, results are filtered to that topic only.
// Returns the content strings of matching chunks.
func QueryCityKnowledge(query, topic string, limit int) ([]string, error) {
	vector, err := GenVector(query)
	if err != nil {
		return nil, fmt.Errorf("generate vector error: %w", err)
	}

	reqBody := cityKnowledgeQueryRequest{
		Query:          vector,
		Limit:          limit,
		ScoreThreshold: 0.5,
		WithPayload:    true,
	}

	if topic != "" {
		reqBody.Filter = &qdrantFilter{
			Must: []qdrantCondition{
				{Key: "topic", Match: qdrantMatch{Value: topic}},
			},
		}
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request error: %w", err)
	}

	url := fmt.Sprintf("%s/collections/%s/points/query",
		global.Qdrant.Url, global.Qdrant.CityCollection)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("new request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", global.Qdrant.ApiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("qdrant returned status %s, body=%s", resp.Status, string(b))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body error: %w", err)
	}

	var result QdrantQueryResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode response error: %w", err)
	}

	contents := make([]string, 0, len(result.Result.Points))
	for _, point := range result.Result.Points {
		if content, ok := point.Payload["content"].(string); ok && content != "" {
			contents = append(contents, content)
		}
	}
	return contents, nil
}
