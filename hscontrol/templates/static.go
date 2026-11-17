package templates

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/juanfont/headscale/hscontrol/assets"
)

const (
	// StaticPrefix is the URL path the static files are served under.
	StaticPrefix = "/static/"

	// staticCacheControl lets browsers keep a file for an hour; a file
	// replaced in web.static_dir reaches them within that.
	staticCacheControl = "public, max-age=3600"
)

// staticTypes are the types of files Go's mime table may not know.
var staticTypes = map[string]string{
	".ico":   "image/x-icon",
	".otf":   "font/otf",
	".ttf":   "font/ttf",
	".woff":  "font/woff",
	".woff2": "font/woff2",
}

// StaticHandler serves [StaticPrefix]: a file of web.static_dir, else the
// built-in file of that name. Directories and dotfiles are not served.
func (r *Renderer) StaticHandler() http.Handler {
	return http.StripPrefix(StaticPrefix, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.orDefault().serveStatic(w, req, req.URL.Path)
	}))
}

// ServeFavicon serves favicon.png of [Renderer.StaticHandler], for
// /favicon.ico.
func (r *Renderer) ServeFavicon(w http.ResponseWriter, req *http.Request) {
	r.orDefault().serveStatic(w, req, "favicon.png")
}

func (r *Renderer) serveStatic(w http.ResponseWriter, req *http.Request, name string) {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)

		return
	}

	f, err := r.openStatic(name)
	if err != nil {
		http.NotFound(w, req)
		return
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		http.NotFound(w, req)
		return
	}

	content, ok := f.(io.ReadSeeker)
	if !ok {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	if t, ok := staticTypes[path.Ext(name)]; ok {
		w.Header().Set("Content-Type", t)
	}

	w.Header().Set("Cache-Control", staticCacheControl)
	http.ServeContent(w, req, name, st.ModTime(), content)
}

// openStatic opens name in web.static_dir, through [os.OpenInRoot] so a
// symlink cannot reach outside it, falling back to the built-in files
// when it is not there.
func (r *Renderer) openStatic(name string) (fs.File, error) {
	if !fs.ValidPath(name) || name == "." || hasDotElem(name) {
		return nil, fs.ErrNotExist
	}

	if r.staticDir != "" {
		f, err := os.OpenInRoot(r.staticDir, name)
		if err == nil {
			return f, nil
		}

		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}

	return assets.Static.Open(name)
}

func hasDotElem(name string) bool {
	for elem := range strings.SplitSeq(name, "/") {
		if strings.HasPrefix(elem, ".") {
			return true
		}
	}

	return false
}
