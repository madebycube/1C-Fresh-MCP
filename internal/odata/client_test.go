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

func TestWriteUsesJSONAndDataVersion(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/a/sbm/1/odata/standard.odata/Catalog_Номенклатура(guid'11111111-1111-1111-1111-111111111111')" || r.Header.Get("Content-Type") != "application/json; charset=utf-8" || r.Header.Get("If-Match") != "v1" {
			t.Errorf("unexpected write request: %s %s", r.Method, r.URL.Path)
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "user" || password != "top-secret" {
			t.Error("missing authentication")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL + "/a/sbm/1")
	client := New(config.Config{BaseURL: base, Username: "user", Password: "top-secret"})
	client.http = server.Client()
	_, err := client.Write(context.Background(), http.MethodPatch, "Catalog_Номенклатура(guid'11111111-1111-1111-1111-111111111111')", []byte(`{"Description":"New"}`), "v1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Write(context.Background(), http.MethodPatch, "Catalog_Номенклатура", []byte(`{}`), ""); err == nil {
		t.Fatal("accepted a patch without a data version")
	}
}

func TestWriteHidesResponseAndReportsConflict(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
		_, _ = w.Write([]byte("private record data"))
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL + "/a/sbm/1")
	client := New(config.Config{BaseURL: base, Username: "user", Password: "top-secret"})
	client.http = server.Client()
	_, err := client.Write(context.Background(), http.MethodPatch, "Catalog_Номенклатура", []byte(`{}`), "old")
	if err == nil || !strings.Contains(err.Error(), "changed") || strings.Contains(err.Error(), "private record data") {
		t.Fatalf("unexpected conflict error: %v", err)
	}
}

func TestStockBalanceUsesValidatedFunctionPath(t *testing.T) {
	id := "11111111-1111-1111-1111-111111111111"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/a/sbm/1/odata/standard.odata/AccumulationRegister_ЗапасыНаСкладах/Balance(Condition='Номенклатура_Key eq guid''" + id + "''')"
		if r.URL.Path != want || r.URL.Query().Get("$top") != "1" {
			t.Errorf("unexpected balance request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"value":[]}`))
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL + "/a/sbm/1")
	client := New(config.Config{BaseURL: base, Username: "user", Password: "password"})
	client.http = server.Client()
	if _, err := client.GetStockBalance(context.Background(), id, url.Values{"$top": {"1"}}, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetStockBalance(context.Background(), "../other", nil, 100); err == nil {
		t.Fatal("accepted an invalid product ID")
	}
	if _, err := client.Get(context.Background(), "AccumulationRegister_ЗапасыНаСкладах/Balance", nil, 100); err == nil {
		t.Fatal("generic OData read accepted a function path")
	}
}
