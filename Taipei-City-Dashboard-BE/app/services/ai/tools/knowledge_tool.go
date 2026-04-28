package tools

import (
	"TaipeiCityDashboardBE/app/models"
	"context"
	"fmt"
	"strings"

	"github.com/tmc/langchaingo/llms"
)

// SearchKnowledgeArgs defines the arguments for the search_knowledge tool
type SearchKnowledgeArgs struct {
	Query string `json:"query"`
	Topic string `json:"topic,omitempty"` // e.g. "disaster"；空白 = 不限主題
}

// SearchKnowledge queries the city_knowledge Qdrant collection semantically.
// It is registered as the "search_knowledge" tool and called by the LLM
// whenever it needs to look up knowledge about any city topic.
func SearchKnowledge(ctx context.Context, args string) (string, error) {
	var params SearchKnowledgeArgs
	if err := parseArgs(args, &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}
	if strings.TrimSpace(params.Query) == "" {
		return "", fmt.Errorf("query 不能為空")
	}

	results, err := models.QueryCityKnowledge(params.Query, params.Topic, 5)
	if err != nil {
		return "", fmt.Errorf("知識庫查詢失敗: %v", err)
	}
	if len(results) == 0 {
		return "查無相關資料。", nil
	}

	var sb strings.Builder
	for i, chunk := range results {
		sb.WriteString(fmt.Sprintf("[%d] %s\n", i+1, chunk))
	}
	return sb.String(), nil
}

// SearchKnowledgeTool returns the langchaingo tool definition for search_knowledge.
// Called by controllers/ai.go to inject this tool into every AI request.
func SearchKnowledgeTool() llms.Tool {
	return llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "search_knowledge",
			Description: "語意搜尋城市知識庫，適用防災、食安、文化、勞動等主題。當需要查詢避難所資訊、水位狀況、醫療資源、政策法規等城市相關知識時使用。",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "搜尋問題或關鍵字，例如：「中山區避難所容量」、「台北市水位警戒狀況」",
					},
					"topic": map[string]interface{}{
						"type":        "string",
						"description": "主題過濾（選填）。可傳入 disaster（防災）等值，限縮搜尋範圍。不填則跨主題搜尋。",
					},
				},
				"required": []string{"query"},
			},
		},
	}
}

// DefaultServerTools returns all BE-managed tools to be injected into every AI request.
// Add new server-side tools here when needed.
func DefaultServerTools() []llms.Tool {
	return []llms.Tool{
		SearchKnowledgeTool(),
	}
}
