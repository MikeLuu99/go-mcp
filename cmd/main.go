package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/agnivade/levenshtein"
	"github.com/joho/godotenv"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		fmt.Println("Error loading .env file")
	}

	s := server.NewMCPServer("research-papers-memory", "1.0.0", server.WithToolCapabilities(true))

	// Add resource with its handler
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

	s.AddTool(setNewResearchPaper, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {

		opt, _ := redis.ParseURL("REDIS_URL")
		client := redis.NewClient(opt)
		args := request.GetArguments()

		title, ok := args["title"].(string)
		if !ok {
			return nil, fmt.Errorf("argument 'title' is missing or not a string")
		}

		summarization, ok := args["summarization"].(string)
		// if !ok {
		// 	// This argument is not required by the tool definition, so we might allow it to be empty or handle it differently
		// 	// For now, let's assume it should be a string if present.
		// 	programmingLanguageNewKnowledge = "" // Default to empty string if not provided or not a string
		// }

		setErr := client.Set(ctx, title, summarization, 0).Err()
		if setErr != nil {
			fmt.Println(setErr)
			return nil, setErr
		}
		return mcp.NewToolResultText("Successful update of the knowledge base"), nil
	})

	s.AddTool(getResearchPaper, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		opt, _ := redis.ParseURL("REDIS_URL")
		client := redis.NewClient(opt)
		args := request.GetArguments()

		title, ok := args["title"].(string)
		if !ok {
			return nil, fmt.Errorf("argument 'title' is missing or not a string")
		}

		// First try exact match
		val, err := client.Get(ctx, title).Result()
		if err == nil {
			return mcp.NewToolResultText(fmt.Sprintf("Found exact match for '%s': %s", title, val)), nil
		}

		// If exact match fails, try fuzzy matching
		var bestMatch string
		var bestValue string
		var bestDistance int = 999999
		const maxDistance = 3 // Maximum acceptable edit distance

		// Use SCAN to iterate through all keys
		iter := client.Scan(ctx, 0, "*", 0).Iterator()
		for iter.Next(ctx) {
			key := iter.Val()
			distance := levenshtein.ComputeDistance(strings.ToLower(title), strings.ToLower(key))

			if distance <= maxDistance && distance < bestDistance {
				bestDistance = distance
				bestMatch = key
			}
		}

		if err := iter.Err(); err != nil {
			return nil, fmt.Errorf("error scanning keys: %v", err)
		}

		if bestMatch == "" {
			return mcp.NewToolResultText(fmt.Sprintf("No research paper found matching '%s'", title)), nil
		}

		// Get the content of the best match
		bestValue, err = client.Get(ctx, bestMatch).Result()
		if err != nil {
			return nil, fmt.Errorf("error retrieving content for key '%s': %v", bestMatch, err)
		}

		return mcp.NewToolResultText(fmt.Sprintf("Found closest match '%s' (distance: %d): %s", bestMatch, bestDistance, bestValue)), nil
	})

	// Start the server
	// if err := server.ServeStdio(s); err != nil {
	// 	fmt.Printf("Server error: %v\n", err)
	// }
	port := 8080
	fmt.Printf("Starting SSE Server on port: %d\n", port)
	sseServer := server.NewSSEServer(
		s,
		server.WithStaticBasePath("/"),
		server.WithSSEEndpoint("/mcp/sse"),
		server.WithMessageEndpoint("/mcp/message"),
	)

	mux := http.NewServeMux()

	mux.Handle("/", sseServer)
	// Create an HTTP server
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	// Print available endpoints
	fmt.Printf("SSE Endpoint: %s\n", sseServer.CompleteSsePath())
	fmt.Printf("Message Endpoint: %s\n", sseServer.CompleteMessagePath())

	// Start the server
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v\n", err)
	}
}
