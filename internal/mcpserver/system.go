package mcpserver

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/sim"
	"github.com/raaj6395/folio/internal/store"
)

//go:embed system_guide.md
var systemGuide string

// checkFormat keeps system blocks and system notebooks together: a system notebook holds exactly
// one system block (or none while it is being created), and a system block lives nowhere else.
// It runs on every write, so it also stops set_theme switching a notebook in or out of "system".
func checkFormat(nb *store.Notebook, bs []model.Block) error {
	systems := 0
	for _, b := range bs {
		if b.Type == "system" {
			systems++
		}
	}
	if nb.Format == "system" {
		if len(bs) > 1 || len(bs) == 1 && systems == 0 {
			return fmt.Errorf(`a system notebook holds exactly one block, of type "system"; to change the system, update_block that block with the whole new spec`)
		}
		return nil
	}
	if systems > 0 {
		return fmt.Errorf(`a "system" block only goes in a system notebook: create_notebook with format "system"`)
	}
	return nil
}

// systemSummary is the write result's description of a system notebook.
func systemSummary(bs []model.Block) (*sim.Summary, error) {
	if len(bs) == 0 {
		return nil, nil
	}
	sp, _ := bs[0].SystemSpec()
	return sim.Validate(sp)
}

type simulateIn struct {
	NotebookID string   `json:"notebook_id"`
	Request    string   `json:"request" jsonschema:"Request id from the spec's requests"`
	Key        string   `json:"key,omitempty" jsonschema:"One of the request's keys; defaults to its first"`
	Kill       []string `json:"kill,omitempty" jsonschema:"Node ids or group names that are down for this run"`
	Warm       bool     `json:"warm,omitempty" jsonschema:"Send the request once first, so caches are warm"`
}

type SimulateOut struct {
	Status  int      `json:"status"`
	TotalMS int      `json:"total_ms"`
	Visited []string `json:"visited"`
	Log     []string `json:"log"`
	Waiting []string `json:"queued_messages_waiting,omitempty"`
}

func (s *Server) registerSystem(m *mcp.Server) {
	addTool(s, m, "simulate_system", "Send one request through a system notebook (optionally with nodes killed) and get the status, latency and step-by-step log. Use it to check a design and to answer \"what if X is down\". Read-only.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in simulateIn) (*mcp.CallToolResult, SimulateOut, error) {
			nb, bs, err := s.load(ctx, in.NotebookID)
			if err != nil {
				return nil, SimulateOut{}, err
			}
			if nb.Format != "system" || len(bs) == 0 {
				return nil, SimulateOut{}, fmt.Errorf("notebook %s is not a system notebook with a system block", nb.ID)
			}
			sp, _ := bs[0].SystemSpec()
			res, err := sim.RunScenario(sp, sim.Scenario{Request: in.Request, Key: in.Key, Kill: in.Kill, Warm: in.Warm})
			if err != nil {
				return nil, SimulateOut{}, err
			}
			out := SimulateOut{Status: res.Status, TotalMS: res.TotalMS, Log: sim.Log(res), Visited: []string{}}
			for _, n := range sp.Nodes {
				if sim.Visited(res.Events)[n.ID] {
					out.Visited = append(out.Visited, n.ID)
				}
			}
			for q, keys := range res.State.Backlog {
				out.Waiting = append(out.Waiting, fmt.Sprintf("%s: %s", q, strings.Join(keys, ", ")))
			}
			return nil, out, nil
		})
}
