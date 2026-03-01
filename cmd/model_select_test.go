package cmd

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestModelEndpointCandidates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		want    []string
	}{
		{
			name:    "with v1 suffix",
			baseURL: "https://api.linkflow.run/v1",
			want:    []string{"https://api.linkflow.run/v1/models"},
		},
		{
			name:    "without v1 suffix",
			baseURL: "https://api.linkflow.run",
			want: []string{
				"https://api.linkflow.run/models",
				"https://api.linkflow.run/v1/models",
			},
		},
		{
			name:    "trim spaces and slash",
			baseURL: " https://api.linkflow.run/v1/ ",
			want:    []string{"https://api.linkflow.run/v1/models"},
		},
		{
			name:    "empty",
			baseURL: "   ",
			want:    nil,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := modelEndpointCandidates(tc.baseURL)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("modelEndpointCandidates(%q)=%v, want %v", tc.baseURL, got, tc.want)
			}
		})
	}
}

func TestParseModelList(t *testing.T) {
	t.Parallel()

	body := []byte(`{
	  "data": [
	    {"id": "gpt-4.1", "display_name": "GPT 4.1"},
	    {"id": "gpt-4o", "display_name": "GPT 4o"},
	    {"id": "gpt-4.1", "display_name": "duplicate should be removed"},
	    {"id": "  "},
	    {"display_name": "missing id"}
	  ]
	}`)

	got, err := parseModelList(body)
	if err != nil {
		t.Fatalf("parseModelList returned error: %v", err)
	}

	want := []modelInfo{
		{ID: "gpt-4.1", DisplayName: "GPT 4.1"},
		{ID: "gpt-4o", DisplayName: "GPT 4o"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseModelList()=%v, want %v", got, want)
	}
}

func TestParseModelListInvalid(t *testing.T) {
	t.Parallel()

	_, err := parseModelList([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestFetchModelsFallbackAndHeaders(t *testing.T) {
	t.Parallel()

	var (
		mu            sync.Mutex
		paths         []string
		authByPath    = map[string]string{}
		xAPIKeyByPath = map[string]string{}
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		authByPath[r.URL.Path] = r.Header.Get("Authorization")
		xAPIKeyByPath[r.URL.Path] = r.Header.Get("X-Api-Key")
		mu.Unlock()

		switch r.URL.Path {
		case "/models":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"not found"}}`))
		case "/v1/models":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5.2-xhigh","display_name":"GPT 5.2 XHigh"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	got, err := fetchModels(srv.URL, "test-token")
	if err != nil {
		t.Fatalf("fetchModels returned error: %v", err)
	}

	if len(got) != 1 || got[0].ID != "gpt-5.2-xhigh" {
		t.Fatalf("unexpected models: %v", got)
	}

	mu.Lock()
	defer mu.Unlock()

	wantPaths := []string{"/models", "/v1/models"}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("request paths=%v, want %v", paths, wantPaths)
	}

	for _, p := range wantPaths {
		if authByPath[p] != "Bearer test-token" {
			t.Fatalf("%s Authorization=%q, want %q", p, authByPath[p], "Bearer test-token")
		}
		if xAPIKeyByPath[p] != "test-token" {
			t.Fatalf("%s X-Api-Key=%q, want %q", p, xAPIKeyByPath[p], "test-token")
		}
	}
}

func TestFetchModelsReturnsUsefulError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer srv.Close()

	_, err := fetchModels(srv.URL+"/v1", "bad-token")
	if err == nil {
		t.Fatal("expected error")
	}

	msg := err.Error()
	if !strings.Contains(msg, "401") {
		t.Fatalf("error %q should include status code 401", msg)
	}
	if !strings.Contains(msg, "invalid api key") {
		t.Fatalf("error %q should include backend message", msg)
	}
}
