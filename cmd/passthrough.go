package cmd

import (
	"ccx/internal"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

var claudePassthroughCommands = map[string]struct{}{
	"agents":         {},
	"auth":           {},
	"mcp":            {},
	"remote-control": {},
	"update":         {},
}

var ccxCommandTokens = map[string]struct{}{
	"add":        {},
	"completion": {},
	"default":    {},
	"edit":       {},
	"help":       {},
	"info":       {},
	"init":       {},
	"list":       {},
	"ls":         {},
	"remove":     {},
	"rm":         {},
}

func isPassthroughCandidate(token string) bool {
	first := strings.TrimSpace(token)
	if first == "" {
		return false
	}
	if _, ok := claudePassthroughCommands[first]; ok {
		return true
	}
	if strings.HasPrefix(first, "-") {
		switch first {
		case "-h", "--help", "-v", "--version":
			return false
		default:
			return true
		}
	}
	return false
}

func shouldPassthroughInvocation(args []string) bool {
	_, _, ok := decideRawPassthrough(args)
	return ok
}

func decideRawPassthrough(rawArgs []string) (passArgs []string, dangerous bool, ok bool) {
	if len(rawArgs) == 0 {
		return nil, false, false
	}

	args := append([]string(nil), rawArgs...)
	for len(args) > 0 {
		switch args[0] {
		case "-d", "--dangerous":
			dangerous = true
			args = args[1:]
		default:
			goto parse
		}
	}

parse:
	if len(args) == 0 {
		return nil, false, false
	}

	if args[0] == "--" {
		args = args[1:]
		if len(args) == 0 {
			return nil, false, false
		}
		return args, dangerous, true
	}

	first := strings.TrimSpace(args[0])
	if _, exists := ccxCommandTokens[first]; exists {
		return nil, false, false
	}
	if !isPassthroughCandidate(first) {
		return nil, false, false
	}
	return args, dangerous, true
}

func launchClaudePassthrough(claudeCmd string, args []string, dangerous bool) error {
	if strings.TrimSpace(claudeCmd) == "" {
		claudeCmd = "claude"
	}

	passArgs := make([]string, 0, len(args)+1)
	if dangerous && !containsDangerousFlag(args) {
		passArgs = append(passArgs, "--dangerously-skip-permissions")
	}
	passArgs = append(passArgs, args...)

	claudePath, err := exec.LookPath(claudeCmd)
	if err != nil {
		return fmt.Errorf("未找到 %s 命令，请确认已安装 Claude Code", claudeCmd)
	}

	fmt.Printf("\n=> %s %s\n\n", claudeCmd, strings.Join(passArgs, " "))
	return syscall.Exec(claudePath, append([]string{claudeCmd}, passArgs...), os.Environ())
}

func containsDangerousFlag(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "--dangerously-skip-permissions", "--allow-dangerously-skip-permissions":
			return true
		}
	}
	return false
}

func resolvePassthroughClaudeCmd() string {
	if !internal.ConfigExists() {
		return "claude"
	}
	cfg, err := internal.LoadAppConfig()
	if err != nil || strings.TrimSpace(cfg.ClaudeCmd) == "" {
		return "claude"
	}
	return cfg.ClaudeCmd
}
