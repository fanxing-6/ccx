package cmd

import (
	"ccx/internal"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

type invocationMode int

const (
	invocationModeCobra invocationMode = iota
	invocationModeLaunch
	invocationModePassthrough
)

type invocationDecision struct {
	mode            invocationMode
	dangerous       bool
	profileName     string
	extraArgs       []string
	passthroughArgs []string
}

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
	"reset":      {},
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
	return false
}

func shouldPassthroughInvocation(args []string) bool {
	_, _, ok := decideRawPassthrough(args)
	return ok
}

func stripLeadingDangerousFlag(rawArgs []string) (args []string, dangerous bool) {
	args = append([]string(nil), rawArgs...)
	for len(args) > 0 {
		switch args[0] {
		case "-d", "--dangerous":
			dangerous = true
			args = args[1:]
		default:
			return args, dangerous
		}
	}
	return args, dangerous
}

func isHelpOrVersionFlag(token string) bool {
	switch token {
	case "-h", "--help", "-v", "--version":
		return true
	default:
		return false
	}
}

func decideRawPassthrough(rawArgs []string) (passArgs []string, dangerous bool, ok bool) {
	if len(rawArgs) == 0 {
		return nil, false, false
	}

	args, dangerous := stripLeadingDangerousFlag(rawArgs)
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
	if isHelpOrVersionFlag(first) {
		return nil, false, false
	}
	if _, exists := ccxCommandTokens[first]; exists {
		return nil, false, false
	}
	if !isPassthroughCandidate(first) {
		return nil, false, false
	}
	return args, dangerous, true
}

func decideInvocation(rawArgs []string) invocationDecision {
	if passArgs, dangerous, ok := decideRawPassthrough(rawArgs); ok {
		return invocationDecision{
			mode:            invocationModePassthrough,
			dangerous:       dangerous,
			passthroughArgs: passArgs,
		}
	}

	args, dangerous := stripLeadingDangerousFlag(rawArgs)
	if len(args) == 0 {
		return invocationDecision{
			mode:      invocationModeLaunch,
			dangerous: dangerous,
		}
	}

	first := strings.TrimSpace(args[0])
	if isHelpOrVersionFlag(first) {
		return invocationDecision{mode: invocationModeCobra}
	}
	if _, exists := ccxCommandTokens[first]; exists {
		return invocationDecision{mode: invocationModeCobra}
	}
	if strings.HasPrefix(first, "-") {
		return invocationDecision{
			mode:      invocationModeLaunch,
			dangerous: dangerous,
			extraArgs: append([]string(nil), args...),
		}
	}

	return invocationDecision{
		mode:        invocationModeLaunch,
		dangerous:   dangerous,
		profileName: first,
		extraArgs:   append([]string(nil), args[1:]...),
	}
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
