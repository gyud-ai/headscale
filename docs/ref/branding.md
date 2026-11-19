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
    primary_color: "#0a7d5a"
    primary_color_dark: "#4fd1a5"
    footer_text: "Operated by the Example IT team"
    hide_powered_by: true
```

- `name` replaces "Headscale" in page titles, page text, and the display name and download file name of the Apple
  configuration profile (`example_vpn_macos.mobileconfig` for "Example VPN").
- `logo` is the image at the top of every page. Set it to `""` to show no logo.
- `primary_color` colours links, buttons and focus rings; on hover they take a lighter mix of it. `primary_color_dark`
  replaces it when the browser asks for a dark colour scheme; without it, `primary_color` is used for both.
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

| File            | Built-in use                                          |
| --------------- | ----------------------------------------------------- |
| `style.css`     | The stylesheet of every page                          |
| `headscale.svg` | The default `logo`                                    |
| `favicon.png`   | The icon of every page, also served at `/favicon.ico` |

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

The built-in stylesheet keeps its colours in CSS variables, such as `--md-primary-fg-color`, `--hs-bg`, `--hs-border`
and `--hs-on-primary` (the text on buttons), and styles the page parts through classes, such as `.hs-box`,
`.hs-device-table` and `.hs-footer`. Read `style.css` for the full list. Sizes and spacing, such as the logo width,
the page width and paddings, are plain values in those rules rather than variables: override the rule in your
stylesheet.

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
| `components.html`        | The icons the pages share                                                              |
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
| `header`          | The logo                                                                     |
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

| `.Site` field                      | Value                                                            |
| ---------------------------------- | ---------------------------------------------------------------- |
| `Name`, `Logo`, `FooterText`       | `theme.name`, `theme.logo`, `theme.footer_text`                  |
| `HidePoweredBy`                    | `theme.hide_powered_by`                                          |
| `ServerURL`                        | `server_url`                                                     |
| `PrimaryColor`, `PrimaryColorDark` | `theme.primary_color`, `theme.primary_color_dark`                |
| `FontFamily`, `CodeFontFamily`     | `theme.font_family`, `theme.code_font_family`                    |
| `FontStylesheets`, `Stylesheets`   | `theme.font_stylesheets`, `theme.stylesheets`, lists of URLs     |
| `ThemeCSS`                         | The CSS variables the colours and fonts above set, for `<style>` |
| `ProfileFileName`                  | The download name of the Apple profiles, from `theme.name`       |

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
- **Sizes and spacing** are plain values in `style.css`. A stylesheet in `theme.stylesheets` loads after it and can
  override any rule.
- **The commands on `auth_web.html`** are built by headscale so they match its CLI. A custom page can build its own from
  `Kind` and `AuthID`.
