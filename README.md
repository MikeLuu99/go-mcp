# Go MCP

A collection of Model Context Protocol (MCP) servers implemented in Go for memory management and research paper storage.

## MCP Servers

### 1. Memory MCP Server
A vector-based memory storage system that provides semantic search capabilities.

**Features:**
- Add/update memories with unique IDs
- Semantic search using vector similarity
- Metadata support for enhanced context
- Upstash Vector integration

**Tools:**
- `add-to-memory`: Store or update memory content
- `search-memory`: Find memories using semantic similarity
- `get-memory`: Retrieve specific memory by ID

### 2. Research Papers MCP Server
A Redis-based system for storing and retrieving research papers with fuzzy matching.

**Features:**
- Store research papers with titles and summaries
- Exact title matching
- Fuzzy matching with edit distance for approximate searches
- Redis backend for reliable storage

**Tools:**
- `set-new-research-paper`: Add new research paper
- `get-research-paper`: Retrieve paper with fuzzy matching support

## Setup

1. Install dependencies:
```bash
go mod tidy
```

2. Create a `.env` file with required environment variables:
```env
# For Memory MCP
VECTOR_DB_URL=your_upstash_vector_url
TOKEN=your_upstash_token

# For Research Papers MCP
REDIS_URL=your_redis_url
```

## Running the Servers

### Memory MCP Server
```bash
go run cmd/memory-mcp/main.go
```
Server runs on port 9090

### Research Papers MCP Server
```bash
go run cmd/research-papers-mcp/main.go
```
Server runs on port 8080

## API Endpoints

Both servers expose SSE (Server-Sent Events) endpoints:
- SSE Endpoint: `/mcp/sse`
- Message Endpoint: `/mcp/message`

## Testing

The project includes comprehensive test suites for both MCP servers located in the `test/` directory.

### Running Tests

Run all tests:
```bash
go test ./test/... -v
```

Run tests for specific server:
```bash
# Memory MCP tests
go test ./test/memory_mcp_test.go -v

# Research Papers MCP tests  
go test ./test/research_papers_mcp_test.go -v
```

### Test Coverage

**Memory MCP Server Tests:**
- `add-to-memory` tool: Storage with/without metadata, error handling
- `search-memory` tool: Semantic search functionality, no results scenarios
- `get-memory` tool: Memory retrieval by ID, not found scenarios

**Research Papers MCP Server Tests:**
- `set-new-research-paper` tool: Paper storage, error handling
- `get-research-paper` tool: Exact matching, fuzzy matching with Levenshtein distance, boundary conditions

Both test suites use mock implementations to avoid external dependencies during testing.

## Dependencies

- [mcp-go](https://github.com/mark3labs/mcp-go) - Go MCP implementation
- [upstash/vector-go](https://github.com/upstash/vector-go) - Vector database client
- [redis/go-redis](https://github.com/redis/go-redis) - Redis client
- [godotenv](https://github.com/joho/godotenv) - Environment variable loading
- [levenshtein](https://github.com/agnivade/levenshtein) - Edit distance calculation
