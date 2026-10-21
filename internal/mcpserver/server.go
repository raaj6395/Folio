package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const instructions = `Folio typesets notes onto A4 pages. You write structured blocks; Folio lays them out. Never think in coordinates.`

type pingIn struct{}

type pingOut struct {
	Message string `json:"message"`
}

func New() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "folio", Version: "0.1.0"}, &mcp.ServerOptions{Instructions: instructions})
	mcp.AddTool(s, &mcp.Tool{Name: "ping", Description: "Health check. Returns pong."},
		func(ctx context.Context, req *mcp.CallToolRequest, in pingIn) (*mcp.CallToolResult, pingOut, error) {
			return nil, pingOut{Message: "pong"}, nil
		})
	return s
}
