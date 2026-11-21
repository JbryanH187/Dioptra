package layoutagent_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"dioptra/internal/layoutagent"
)

// Un 500 puede llegar después de que el proveedor generó (y cobró) la respuesta:
// no se reintenta. 503/429/529 indican que no se procesó: sí se reintenta.
func TestRetryPolicy_OnlyRetriesUnprocessedRequests(t *testing.T) {
	for _, tc := range []struct {
		status int
		calls  int32
	}{
		{http.StatusInternalServerError, 1},
		{http.StatusGatewayTimeout, 1},
		{http.StatusServiceUnavailable, 2},
		{http.StatusTooManyRequests, 2},
	} {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Header().Set("Retry-After", "0")
			http.Error(w, `{"error":"x"}`, tc.status)
		}))
		cfg := layoutagent.Config{Provider: "openai-compatible", APIURL: srv.URL + "/v1/chat/completions", APIKey: "k", Model: "m", Timeout: 10 * time.Second}
		_, err := layoutagent.Synthesize(context.Background(), cfg, layoutagent.Input{Title: "T", Markdown: "# T\n\ntexto"})
		srv.Close()
		if err == nil {
			t.Fatalf("status %d: se esperaba error", tc.status)
		}
		if got := calls.Load(); got != tc.calls {
			t.Errorf("status %d: llamadas al proveedor = %d, se esperaban %d", tc.status, got, tc.calls)
		}
	}
}
