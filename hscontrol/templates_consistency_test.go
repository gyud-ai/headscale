package hscontrol

import (
	"strings"
	"testing"

	"github.com/juanfont/headscale/hscontrol/templates"
	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type renderedPage struct {
	name string
	html string
	// externalURLs are links that must open safely in a new tab.
	externalURLs []string
}

// renderedPages are the built-in pages, rendered with the default theme as
// the handlers render them.
func renderedPages(t *testing.T) []renderedPage {
	t.Helper()

	r, err := templates.New(&types.Config{
		ServerURL: "https://example.com",
		Web:       types.WebConfig{Theme: types.DefaultWebTheme()},
	})
	require.NoError(t, err)

	render := func(b []byte, err error) string {
		t.Helper()
		require.NoError(t, err)

		return string(b)
	}

	return []renderedPage{
		{
			name: "Auth Success",
			html: render(r.AuthSuccess(templates.AuthSuccessResult{
				Title:   "Node Registered",
				Heading: "Node registered",
				Verb:    "Registered",
				User:    "test@example.com",
				Message: "You can now close this window.",
			})),
			externalURLs: []string{
				"https://headscale.net/stable/",
				"https://tailscale.com/docs",
			},
		},
		{
			name: "Auth Error",
			html: render(r.AuthError(templates.AuthErrorResult{
				Title:   "Error",
				Heading: "Forbidden",
				Message: "You are not authorized. Please contact your administrator.",
			})),
		},
		{
			name: "Auth Web Register",
			html: render(r.AuthWeb(templates.AuthWebData{
				Title:       "Machine registration",
				Description: "Run the command below in the headscale server to add this machine to your network:",
				Command:     "headscale auth register --auth-id test-key-123 --user USERNAME",
			})),
		},
		{
			name: "Auth Web Approve",
			html: render(r.AuthWeb(templates.AuthWebData{
				Title:       "Authentication check",
				Description: "Run the command below in the headscale server to approve this authentication request:",
				Command:     "headscale auth approve --auth-id test-key-123",
			})),
		},
		{
			name: "Windows Config",
			html: render(r.Windows()),
			externalURLs: []string{
				"https://tailscale.com/download/windows",
			},
		},
		{
			name: "Apple Config",
			html: render(r.Apple()),
			externalURLs: []string{
				"https://apps.apple.com/app/tailscale/id1470499037",
			},
		},
	}
}

func TestTemplateHTMLConsistency(t *testing.T) {
	// Test all templates produce consistent modern HTML
	for _, tc := range renderedPages(t) {
		t.Run(tc.name, func(t *testing.T) {
			// Check DOCTYPE
			assert.True(t, strings.HasPrefix(tc.html, "<!DOCTYPE html>"),
				"%s should start with <!DOCTYPE html>", tc.name)

			// Check HTML5 lang attribute
			assert.Contains(t, tc.html, `<html lang="en">`,
				"%s should have html lang=\"en\"", tc.name)

			// Check UTF-8 charset
			assert.Contains(t, tc.html, `charset="UTF-8"`,
				"%s should have UTF-8 charset", tc.name)

			// Check viewport meta tag
			assert.Contains(t, tc.html, `name="viewport"`,
				"%s should have viewport meta tag", tc.name)

			// Check IE compatibility meta tag
			assert.Contains(t, tc.html, `X-UA-Compatible`,
				"%s should have X-UA-Compatible meta tag", tc.name)

			// Check closing tags
			assert.Contains(t, tc.html, "</html>",
				"%s should have closing html tag", tc.name)
			assert.Contains(t, tc.html, "</head>",
				"%s should have closing head tag", tc.name)
			assert.Contains(t, tc.html, "</body>",
				"%s should have closing body tag", tc.name)
		})
	}
}

func TestTemplateModernHTMLFeatures(t *testing.T) {
	for _, tc := range renderedPages(t) {
		t.Run(tc.name, func(t *testing.T) {
			// Check no deprecated tags
			assert.NotContains(t, tc.html, "<font",
				"%s should not use deprecated <font> tag", tc.name)
			assert.NotContains(t, tc.html, "<center",
				"%s should not use deprecated <center> tag", tc.name)

			// Check modern structure
			assert.Contains(t, tc.html, "<head>",
				"%s should have <head> section", tc.name)
			assert.Contains(t, tc.html, "<body",
				"%s should have <body> section", tc.name)
			assert.Contains(t, tc.html, "<title>",
				"%s should have <title> tag", tc.name)
		})
	}
}

func TestTemplateExternalLinkSecurity(t *testing.T) {
	// Test that all external links (http/https) have proper security attributes
	for _, tc := range renderedPages(t) {
		t.Run(tc.name, func(t *testing.T) {
			for _, url := range tc.externalURLs {
				// Find the link tag containing this URL
				if !strings.Contains(tc.html, url) {
					t.Errorf("%s should contain external link %s", tc.name, url)
					continue
				}

				// Check for rel="noreferrer noopener"
				// We look for the pattern: href="URL"...rel="noreferrer noopener"
				// The attributes might be in any order, so we check within a reasonable window
				idx := strings.Index(tc.html, url)
				if idx == -1 {
					continue
				}

				// Look for the closing > of the <a> tag (within 200 chars should be safe)
				endIdx := strings.Index(tc.html[idx:idx+200], ">")
				if endIdx == -1 {
					endIdx = 200
				}

				linkTag := tc.html[idx : idx+endIdx]

				assert.Contains(t, linkTag, `rel="noreferrer noopener"`,
					"%s external link %s should have rel=\"noreferrer noopener\"", tc.name, url)
				assert.Contains(t, linkTag, `target="_blank"`,
					"%s external link %s should have target=\"_blank\"", tc.name, url)
			}
		})
	}
}

func TestTemplateAccessibilityAttributes(t *testing.T) {
	// Test that all templates have proper accessibility attributes
	for _, tc := range renderedPages(t) {
		t.Run(tc.name, func(t *testing.T) {
			// Check for translate="no" on body tag to prevent browser translation
			// This is important for technical documentation with commands
			assert.Contains(t, tc.html, `translate="no"`,
				"%s should have translate=\"no\" attribute on body tag", tc.name)
		})
	}
}
