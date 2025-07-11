package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"
	"github.com/upstash/vector-go"
)

type MockVectorIndex struct {
	data map[string]string
}

func NewMockVectorIndex() *MockVectorIndex {
	return &MockVectorIndex{
		data: make(map[string]string),
	}
}

func (m *MockVectorIndex) UpsertData(data vector.UpsertData) error {
	m.data[data.Id] = data.Data
	return nil
}

type MockScore struct {
	Id    string
	Score float64
	Data  string
}

func (m *MockVectorIndex) QueryData(query vector.QueryData) ([]MockScore, error) {
	var results []MockScore
	
	if query.TopK == 1 {
		if content, exists := m.data[query.Data]; exists {
			results = append(results, MockScore{
				Id:    query.Data,
				Score: 1.0,
				Data:  content,
			})
		}
	} else {
		for id, content := range m.data {
			if strings.Contains(strings.ToLower(content), strings.ToLower(query.Data)) {
				results = append(results, MockScore{
					Id:    id,
					Score: 0.95,
					Data:  content,
				})
			}
		}
		
		if len(results) > query.TopK {
			results = results[:query.TopK]
		}
	}
	
	return results, nil
}

func createMemoryMCPServer(t *testing.T) *mcptest.Server {
	srv := mcptest.NewUnstartedServer(t)
	
	mockIndex := NewMockVectorIndex()
	
	addToMemory := mcp.NewTool("add-to-memory",
		mcp.WithDescription("Add a new memory or update an existing memory"),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("Unique identifier for the memory"),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("The memory content to store"),
		),
		mcp.WithString("metadata",
			mcp.Description("Additional metadata for the memory"),
		),
	)

	searchMemory := mcp.NewTool("search-memory",
		mcp.WithDescription("Search for memories using semantic similarity"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query text"),
		),
		mcp.WithNumber("top_k",
			mcp.Description("Number of results to return (default: 5)"),
		),
	)

	getMemory := mcp.NewTool("get-memory",
		mcp.WithDescription("Get a specific memory by ID"),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("Memory ID to retrieve"),
		),
	)

	srv.AddTool(addToMemory, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		id, ok := args["id"].(string)
		if !ok {
			return nil, fmt.Errorf("argument 'id' is missing or not a string")
		}

		content, ok := args["content"].(string)
		if !ok {
			return nil, fmt.Errorf("argument 'content' is missing or not a string")
		}

		metadata, _ := args["metadata"].(string)

		data := content
		if metadata != "" {
			data = fmt.Sprintf("%s [metadata: %s]", content, metadata)
		}

		err := mockIndex.UpsertData(vector.UpsertData{
			Id:   id,
			Data: data,
		})

		if err != nil {
			return nil, fmt.Errorf("error storing memory: %v", err)
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully stored memory with ID: %s", id)), nil
	})

	srv.AddTool(searchMemory, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		query, ok := args["query"].(string)
		if !ok {
			return nil, fmt.Errorf("argument 'query' is missing or not a string")
		}

		topK := 5
		if topKArg, exists := args["top_k"]; exists {
			if topKFloat, ok := topKArg.(float64); ok {
				topK = int(topKFloat)
			} else if topKStr, ok := topKArg.(string); ok {
				if parsed, err := strconv.Atoi(topKStr); err == nil {
					topK = parsed
				}
			}
		}

		scores, err := mockIndex.QueryData(vector.QueryData{
			Data: query,
			TopK: topK,
		})

		if err != nil {
			return nil, fmt.Errorf("error searching memories: %v", err)
		}

		if len(scores) == 0 {
			return mcp.NewToolResultText("No memories found matching your query"), nil
		}

		result := fmt.Sprintf("Found %d memories:\n", len(scores))
		for i, score := range scores {
			result += fmt.Sprintf("%d. ID: %s, Score: %.4f, Content: %s\n", i+1, score.Id, score.Score, score.Data)
		}

		return mcp.NewToolResultText(result), nil
	})

	srv.AddTool(getMemory, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		id, ok := args["id"].(string)
		if !ok {
			return nil, fmt.Errorf("argument 'id' is missing or not a string")
		}

		scores, err := mockIndex.QueryData(vector.QueryData{
			Data: id,
			TopK: 1,
		})

		if err != nil {
			return nil, fmt.Errorf("error retrieving memory: %v", err)
		}

		if len(scores) == 0 || scores[0].Id != id {
			return mcp.NewToolResultText(fmt.Sprintf("Memory with ID '%s' not found", id)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Memory ID: %s\nContent: %s", scores[0].Id, scores[0].Data)), nil
	})

	return srv
}

func TestAddToMemory(t *testing.T) {
	ctx := context.Background()
	srv := createMemoryMCPServer(t)
	defer srv.Close()

	err := srv.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}

	client := srv.Client()

	tests := []struct {
		name     string
		args     map[string]any
		expected string
	}{
		{
			name: "add memory without metadata",
			args: map[string]any{
				"id":      "test-1",
				"content": "This is a test memory",
			},
			expected: "Successfully stored memory with ID: test-1",
		},
		{
			name: "add memory with metadata",
			args: map[string]any{
				"id":       "test-2",
				"content":  "This is another test memory",
				"metadata": "important, urgent",
			},
			expected: "Successfully stored memory with ID: test-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req mcp.CallToolRequest
			req.Params.Name = "add-to-memory"
			req.Params.Arguments = tt.args

			result, err := client.CallTool(ctx, req)
			if err != nil {
				t.Fatal("CallTool:", err)
			}

			got, err := resultToString(result)
			if err != nil {
				t.Fatal(err)
			}

			if got != tt.expected {
				t.Errorf("Got %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestAddToMemoryErrors(t *testing.T) {
	ctx := context.Background()
	srv := createMemoryMCPServer(t)
	defer srv.Close()

	err := srv.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}

	client := srv.Client()

	tests := []struct {
		name string
		args map[string]any
	}{
		{
			name: "missing id",
			args: map[string]any{
				"content": "This is a test memory",
			},
		},
		{
			name: "missing content",
			args: map[string]any{
				"id": "test-1",
			},
		},
		{
			name: "id is not string",
			args: map[string]any{
				"id":      123,
				"content": "This is a test memory",
			},
		},
		{
			name: "content is not string",
			args: map[string]any{
				"id":      "test-1",
				"content": 123,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req mcp.CallToolRequest
			req.Params.Name = "add-to-memory"
			req.Params.Arguments = tt.args

			_, err := client.CallTool(ctx, req)
			if err == nil {
				t.Error("Expected error but got none")
			}
		})
	}
}

func TestSearchMemory(t *testing.T) {
	ctx := context.Background()
	srv := createMemoryMCPServer(t)
	defer srv.Close()

	err := srv.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}

	client := srv.Client()

	var addReq mcp.CallToolRequest
	addReq.Params.Name = "add-to-memory"
	addReq.Params.Arguments = map[string]any{
		"id":      "test-1",
		"content": "This is a test memory about programming",
	}
	_, err = client.CallTool(ctx, addReq)
	if err != nil {
		t.Fatal("Setup failed:", err)
	}

	var searchReq mcp.CallToolRequest
	searchReq.Params.Name = "search-memory"
	searchReq.Params.Arguments = map[string]any{
		"query": "programming",
		"top_k": 5,
	}

	result, err := client.CallTool(ctx, searchReq)
	if err != nil {
		t.Fatal("CallTool:", err)
	}

	got, err := resultToString(result)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "Found 1 memories:") {
		t.Errorf("Expected to find 1 memory, got: %s", got)
	}
	if !strings.Contains(got, "test-1") {
		t.Errorf("Expected to find test-1, got: %s", got)
	}
}

func TestSearchMemoryNoResults(t *testing.T) {
	ctx := context.Background()
	srv := createMemoryMCPServer(t)
	defer srv.Close()

	err := srv.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}

	client := srv.Client()

	var searchReq mcp.CallToolRequest
	searchReq.Params.Name = "search-memory"
	searchReq.Params.Arguments = map[string]any{
		"query": "nonexistent",
	}

	result, err := client.CallTool(ctx, searchReq)
	if err != nil {
		t.Fatal("CallTool:", err)
	}

	got, err := resultToString(result)
	if err != nil {
		t.Fatal(err)
	}

	expected := "No memories found matching your query"
	if got != expected {
		t.Errorf("Got %q, want %q", got, expected)
	}
}

func TestGetMemory(t *testing.T) {
	ctx := context.Background()
	srv := createMemoryMCPServer(t)
	defer srv.Close()

	err := srv.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}

	client := srv.Client()

	var addReq mcp.CallToolRequest
	addReq.Params.Name = "add-to-memory"
	addReq.Params.Arguments = map[string]any{
		"id":      "test-memory",
		"content": "This is a specific test memory",
	}
	_, err = client.CallTool(ctx, addReq)
	if err != nil {
		t.Fatal("Setup failed:", err)
	}

	var getReq mcp.CallToolRequest
	getReq.Params.Name = "get-memory"
	getReq.Params.Arguments = map[string]any{
		"id": "test-memory",
	}

	result, err := client.CallTool(ctx, getReq)
	if err != nil {
		t.Fatal("CallTool:", err)
	}

	got, err := resultToString(result)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "Memory ID: test-memory") {
		t.Errorf("Expected memory ID in response, got: %s", got)
	}
	if !strings.Contains(got, "This is a specific test memory") {
		t.Errorf("Expected memory content in response, got: %s", got)
	}
}

func TestGetMemoryNotFound(t *testing.T) {
	ctx := context.Background()
	srv := createMemoryMCPServer(t)
	defer srv.Close()

	err := srv.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}

	client := srv.Client()

	var getReq mcp.CallToolRequest
	getReq.Params.Name = "get-memory"
	getReq.Params.Arguments = map[string]any{
		"id": "nonexistent",
	}

	result, err := client.CallTool(ctx, getReq)
	if err != nil {
		t.Fatal("CallTool:", err)
	}

	got, err := resultToString(result)
	if err != nil {
		t.Fatal(err)
	}

	expected := "Memory with ID 'nonexistent' not found"
	if got != expected {
		t.Errorf("Got %q, want %q", got, expected)
	}
}

func resultToString(result *mcp.CallToolResult) (string, error) {
	var b strings.Builder

	for _, content := range result.Content {
		text, ok := content.(mcp.TextContent)
		if !ok {
			return "", fmt.Errorf("unsupported content type: %T", content)
		}
		b.WriteString(text.Text)
	}

	if result.IsError {
		return "", fmt.Errorf("%s", b.String())
	}

	return b.String(), nil
}