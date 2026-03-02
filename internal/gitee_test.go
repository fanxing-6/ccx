package internal

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestNewGistClientHasDefaultTimeout(t *testing.T) {
	client := NewGistClient("token", "owner", "gist")
	if client.client == nil {
		t.Fatal("http client should not be nil")
	}
	if client.client.Timeout != defaultGiteeTimeout {
		t.Fatalf("timeout=%v, want %v", client.client.Timeout, defaultGiteeTimeout)
	}
}

func TestListSettingsFilesUsesAuthorizationHeader(t *testing.T) {
	var gotAuth string
	var gotQuery string

	g := &GistClient{
		Token:  "abc",
		GistID: "gid",
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				gotAuth = req.Header.Get("Authorization")
				gotQuery = req.URL.RawQuery
				return newHTTPResponse(http.StatusOK, `{"files":{}}`), nil
			}),
		},
	}

	_, err := g.ListSettingsFiles()
	if err != nil {
		t.Fatalf("ListSettingsFiles failed: %v", err)
	}

	if gotAuth != "token abc" {
		t.Fatalf("Authorization=%q, want %q", gotAuth, "token abc")
	}
	if strings.Contains(gotQuery, "access_token=") {
		t.Fatalf("request query should not include access_token, got %q", gotQuery)
	}
}

func TestListSettingsFilesDecodeError(t *testing.T) {
	g := &GistClient{
		Token:  "abc",
		GistID: "gid",
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return newHTTPResponse(http.StatusOK, `{invalid-json`), nil
			}),
		},
	}

	_, err := g.ListSettingsFiles()
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "解析 Gitee API 响应失败") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListSettingsFilesPropagatesRequestError(t *testing.T) {
	g := &GistClient{
		Token:  "abc",
		GistID: "gid",
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return nil, context.DeadlineExceeded
			}),
		},
	}

	_, err := g.ListSettingsFiles()
	if err == nil {
		t.Fatal("expected request error")
	}
	if !strings.Contains(err.Error(), "请求 Gitee API 失败") {
		t.Fatalf("unexpected error: %v", err)
	}
}
