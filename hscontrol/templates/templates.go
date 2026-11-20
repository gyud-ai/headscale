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
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/rs/zerolog/log"
)

//go:embed html/*.html
var builtinFiles embed.FS

var (
	errNotDir         = errors.New("is not a directory")
	errLogoDarkNoLogo = errors.New("needs web.theme.logo: the dark logo replaces it")
)

// builtinLogo and builtinLogoDark are the built-in logo's files under
// [StaticPrefix], for a light and a dark colour scheme.
const (
	builtinLogo     = "headscale.svg"
	builtinLogoDark = "headscale-dark.svg"
)

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

	// LogoDark is the logo for a dark colour scheme: web.theme.logo_dark,
	// or the built-in dark logo with the built-in logo. Empty uses Logo.
	// Commas and whitespace are percent-encoded, for a srcset attribute.
	LogoDark string

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

	// ProfileFileName is the file name the macOS configuration profiles
	// download as, from Name: "headscale_macos.mobileconfig" by default.
	ProfileFileName string

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
	text         types.WebTextConfig
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

	text, err := newText(cfg)
	if err != nil {
		return nil, err
	}

	r := &Renderer{site: site, text: text}

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

	_, err = newText(cfg)
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

	// A components.html of its own replaces only the components it
	// defines: the built-in ones go first, so a copy from an older
	// version still has the components added since.
	if _, ok := custom[componentsFile]; ok {
		b, err := builtinFiles.ReadFile("html/" + componentsFile)
		if err != nil {
			return nil, loaded, err
		}

		_, err = base.New("built-in " + componentsFile).Parse(string(b))
		if err != nil {
			return nil, loaded, fmt.Errorf("web template built-in %s: %w", componentsFile, err)
		}
	}

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

	checks := make([]themeCheck, 0, 8+len(theme.FontStylesheets)+len(theme.Stylesheets)) //nolint:mnd // the eight below
	checks = append(checks,
		themeCheck{"name", string(theme.Name), new(types.PlainText)},
		themeCheck{"footer_text", string(theme.FooterText), new(types.PlainText)},
		themeCheck{"logo", string(theme.Logo), new(types.WebURL)},
		themeCheck{"logo_dark", string(theme.LogoDark), new(types.WebURL)},
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

	if theme.LogoDark != "" && theme.Logo == "" {
		return nil, fmt.Errorf("web.theme.logo_dark %q: %w", theme.LogoDark, errLogoDarkNoLogo)
	}

	urls := func(us []types.WebURL) []string {
		out := make([]string, len(us))
		for i, u := range us {
			out[i] = string(u)
		}

		return out
	}

	return &Site{
		Name:             string(cmp.Or(theme.Name, types.DefaultWebTheme().Name)),
		Logo:             string(theme.Logo),
		LogoDark:         logoDark(cfg),
		ServerURL:        cfg.ServerURL,
		PrimaryColor:     string(theme.PrimaryColor),
		PrimaryColorDark: string(theme.PrimaryColorDark),
		FontFamily:       string(theme.FontFamily),
		CodeFontFamily:   string(theme.CodeFontFamily),
		FontStylesheets:  urls(theme.FontStylesheets),
		Stylesheets:      urls(theme.Stylesheets),
		FooterText:       string(theme.FooterText),
		HidePoweredBy:    theme.HidePoweredBy,
		ProfileFileName:  profileFileName(string(cmp.Or(theme.Name, types.DefaultWebTheme().Name))),
		ThemeCSS:         themeCSS(theme),
	}, nil
}

// newText is the page text of cfg, with the built-in text where a key is
// empty. The loader has validated it; text set from Go is checked again.
func newText(cfg *types.Config) (types.WebTextConfig, error) {
	text := cfg.Web.Text.WithDefaults()

	for _, f := range types.ConfigFields(&types.Config{Web: types.WebConfig{Text: text}}) {
		if !strings.HasPrefix(f.Path, "web.text.") {
			continue
		}

		err := new(types.PageText).UnmarshalText([]byte(f.Value.String()))
		if err != nil {
			return types.WebTextConfig{}, fmt.Errorf("%s %q: %w", f.Path, f.Value.String(), err)
		}
	}

	return text, nil
}

// srcsetEscaper percent-encodes the characters that split a srcset
// attribute into candidates, so a URL with them stays one URL; a server
// reads the encoded URL the same.
var srcsetEscaper = strings.NewReplacer(",", "%2C", " ", "%20", "\t", "%09", "\n", "%0A", "\r", "%0D", "\f", "%0C")

// logoDark is the logo of cfg for a dark colour scheme: logo_dark, else
// the built-in dark logo when logo is the built-in one. A web.static_dir
// that replaces headscale.svg without a headscale-dark.svg keeps its
// logo in both schemes.
func logoDark(cfg *types.Config) string {
	theme := cfg.Web.Theme
	if theme.LogoDark != "" || theme.Logo != types.DefaultWebTheme().Logo {
		return srcsetEscaper.Replace(string(theme.LogoDark))
	}

	if dir := cfg.Web.StaticDir; dir != "" {
		has := func(name string) bool {
			_, err := os.Stat(filepath.Join(cfg.AbsPath(dir), name))

			return err == nil
		}

		if has(builtinLogo) && !has(builtinLogoDark) {
			return ""
		}
	}

	return StaticPrefix + builtinLogoDark
}

// profileFileName turns a site name into a file name for the macOS
// profiles: lower case, every run of characters other than a-z and 0-9
// one underscore. A name with none of those falls back to "headscale".
func profileFileName(name string) string {
	var (
		b   strings.Builder
		sep bool
	)

	for _, r := range strings.ToLower(name) {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			sep = true

			continue
		}

		if sep && b.Len() > 0 {
			b.WriteByte('_')
		}

		sep = false

		b.WriteRune(r)
	}

	return cmp.Or(b.String(), "headscale") + "_macos.mobileconfig"
}

// themeCSS overrides the variables of style.css that theme sets. A rule
// later in the page wins over the built-in light and dark ones alike, so
// a primary colour without a dark one applies to both schemes.
func themeCSS(theme types.WebThemeConfig) template.CSS {
	// The text on the primary colour is black or white, whichever
	// contrasts more. The hover colour is the primary colour mixed with
	// the other one, so links and buttons still change colour on hover and
	// the button text contrasts more on hover, not less.
	colors := func(c types.CSSColor) []string {
		on, away := "#ffffff", "black"
		if !whiteOn(string(c)) {
			on, away = "#000000", "white"
		}

		return []string{
			"--md-primary-fg-color:" + string(c),
			"--md-accent-fg-color:color-mix(in srgb," + string(c) + " 80%," + away + ")",
			"--hs-focus-ring:" + string(c),
			"--hs-on-primary:" + on,
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

// whiteOn reports whether white text contrasts more than black on the
// hex colour c, by the WCAG contrast ratio. The alpha of #rgba and
// #rrggbbaa is ignored: what shows through is not known here. One of
// the two is always at least 4.5:1, the WCAG minimum for text.
func whiteOn(c string) bool {
	l := luminance(c)

	return 1.05/(l+0.05) >= (l+0.05)/0.05 //nolint:mnd // WCAG contrast ratio with white (1) and black (0)
}

// luminance is the WCAG relative luminance of the hex colour c, 0 for
// black to 1 for white. c has been validated as a CSSColor.
func luminance(c string) float64 {
	hex := strings.TrimPrefix(c, "#")
	if len(hex) <= 4 { //nolint:mnd // #rgb and #rgba
		var b strings.Builder
		for _, r := range hex {
			b.WriteRune(r)
			b.WriteRune(r)
		}

		hex = b.String()
	}

	channel := func(i int) float64 {
		v, _ := strconv.ParseUint(hex[i:i+2], 16, 8)

		s := float64(v) / 255 //nolint:mnd // 8-bit channel
		if s <= 0.04045 {     //nolint:mnd // sRGB transfer function
			return s / 12.92 //nolint:mnd // sRGB transfer function
		}

		return math.Pow((s+0.055)/1.055, 2.4) //nolint:mnd // sRGB transfer function
	}

	return 0.2126*channel(0) + 0.7152*channel(2) + 0.0722*channel(4) //nolint:mnd // WCAG luminance weights
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
// registration or an auth request. Its title and description are the
// web.text of d.Kind.
func (r *Renderer) AuthWeb(d AuthWebData) ([]byte, error) {
	r = r.orDefault()

	t := r.text.AuthWeb.Register
	if d.Kind == AuthWebAuth {
		t = r.text.AuthWeb.Auth
	}

	d.Title, d.Description = r.expand(t.Title), r.expand(t.Description)

	return r.render(pageAuthWeb, d)
}

// AuthSuccess renders the page after a confirmed login. Its text is the
// web.text of d.Kind.
func (r *Renderer) AuthSuccess(d AuthSuccessResult) ([]byte, error) {
	r = r.orDefault()

	var t types.WebSuccessText

	switch d.Kind {
	case AuthSuccessReauth:
		t = r.text.AuthSuccess.Reauth
	case AuthSuccessSSH:
		t = r.text.AuthSuccess.SSH
	default:
		t = r.text.AuthSuccess.Register
	}

	d.Title, d.Heading, d.Verb, d.Message = r.expand(t.Title), r.expand(t.Heading), r.expand(t.Verb), r.expand(t.Message)

	return r.render(pageAuthSuccess, d)
}

// AuthError renders the page of a failed browser step. Its text is the
// web.text of d.Reason; an empty reason is derived from d.Code.
func (r *Renderer) AuthError(d AuthErrorResult) ([]byte, error) {
	r = r.orDefault()

	if d.Reason == "" {
		d.Reason = AuthErrorReason(d.Code)
	}

	e := r.text.AuthError

	var t types.WebErrorText

	switch d.Reason {
	case AuthErrorUnauthorized:
		t = e.Unauthorized
	case AuthErrorExpired:
		t = e.Expired
	case AuthErrorBadRequest:
		t = e.BadRequest
	case AuthErrorLinkSpent:
		t = e.LinkSpent
	case AuthErrorNodeReportTooLarge:
		t = e.NodeReportTooLarge
	default:
		t = e.ServerError
	}

	d.Title, d.Heading, d.Message = r.expand(e.Title), r.expand(t.Heading), r.expand(t.Message)

	return r.render(pageAuthError, d)
}

// expand is t with {name} replaced by the site name.
func (r *Renderer) expand(t types.PageText) string { return t.Expand(r.site.Name) }

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
