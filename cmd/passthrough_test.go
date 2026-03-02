package cmd

import "testing"

func TestIsPassthroughCandidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		arg  string
		want bool
	}{
		{name: "auth command", arg: "auth", want: true},
		{name: "update command", arg: "update", want: true},
		{name: "mcp command", arg: "mcp", want: true},
		{name: "flag style invocation", arg: "-p", want: true},
		{name: "ccx subcommand should not match", arg: "add", want: false},
		{name: "ccx reset should not match", arg: "reset", want: false},
		{name: "profile style arg should not match", arg: "volc", want: false},
		{name: "help flag should not passthrough", arg: "--help", want: false},
		{name: "empty args", arg: "", want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isPassthroughCandidate(tc.arg)
			if got != tc.want {
				t.Fatalf("isPassthroughCandidate(%q)=%v, want %v", tc.arg, got, tc.want)
			}
		})
	}
}

func TestShouldPassthroughInvocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "auth command", args: []string{"auth", "status"}, want: true},
		{name: "flag invocation", args: []string{"-p", "hello"}, want: true},
		{name: "non candidate", args: []string{"volc"}, want: false},
		{name: "ccx subcommand", args: []string{"list"}, want: false},
		{name: "ccx reset", args: []string{"reset"}, want: false},
		{name: "help flag", args: []string{"--help"}, want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := shouldPassthroughInvocation(tc.args)
			if got != tc.want {
				t.Fatalf("shouldPassthroughInvocation(%v)=%v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestDecideRawPassthrough(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		raw          []string
		wantOK       bool
		wantDanger   bool
		wantPassArgs []string
	}{
		{
			name:         "plain command passthrough",
			raw:          []string{"auth", "status"},
			wantOK:       true,
			wantDanger:   false,
			wantPassArgs: []string{"auth", "status"},
		},
		{
			name:         "dangerous root flag is extracted",
			raw:          []string{"-d", "auth", "status"},
			wantOK:       true,
			wantDanger:   true,
			wantPassArgs: []string{"auth", "status"},
		},
		{
			name:         "double-dash forces passthrough",
			raw:          []string{"--", "-p", "hello"},
			wantOK:       true,
			wantDanger:   false,
			wantPassArgs: []string{"-p", "hello"},
		},
		{
			name:   "ccx subcommand should not passthrough",
			raw:    []string{"list"},
			wantOK: false,
		},
		{
			name:   "ccx help flag should not passthrough",
			raw:    []string{"--help"},
			wantOK: false,
		},
		{
			name:   "profile-like arg should not passthrough",
			raw:    []string{"volc"},
			wantOK: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotArgs, gotDanger, gotOK := decideRawPassthrough(tc.raw)
			if gotOK != tc.wantOK {
				t.Fatalf("decideRawPassthrough(%v) ok=%v, want %v", tc.raw, gotOK, tc.wantOK)
			}
			if gotDanger != tc.wantDanger {
				t.Fatalf("decideRawPassthrough(%v) dangerous=%v, want %v", tc.raw, gotDanger, tc.wantDanger)
			}
			if len(tc.wantPassArgs) > 0 {
				if len(gotArgs) != len(tc.wantPassArgs) {
					t.Fatalf("decideRawPassthrough(%v) args=%v, want %v", tc.raw, gotArgs, tc.wantPassArgs)
				}
				for i := range gotArgs {
					if gotArgs[i] != tc.wantPassArgs[i] {
						t.Fatalf("decideRawPassthrough(%v) args=%v, want %v", tc.raw, gotArgs, tc.wantPassArgs)
					}
				}
			}
		})
	}
}

func TestContainsDangerousFlag(t *testing.T) {
	t.Parallel()

	if !containsDangerousFlag([]string{"--dangerously-skip-permissions"}) {
		t.Fatalf("expected --dangerously-skip-permissions to be detected")
	}
	if !containsDangerousFlag([]string{"--allow-dangerously-skip-permissions"}) {
		t.Fatalf("expected --allow-dangerously-skip-permissions to be detected")
	}
	if containsDangerousFlag([]string{"--model", "sonnet"}) {
		t.Fatalf("unexpected dangerous flag detection")
	}
}
