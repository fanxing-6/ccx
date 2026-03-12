# AGENTS.md

## Purpose
- This repository contains `ccx`, a Go CLI for managing multiple Claude Code profiles.
- Profiles are stored in Gitee Gist; local state only keeps connection metadata in `~/.config/ccx/config.json`.
- Agents should prefer small, targeted changes that preserve the existing CLI behavior.
- Follow repository conventions first; do not introduce broad refactors unless the task requires them.

## Sources Reviewed
- `CLAUDE.md` in the repository root contains the primary project-specific guidance.
- No Cursor rules were found in `.cursor/rules/`.
- No `.cursorrules` file was found.
- No Copilot instructions were found at `.github/copilot-instructions.md`.

## Project Summary
- Entry point: `main.go` calls `cmd.Execute()`.
- CLI commands live in `cmd/` and are built with Cobra.
- Shared config and Gitee Gist logic live in `internal/`.
- The OpenAI-to-Anthropic translation proxy lives in `internal/proxy/`.
- The two supported API modes are `anthropic` and `openai`.
- In `openai` mode, `ccx` starts a local proxy and translates Claude Messages API traffic to OpenAI `POST /v1/responses`.

## Key Paths
- `main.go`: program entry point.
- `cmd/root.go`: root command, profile selection, Claude launch logic.
- `cmd/manage.go`: `add`, `edit`, `remove`, config menu, editor flow.
- `cmd/passthrough.go`: passthrough command detection and execution.
- `cmd/model_select.go`: model discovery, pagination, filtering, manual fallback.
- `internal/config.go`: local config model and config-path helpers.
- `internal/gitee.go`: Gitee Gist API client and profile CRUD.
- `internal/proxy/server.go`: local proxy server, request routing, token masking.
- `internal/proxy/translate_request.go`: Anthropic -> OpenAI request conversion.
- `internal/proxy/translate_response.go`: OpenAI -> Anthropic response conversion.

## Build, Test, and Dev Commands
- Build the CLI locally: `go build -o ccx .`
- Build with an injected version string: `go build -ldflags "-X ccx/cmd.Version=x.x.x" -o ccx .`
- Match the release-style build more closely: `CGO_ENABLED=0 go build -ldflags "-s -w -X ccx/cmd.Version=x.x.x" -o ccx .`
- Run the full test suite: `go test ./...`
- Run tests for one package: `go test ./cmd` or `go test ./internal/proxy`
- Run one named test in a package: `go test ./internal/proxy -run TestApplyProfileThinking`
- Another single-test example: `go test ./cmd -run TestModelEndpointCandidates`
- Run a single test anywhere in the repo: `go test ./... -run 'TestName$'`
- Disable test caching when iterating: `go test ./internal/proxy -run TestName -count=1`
- Run a specific translation test: `go test ./internal/proxy -run TestConvertClaudeRequestToResponses_AlignedShape -count=1`
- Run a specific CLI test: `go test ./cmd -run TestFetchModelsFallbackAndHeaders -count=1`
- There is no repo-owned lint config such as `golangci-lint` in this codebase.
- The practical validation loop is: `gofmt` on touched files, then `go test ./...`.

## Formatting and Linting
- Always run `gofmt` on changed Go files.
- Example: `gofmt -w main.go cmd/*.go internal/*.go internal/proxy/*.go`
- Do not reformat unrelated files just to normalize style.
- Import ordering is not enforced by a dedicated tool here; preserve the touched file's local style and keep it `gofmt`-valid.
- Avoid introducing `goimports`-only churn unless the task explicitly includes import cleanup.
- Keep markdown and JSON formatting stable and minimal.

## General Coding Style
- Prefer simple functions and direct control flow over abstraction-heavy designs.
- Keep changes local to the package that owns the behavior.
- Match the surrounding file's structure before inventing a new pattern.
- Preserve the current package split: CLI in `cmd`, shared domain logic in `internal`, protocol translation in `internal/proxy`.

## Imports and File Layout
- Use standard Go naming for files and packages.
- Keep package names short and lowercase: `cmd`, `internal`, `proxy`.
- New command implementations should stay in `cmd/`.
- Shared non-command logic should stay in `internal/`.
- Keep exported declarations documented when a comment improves discovery or clarifies intent.

## Types and Data Modeling
- Use structs for durable state and clearly defined payloads, as with `AppConfig`, `Profile`, and `ProfileInfo`.
- Use `json.RawMessage` when raw JSON must be preserved and forwarded without full decoding.
- Use `map[string]any` or `map[string]string` only for small, ad hoc JSON assembly.
- In the proxy translators, prefer `gjson` and `sjson` for targeted JSON transformations instead of large intermediary structs.
- Keep configuration keys and wire-format field names exact; this code talks to external APIs.
- Do not rename env keys such as `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, or `OPENAI_REASONING_EFFORT`.

## Naming Conventions
- Exported Go identifiers use PascalCase.
- Unexported identifiers use lowerCamelCase.
- Cobra command variables follow the existing `addCmd`, `editCmd`, `removeCmd` pattern.
- Constants tend to use lowerCamelCase with a domain prefix, for example `modelFetchTimeout` or `defaultGiteeTimeout`.
- Prefer names that describe the domain behavior, not generic utility wording.

## Error Handling
- Follow a fail-fast style.
- Return errors instead of swallowing them.
- Wrap errors with context using `fmt.Errorf(...: %w)` when the caller needs more detail.
- Use direct, user-facing Chinese error messages for CLI failures because the rest of the CLI already does this.
- Do not add broad recovery layers, silent fallbacks, or defensive wrappers that hide failures.
- Only keep fallbacks that are already part of the product behavior, such as manual model input when automatic model discovery fails.
- Validate critical invariants, but do not over-engineer validation beyond what the command or protocol needs.

## CLI and UX Conventions
- User-visible CLI text is primarily Chinese in this repository; keep new prompts and errors consistent.
- Cobra handlers should usually use `RunE` so errors propagate naturally.
- Destructive flows should ask for confirmation when the current UX already expects it, such as profile deletion.
- Preserve both TTY and non-TTY interaction paths.
- When adding menu items, keep the current selection/fallback behavior intact.
- Keep dangerous-mode handling aligned with existing `-d` and `--dangerous` semantics.

## HTTP and External API Conventions
- Create explicit `http.Client` instances with timeouts for outbound calls.
- Always close response bodies.
- Include useful status-code and backend-message context in returned errors.
- Preserve authentication header behavior exactly where it matters.
- Gitee calls use `Authorization: token <token>`.
- OpenAI-compatible upstream calls in the proxy send both `Authorization: Bearer <token>` and `X-Api-Key: <token>`.
- Keep request/response transformations conservative; compatibility is more important than elegance.

## JSON and Proxy Translation Rules
- Translation code is shape-sensitive; avoid cosmetic rewrites that may alter wire behavior.
- Preserve streaming and non-streaming behavior separately.
- Respect the existing `thinking` / `reasoning` mapping semantics.
- `OPENAI_REASONING_EFFORT` is normalized strictly and should fail fast on invalid values.
- If a field is intentionally internal to `ccx`, strip it before passing settings to Claude, as `api_format` already is.
- When dealing with tool calls, preserve `call_id`, tool names, and arguments exactly unless the existing shortening logic applies.

## Testing Guidelines
- Add or update tests alongside the code you change.
- Keep tests in `_test.go` files beside the implementation.
- Prefer table-driven tests when validating multiple input/output combinations.
- Use `t.Run` for named subcases.
- Use `t.Parallel()` where the test is obviously safe to run concurrently.
- Use `httptest` servers or custom transports for HTTP behavior.
- Use `t.TempDir()` and `t.Setenv()` for filesystem and environment isolation.
- Assert both the success path and the error message content when the error text is part of the contract.
- Favor focused unit tests over end-to-end shell-driven tests.

## Change Management for Agents
- Check for uncommitted work before making large edits, and avoid overwriting unrelated changes.
- Do not rewrite files just to change formatting, imports, or comments.
- Do not commit build outputs such as `ccx` or `dist/` artifacts.
- Keep edits minimal and review adjacent code before changing shared helpers.
- If you touch translation logic, inspect both translators and their tests together.
- If you touch CLI command behavior, inspect both interactive and passthrough flows, and wire new commands in via `init()`.

## When Unsure
- Read the nearest command or test file and copy its pattern.
- Prefer extending an existing helper over creating a new utility package.
- Prefer explicit behavior over hidden magic.
- Prefer a clear returned error over a silent fallback.
- If a change affects protocol translation, verify with package-level tests before considering it done.

## Quick Examples
- Format touched files: `gofmt -w cmd/root.go internal/proxy/server.go`
- Full regression check: `go test ./...`
- Single proxy test: `go test ./internal/proxy -run TestResponsesStreamConverter_ToolCallDoneWithoutDelta -count=1`
- Single config test: `go test ./internal -run TestLoadAppConfigInvalidJSON -count=1`
- Single CLI test: `go test ./cmd -run TestNormalizeBaseURL -count=1`

## Done Criteria
- The changed code is `gofmt`-formatted.
- Relevant package tests pass.
- `go test ./...` passes for cross-package behavior changes.
- User-facing messages match the existing Chinese CLI tone.
- New code follows the existing fail-fast error style.
- Protocol changes are covered by tests when they affect JSON or SSE shapes.
