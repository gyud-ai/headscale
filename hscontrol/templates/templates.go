// Package templates renders headscale's web pages from Go html/template
// files. The built-in ones are embedded from html/; a file of the same name
// in web.templates_dir replaces one, and any other .html file there is
// loaded into every page, where it can redefine a block of the layout. The
// package also serves /static/: the built-in assets, each replaceable by a
// file of the same name in web.static_dir.
package templates

import (
	"bytes"
	"cmp"
	"embed"
	"encoding"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/rs/zerolog/log"
)

//go:embed html/*.html
var builtinFiles embed.FS

var errNotDir = errors.New("is not a directory")

// Builtin is the built-in template files, the starting point for a
// web.templates_dir.
func Builtin() fs.FS {
	sub, err := fs.Sub(builtinFiles, "html")
	if err != nil {
		panic(err) // html/ is embedded above
	}

	return sub
}

// Site is the theme every page gets as .Site. Templates in
// web.templates_dir read these fields by name: add fields, and document
// them in docs/ref/branding.md.
type Site struct {
	// Name is web.theme.name, "Headscale" by default.
	Name string

	// Logo is web.theme.logo, a URL; empty shows no logo.
	Logo string

	// ServerURL is server_url, which clients log in to.
	ServerURL string

	// PrimaryColor and PrimaryColorDark are web.theme.primary_color and
	// web.theme.primary_color_dark, hex colours or empty.
	PrimaryColor     string
	PrimaryColorDark string

	// FontFamily and CodeFontFamily are web.theme.font_family and
	// web.theme.code_font_family, CSS font lists or empty.
	FontFamily     string
	CodeFontFamily string

	// FontStylesheets and Stylesheets are URLs for <link rel="stylesheet">,
	// before and after the built-in /static/style.css.
	FontStylesheets []string
	Stylesheets     []string

	// FooterText is web.theme.footer_text, plain text.
	FooterText string

	// HidePoweredBy is web.theme.hide_powered_by.
	HidePoweredBy bool

	// ThemeCSS sets the CSS variables of the colours and fonts above, for
	// a <style> element; empty when none is set.
	ThemeCSS template.CSS
}

type pageData struct {
	Site *Site
	Page any
}

type pageSet map[string]*template.Template

// Renderer renders the pages and serves /static/. A nil *Renderer uses
// the built-in templates and theme.
type Renderer struct {
	site         *Site
	templatesDir string
	staticDir    string
	pages        atomic.Pointer[pageSet]
}

var defaultRenderer = sync.OnceValue(func() *Renderer {
	cfg := types.Config{Web: types.WebConfig{Theme: types.DefaultWebTheme()}}

	r, err := New(&cfg)
	if err != nil {
		panic(fmt.Sprintf("built-in web templates: %v", err))
	}

	return r
})

func (r *Renderer) orDefault() *Renderer {
	if r == nil {
		return defaultRenderer()
	}

	return r
}

// New loads the templates of cfg.Web and renders every page once with
// sample data, so a template that cannot render fails here rather than
// on a user's login.
func New(cfg *types.Config) (*Renderer, error) {
	site, err := newSite(cfg)
	if err != nil {
		return nil, err
	}

	r := &Renderer{site: site}

	if dir := cfg.Web.StaticDir; dir != "" {
		r.staticDir = cfg.AbsPath(dir)

		err := CheckStaticDir(r.staticDir)
		if err != nil {
			return nil, fmt.Errorf("web.static_dir: %w", err)
		}
	}

	if dir := cfg.Web.TemplatesDir; dir != "" {
		r.templatesDir = cfg.AbsPath(dir)
	}

	err = r.Reload()
	if err != nil {
		return nil, err
	}

	return r, nil
}

// CheckTemplates reports whether the templates of cfg.Web load and render.
func CheckTemplates(cfg *types.Config) error {
	site, err := newSite(cfg)
	if err != nil {
		return err
	}

	dir := ""
	if cfg.Web.TemplatesDir != "" {
		dir = cfg.AbsPath(cfg.Web.TemplatesDir)
	}

	_, _, err = loadPages(site, dir)

	return err
}

// CheckStaticDir reports whether dir is a directory headscale can read.
func CheckStaticDir(dir string) error {
	st, err := os.Stat(dir)
	if err != nil {
		return err
	}

	if !st.IsDir() {
		return fmt.Errorf("%s %w", dir, errNotDir)
	}

	f, err := os.Open(dir)
	if err != nil {
		return err
	}

	return f.Close()
}

// Reload reads web.templates_dir again. On an error the templates in use
// are kept.
func (r *Renderer) Reload() error {
	pages, loaded, err := loadPages(r.site, r.templatesDir)
	if err != nil {
		return err
	}

	r.pages.Store(&pages)

	if r.templatesDir != "" {
		log.Info().
			Str("dir", r.templatesDir).
			Strs("overrides", loaded.overrides).
			Strs("partials", loaded.partials).
			Msg("loaded web templates")
	}

	return nil
}

// loadedFiles are the files of web.templates_dir by role: overrides
// replace a built-in file, partials are loaded into every page.
type loadedFiles struct {
	overrides []string
	partials  []string
}

// builtinPages are the pages without web.templates_dir, parsed once: they
// do not change, and every server, every test server included, uses them.
var builtinPages = sync.OnceValues(func() (pageSet, error) {
	pages, _, err := parsePages(&Site{}, nil, "")

	return pages, err
})

// loadPages parses every page: the built-in base files, then the partials
// of dir, then the page, each file read from dir when it is there. Each
// page is then rendered with its samples.
func loadPages(site *Site, dir string) (pageSet, loadedFiles, error) {
	if dir == "" {
		pages, err := builtinPages()

		return pages, loadedFiles{}, err
	}

	custom, err := readTemplatesDir(dir)
	if err != nil {
		return nil, loadedFiles{}, fmt.Errorf("web.templates_dir: %w", err)
	}

	return parsePages(site, custom, dir)
}

// parsePages parses every page, reading a file from custom, the files of
// dir by name, when it is there and from the built-in files otherwise.
func parsePages(site *Site, custom map[string]string, dir string) (pageSet, loadedFiles, error) {
	var loaded loadedFiles

	pageNames := slices.Sorted(maps.Keys(samples))

	for _, name := range slices.Sorted(maps.Keys(custom)) {
		if slices.Contains(pageNames, name) || slices.Contains(baseFiles, name) {
			loaded.overrides = append(loaded.overrides, name)
		} else {
			loaded.partials = append(loaded.partials, name)
		}
	}

	parse := func(t *template.Template, name string) error {
		text, origin := custom[name], filepath.Join(dir, name)
		if _, ok := custom[name]; !ok {
			b, err := builtinFiles.ReadFile("html/" + name)
			if err != nil {
				return err
			}

			text, origin = string(b), "built-in "+name
		}

		_, err := t.New(name).Parse(text)
		if err != nil {
			return fmt.Errorf("web template %s: %w", origin, err)
		}

		return nil
	}

	base := template.New("").Funcs(template.FuncMap{"join": strings.Join})
	for _, name := range slices.Concat(baseFiles, loaded.partials) {
		err := parse(base, name)
		if err != nil {
			return nil, loaded, err
		}
	}

	pages := pageSet{}

	for _, name := range pageNames {
		t, err := base.Clone()
		if err != nil {
			return nil, loaded, err
		}

		err = parse(t, name)
		if err != nil {
			return nil, loaded, err
		}

		for _, sample := range samples[name] {
			err := t.ExecuteTemplate(io.Discard, name, pageData{Site: site, Page: sample})
			if err != nil {
				return nil, loaded, fmt.Errorf("web template %s does not render: %w", name, err)
			}
		}

		pages[name] = t
	}

	return pages, loaded, nil
}

// readTemplatesDir returns the .html files of dir by name; none for "".
// Files are read through an [os.Root], so a symlink cannot reach outside
// dir.
func readTemplatesDir(dir string) (map[string]string, error) {
	if dir == "" {
		return nil, nil //nolint:nilnil // no directory, no files
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	fsys := root.FS()

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}

	files := map[string]string{}

	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || filepath.Ext(name) != ".html" {
			continue
		}

		st, err := fs.Stat(fsys, name)
		if err != nil {
			return nil, err
		}

		if !st.Mode().IsRegular() {
			continue
		}

		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}

		files[name] = string(b)
	}

	return files, nil
}

// themeCheck is a theme value and the type that validates it.
type themeCheck struct {
	key string
	val string
	dst encoding.TextUnmarshaler
}

// newSite builds the theme of cfg. The loader has validated it; values
// set from Go are checked again, as ThemeCSS writes them into a
// stylesheet unescaped.
func newSite(cfg *types.Config) (*Site, error) {
	theme := cfg.Web.Theme

	checks := make([]themeCheck, 0, 5+len(theme.FontStylesheets)+len(theme.Stylesheets)) //nolint:mnd // the five below
	checks = append(checks,
		themeCheck{"logo", string(theme.Logo), new(types.WebURL)},
		themeCheck{"primary_color", string(theme.PrimaryColor), new(types.CSSColor)},
		themeCheck{"primary_color_dark", string(theme.PrimaryColorDark), new(types.CSSColor)},
		themeCheck{"font_family", string(theme.FontFamily), new(types.FontFamily)},
		themeCheck{"code_font_family", string(theme.CodeFontFamily), new(types.FontFamily)},
	)

	for _, u := range theme.FontStylesheets {
		checks = append(checks, themeCheck{"font_stylesheets", string(u), new(types.WebURL)})
	}

	for _, u := range theme.Stylesheets {
		checks = append(checks, themeCheck{"stylesheets", string(u), new(types.WebURL)})
	}

	for _, c := range checks {
		err := c.dst.UnmarshalText([]byte(c.val))
		if err != nil {
			return nil, fmt.Errorf("web.theme.%s %q: %w", c.key, c.val, err)
		}
	}

	urls := func(us []types.WebURL) []string {
		out := make([]string, len(us))
		for i, u := range us {
			out[i] = string(u)
		}

		return out
	}

	return &Site{
		Name:             cmp.Or(theme.Name, types.DefaultWebTheme().Name),
		Logo:             string(theme.Logo),
		ServerURL:        cfg.ServerURL,
		PrimaryColor:     string(theme.PrimaryColor),
		PrimaryColorDark: string(theme.PrimaryColorDark),
		FontFamily:       string(theme.FontFamily),
		CodeFontFamily:   string(theme.CodeFontFamily),
		FontStylesheets:  urls(theme.FontStylesheets),
		Stylesheets:      urls(theme.Stylesheets),
		FooterText:       theme.FooterText,
		HidePoweredBy:    theme.HidePoweredBy,
		ThemeCSS:         themeCSS(theme),
	}, nil
}

// themeCSS overrides the variables of style.css that theme sets. A rule
// later in the page wins over the built-in light and dark ones alike, so
// a primary colour without a dark one applies to both schemes.
func themeCSS(theme types.WebThemeConfig) template.CSS {
	colors := func(c types.CSSColor) []string {
		return []string{
			"--md-primary-fg-color:" + string(c),
			"--md-accent-fg-color:" + string(c),
			"--hs-focus-ring:" + string(c),
		}
	}

	var root []string
	if c := theme.PrimaryColor; c != "" {
		root = append(root, colors(c)...)
	}

	if f := theme.FontFamily; f != "" {
		root = append(root, "--md-text-font:"+string(f))
	}

	if f := theme.CodeFontFamily; f != "" {
		root = append(root, "--md-code-font:"+string(f))
	}

	var b strings.Builder
	if len(root) > 0 {
		b.WriteString(":root{" + strings.Join(root, ";") + "}")
	}

	if c := theme.PrimaryColorDark; c != "" {
		b.WriteString("@media (prefers-color-scheme: dark){:root{" + strings.Join(colors(c), ";") + "}}")
	}

	return template.CSS(b.String()) //nolint:gosec // G203: built from validated colours and font lists only
}

func (r *Renderer) render(name string, page any) ([]byte, error) {
	r = r.orDefault()

	var buf bytes.Buffer

	err := (*r.pages.Load())[name].ExecuteTemplate(&buf, name, pageData{Site: r.site, Page: page})
	if err != nil {
		return nil, fmt.Errorf("rendering %s: %w", name, err)
	}

	return buf.Bytes(), nil
}

// Apple renders /apple.
func (r *Renderer) Apple() ([]byte, error) { return r.render(pageApple, nil) }

// Windows renders /windows.
func (r *Renderer) Windows() ([]byte, error) { return r.render(pageWindows, nil) }

// Blank renders /.
func (r *Renderer) Blank() ([]byte, error) { return r.render(pageBlank, nil) }

// AuthWeb renders the page that names the command approving a
// registration or an auth request.
func (r *Renderer) AuthWeb(d AuthWebData) ([]byte, error) { return r.render(pageAuthWeb, d) }

// AuthSuccess renders the page after a confirmed login.
func (r *Renderer) AuthSuccess(d AuthSuccessResult) ([]byte, error) {
	return r.render(pageAuthSuccess, d)
}

// AuthError renders the page of a failed browser step.
func (r *Renderer) AuthError(d AuthErrorResult) ([]byte, error) {
	return r.render(pageAuthError, d)
}

// RegisterConfirm renders the page that asks the user to confirm that a
// registering device is theirs.
func (r *Renderer) RegisterConfirm(d RegisterConfirmInfo) ([]byte, error) {
	return r.render(pageRegisterConfirm, d)
}

// SSHCheckConfirm renders the page that asks the owner to approve an SSH
// session.
func (r *Renderer) SSHCheckConfirm(d SSHCheckData) ([]byte, error) {
	return r.render(pageSSHCheckConfirm, d)
}

// Ping renders /debug/ping.
func (r *Renderer) Ping(d PingData) ([]byte, error) {
	if d.Result != nil {
		res := *d.Result
		res.Latency = res.Latency.Round(pingLatencyPrecision)
		d.Result = &res
	}

	return r.render(pagePing, d)
}

// Name is the site name, web.theme.name.
func (r *Renderer) Name() string { return r.orDefault().site.Name }
