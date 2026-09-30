package prometheus

import (
	"bufio"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newMetricsHandler(t *testing.T, inner http.HandlerFunc) http.Handler {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Namespace = "test"
	cfg.Registry = newTestRegistry()
	return NewBundle(cfg).HTTPMiddleware()(inner)
}

func TestHTTPMiddleware_Unwrap_AllowsResponseController(t *testing.T) {
	var ctlErr error
	srv := httptest.NewServer(newMetricsHandler(t, func(w http.ResponseWriter, r *http.Request) {
		ctlErr = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if ctlErr != nil {
		t.Fatalf("SetWriteDeadline through middleware: %v", ctlErr)
	}
}

func TestHTTPMiddleware_Flush_DeliversBeforeHandlerReturns(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(newMetricsHandler(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "first\n")
		w.(http.Flusher).Flush()
		<-release
	}))
	defer srv.Close()
	defer close(release)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil || line != "first\n" {
		t.Fatalf("read first line = %q, %v", line, err)
	}
}
