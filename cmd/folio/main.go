package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/raaj6395/folio/internal/doctor"
	"github.com/raaj6395/folio/internal/layout"
	"github.com/raaj6395/folio/internal/mcpserver"
	"github.com/raaj6395/folio/internal/store"
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

func open() (*store.Store, *layout.Builder) {
	h := home()
	if err := os.MkdirAll(h, 0o755); err != nil {
		fatal(err)
	}
	st, err := store.Open(filepath.Join(h, "folio.db"))
	if err != nil {
		fatal(err)
	}
	lb, err := layout.New(h)
	if err != nil {
		fatal(err)
	}
	return st, lb
}

func fatal(err error) {
	slog.Error("fatal", "err", err)
	fmt.Fprintln(os.Stderr, "folio:", err)
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "mcp":
		setupLog(false)
		if _, err := doctor.TypstVersion(); err != nil {
			fatal(err)
		}
		st, lb := open()
		defer st.Close()
		if err := mcpserver.New(st, lb, home()).Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			fatal(err)
		}
	case "export":
		setupLog(false)
		fs := flag.NewFlagSet("export", flag.ExitOnError)
		out := fs.String("o", "", "output PDF path")
		if len(os.Args) < 3 {
			fatal(fmt.Errorf("usage: folio export <notebook-id> [-o file.pdf]"))
		}
		fs.Parse(os.Args[3:])
		st, lb := open()
		defer st.Close()
		path := *out
		if path != "" {
			path, _ = filepath.Abs(path)
		}
		p, err := mcpserver.ExportByID(context.Background(), st, lb, home(), os.Args[2], path)
		if err != nil {
			fatal(err)
		}
		fmt.Println(p)
	case "doctor":
		setupLog(false)
		h := home()
		ok := doctor.Run(os.Stdout, []doctor.Check{
			{Name: "typst", Run: doctor.TypstVersion},
			{Name: "data dir", Run: func() (string, error) { return doctor.Writable(h) }},
			{Name: "database", Run: func() (string, error) {
				st, err := store.Open(filepath.Join(h, "folio.db"))
				if err != nil {
					return "", err
				}
				defer st.Close()
				nbs, err := st.Notebooks()
				return fmt.Sprintf("%d notebook(s) in %s", len(nbs), filepath.Join(h, "folio.db")), err
			}},
			{Name: "fonts", Run: func() (string, error) {
				lb, err := layout.New(h)
				if err != nil {
					return "", err
				}
				return doctor.FontsPresent(lb.FontDir())
			}},
		})
		if !ok {
			os.Exit(1)
		}
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}
