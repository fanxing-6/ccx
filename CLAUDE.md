# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

CCX (Claude Code eXecutor) is a Go CLI tool that manages multiple Claude Code configurations and switches between them. Configurations are stored in Gitee Gist (cloud), with only connection info saved locally at `~/.config/ccx/config.json`.

## Build Commands

```bash
# Build binary
go build -o ccx .

# Run all tests
go test ./...

# Run specific test
go test ./internal/proxy -run TestName

# Build with version (as used in releases)
go build -ldflags "-X ccx/cmd.Version=x.x.x" -o ccx .
```

## Architecture

### Package Structure

- `cmd/` - Cobra CLI commands
  - `root.go` - Main entry, profile selection/launch logic
  - `manage.go` - `add`, `edit`, `remove` commands
  - `init_cmd.go`, `reset.go` - Initialization and reset
  - `passthrough.go` - Commands forwarded to `claude` CLI

- `internal/` - Core logic
  - `config.go` - Local config (`AppConfig`), profile types, config paths
  - `gitee.go` - Gitee Gist API client (CRUD operations)
  - `tui.go` - Interactive prompts using `promptui`

- `internal/proxy/` - OpenAI-to-Anthropic translation layer
  - `server.go` - HTTP proxy server, request routing
  - `translate_request.go` - Anthropic → OpenAI `/responses` format
  - `translate_response.go` - OpenAI SSE/JSON → Anthropic format
  - `profile_thinking.go` - `OPENAI_REASONING_EFFORT` handling

### Key Design Patterns

**Two API Modes:**
- `anthropic`: Direct launch, settings passed to `claude --settings`
- `openai`: Local proxy started (127.0.0.1:random), translates Anthropic protocol to OpenAI `/v1/responses`

**Profile Storage:**
- Profiles stored as `settings-<name>.json` files in Gitee Gist
- Local config only stores Gitee token, gist ID/owner, default profile, claude command

**Passthrough Commands:**
Commands like `auth`, `update`, `mcp`, `agents` are forwarded directly to `claude` CLI if they don't match ccx commands.

**Proxy Translation:**
The proxy layer converts between Anthropic Messages API and OpenAI Responses API:
- Claude Code sends Anthropic format to local proxy
- Proxy converts to OpenAI Responses format and forwards to upstream
- Response converted back to Anthropic format via SSE streaming

### Dependencies

Key external packages:
- `github.com/spf13/cobra` - CLI framework
- `github.com/manifoldco/promptui` - Interactive prompts
- `github.com/tidwall/gjson/sjson` - JSON manipulation without structs

## Testing Notes

- Tests are in `_test.go` files alongside source
- Proxy translation tests verify JSON format conversions
- TUI tests use fallback mode (non-TTY) for CI compatibility
