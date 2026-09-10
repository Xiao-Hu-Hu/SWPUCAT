package business

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientForwardsIdentityAndIdempotencyKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-token" {
			t.Errorf("Authorization header = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Agent-Internal-Secret") != "internal-secret" {
			t.Errorf("internal secret header = %q", r.Header.Get("X-Agent-Internal-Secret"))
		}
		if r.Header.Get("Idempotency-Key") != "agent:req-1:clock-in" {
			t.Errorf("Idempotency-Key header = %q", r.Header.Get("Idempotency-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"id":7}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "internal-secret", time.Second)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	ctx := WithRequestContext(context.Background(), RequestContext{Authorization: "Bearer access-token"})
	if _, err := client.ClockIn(ctx, "agent:req-1:clock-in"); err != nil {
		t.Fatalf("ClockIn() error = %v", err)
	}
}
