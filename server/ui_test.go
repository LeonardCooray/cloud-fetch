package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRootServesNewShell(t *testing.T) {
	s, _ := newTestServer(t, "")
	w := do(t, s.handler(), httptest.NewRequest("GET", "/", nil))
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, `<script type="importmap">`) || !strings.Contains(body, `src="js/main.js"`) {
		t.Fatalf("status %d; shell is missing the import map or main.js:\n%s", w.Code, body)
	}
}

// module scripts are refused by browsers unless served with a JavaScript MIME type
func TestModulesServedAsJavaScript(t *testing.T) {
	s, _ := newTestServer(t, "")
	h := s.handler()
	for _, p := range []string{"/js/main.js", "/js/vendor/preact.mjs", "/js/vendor/hooks.mjs", "/js/vendor/htm.mjs", "/js/components/App.js"} {
		w := do(t, h, httptest.NewRequest("GET", p, nil))
		ct := w.Header().Get("Content-Type")
		if w.Code != 200 || !(strings.HasPrefix(ct, "text/javascript") || strings.HasPrefix(ct, "application/javascript")) {
			t.Errorf("%s: status %d, Content-Type %q", p, w.Code, ct)
		}
	}
}

func TestOldUIFilesAreGone(t *testing.T) {
	s, _ := newTestServer(t, "")
	h := s.handler()
	for _, p := range []string{"/js/vendor/angular.min.js", "/css/semantic.min.css", "/template/omni.html", "/js/run.js", "/css/Lato/Lato.css"} {
		if w := do(t, h, httptest.NewRequest("GET", p, nil)); w.Code != 404 {
			t.Errorf("%s still served (status %d)", p, w.Code)
		}
	}
}
