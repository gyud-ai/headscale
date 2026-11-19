package templates

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/juanfont/headscale/hscontrol/assets"
	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRenderer(t *testing.T, web types.WebConfig) *Renderer {
	t.Helper()

	if web.Theme.Name == "" && web.Theme.Logo == "" {
		theme := types.DefaultWebTheme()
		theme.FooterText = web.Theme.FooterText
		theme.HidePoweredBy = web.Theme.HidePoweredBy
		web.Theme = theme
	}

	r, err := New(&types.Config{ServerURL: "https://hs.example.com", Web: web})
	require.NoError(t, err)

	return r
}

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	}

	return dir
}

// mustRender returns a function that takes a render result, failing t on
// an error: mustRender(t)(r.Apple()).
func mustRender(t *testing.T) func([]byte, error) string {
	t.Helper()

	return func(b []byte, err error) string {
		t.Helper()
		require.NoError(t, err)

		return string(b)
	}
}

// TestBuiltinPages renders every page as headscale does and checks the
// markup that clients and tests parse.
func TestBuiltinPages(t *testing.T) {
	r := newTestRenderer(t, types.WebConfig{})

	pages := map[string]string{
		"apple":   mustRender(t)(r.Apple()),
		"windows": mustRender(t)(r.Windows()),
		"blank":   mustRender(t)(r.Blank()),
		"auth web": mustRender(t)(r.AuthWeb(AuthWebData{
			Title: "Node registration", Description: "Run:", Command: "headscale auth register --auth-id abc --user USERNAME",
		})),
		"success": mustRender(t)(r.AuthSuccess(AuthSuccessResult{
			Title: "Node Registered", Heading: "Node registered", Verb: "Registered", User: "alice", Message: "Done.",
		})),
		"error":            mustRender(t)(r.AuthError(AuthErrorResult{Title: "Error", Heading: "Forbidden", Message: "No."})),
		"register confirm": mustRender(t)(r.RegisterConfirm(sampleConfirm())),
		"ssh check":        mustRender(t)(r.SSHCheckConfirm(SSHCheckData{RegisterConfirmInfo: sampleConfirm(), Source: "a", Destination: "b"})),
		"ping":             mustRender(t)(r.Ping(PingData{})),
	}

	for name, html := range pages {
		t.Run(name, func(t *testing.T) {
			assert.True(t, strings.HasPrefix(html, "<!DOCTYPE html>"), "starts with the doctype: %.40q", html)
			assert.Contains(t, html, `<link rel="icon" href="/favicon.ico">`)
			assert.Contains(t, html, "<title>")
			assert.Contains(t, html, "Headscale</title>")
		})
	}

	assert.Equal(t, 1, strings.Count(pages["auth web"], "</code>"), "the register page has one command")
	assert.Contains(t, pages["auth web"], "<code>headscale auth register --auth-id abc --user USERNAME</code>")
	assert.Contains(t, pages["apple"], "<code>https://hs.example.com</code>")
	assert.Contains(t, pages["windows"], "tailscale login --login-server https://hs.example.com")
	assert.Contains(t, pages["success"], "Registered as <strong>alice</strong>. Done.")
	assert.Contains(t, pages["success"], `id="checkbox"`)
	assert.Contains(t, pages["success"], "<h2>Getting started</h2>")
	assert.Contains(t, pages["success"], `href="https://headscale.net/stable/"`)
	assert.Equal(t, 2, strings.Count(pages["apple"], `download="headscale_macos.mobileconfig"`))
	assert.Contains(t, pages["error"], `id="error-icon"`)
	assert.Contains(t, pages["error"], `role="alert"`)
	assert.Contains(t, pages["register confirm"], "<tr><td>Existing node</td><td>laptop</td></tr>")
	assert.Contains(t, pages["register confirm"], `action="/register/confirm/sample"`)
	assert.Contains(t, pages["register confirm"], `<input type="hidden" name="headscale_register_confirm" value="sample">`)
	assert.Contains(t, pages["ssh check"], "<tr><td>From</td><td>a</td></tr>")
	assert.Less(t, len(pages["error"]), 16*1024, "the error page stays small")
}

// TestPageEscaping: values a client or user controls cannot add markup.
func TestPageEscaping(t *testing.T) {
	r := newTestRenderer(t, types.WebConfig{})

	payloads := []string{
		`" autofocus onfocus=alert(1) x="`,
		`"><script>alert(1)</script>`,
		`<img src=x onerror=alert(1)>`,
	}

	for _, p := range payloads {
		t.Run(p, func(t *testing.T) {
			ping := mustRender(t)(r.Ping(PingData{Query: p}))
			assert.NotContains(t, ping, p)

			c := sampleConfirm()
			c.Hostname, c.OS, c.Tags = p, p, []string{p}
			confirm := mustRender(t)(r.RegisterConfirm(c))
			assert.NotContains(t, confirm, p)

			e := mustRender(t)(r.AuthError(AuthErrorResult{Title: p, Heading: p, Message: p}))
			assert.NotContains(t, e, p)
		})
	}
}

func TestRegisterConfirmTags(t *testing.T) {
	r := newTestRenderer(t, types.WebConfig{})

	c := sampleConfirm()
	c.Tags = []string{"tag:a", "tag:b"}
	c.OS, c.ExistingNode = "", ""

	html := mustRender(t)(r.RegisterConfirm(c))
	assert.Contains(t, html, "<tr><td>Registered to</td><td>tag:a, tag:b</td></tr>")
	assert.Contains(t, html, "<tr><td>Approved by</td><td>user@example.com</td></tr>")
	assert.Contains(t, html, "<tr><td>OS</td><td>(unknown)</td></tr>")
	assert.Contains(t, html, "<tr><td>Existing node</td><td>None, this is a new device</td></tr>")
}

func TestPingResults(t *testing.T) {
	r := newTestRenderer(t, types.WebConfig{})

	ok := mustRender(t)(r.Ping(PingData{
		Result: &PingResult{Status: "ok", NodeID: 7, Latency: 12345678 * time.Nanosecond},
		Nodes:  []ConnectedNode{{ID: 7, Hostname: "laptop", IPs: []string{"100.64.0.7", "fd7a::7"}}},
	}))
	assert.Contains(t, ok, "Node 7 responded in 12ms")
	assert.Contains(t, ok, `<a href="/debug/ping?node=7">laptop (ID: 7, 100.64.0.7, fd7a::7)</a>`)

	timeout := mustRender(t)(r.Ping(PingData{Result: &PingResult{Status: "timeout", NodeID: 7, Message: "Gone."}}))
	assert.Contains(t, timeout, "Node 7 did not respond. Gone.")
}

func TestTheme(t *testing.T) {
	r := newTestRenderer(t, types.WebConfig{Theme: types.WebThemeConfig{
		Name:             "Example VPN",
		Logo:             "/static/logo.svg",
		PrimaryColor:     "#0a7d5a",
		PrimaryColorDark: "#4fd1a5",
		FontStylesheets:  []types.WebURL{"/static/fonts.css"},
		FontFamily:       `"Inter", sans-serif`,
		CodeFontFamily:   "monospace",
		Stylesheets:      []types.WebURL{"https://cdn.example.com/brand.css"},
		FooterText:       "Run by <IT>",
		HidePoweredBy:    true,
	}})

	html := mustRender(t)(r.Windows())

	assert.Contains(t, html, "<title>Windows - Example VPN</title>")
	assert.Contains(t, html, `<img class="headscale-logo" src="/static/logo.svg" alt="Example VPN">`)
	assert.Contains(t, html, "connect with Example VPN:")
	assert.Contains(t, html, `<link rel="stylesheet" href="/static/fonts.css">`)
	assert.Contains(t, html, `<link rel="stylesheet" href="https://cdn.example.com/brand.css">`)
	assert.NotContains(t, html, "fonts.googleapis.com")
	assert.Contains(t, html, `<style>:root{--md-primary-fg-color:#0a7d5a;--md-accent-fg-color:color-mix(in srgb,#0a7d5a 80%,white);--hs-focus-ring:#0a7d5a;--md-text-font:"Inter", sans-serif;--md-code-font:monospace}`+
		`@media (prefers-color-scheme: dark){:root{--md-primary-fg-color:#4fd1a5;--md-accent-fg-color:color-mix(in srgb,#4fd1a5 80%,white);--hs-focus-ring:#4fd1a5}}</style>`)
	assert.Contains(t, html, "<p>Run by &lt;IT&gt;</p>")
	assert.NotContains(t, html, "Powered by")
	assert.Equal(t, "Example VPN", r.Name())

	apple := mustRender(t)(r.Apple())
	assert.Equal(t, 2, strings.Count(apple, `download="example_vpn_macos.mobileconfig"`))

	success := mustRender(t)(r.AuthSuccess(AuthSuccessResult{Heading: "Node registered", Verb: "Registered", User: "alice"}))
	assert.Contains(t, success, "Registered as <strong>alice</strong>.")
	assert.NotContains(t, success, "Getting started", "hide_powered_by hides the headscale links")

	// Order: fonts, then the built-in stylesheet, the theme, then extras.
	fonts := strings.Index(html, "/static/fonts.css")
	builtin := strings.Index(html, "/static/style.css")
	theme := strings.Index(html, "<style>")
	extra := strings.Index(html, "brand.css")
	assert.True(t, fonts < builtin && builtin < theme && theme < extra, "stylesheet order")
}

func TestThemeDefaults(t *testing.T) {
	r := newTestRenderer(t, types.WebConfig{})
	html := mustRender(t)(r.Windows())

	assert.Contains(t, html, `<img class="headscale-logo" src="/static/headscale.svg" alt="Headscale">`)
	assert.Contains(t, html, "https://fonts.googleapis.com/css2?family=Roboto")
	assert.NotContains(t, html, "<style>", "no theme overrides")
	assert.Contains(t, html, "Powered by")
	assert.NotContains(t, html, "<footer class=\"hs-footer\"><p></p>")
}

func TestThemeNoLogo(t *testing.T) {
	r := newTestRenderer(t, types.WebConfig{Theme: types.WebThemeConfig{Name: "X"}})
	html := mustRender(t)(r.Windows())

	assert.NotContains(t, html, "<img")
	assert.Contains(t, html, "<title>Windows - X</title>")
}

// TestThemeRejectsInvalid: a theme built in Go skips the loader's checks;
// New must refuse what would break out of the stylesheet or the page.
func TestThemeRejectsInvalid(t *testing.T) {
	for name, theme := range map[string]types.WebThemeConfig{
		"colour":     {PrimaryColor: "red;}body{display:none"},
		"dark":       {PrimaryColorDark: "#fff}"},
		"font":       {FontFamily: "x</style><script>"},
		"code font":  {CodeFontFamily: "x;y"},
		"logo":       {Logo: "javascript:alert(1)"},
		"stylesheet": {Stylesheets: []types.WebURL{"//evil.example.com/x.css"}},
		"font sheet": {FontStylesheets: []types.WebURL{"fonts.css"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := New(&types.Config{Web: types.WebConfig{Theme: theme}})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "web.theme.")
		})
	}
}

func TestNilRenderer(t *testing.T) {
	var r *Renderer

	html := mustRender(t)(r.AuthError(AuthErrorResult{Title: "Error", Heading: "Gone", Message: "Expired."}))
	assert.Contains(t, html, "<title>Error - Headscale</title>")
	assert.Contains(t, html, "Expired.")
	assert.Equal(t, "Headscale", r.Name())
}

func TestTemplateOverrides(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		// A page replaced outright, without the layout.
		"windows.html": `<!DOCTYPE html><p>{{.Site.Name}} on Windows: {{.Site.ServerURL}}</p>`,
		// A partial replacing one block of the layout.
		"brand.html": `{{define "footer"}}<footer>ACME</footer>{{end}}{{define "head"}}<meta name="x" content="y">{{end}}`,
		// Not templates: ignored.
		"notes.txt":    "{{",
		".hidden.html": "{{",
	})

	r := newTestRenderer(t, types.WebConfig{TemplatesDir: dir})

	assert.Equal(t, "<!DOCTYPE html><p>Headscale on Windows: https://hs.example.com</p>", mustRender(t)(r.Windows()))

	apple := mustRender(t)(r.Apple())
	assert.Contains(t, apple, "<footer>ACME</footer>")
	assert.NotContains(t, apple, "Powered by")
	assert.Contains(t, apple, `<meta name="x" content="y">`)
	assert.Contains(t, apple, "iOS configuration", "the built-in content stays")
}

// TestPageKinds: an override can word each case on its own, from Kind,
// Code and AuthID rather than the built-in English text.
func TestPageKinds(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"auth_success.html": `{{if eq .Page.Kind "reauth"}}welcome back{{else if eq .Page.Kind "ssh"}}ssh ok{{else}}registered{{end}}`,
		"auth_error.html":   `{{if eq .Page.Code 410}}link used{{else}}error {{.Page.Code}}{{end}}`,
		"auth_web.html":     `{{.Page.Kind}}:{{.Page.AuthID}}`,
	})

	r := newTestRenderer(t, types.WebConfig{TemplatesDir: dir})

	for kind, want := range map[string]string{
		AuthSuccessRegister: "registered",
		AuthSuccessReauth:   "welcome back",
		AuthSuccessSSH:      "ssh ok",
	} {
		assert.Equal(t, want, mustRender(t)(r.AuthSuccess(AuthSuccessResult{Kind: kind})), kind)
	}

	assert.Equal(t, "link used", mustRender(t)(r.AuthError(AuthErrorResult{Code: 410})))
	assert.Equal(t, "error 403", mustRender(t)(r.AuthError(AuthErrorResult{Code: 403})))
	assert.Equal(t, "auth:abc", mustRender(t)(r.AuthWeb(AuthWebData{Kind: AuthWebAuth, AuthID: "abc"})))
}

func TestGettingStartedOverride(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"brand.html": `{{define "getting_started"}}<p>Ask IT for help.</p>{{end}}`,
	})

	r := newTestRenderer(t, types.WebConfig{TemplatesDir: dir})

	html := mustRender(t)(r.AuthSuccess(AuthSuccessResult{Verb: "Registered", User: "alice"}))
	assert.Contains(t, html, "<p>Ask IT for help.</p>")
	assert.NotContains(t, html, "Getting started")
}

// TestOldComponentsOverride: a components.html copied from an older
// version, without getting_started, still loads; its own icons win.
func TestOldComponentsOverride(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"components.html": `{{define "icon_success"}}<i>ok</i>{{end}}{{define "icon_error"}}<i>x</i>{{end}}{{define "icon_warning"}}<i>!</i>{{end}}`,
	})

	r := newTestRenderer(t, types.WebConfig{TemplatesDir: dir})

	html := mustRender(t)(r.AuthSuccess(AuthSuccessResult{Verb: "Registered", User: "alice"}))
	assert.Contains(t, html, "<i>ok</i>")
	assert.NotContains(t, html, `id="checkbox"`)
	assert.Contains(t, html, "<h2>Getting started</h2>", "the built-in getting_started fills in")
}

// TestRemoveBlock: an empty definition does not replace a block, as in
// text/template; one holding an HTML comment does, and renders nothing.
func TestRemoveBlock(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"brand.html": `{{define "getting_started"}}<!-- none -->{{end}}`,
	})

	r := newTestRenderer(t, types.WebConfig{TemplatesDir: dir})

	html := mustRender(t)(r.AuthSuccess(AuthSuccessResult{Verb: "Registered", User: "alice"}))
	assert.NotContains(t, html, "Getting started")
	assert.NotContains(t, html, "none")
}

func TestProfileFileName(t *testing.T) {
	for name, want := range map[string]string{
		"Headscale":         "headscale_macos.mobileconfig",
		"Example VPN":       "example_vpn_macos.mobileconfig",
		"  ACME -- Corp! 2": "acme_corp_2_macos.mobileconfig",
		"Café Réseau":       "caf_r_seau_macos.mobileconfig",
		"":                  "headscale_macos.mobileconfig",
		"!!!":               "headscale_macos.mobileconfig",
		"Ωμέγα":             "headscale_macos.mobileconfig",
	} {
		assert.Equal(t, want, profileFileName(name), "%q", name)
	}
}

func TestLayoutOverride(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"layout.html": `{{define "layout"}}<!DOCTYPE html><title>{{template "title" .}}</title><div class="brand">{{template "content" .}}</div>{{end}}`,
	})

	r := newTestRenderer(t, types.WebConfig{TemplatesDir: dir})

	html := mustRender(t)(r.AuthError(AuthErrorResult{Title: "Error", Heading: "Gone", Message: "Expired."}))
	assert.True(t, strings.HasPrefix(html, `<!DOCTYPE html><title>Error - Headscale</title><div class="brand">`), html)
	assert.Contains(t, html, "Expired.")
}

func TestTemplateErrors(t *testing.T) {
	tests := map[string]struct {
		files   map[string]string
		wantErr []string
	}{
		"parse error names the file": {
			files:   map[string]string{"apple.html": `{{template "layout" .}}{{define "content"}}{{.Site.Name}{{end}}`},
			wantErr: []string{"apple.html", "web template"},
		},
		"missing field fails at load": {
			files:   map[string]string{"register_confirm.html": `{{.Page.Hostnme}}`},
			wantErr: []string{"register_confirm.html", "does not render", "Hostnme"},
		},
		"broken partial": {
			files:   map[string]string{"brand.html": `{{define "footer"}}{{.Site.Nope}}{{end}}`},
			wantErr: []string{"does not render", "Nope"},
		},
		"unknown function": {
			files:   map[string]string{"windows.html": `{{upper .Site.Name}}`},
			wantErr: []string{"windows.html", "upper"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := &types.Config{Web: types.WebConfig{TemplatesDir: writeFiles(t, tt.files)}}

			_, err := New(cfg)
			require.Error(t, err)

			for _, want := range tt.wantErr {
				assert.Contains(t, err.Error(), want)
			}

			assert.Equal(t, err.Error(), CheckTemplates(cfg).Error(), "headscale check reports the same")
		})
	}
}

func TestMissingDirs(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")

	_, err := New(&types.Config{Web: types.WebConfig{TemplatesDir: missing}})
	require.ErrorContains(t, err, "web.templates_dir")

	_, err = New(&types.Config{Web: types.WebConfig{StaticDir: missing}})
	require.ErrorContains(t, err, "web.static_dir")

	file := filepath.Join(writeFiles(t, map[string]string{"f": ""}), "f")
	_, err = New(&types.Config{Web: types.WebConfig{StaticDir: file}})
	require.ErrorIs(t, err, errNotDir)
}

func TestRelativeDirs(t *testing.T) {
	dir := writeFiles(t, map[string]string{"web/windows.html": "custom"})

	r, err := New(&types.Config{ConfigDir: dir, Web: types.WebConfig{TemplatesDir: "web", StaticDir: "web"}})
	require.NoError(t, err)
	assert.Equal(t, "custom", mustRender(t)(r.Windows()))
}

func TestReload(t *testing.T) {
	dir := writeFiles(t, map[string]string{"windows.html": "one"})
	r := newTestRenderer(t, types.WebConfig{TemplatesDir: dir})
	assert.Equal(t, "one", mustRender(t)(r.Windows()))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "windows.html"), []byte("two"), 0o600))
	require.NoError(t, r.Reload())
	assert.Equal(t, "two", mustRender(t)(r.Windows()))

	// A broken template keeps the ones in use.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "windows.html"), []byte("{{"), 0o600))
	require.Error(t, r.Reload())
	assert.Equal(t, "two", mustRender(t)(r.Windows()))

	// Removing the file falls back to the built-in page.
	require.NoError(t, os.Remove(filepath.Join(dir, "windows.html")))
	require.NoError(t, r.Reload())
	assert.Contains(t, mustRender(t)(r.Windows()), "Windows configuration")
}

func TestTemplatesDirSymlinkEscape(t *testing.T) {
	outside := writeFiles(t, map[string]string{"secret.html": "secret"})
	dir := t.TempDir()
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.html"), filepath.Join(dir, "windows.html")))

	_, err := New(&types.Config{Web: types.WebConfig{TemplatesDir: dir}})
	require.Error(t, err, "a symlink out of the directory is not followed")
}

func TestStatic(t *testing.T) {
	outside := writeFiles(t, map[string]string{"secret.txt": "secret"})
	dir := writeFiles(t, map[string]string{
		"style.css":       "body{color:red}",
		"brand/logo.svg":  "<svg/>",
		"fonts/a.woff2":   "font",
		".env":            "secret",
		".git/config":     "secret",
		"sub/favicon.png": "png",
	})
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "escape.txt")))

	r := newTestRenderer(t, types.WebConfig{StaticDir: dir})
	h := r.StaticHandler()

	get := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), method, path, nil))

		return rec
	}

	tests := []struct {
		path       string
		wantStatus int
		wantBody   string
		wantType   string
	}{
		{"/static/style.css", http.StatusOK, "body{color:red}", "text/css; charset=utf-8"},
		{"/static/brand/logo.svg", http.StatusOK, "<svg/>", "image/svg+xml"},
		{"/static/fonts/a.woff2", http.StatusOK, "font", "font/woff2"},
		{"/static/headscale.svg", http.StatusOK, "<svg", "image/svg+xml"},
		{"/static/headscale-dark.svg", http.StatusOK, "<svg", "image/svg+xml"},
		{"/static/favicon.png", http.StatusOK, "", "image/png"},
		{"/static/", http.StatusNotFound, "", ""},
		{"/static/brand", http.StatusNotFound, "", ""},
		{"/static/brand/", http.StatusNotFound, "", ""},
		{"/static/.env", http.StatusNotFound, "", ""},
		{"/static/.git/config", http.StatusNotFound, "", ""},
		{"/static/../secret.txt", http.StatusNotFound, "", ""},
		{"/static/escape.txt", http.StatusNotFound, "", ""},
		{"/static/missing.css", http.StatusNotFound, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := get(http.MethodGet, tt.path)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus != http.StatusOK {
				assert.NotContains(t, rec.Body.String(), "secret")
				return
			}

			assert.Contains(t, rec.Body.String(), tt.wantBody)
			assert.Equal(t, tt.wantType, rec.Header().Get("Content-Type"))
			assert.Equal(t, "public, max-age=3600", rec.Header().Get("Cache-Control"))
		})
	}

	rec := get(http.MethodPost, "/static/style.css")
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, "GET, HEAD", rec.Header().Get("Allow"))

	rec = get(http.MethodHead, "/static/style.css")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestStaticBuiltin(t *testing.T) {
	var r *Renderer

	rec := httptest.NewRecorder()
	r.StaticHandler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/static/style.css", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), ".md-typeset")

	rec = httptest.NewRecorder()
	r.ServeFavicon(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/favicon.ico", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "image/png", rec.Header().Get("Content-Type"))
}

// TestStyleVariables checks that every variable style.css reads, and every
// one the theme sets, is declared in style.css. A misspelt name in var()
// drops the whole declaration without an error in the browser.
func TestStyleVariables(t *testing.T) {
	b, err := assets.Static.ReadFile("style.css")
	require.NoError(t, err)

	css := string(b)

	declared := map[string]bool{}
	for _, m := range regexp.MustCompile(`(--[\w-]+)\s*:`).FindAllStringSubmatch(css, -1) {
		declared[m[1]] = true
	}

	require.Contains(t, declared, "--hs-space-4")

	undeclared := func(re, s string) []string {
		var missing []string

		for _, m := range regexp.MustCompile(re).FindAllStringSubmatch(s, -1) {
			if !declared[m[1]] {
				missing = append(missing, m[1])
			}
		}

		return missing
	}

	assert.Empty(t, undeclared(`var\((--[\w-]+)`, css), "style.css reads undeclared variables")

	theme := themeCSS(types.WebThemeConfig{
		PrimaryColor:     "#0a7d5a",
		PrimaryColorDark: "#4fd1a5",
		FontFamily:       "sans-serif",
		CodeFontFamily:   "monospace",
	})
	assert.Empty(t, undeclared(`(--[\w-]+):`, string(theme)), "the theme sets variables style.css does not declare")
}

func TestFaviconOverride(t *testing.T) {
	dir := writeFiles(t, map[string]string{"favicon.png": "custom"})
	r := newTestRenderer(t, types.WebConfig{StaticDir: dir})

	rec := httptest.NewRecorder()
	r.ServeFavicon(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/favicon.ico", nil))
	assert.Equal(t, "custom", rec.Body.String())
}
