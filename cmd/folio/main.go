package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/raaj6395/folio/internal/doctor"
	"github.com/raaj6395/folio/internal/mcpserver"
)

const usage = `usage: folio <command>

commands:
  mcp                         run the MCP server over stdio
  serve [--port 7420]         run the local web viewer
  export <notebook-id> [-o file.pdf]
  doctor                      check the environment
`

func home() string {
	if h := os.Getenv("FOLIO_HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".folio")
}

func setupLog(toStderr bool) {
	dir := filepath.Join(home(), "logs")
	os.MkdirAll(dir, 0o755)
	var w io.Writer = os.Stderr
	if f, err := os.OpenFile(filepath.Join(dir, "folio.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		w = f
		if toStderr {
			w = io.MultiWriter(f, os.Stderr)
		}
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(w, nil)))
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "mcp":
		setupLog(false)
		if err := mcpserver.New().Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			slog.Error("mcp server exited", "err", err)
			os.Exit(1)
		}
	case "doctor":
		ok := doctor.Run(os.Stdout, []doctor.Check{
			{Name: "typst", Run: doctor.TypstVersion},
			{Name: "data dir", Run: func() (string, error) { return doctor.Writable(home()) }},
		})
		if !ok {
			os.Exit(1)
		}
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}
