package server

import (
	"bytes"
	"net/http"
	"time"

	veloxbuild "github.com/jpillora/velox/js/build"
)

// veloxJS is velox's browser bundle minus its inline base64 sourcemap, which
// is 123 KB of the 146 KB file and only of use with devtools open.
var veloxJS = func() []byte {
	b, err := veloxbuild.FS.ReadFile("bundle.js")
	if err != nil {
		panic(err)
	}
	if i := bytes.LastIndex(b, []byte("\n//# sourceMappingURL=")); i >= 0 {
		b = b[:i+1]
	}
	return b
}()

func serveVeloxJS(w http.ResponseWriter, r *http.Request) {
	http.ServeContent(w, r, "velox.js", time.Time{}, bytes.NewReader(veloxJS))
}
