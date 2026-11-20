---
# The templates below are Go templates, not macros for this site.
render_macros: false
---

# Branding

Headscale serves a few pages to browsers: the client setup pages `/apple` and `/windows`, the pages of a
[registration](registration.md) or an [OpenID Connect](oidc.md) login, and `/`. They can carry your own name, logo,
colours and fonts, and their HTML can be replaced entirely, without rebuilding headscale. There are three levels, each
building on the previous one:

1. A **theme**, set in the configuration file: name, logo, colours, fonts and footer.
1. **Static files**: your logo, stylesheets and fonts, served by headscale under `/static/`.
1. **Templates**: replace one block of the layout, a whole page, or the layout of every page.

All three are read from the `web` section of the [configuration file](configuration.md). See the
[configuration options](configuration-options.md) for every key.

## Theme

```yaml title="config.yaml"
web:
  theme:
    name: "Example VPN"
    logo: "https://cdn.example.com/example-vpn.svg"
    logo_dark: "https://cdn.example.com/example-vpn-dark.svg"
    primary_color: "#0a7d5a"
    primary_color_dark: "#4fd1a5"
    footer_text: "Operated by the Example IT team"
    hide_powered_by: true
```

- `name` replaces "Headscale" in page titles, page text, and the display name and download file name of the Apple
  configuration profile (`example_vpn_macos.mobileconfig` for "Example VPN").
- `logo` is the image at the top of every page. Set it to `""` to show no logo. `logo_dark` replaces it when the browser
  asks for a dark colour scheme; without it, `logo` is used for both. The built-in logo has a built-in dark version, so
  `logo_dark` matters only for a logo of your own. Use two files rather than an SVG with its own
  `prefers-color-scheme` rule: shown as an image, it can pick its colour scheme apart from the page.
- `primary_color` colours links, buttons and focus rings. `primary_color_dark` replaces it when the browser asks for a
  dark colour scheme; without it, `primary_color` is used for both. The text on buttons is black or white, whichever
  contrasts more with the colour (always at least the 4.5:1 [WCAG](https://www.w3.org/TR/WCAG22/#contrast-minimum)
  asks for text). On hover, links and buttons take a mix of the colour towards the other one, so the button text
  contrasts more on hover, not less. A colour with an alpha channel is judged as if opaque.
- `footer_text` is a line of plain text at the bottom of every page. `hide_powered_by` removes the "Powered by
  Headscale" link below it, and the "Getting started" links to the headscale and Tailscale documentation on the login
  success page.

### Fonts

By default the pages load Roboto and Roboto Mono from Google Fonts. `font_stylesheets` lists the stylesheets that load
fonts, and `font_family` and `code_font_family` choose them:

=== "Another CDN"

    ```yaml title="config.yaml"
    web:
      theme:
        font_stylesheets:
          - "https://fonts.bunny.net/css?family=inter:400,500,700"
        font_family: '"Inter", sans-serif'
    ```

=== "Self-hosted"

    ```yaml title="config.yaml"
    web:
      static_dir: /etc/headscale/web/static
      theme:
        font_stylesheets:
          - "/static/fonts/inter.css"
        font_family: '"Inter", sans-serif'
    ```

    With `/etc/headscale/web/static/fonts/inter.css` declaring the font files next to it:

    ```css title="fonts/inter.css"
    @font-face {
      font-family: "Inter";
      src: url("inter.woff2") format("woff2");
      font-weight: 100 900;
    }
    ```

=== "System fonts only"

    ```yaml title="config.yaml"
    web:
      theme:
        font_stylesheets: []
    ```

    The pages then make no request to a font service.

## Static files

`static_dir` is a directory served under `/static/`. A file named like a built-in one replaces it:

| File                 | Built-in use                                          |
| -------------------- | ----------------------------------------------------- |
| `style.css`          | The stylesheet of every page                          |
| `headscale.svg`      | The default `logo`                                    |
| `headscale-dark.svg` | The default `logo` in a dark colour scheme            |
| `favicon.png`        | The icon of every page, also served at `/favicon.ico` |

Any other file is served as is, for example a logo, a stylesheet or fonts. Files are read on each request, so a change
needs no restart. Directories are not listed, files whose name starts with a dot are not served, and symbolic links may
not lead outside the directory.

`theme.stylesheets` adds stylesheets after the built-in one, so their rules win without replacing `style.css`:

```yaml title="config.yaml"
web:
  static_dir: /etc/headscale/web/static
  theme:
    logo: "/static/logo.svg"
    stylesheets:
      - "/static/brand.css"
```

The built-in stylesheet styles the page parts through classes, such as `.hs-box`, `.hs-device-table` and `.hs-footer`,
and reads its colours and sizes from the CSS variables below. A stylesheet in `theme.stylesheets` can set a variable to
change every rule that uses it, or override a rule to change one place:

```css title="brand.css"
:root {
  --hs-radius-md: 0; /* square buttons and inputs */
  --hs-page-width: 960px;
  --hs-logo-width: 240px;
}
```

### Stylesheet variables

Variables named `--md-*` come from [Material for MkDocs](https://squidfunk.github.io/mkdocs-material/), whose look the
pages share, and keep its names; the others are named `--hs-*`. The `theme` settings set the variables marked with
the setting. A size in `em` is relative to the font size of the element that uses it, so `--hs-flow-space-sm` is
narrower between the footer lines, whose text is smaller, than between list items.

Colours, with the light and the dark default (`prefers-color-scheme: dark`):

| Variable                          | Light                 | Dark                        | Used for                                        |
| --------------------------------- | --------------------- | --------------------------- | ----------------------------------------------- |
| `--md-primary-fg-color`           | `#4051b5`             | `#7b8fdb`                   | Links and buttons (`theme.primary_color`)       |
| `--md-accent-fg-color`            | `#526cfe`             | `#8fa4ff`                   | Hover colour (mixed from `theme.primary_color`) |
| `--md-typeset-a-color`            | primary colour        | primary colour              | Links                                           |
| `--hs-focus-ring`                 | `#4051b5`             | `#7b8fdb`                   | Keyboard focus ring (`theme.primary_color`)     |
| `--hs-on-primary`                 | `#ffffff`             | `#000000`                   | Button text (picked for `theme.primary_color`)  |
| `--md-default-fg-color`           | `rgba(0, 0, 0, 0.87)` | `rgba(255, 255, 255, 0.87)` | Text                                            |
| `--md-default-fg-color--light`    | `rgba(0, 0, 0, 0.54)` | `rgba(255, 255, 255, 0.6)`  | Headings, footer, table labels                  |
| `--md-default-fg-color--lighter`  | `rgba(0, 0, 0, 0.32)` | `rgba(255, 255, 255, 0.38)` | Not used by the built-in pages                  |
| `--md-default-fg-color--lightest` | `rgba(0, 0, 0, 0.07)` | `rgba(255, 255, 255, 0.07)` | Not used by the built-in pages                  |
| `--md-code-fg-color`              | `#36464e`             | `#c9d1d9`                   | Code text                                       |
| `--md-code-bg-color`              | `#f5f5f5`             | `#1e1e1e`                   | Code background                                 |
| `--hs-bg`                         | `#ffffff`             | `#111827`                   | Page, input and collapsible-section background  |
| `--hs-border`                     | `#e5e7eb`             | `#374151`                   | Borders, table rules, the "or" divider          |
| `--hs-success`, `--hs-success-bg` | `#059669`, `#d1fae5`  | `#34d399`, `#064e3b`        | Success box                                     |
| `--hs-error`, `--hs-error-bg`     | `#dc2626`, `#fee2e2`  | `#f87171`, `#450a0a`        | Error box                                       |
| `--hs-warning-text`               | `#92400e`             | `#fbbf24`                   | Warning box text                                |
| `--hs-warning-bg`                 | `#fef3c7`             | `#451a03`                   | Warning box background                          |
| `--hs-warning-border`             | `#f59e0b`             | `#d97706`                   | Warning box border and icon                     |

Fonts:

| Variable         | Default                               | Used for                        |
| ---------------- | ------------------------------------- | ------------------------------- |
| `--md-text-font` | `"Roboto"`, then system fonts         | Text (`theme.font_family`)      |
| `--md-code-font` | `"Roboto Mono"`, then monospace fonts | Code (`theme.code_font_family`) |

Sizes, the same in light and dark:

| Variable                                                                         | Default                                         | Used for                                                         |
| -------------------------------------------------------------------------------- | ----------------------------------------------- | ---------------------------------------------------------------- |
| `--hs-space-1`, `-2`, `-3`, `-4`, `-6`, `-8`, `-12`, `-16`                       | `0.25rem` to `4rem`: the number times `0.25rem` | Paddings, gaps and margins of boxes, buttons, forms and the page |
| `--hs-flow-space`                                                                | `1em`                                           | Space above and below paragraphs, lists and code blocks          |
| `--hs-flow-space-sm`                                                             | `0.25em`                                        | Space between list items and footer lines                        |
| `--hs-flow-space-lg`                                                             | `1.5em`                                         | Space below the device table                                     |
| `--hs-list-indent`                                                               | `2em`                                           | List indent                                                      |
| `--hs-font-size`                                                                 | `1rem`                                          | Text and buttons; below `1rem`, iOS zooms into inputs            |
| `--hs-font-size-sm`                                                              | `0.8em`                                         | Footer                                                           |
| `--hs-font-size-label`                                                           | `0.8rem`                                        | The "or" divider                                                 |
| `--hs-font-size-code`                                                            | `0.85em`                                        | Code                                                             |
| `--hs-font-size-lg`, `-xl`, `-2xl`                                               | `1.25em`, `1.5625em`, `2em`                     | `h3` and box headings, `h2`, `h1`                                |
| `--hs-font-weight-light`, `-regular`, `-medium`, `-semibold`, `-bold`            | `300`, `400`, `500`, `600`, `700`               | `h1` and `h2`, `h3`, buttons, table labels, box headings         |
| `--hs-line-height`                                                               | `1.6`                                           | Text                                                             |
| `--hs-line-height-code`                                                          | `1.4`                                           | Code blocks                                                      |
| `--hs-line-height-h1`, `-h2`, `-h3`                                              | `1.3`, `1.4`, `1.5`                             | Headings                                                         |
| `--hs-letter-spacing-heading`, `--hs-letter-spacing-label`                       | `-0.01em`, `0.05em`                             | Headings, the "or" divider                                       |
| `--hs-margin-h1`, `--hs-margin-h2`, `--hs-margin-h3`                             | `0 0 1.25em`, `2.4em 0 0.64em`, `2em 0 0.8em`   | Heading margins                                                  |
| `--hs-margin-h1-top`                                                             | `2em`                                           | Space above an `h1` that is not the first element                |
| `--hs-radius-xs`, `-sm`, `-md`, `-lg`                                            | `0.1rem`, `0.25rem`, `0.375rem`, `0.5rem`       | Inline code, code blocks, buttons and inputs, boxes              |
| `--hs-border-width`                                                              | `1px`                                           | Borders, table rules, the "or" divider                           |
| `--hs-link-underline`, `--hs-link-underline-hover`, `--hs-link-underline-offset` | `1px`, `2px`, `2px`                             | Link underline                                                   |
| `--hs-focus-ring-width`, `--hs-focus-ring-offset`, `--hs-focus-ring-radius`      | `2px`                                           | Keyboard focus ring                                              |
| `--hs-page-width`                                                                | `800px`                                         | Page width, at most 90% of the window                            |
| `--hs-logo-width`, `--hs-logo-width-mobile`                                      | `400px`, `200px`                                | Logo width, above and below a `768px` wide window                |
| `--hs-input-width`                                                               | `280px`                                         | Text inputs                                                      |
| `--hs-touch-target`                                                              | `44px`                                          | Minimum height of buttons and button links                       |
| `--hs-code-padding`, `--hs-code-block-padding`                                   | `0 0.29em`, `0.77em 1.18em`                     | Inline code, code blocks                                         |
| `--hs-button-padding`                                                            | `var(--hs-space-3) var(--hs-space-6)`           | Buttons and button links                                         |
| `--hs-button-shadow`                                                             | `0 1px 3px rgba(0, 0, 0, 0.12)`                 | Button shadow on hover                                           |
| `--hs-transition`                                                                | `150ms ease-out`                                | Button colour change                                             |

The `768px` breakpoint stays in the rule: CSS does not allow variables in a media query.

### One colour scheme

The built-in stylesheet has a light and a dark set of colours. For one look in both, set the colours in `:root` outside
any `@media (prefers-color-scheme: dark)` rule: a stylesheet in `theme.stylesheets` comes later, so its values win over
the built-in light and dark ones alike. A colour you leave out keeps its built-in light and dark values. In the
`theme`, set `primary_color` without `primary_color_dark`, and `logo` without `logo_dark`.

```yaml title="config.yaml"
web:
  static_dir: /etc/headscale/web/static
  theme:
    logo: "/static/headscale-dark.svg" # the built-in logo for dark backgrounds, in both schemes
    primary_color: "#cba6f7"
    stylesheets:
      - "/static/dark.css"
```

```css title="dark.css"
/* Always dark, whatever the browser asks for */
:root {
  color-scheme: dark; /* scrollbars and form controls */
  --hs-bg: #1e1e2e;
  --hs-border: #45475a;
  --md-default-fg-color: #cdd6f4;
  --md-default-fg-color--light: #bac2de;
  --md-code-fg-color: #cdd6f4;
  --md-code-bg-color: #181825;
  --hs-success: #a6e3a1;
  --hs-success-bg: #1f3329;
  --hs-error: #f38ba8;
  --hs-error-bg: #3b2030;
  --hs-warning-text: #f9e2af;
  --hs-warning-bg: #3a3226;
  --hs-warning-border: #fab387;
}
```

!!! warning "Reverse proxy"

    Pages link `/static/` and `/favicon.ico` on the headscale server. A [reverse proxy](integration/reverse-proxy.md)
    must forward them.

## Templates

The pages are [Go `html/template`](https://pkg.go.dev/html/template) files. `templates_dir` is a directory whose
`.html` files replace the built-in ones by name; any built-in file you do not replace stays in use.

```yaml title="config.yaml"
web:
  templates_dir: /etc/headscale/web/templates
```

To start, write the built-in templates and static files to a directory, then delete every file you do not change. A file
left in place replaces the built-in one, so it stops following headscale upgrades.

```console
headscale generate web /etc/headscale/web
```

| File                     | Page                                                                                   |
| ------------------------ | -------------------------------------------------------------------------------------- |
| `layout.html`            | The frame of every page but `blank.html`, with the blocks below                        |
| `components.html`        | The logo and the icons the pages share                                                 |
| `apple.html`             | `/apple`, setting up iOS, macOS and tvOS                                               |
| `windows.html`           | `/windows`, setting up Windows                                                         |
| `auth_web.html`          | `/register/<id>` and `/auth/<id>` without OpenID Connect: the command that approves it |
| `register_confirm.html`  | OpenID Connect: confirming that a registering device is yours                          |
| `ssh_check_confirm.html` | OpenID Connect: approving an SSH session held by a `check` rule                        |
| `auth_success.html`      | After a confirmed login                                                                |
| `auth_error.html`        | A failed login or confirmation                                                         |
| `blank.html`             | `/`, empty by default                                                                  |
| `ping.html`              | `/debug/ping` on `metrics_listen_addr`                                                 |

### Replacing one block

Every other `.html` file in `templates_dir` is loaded into every page, after the layout. Define one of these blocks
there to replace it on every page, and keep the rest of the built-in layout:

| Block             | Default                                                                      |
| ----------------- | ---------------------------------------------------------------------------- |
| `title`           | The page title; each page sets it                                            |
| `head`            | Empty; extra tags at the end of `<head>`                                     |
| `header`          | The logo: the `logo` component, a `<picture>` when there is a dark logo      |
| `content`         | The page itself; each page sets it                                           |
| `footer`          | `footer_text` and the "Powered by Headscale" link                            |
| `getting_started` | The documentation links on `auth_success.html`; empty with `hide_powered_by` |

```html title="brand.html"
{{define "header"}}
<a href="https://example.com"><img class="headscale-logo" src="/static/logo.svg" alt="{{.Site.Name}}"></a>
{{end}}

{{define "footer"}}
<footer class="hs-footer">
  <p>Example VPN · <a href="https://example.com/support">Support</a></p>
</footer>
{{end}}
```

To remove a block, define it with an HTML comment. An empty definition, `{{define "footer"}}{{end}}`, leaves the
built-in block in place, as Go templates do; a comment replaces it and is not sent to the browser:

```html title="brand.html"
{{define "getting_started"}}<!-- none -->{{end}}
```

A `components.html` of your own replaces only the components it defines. A copy made by an older
`headscale generate web` keeps working: components added since come from the built-in file.

### Replacing a page

A file named like a page replaces it. It may use the layout, defining its `title` and `content` blocks, or write the
whole document. A landing page at `/`:

```html title="blank.html"
{{template "layout" .}}
{{define "title"}}{{.Site.Name}}{{end}}
{{define "content"}}
<h1>Welcome to {{.Site.Name}}</h1>
<p>Set up <a href="/apple">Apple devices</a> or <a href="/windows">Windows</a>.</p>
{{end}}
```

### Template data

Every template gets the theme as `.Site` and the page's own data as `.Page`. Values are escaped for where they appear,
as `html/template` does: a hostname a device reports cannot add markup to the page.

| `.Site` field                      | Value                                                                                           |
| ---------------------------------- | ----------------------------------------------------------------------------------------------- |
| `Name`, `Logo`, `FooterText`       | `theme.name`, `theme.logo`, `theme.footer_text`                                                 |
| `LogoDark`                         | `theme.logo_dark`, or the built-in dark logo with the built-in `logo`; URL-encoded for `srcset` |
| `HidePoweredBy`                    | `theme.hide_powered_by`                                                                         |
| `ServerURL`                        | `server_url`                                                                                    |
| `PrimaryColor`, `PrimaryColorDark` | `theme.primary_color`, `theme.primary_color_dark`                                               |
| `FontFamily`, `CodeFontFamily`     | `theme.font_family`, `theme.code_font_family`                                                   |
| `FontStylesheets`, `Stylesheets`   | `theme.font_stylesheets`, `theme.stylesheets`, lists of URLs                                    |
| `ThemeCSS`                         | The CSS variables the colours and fonts above set, for `<style>`                                |
| `ProfileFileName`                  | The download name of the Apple profiles, from `theme.name`                                      |

| Page                                       | `.Page` fields                                                                                             |
| ------------------------------------------ | ---------------------------------------------------------------------------------------------------------- |
| `apple.html`, `windows.html`, `blank.html` | None                                                                                                       |
| `auth_web.html`                            | `Title`, `Description`, `Command`, `Kind`, `AuthID`                                                        |
| `register_confirm.html`                    | `FormAction`, `CSRFTokenName`, `CSRFToken`, `User`, `Hostname`, `OS`, `MachineKey`, `ExistingNode`, `Tags` |
| `ssh_check_confirm.html`                   | `Source`, `Destination`, and the fields of `register_confirm.html`                                         |
| `auth_success.html`                        | `Title`, `Heading`, `Verb`, `User`, `Message`, `Kind`                                                      |
| `auth_error.html`                          | `Title`, `Heading`, `Message`, `Code`                                                                      |
| `ping.html`                                | `Query`, `Result` (`Status`, `Latency`, `NodeID`, `Message`), `Nodes` (`ID`, `Hostname`, `IPs`)            |

The text fields hold headscale's English wording. To word or translate a page yourself, branch on the fields that say
which case it is:

- `auth_web.html`: `Kind` is `register` (`/register/{id}`, a new node) or `auth` (`/auth/{id}`, an auth request), and
  `AuthID` is the ID the command names.
- `auth_success.html`: `Kind` is `register`, `reauth` (a node logged in again) or `ssh` (an SSH session approved).
- `auth_error.html`: `Code` is the HTTP status code, such as `403` or `410`; `Heading` is its status text.

```html title="auth_success.html"
{{template "layout" .}}
{{define "title"}}Signed in - {{.Site.Name}}{{end}}
{{define "content"}}
{{if eq .Page.Kind "ssh"}}<p>SSH access approved. Return to your terminal.</p>
{{else if eq .Page.Kind "reauth"}}<p>Welcome back, {{.Page.User}}.</p>
{{else}}<p>Your device is connected as {{.Page.User}}.</p>{{end}}
{{end}}
```

Besides the [built-in functions](https://pkg.go.dev/text/template#hdr-Functions), such as `or` for a default value,
templates can call `join`: `{{join .Page.Tags ", "}}`.

!!! warning "Confirmation forms"

    `register_confirm.html` and `ssh_check_confirm.html` must keep their form: a `POST` to `.Page.FormAction` with a
    hidden input named `.Page.CSRFTokenName` holding `.Page.CSRFToken`. Without it, the login cannot complete.

    ```html
    <form method="POST" action="{{.Page.FormAction}}">
      <input type="hidden" name="{{.Page.CSRFTokenName}}" value="{{.Page.CSRFToken}}">
      <button type="submit">Confirm registration</button>
    </form>
    ```

### Checking and reloading

Headscale reads the templates at startup and renders every page once with sample data, so a template that does not parse
or names a field that does not exist stops headscale with the file and line, instead of failing on a user's login.
Check templates without starting the server:

```console
headscale check all
```

Send headscale a `SIGHUP`, or run `systemctl reload headscale`, to read the templates again. A template that fails to
load is logged and the previous templates keep serving. The `theme` is read only at startup.

Templates are code run by headscale: keep `templates_dir` writable only by the administrators of the server.

## What stays fixed

Some parts of the pages are not settings, on purpose:

- **The `/debug/ping` page** is on `metrics_listen_addr`, for administrators. Its text is not themed beyond the layout.
- **`lang="en"`** on every page. The built-in text is English; declaring another language without translating it would
  mislead browsers and screen readers. A page or layout you write yourself can set its own.
- **Sizes and spacing** are [stylesheet variables](#stylesheet-variables), not `theme` settings: a stylesheet in
  `theme.stylesheets` sets them. A config key for each would only repeat CSS.
- **The commands on `auth_web.html`** are built by headscale so they match its CLI. A custom page can build its own from
  `Kind` and `AuthID`.
