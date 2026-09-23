package odata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

func TestGetDoesNotExposeCredentialsOrResponseBody(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "user" || password != "top-secret" {
			t.Error("Basic authentication missing")
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("private record data"))
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL + "/a/sbm/1")
	client := New(config.Config{BaseURL: base, Username: "user", Password: "top-secret"})
	client.http = server.Client()
	_, err := client.Get(context.Background(), "Catalog_Номенклатура", nil, 100)
	if err == nil || strings.Contains(err.Error(), "top-secret") || strings.Contains(err.Error(), "private record data") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetRejectsRedirect(t *testing.T) {
	redirected := false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/target" {
			redirected = true
			return
		}
		http.Redirect(w, r, "/target", http.StatusFound)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL + "/a/sbm/1")
	client := New(config.Config{BaseURL: base, Username: "user", Password: "top-secret"})
	client.http.Transport = server.Client().Transport
	_, err := client.Get(context.Background(), "$metadata", nil, 100)
	if err == nil || redirected {
		t.Fatalf("redirect followed or accepted: %v", err)
	}
}
