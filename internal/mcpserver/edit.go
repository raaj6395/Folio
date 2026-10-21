package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/raaj6395/folio/internal/model"
	"github.com/raaj6395/folio/internal/store"
)

type insertIn struct {
	NotebookID      string            `json:"notebook_id" jsonschema:"ID of the notebook"`
	AfterBlockID    string            `json:"after_block_id" jsonschema:"Insert after this block ID, or \"start\" to insert at the beginning"`
	Blocks          []json.RawMessage `json:"blocks" jsonschema:"Blocks to insert, in order (max 50)"`
	ExpectedVersion *int              `json:"expected_version,omitempty" jsonschema:"Reject the write unless the notebook is at this version"`
}

type updateIn struct {
	NotebookID      string          `json:"notebook_id" jsonschema:"ID of the notebook"`
	BlockID         string          `json:"block_id" jsonschema:"ID of the block to replace"`
	Block           json.RawMessage `json:"block" jsonschema:"The full new block (type and all fields). Replaces the old content; the type may change. The ID stays the same"`
	ExpectedVersion *int            `json:"expected_version,omitempty" jsonschema:"Reject the write unless the notebook is at this version"`
}

type deleteIn struct {
	NotebookID      string   `json:"notebook_id" jsonschema:"ID of the notebook"`
	BlockIDs        []string `json:"block_ids" jsonschema:"IDs of the blocks to delete"`
	ExpectedVersion *int     `json:"expected_version,omitempty" jsonschema:"Reject the write unless the notebook is at this version"`
}

type moveIn struct {
	NotebookID      string   `json:"notebook_id" jsonschema:"ID of the notebook"`
	BlockIDs        []string `json:"block_ids" jsonschema:"IDs of the blocks to move; they keep their current relative order"`
	AfterBlockID    string   `json:"after_block_id" jsonschema:"Move them to just after this block ID, or \"start\""`
	ExpectedVersion *int     `json:"expected_version,omitempty" jsonschema:"Reject the write unless the notebook is at this version"`
}

type readIn struct {
	NotebookID  string   `json:"notebook_id" jsonschema:"ID of the notebook"`
	BlockIDs    []string `json:"block_ids,omitempty" jsonschema:"Read these blocks. Use exactly one of block_ids, from_block_id (+count) or page"`
	FromBlockID string   `json:"from_block_id,omitempty" jsonschema:"Read starting at this block"`
	Count       int      `json:"count,omitempty" jsonschema:"With from_block_id: how many blocks to read (default 10)"`
	Page        int      `json:"page,omitempty" jsonschema:"Read every block that starts on this 1-indexed page"`
}

type ReadBlock struct {
	Page  int         `json:"page"`
	Block model.Block `json:"block"`
}

type versionsIn struct {
	NotebookID string `json:"notebook_id" jsonschema:"ID of the notebook"`
	Limit      int    `json:"limit,omitempty" jsonschema:"How many versions to list, newest first (default 20)"`
}

type undoIn struct {
	NotebookID      string `json:"notebook_id" jsonschema:"ID of the notebook"`
	ExpectedVersion *int   `json:"expected_version,omitempty" jsonschema:"Reject unless the notebook is at this version"`
}

type restoreIn struct {
	NotebookID      string `json:"notebook_id" jsonschema:"ID of the notebook"`
	Version         int    `json:"version" jsonschema:"Version to restore, from list_versions"`
	ExpectedVersion *int   `json:"expected_version,omitempty" jsonschema:"Reject unless the notebook is at this version"`
}

func indexOf(bs []model.Block, id string) int {
	return slices.IndexFunc(bs, func(b model.Block) bool { return b.ID == id })
}

func afterIndex(bs []model.Block, after string) (int, error) {
	if after == "start" {
		return 0, nil
	}
	i := indexOf(bs, after)
	if i < 0 {
		return 0, fmt.Errorf("after_block_id %q not found in this notebook; use a block ID from get_outline, or \"start\"", after)
	}
	return i + 1, nil
}

func missing(bs []model.Block, ids []string) error {
	var miss []string
	for _, id := range ids {
		if indexOf(bs, id) < 0 {
			miss = append(miss, id)
		}
	}
	if len(miss) > 0 {
		return fmt.Errorf("block(s) not found: %s; use IDs from get_outline", strings.Join(miss, ", "))
	}
	return nil
}

func (s *Server) registerEdit(m *mcp.Server) {
	addTool(m, "insert_blocks", "Insert blocks after a given block (or at the start). Validated as a whole; nothing is written if any block is invalid.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in insertIn) (*mcp.CallToolResult, MutationResult, error) {
			add, err := parseBlocks(ctx, in.Blocks)
			if err != nil {
				return nil, MutationResult{}, err
			}
			res, err := s.mutate(ctx, in.NotebookID, in.ExpectedVersion, "insert_blocks", fmt.Sprintf("inserted %d block(s) after %s", len(add), in.AfterBlockID),
				func(nb *store.Notebook, bs []model.Block) ([]model.Block, []string, error) {
					i, err := afterIndex(bs, in.AfterBlockID)
					if err != nil {
						return nil, nil, err
					}
					ids := newIDs(add)
					return model.InsertAt(bs, i, add), ids, nil
				})
			return nil, res, err
		})

	addTool(m, "update_block", "Replace one block's content. Send the complete new block; the ID and position stay the same.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in updateIn) (*mcp.CallToolResult, MutationResult, error) {
			upd, err := model.Parse(in.Block)
			if err != nil {
				if upd.Type == "" {
					return nil, MutationResult{}, fmt.Errorf("block: %v", err)
				}
				return nil, MutationResult{}, fmt.Errorf("block (%s): %v", upd.Type, err)
			}
			if err := checkDiagram(ctx, upd); err != nil {
				return nil, MutationResult{}, fmt.Errorf("block (diagram): %v", err)
			}
			res, err := s.mutate(ctx, in.NotebookID, in.ExpectedVersion, "update_block", "updated "+in.BlockID,
				func(_ *store.Notebook, bs []model.Block) ([]model.Block, []string, error) {
					i := indexOf(bs, in.BlockID)
					if i < 0 {
						return nil, nil, missing(bs, []string{in.BlockID})
					}
					bs[i].Type, bs[i].Data = upd.Type, upd.Data
					return bs, []string{in.BlockID}, nil
				})
			return nil, res, err
		})

	addTool(m, "delete_blocks", "Delete blocks by ID.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in deleteIn) (*mcp.CallToolResult, MutationResult, error) {
			if len(in.BlockIDs) == 0 {
				return nil, MutationResult{}, fmt.Errorf("block_ids must not be empty")
			}
			res, err := s.mutate(ctx, in.NotebookID, in.ExpectedVersion, "delete_blocks", fmt.Sprintf("deleted %d block(s)", len(in.BlockIDs)),
				func(_ *store.Notebook, bs []model.Block) ([]model.Block, []string, error) {
					if err := missing(bs, in.BlockIDs); err != nil {
						return nil, nil, err
					}
					return slices.DeleteFunc(bs, func(b model.Block) bool { return slices.Contains(in.BlockIDs, b.ID) }), nil, nil
				})
			return nil, res, err
		})

	addTool(m, "move_blocks", "Move blocks to just after another block (or to the start). Moved blocks keep their relative order.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in moveIn) (*mcp.CallToolResult, MutationResult, error) {
			if len(in.BlockIDs) == 0 {
				return nil, MutationResult{}, fmt.Errorf("block_ids must not be empty")
			}
			if slices.Contains(in.BlockIDs, in.AfterBlockID) {
				return nil, MutationResult{}, fmt.Errorf("after_block_id %s is one of the blocks being moved", in.AfterBlockID)
			}
			res, err := s.mutate(ctx, in.NotebookID, in.ExpectedVersion, "move_blocks", fmt.Sprintf("moved %d block(s) after %s", len(in.BlockIDs), in.AfterBlockID),
				func(_ *store.Notebook, bs []model.Block) ([]model.Block, []string, error) {
					if err := missing(bs, in.BlockIDs); err != nil {
						return nil, nil, err
					}
					var moved, rest []model.Block
					var ids []string
					for _, b := range bs {
						if slices.Contains(in.BlockIDs, b.ID) {
							moved = append(moved, b)
							ids = append(ids, b.ID)
						} else {
							rest = append(rest, b)
						}
					}
					i, err := afterIndex(rest, in.AfterBlockID)
					if err != nil {
						return nil, nil, err
					}
					return model.InsertAt(rest, i, moved), ids, nil
				})
			return nil, res, err
		})

	addTool(m, "read_blocks", "Read full block content (to edit it). Select by block_ids, by from_block_id + count, or by page.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in readIn) (*mcp.CallToolResult, map[string][]ReadBlock, error) {
			n := 0
			for _, set := range []bool{len(in.BlockIDs) > 0, in.FromBlockID != "", in.Page > 0} {
				if set {
					n++
				}
			}
			if n != 1 {
				return nil, nil, fmt.Errorf("use exactly one of block_ids, from_block_id or page")
			}
			nb, bs, err := s.load(in.NotebookID)
			if err != nil {
				return nil, nil, err
			}
			r, err := s.lb.Build(ctx, nb, bs)
			if err != nil {
				return nil, nil, err
			}
			var sel []model.Block
			switch {
			case len(in.BlockIDs) > 0:
				if err := missing(bs, in.BlockIDs); err != nil {
					return nil, nil, err
				}
				for _, id := range in.BlockIDs {
					sel = append(sel, bs[indexOf(bs, id)])
				}
			case in.FromBlockID != "":
				i := indexOf(bs, in.FromBlockID)
				if i < 0 {
					return nil, nil, missing(bs, []string{in.FromBlockID})
				}
				if in.Count <= 0 {
					in.Count = 10
				}
				sel = bs[i:min(len(bs), i+in.Count)]
			default:
				if in.Page > r.PageCount {
					return nil, nil, fmt.Errorf("page %d does not exist; the notebook has %d page(s)", in.Page, r.PageCount)
				}
				for _, b := range bs {
					if r.Page(b.ID) == in.Page {
						sel = append(sel, b)
					}
				}
			}
			out := []ReadBlock{}
			for _, b := range sel {
				out = append(out, ReadBlock{Page: r.Page(b.ID), Block: b})
			}
			return nil, map[string][]ReadBlock{"blocks": out}, nil
		})

	addTool(m, "list_versions", "List recent versions (snapshots) of a notebook, newest first.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in versionsIn) (*mcp.CallToolResult, map[string][]store.Snapshot, error) {
			if _, err := s.st.Notebook(in.NotebookID); err != nil {
				return nil, nil, err
			}
			if in.Limit <= 0 {
				in.Limit = 20
			}
			vs, err := s.st.Versions(in.NotebookID, in.Limit)
			return nil, map[string][]store.Snapshot{"versions": vs}, err
		})

	addTool(m, "undo", "Undo the most recent change. Call again to step further back.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in undoIn) (*mcp.CallToolResult, MutationResult, error) {
			nb, bs, err := s.st.Undo(in.NotebookID, in.ExpectedVersion)
			if err != nil {
				return nil, MutationResult{}, err
			}
			return nil, s.result(ctx, nb, bs, nil), nil
		})

	addTool(m, "restore_version", "Restore the notebook to an earlier version from list_versions. This is itself a new version, so it can be undone.",
		func(ctx context.Context, _ *mcp.CallToolRequest, in restoreIn) (*mcp.CallToolResult, MutationResult, error) {
			nb, bs, err := s.st.Restore(in.NotebookID, in.Version, in.ExpectedVersion)
			if err != nil {
				return nil, MutationResult{}, err
			}
			return nil, s.result(ctx, nb, bs, nil), nil
		})
}

