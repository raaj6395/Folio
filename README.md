# Folio

Folio lets an AI model (Claude Code or any MCP client) write notes on A4 pages. The AI writes structured blocks, and Folio lays them out with Typst.

## Setup

Requirements: Go 1.25+, and the [Typst CLI](https://github.com/typst/typst) on your PATH (macOS: `brew install typst`).

```sh
make build        # produces bin/folio
bin/folio doctor  # every line should say PASS
```

Data lives in `~/.folio/` by default. Set `FOLIO_HOME` to use another directory.

## Register with Claude Code

```sh
claude mcp add -s user folio -- "$PWD/bin/folio" mcp
claude mcp list   # folio should show "Connected"
```

Check that it works:

```sh
claude -p "Call the mcp__folio__ping tool and reply with what it returned." --allowedTools mcp__folio__ping
```
