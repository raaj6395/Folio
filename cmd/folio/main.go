package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/raaj6395/folio/internal/doctor"
	"github.com/raaj6395/folio/internal/httpserver"
	"github.com/raaj6395/folio/internal/layout"
	"github.com/raaj6395/folio/internal/mcpserver"
	"github.com/raaj6395/folio/internal/store"
	"github.com/raaj6395/folio/internal/viewer"
)

const usage = `usage: folio <command>

commands:
  mcp                         run the MCP server over stdio
  serve [--port 7420]         run the local web viewer
  http [--unsafe-public]      serve MCP over HTTP (/mcp) plus the viewer; address from FOLIO_HTTP_ADDR
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

func dsn() string {
	if u := os.Getenv("FOLIO_DB_URL"); u != "" {
		return u
	}
	return filepath.Join(home(), "folio.db")
}

func describeDSN(d string) string {
	if u, err := url.Parse(d); err == nil && store.IsPostgres(d) {
		return "postgres " + u.Redacted()
	}
	return "sqlite " + d
}

func open() (*store.Store, *layout.Builder) {
	h := home()
	if err := os.MkdirAll(h, 0o755); err != nil {
		fatal(err)
	}
	st, err := store.Open(dsn())
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
		if err := mcpserver.New(st, lb, home(), "").Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			fatal(err)
		}
	case "serve":
		setupLog(true)
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		port := fs.Int("port", viewer.DefaultPort, "port to listen on (bound to 127.0.0.1)")
		fs.Parse(os.Args[2:])
		st, lb := open()
		defer st.Close()
		lb.Root = "build-viewer"
		addr := fmt.Sprintf("127.0.0.1:%d", *port)
		fmt.Fprintf(os.Stderr, "Folio viewer on http://%s\n", addr)
		if err := http.ListenAndServe(addr, viewer.New(st, lb)); err != nil {
			fatal(err)
		}
	case "http":
		setupLog(true)
		fs := flag.NewFlagSet("http", flag.ExitOnError)
		unsafe := fs.Bool("unsafe-public", false, "allow a non-loopback listen address (there is NO authentication yet)")
		fs.Parse(os.Args[2:])
		addr := os.Getenv("FOLIO_HTTP_ADDR")
		if addr == "" {
			addr = "127.0.0.1:8080"
		}
		if !httpserver.IsLoopback(addr) {
			if !*unsafe {
				fatal(fmt.Errorf("refusing to listen on %s: Folio has no authentication yet, so only loopback addresses (127.0.0.1, ::1, localhost) are allowed; pass --unsafe-public to override", addr))
			}
			fmt.Fprintf(os.Stderr, "WARNING: listening on %s with NO authentication. Anyone who can reach this address can read and change every notebook.\n", addr)
		}
		public := os.Getenv("FOLIO_PUBLIC_URL")
		if public == "" {
			public = "http://" + addr
		}
		if _, err := doctor.TypstVersion(); err != nil {
			fatal(err)
		}
		st, lb := open()
		defer st.Close()
		srv := &http.Server{Addr: addr, Handler: httpserver.Handler(st, lb, home(), public)}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		go func() {
			<-ctx.Done()
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			srv.Shutdown(shutdown)
		}()
		fmt.Fprintf(os.Stderr, "Folio HTTP on %s (MCP: %s/mcp, viewer: %s/, storage: %s)\n", addr, public, public, describeDSN(dsn()))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fatal(err)
		}
		slog.Info("folio http stopped")
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
				st, err := store.Open(dsn())
				if err != nil {
					return "", fmt.Errorf("%s: %w", describeDSN(dsn()), err)
				}
				defer st.Close()
				nbs, err := st.Notebooks()
				return fmt.Sprintf("%d notebook(s) in %s", len(nbs), describeDSN(dsn())), err
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
