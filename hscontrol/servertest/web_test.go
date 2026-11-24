package servertest_test

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/juanfont/headscale/hscontrol/servertest"
	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWebPages serves the pages of a branded server: a theme, a partial
// replacing the footer, a landing page at /, and static files overriding
// the built-in favicon next to the built-in stylesheet.
func TestWebPages(t *testing.T) {
	t.Parallel()

	templatesDir, staticDir := t.TempDir(), t.TempDir()
	for dir, files := range map[string]map[string]string{
		templatesDir: {
			"brand.html": `{{define "footer"}}<footer>Run by Example IT</footer>{{end}}`,
			"blank.html": `<!DOCTYPE html><h1>Welcome to {{.Site.Name}}</h1>`,
		},
		staticDir: {
			"brand.css":   "main{color:red}",
			"favicon.png": "custom favicon",
		},
	} {
		for name, content := range files {
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
		}
	}

	srv := servertest.NewServer(t, servertest.WithWeb(types.WebConfig{
		TemplatesDir: templatesDir,
		StaticDir:    staticDir,
		Theme: types.WebThemeConfig{
			Name:         "Example VPN",
			Logo:         "/static/logo.svg",
			PrimaryColor: "#0a7d5a",
			Stylesheets:  []types.WebURL{"/static/brand.css"},
		},
	}))

	client := &http.Client{Transport: &http.Transport{DialContext: srv.MemNet().Dial}}

	type response struct {
		status int
		header http.Header
		body   string
	}

	// fetch sends a GET with accept as its Accept header ("" for none).
	fetch := func(path, accept string) response {
		t.Helper()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, nil)
		require.NoError(t, err)

		if accept != "" {
			req.Header.Set("Accept", accept)
		}

		resp, err := client.Do(req)
		require.NoError(t, err)

		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		return response{resp.StatusCode, resp.Header, string(body)}
	}

	get := func(path string) response { t.Helper(); return fetch(path, "") }

	// browse asks as a browser's navigation does.
	browse := func(path string) response {
		t.Helper()

		return fetch(path, "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	}

	resp := get("/apple")
	apple := resp.body
	assert.Equal(t, http.StatusOK, resp.status)
	assert.Equal(t, "text/html; charset=utf-8", resp.header.Get("Content-Type"))
	assert.Equal(t, "DENY", resp.header.Get("X-Frame-Options"), "security headers still apply")
	assert.Contains(t, apple, "<title>Apple - Example VPN</title>")
	assert.Contains(t, apple, "<code>"+srv.URL+"</code>")
	assert.Contains(t, apple, `<img class="headscale-logo" src="/static/logo.svg" alt="Example VPN">`)
	assert.Contains(t, apple, `<link rel="stylesheet" href="/static/brand.css">`)
	assert.Contains(t, apple, "--md-primary-fg-color:#0a7d5a")
	assert.Contains(t, apple, "<footer>Run by Example IT</footer>")
	assert.NotContains(t, apple, "fonts.googleapis.com", "the theme sets no font stylesheets")

	assert.Contains(t, get("/windows").body, "connect with Example VPN:")
	assert.Equal(t, "<!DOCTYPE html><h1>Welcome to Example VPN</h1>", get("/").body)

	resp = get("/static/brand.css")
	assert.Equal(t, http.StatusOK, resp.status)
	assert.Equal(t, "main{color:red}", resp.body)
	assert.Equal(t, "text/css; charset=utf-8", resp.header.Get("Content-Type"))

	resp = get("/static/style.css")
	assert.Equal(t, http.StatusOK, resp.status)
	assert.Contains(t, resp.body, ".md-typeset")

	assert.Equal(t, "custom favicon", get("/favicon.ico").body)
	assert.Equal(t, http.StatusNotFound, get("/static/").status, "no directory listing")
	assert.Contains(t, get("/apple/macos-app-store").body, "<key>PayloadDisplayName</key>\n    <string>Example VPN</string>")

	// Errors a browser reaches get the branded error page.
	resp = browse("/nope")
	assert.Equal(t, http.StatusNotFound, resp.status)
	assert.Equal(t, "text/html; charset=utf-8", resp.header.Get("Content-Type"))
	assert.Equal(t, "DENY", resp.header.Get("X-Frame-Options"))
	assert.Contains(t, resp.body, "<title>Error - Example VPN</title>")
	assert.Contains(t, resp.body, "Page not found")
	assert.Contains(t, resp.body, "<footer>Run by Example IT</footer>")

	resp = browse("/apple/bogus")
	assert.Equal(t, http.StatusBadRequest, resp.status)
	assert.Contains(t, resp.body, "Request not accepted")
	assert.NotContains(t, resp.body, "platform must be")

	resp = browse("/register/not-an-auth-id")
	assert.Equal(t, http.StatusBadRequest, resp.status)
	assert.Contains(t, resp.body, "<title>Error - Example VPN</title>")
	assert.NotContains(t, resp.body, "invalid auth id")

	// Clients and API callers keep plain text.
	resp = get("/nope")
	assert.Equal(t, http.StatusNotFound, resp.status)
	assert.Equal(t, "404 page not found\n", resp.body)

	resp = get("/apple/bogus")
	assert.Equal(t, http.StatusBadRequest, resp.status)
	assert.Equal(t, "platform must be ios, macos-app-store or macos-standalone\n", resp.body,
		"a client keeps the plain-text message")

	resp = browse("/api/v1/nope")
	assert.Equal(t, http.StatusNotFound, resp.status)
	assert.NotContains(t, resp.header.Get("Content-Type"), "text/html")

	resp = browse("/static/missing.css")
	assert.Equal(t, http.StatusNotFound, resp.status)
	assert.NotContains(t, resp.body, "Page not found", "static files keep their plain 404")
}
