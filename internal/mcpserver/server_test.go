package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPing(t *testing.T) {
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := New().Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "ping"})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || res.Content[0].(*mcp.TextContent).Text != `{"message":"pong"}` {
		t.Fatalf("unexpected result: %+v", res.Content[0])
	}
}
