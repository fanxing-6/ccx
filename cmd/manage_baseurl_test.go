package cmd

import "testing"

func TestNormalizeBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		in       string
		wantURL  string
		wantWarn bool
		wantErr  bool
	}{
		{
			name:     "v1 and trailing slash",
			in:       "https://api.linkflow.run/v1/",
			wantURL:  "https://api.linkflow.run/v1",
			wantWarn: false,
			wantErr:  false,
		},
		{
			name:     "no v1 gives warning",
			in:       "https://api.linkflow.run/",
			wantURL:  "https://api.linkflow.run",
			wantWarn: true,
			wantErr:  false,
		},
		{
			name:     "with spaces",
			in:       "  https://api.linkflow.run/v1  ",
			wantURL:  "https://api.linkflow.run/v1",
			wantWarn: false,
			wantErr:  false,
		},
		{
			name:    "invalid url",
			in:      "api.linkflow.run/v1",
			wantErr: true,
		},
		{
			name:    "empty",
			in:      "   ",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotURL, gotWarning, err := normalizeBaseURL(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("normalizeBaseURL(%q) expected error", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeBaseURL(%q) unexpected error: %v", tc.in, err)
			}
			if gotURL != tc.wantURL {
				t.Fatalf("normalizeBaseURL(%q) url=%q, want %q", tc.in, gotURL, tc.wantURL)
			}
			if (gotWarning != "") != tc.wantWarn {
				t.Fatalf("normalizeBaseURL(%q) warning=%q, wantWarn=%v", tc.in, gotWarning, tc.wantWarn)
			}
		})
	}
}
