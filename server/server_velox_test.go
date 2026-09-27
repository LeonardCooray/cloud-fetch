package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVeloxJSIsServedWithoutItsInlineSourcemap(t *testing.T) {
	s, _ := newTestServer(t, "")
	w := do(t, s.handler(), httptest.NewRequest("GET", "/js/velox.js", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") && !strings.HasPrefix(ct, "application/javascript") {
		t.Fatalf("Content-Type %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "window.velox") {
		t.Fatal("the velox client code is missing")
	}
	if strings.Contains(body, "sourceMappingURL") {
		t.Fatal("the inline sourcemap is still served")
	}
	// the full bundle is 146 KB, 123 KB of it the sourcemap
	if w.Body.Len() > 40_000 {
		t.Fatalf("velox.js is %d bytes", w.Body.Len())
	}
}
