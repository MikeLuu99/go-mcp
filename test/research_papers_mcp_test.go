package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/agnivade/levenshtein"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"
)

type MockRedisClient struct {
	data map[string]string
}

func NewMockRedisClient() *MockRedisClient {
	return &MockRedisClient{
		data: make(map[string]string),
	}
}

func (m *MockRedisClient) Set(ctx context.Context, key string, value interface{}, expiration interface{}) error {
	if str, ok := value.(string); ok {
		m.data[key] = str
	}
	return nil
}

func (m *MockRedisClient) Get(ctx context.Context, key string) (string, error) {
	if value, exists := m.data[key]; exists {
		return value, nil
	}
	return "", fmt.Errorf("key not found")
}

func (m *MockRedisClient) Scan(ctx context.Context, cursor uint64, match string, count int64) []string {
	var keys []string
	for key := range m.data {
		keys = append(keys, key)
	}
	return keys
}

func createResearchPapersMCPServer(t *testing.T) *mcptest.Server {
	srv := mcptest.NewUnstartedServer(t)
	
	mockClient := NewMockRedisClient()

	setNewResearchPaper := mcp.NewTool("set-new-research-paper",
		mcp.WithDescription("Add a new research paper"),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("The name of the paper"),
		),
		mcp.WithString("summarization",
			mcp.Description("The main content of the paper"),
		),
	)

	getResearchPaper := mcp.NewTool("get-research-paper",
		mcp.WithDescription("Get the content of a research paper based on its name"),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("The name of the paper"),
		),
	)

	srv.AddTool(setNewResearchPaper, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		title, ok := args["title"].(string)
		if !ok {
			return nil, fmt.Errorf("argument 'title' is missing or not a string")
		}

		summarization, _ := args["summarization"].(string)

		err := mockClient.Set(ctx, title, summarization, 0)
		if err != nil {
			return nil, err
		}
		return mcp.NewToolResultText("Successful update of the knowledge base"), nil
	})

	srv.AddTool(getResearchPaper, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		title, ok := args["title"].(string)
		if !ok {
			return nil, fmt.Errorf("argument 'title' is missing or not a string")
		}

		val, err := mockClient.Get(ctx, title)
		if err == nil {
			return mcp.NewToolResultText(fmt.Sprintf("Found exact match for '%s': %s", title, val)), nil
		}

		var bestMatch string
		var bestValue string
		var bestDistance int = 999999
		const maxDistance = 3

		keys := mockClient.Scan(ctx, 0, "*", 0)
		for _, key := range keys {
			distance := levenshtein.ComputeDistance(strings.ToLower(title), strings.ToLower(key))

			if distance <= maxDistance && distance < bestDistance {
				bestDistance = distance
				bestMatch = key
			}
		}

		if bestMatch == "" {
			return mcp.NewToolResultText(fmt.Sprintf("No research paper found matching '%s'", title)), nil
		}

		bestValue, err = mockClient.Get(ctx, bestMatch)
		if err != nil {
			return nil, fmt.Errorf("error retrieving content for key '%s': %v", bestMatch, err)
		}

		return mcp.NewToolResultText(fmt.Sprintf("Found closest match '%s' (distance: %d): %s", bestMatch, bestDistance, bestValue)), nil
	})

	return srv
}

func TestSetNewResearchPaper(t *testing.T) {
	ctx := context.Background()
	srv := createResearchPapersMCPServer(t)
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
			name: "add paper with title and summarization",
			args: map[string]any{
				"title":         "Deep Learning Fundamentals",
				"summarization": "This paper covers the basic concepts of deep learning...",
			},
			expected: "Successful update of the knowledge base",
		},
		{
			name: "add paper with title only",
			args: map[string]any{
				"title": "Machine Learning Overview",
			},
			expected: "Successful update of the knowledge base",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req mcp.CallToolRequest
			req.Params.Name = "set-new-research-paper"
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

func TestSetNewResearchPaperErrors(t *testing.T) {
	ctx := context.Background()
	srv := createResearchPapersMCPServer(t)
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
			name: "missing title",
			args: map[string]any{
				"summarization": "This paper covers...",
			},
		},
		{
			name: "title is not string",
			args: map[string]any{
				"title":         123,
				"summarization": "This paper covers...",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req mcp.CallToolRequest
			req.Params.Name = "set-new-research-paper"
			req.Params.Arguments = tt.args

			_, err := client.CallTool(ctx, req)
			if err == nil {
				t.Error("Expected error but got none")
			}
		})
	}
}

func TestGetResearchPaperExactMatch(t *testing.T) {
	ctx := context.Background()
	srv := createResearchPapersMCPServer(t)
	defer srv.Close()

	err := srv.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}

	client := srv.Client()

	var setReq mcp.CallToolRequest
	setReq.Params.Name = "set-new-research-paper"
	setReq.Params.Arguments = map[string]any{
		"title":         "Neural Networks",
		"summarization": "A comprehensive study of neural networks and their applications",
	}
	_, err = client.CallTool(ctx, setReq)
	if err != nil {
		t.Fatal("Setup failed:", err)
	}

	var getReq mcp.CallToolRequest
	getReq.Params.Name = "get-research-paper"
	getReq.Params.Arguments = map[string]any{
		"title": "Neural Networks",
	}

	result, err := client.CallTool(ctx, getReq)
	if err != nil {
		t.Fatal("CallTool:", err)
	}

	got, err := resultToString(result)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "Found exact match for 'Neural Networks'") {
		t.Errorf("Expected exact match message, got: %s", got)
	}
	if !strings.Contains(got, "A comprehensive study of neural networks") {
		t.Errorf("Expected paper content in response, got: %s", got)
	}
}

func TestGetResearchPaperFuzzyMatch(t *testing.T) {
	ctx := context.Background()
	srv := createResearchPapersMCPServer(t)
	defer srv.Close()

	err := srv.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}

	client := srv.Client()

	var setReq mcp.CallToolRequest
	setReq.Params.Name = "set-new-research-paper"
	setReq.Params.Arguments = map[string]any{
		"title":         "Deep Learning",
		"summarization": "An introduction to deep learning techniques",
	}
	_, err = client.CallTool(ctx, setReq)
	if err != nil {
		t.Fatal("Setup failed:", err)
	}

	var getReq mcp.CallToolRequest
	getReq.Params.Name = "get-research-paper"
	getReq.Params.Arguments = map[string]any{
		"title": "Deep Leaning",
	}

	result, err := client.CallTool(ctx, getReq)
	if err != nil {
		t.Fatal("CallTool:", err)
	}

	got, err := resultToString(result)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "Found closest match 'Deep Learning'") {
		t.Errorf("Expected fuzzy match message, got: %s", got)
	}
	if !strings.Contains(got, "distance: 1") {
		t.Errorf("Expected distance information, got: %s", got)
	}
	if !strings.Contains(got, "An introduction to deep learning") {
		t.Errorf("Expected paper content in response, got: %s", got)
	}
}

func TestGetResearchPaperNotFound(t *testing.T) {
	ctx := context.Background()
	srv := createResearchPapersMCPServer(t)
	defer srv.Close()

	err := srv.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}

	client := srv.Client()

	var getReq mcp.CallToolRequest
	getReq.Params.Name = "get-research-paper"
	getReq.Params.Arguments = map[string]any{
		"title": "Nonexistent Paper",
	}

	result, err := client.CallTool(ctx, getReq)
	if err != nil {
		t.Fatal("CallTool:", err)
	}

	got, err := resultToString(result)
	if err != nil {
		t.Fatal(err)
	}

	expected := "No research paper found matching 'Nonexistent Paper'"
	if got != expected {
		t.Errorf("Got %q, want %q", got, expected)
	}
}

func TestGetResearchPaperFuzzyMatchBoundary(t *testing.T) {
	ctx := context.Background()
	srv := createResearchPapersMCPServer(t)
	defer srv.Close()

	err := srv.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}

	client := srv.Client()

	var setReq mcp.CallToolRequest
	setReq.Params.Name = "set-new-research-paper"
	setReq.Params.Arguments = map[string]any{
		"title":         "AI",
		"summarization": "Artificial Intelligence overview",
	}
	_, err = client.CallTool(ctx, setReq)
	if err != nil {
		t.Fatal("Setup failed:", err)
	}

	var getReq mcp.CallToolRequest
	getReq.Params.Name = "get-research-paper"
	getReq.Params.Arguments = map[string]any{
		"title": "AIMLNLP",
	}

	result, err := client.CallTool(ctx, getReq)
	if err != nil {
		t.Fatal("CallTool:", err)
	}

	got, err := resultToString(result)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "No research paper found matching 'AIMLNLP'") {
		t.Errorf("Expected no match for distance > 3, got: %s", got)
	}
}

func TestGetResearchPaperErrors(t *testing.T) {
	ctx := context.Background()
	srv := createResearchPapersMCPServer(t)
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
			name: "missing title",
			args: map[string]any{},
		},
		{
			name: "title is not string",
			args: map[string]any{
				"title": 123,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req mcp.CallToolRequest
			req.Params.Name = "get-research-paper"
			req.Params.Arguments = tt.args

			_, err := client.CallTool(ctx, req)
			if err == nil {
				t.Error("Expected error but got none")
			}
		})
	}
}

