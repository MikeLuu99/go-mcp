package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/upstash/vector-go"
)

var ctx = context.Background()

func main() {
	err := godotenv.Load(".env")
	VECTOR_DB_URL := os.Getenv("VECTOR_DB_URL")
	TOKEN := os.Getenv("TOKEN")
	if err != nil {
		fmt.Println("Error loading .env file:", err)
		return
	}

	s := server.NewMCPServer("memory-mcp", "1.0.0", server.WithToolCapabilities(true))

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

	opts := vector.Options{
		Url:   VECTOR_DB_URL,
		Token: TOKEN,
	}

	index := vector.NewIndexWith(opts)

	s.AddTool(addToMemory, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		err := index.UpsertData(vector.UpsertData{
			Id:   id,
			Data: data,
		})

		if err != nil {
			return nil, fmt.Errorf("error storing memory: %v", err)
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully stored memory with ID: %s", id)), nil
	})

	s.AddTool(searchMemory, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		scores, err := index.QueryData(vector.QueryData{
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

	s.AddTool(getMemory, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		id, ok := args["id"].(string)
		if !ok {
			return nil, fmt.Errorf("argument 'id' is missing or not a string")
		}

		scores, err := index.QueryData(vector.QueryData{
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

	port := 9090
	fmt.Printf("Starting SSE Server on port: %d\n", port)
	sseServer := server.NewSSEServer(
		s,
		server.WithStaticBasePath("/"),
		server.WithSSEEndpoint("/mcp/sse"),
		server.WithMessageEndpoint("/mcp/message"),
	)

	mux := http.NewServeMux()

	mux.Handle("/", sseServer)
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	fmt.Printf("SSE Endpoint: %s\n", sseServer.CompleteSsePath())
	fmt.Printf("Message Endpoint: %s\n", sseServer.CompleteMessagePath())

	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v\n", err)
	}
}
