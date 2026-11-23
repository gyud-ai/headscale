// Package assets holds the built-in static files of headscale's web pages,
// served under /static/ and replaceable by file name from web.static_dir.
package assets

import "embed"

// Static is the built-in /static/ tree: style.css, the stylesheet every
// built-in page links; headscale.svg, the default logo; and favicon.png,
// also served at /favicon.ico.
//
//go:embed favicon.png headscale.svg style.css
var Static embed.FS
