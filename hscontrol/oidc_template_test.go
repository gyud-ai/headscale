package hscontrol

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/juanfont/headscale/hscontrol/templates"
	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthErrorTemplate(t *testing.T) {
	tests := []struct {
		name   string
		result templates.AuthErrorResult
	}{
		{
			name: "bad_request",
			result: templates.AuthErrorResult{
				Title:   "Error",
				Heading: "Bad Request",
				Message: "The request could not be processed. Please try again.",
			},
		},
		{
			name: "forbidden",
			result: templates.AuthErrorResult{
				Title:   "Error",
				Heading: "Forbidden",
				Message: "You are not authorized. Please contact your administrator.",
			},
		},
		{
			name: "gone_expired",
			result: templates.AuthErrorResult{
				Title:   "Error",
				Heading: "Gone",
				Message: "Your session has expired. Please try again.",
			},
		},
		{
			name: "internal_server_error",
			result: templates.AuthErrorResult{
				Title:   "Error",
				Heading: "Internal Server Error",
				Message: "Something went wrong. Please try again later.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := (*templates.Renderer)(nil).AuthError(tt.result)
			require.NoError(t, err)

			html := string(b)

			// Verify the HTML contains expected structural elements
			assert.Contains(t, html, "<!DOCTYPE html>")
			assert.Contains(t, html, "<title>"+tt.result.Title+" - Headscale</title>")
			assert.Contains(t, html, tt.result.Heading)
			assert.Contains(t, html, tt.result.Message)

			// Verify the stylesheets and the logo are linked
			assert.Contains(t, html, `<link rel="stylesheet" href="/static/style.css">`)
			assert.Contains(t, html, "Roboto")
			assert.Contains(t, html, `class="md-typeset"`)
			assert.Contains(t, html, `<img class="headscale-logo" src="/static/headscale.svg" alt="Headscale">`)

			// Verify SVG elements are present
			assert.Contains(t, html, "<svg")
			assert.Contains(t, html, "id=\"error-icon\"")

			// Verify no success checkbox icon
			assert.NotContains(t, html, "id=\"checkbox\"")
		})
	}
}

func TestAuthSuccessTemplate(t *testing.T) {
	tests := []struct {
		name   string
		result templates.AuthSuccessResult
	}{
		{
			name: "node_registered",
			result: templates.AuthSuccessResult{
				Title:   "Node Registered",
				Heading: "Node registered",
				Verb:    "Registered",
				User:    "newuser@example.com",
				Message: "You can now close this window.",
			},
		},
		{
			name: "node_reauthenticated",
			result: templates.AuthSuccessResult{
				Title:   "Node Reauthenticated",
				Heading: "Node reauthenticated",
				Verb:    "Reauthenticated",
				User:    "test@example.com",
				Message: "You can now close this window.",
			},
		},
		{
			name: "ssh_session_authorized",
			result: templates.AuthSuccessResult{
				Title:   "SSH Session Authorized",
				Heading: "SSH session authorized",
				Verb:    "Authorized",
				User:    "test@example.com",
				Message: "You may return to your terminal.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := (*templates.Renderer)(nil).AuthSuccess(tt.result)
			require.NoError(t, err)

			html := string(b)

			// Verify the HTML contains expected structural elements
			assert.Contains(t, html, "<!DOCTYPE html>")
			assert.Contains(t, html, "<title>"+tt.result.Title+" - Headscale</title>")
			assert.Contains(t, html, tt.result.Heading)
			assert.Contains(t, html, tt.result.Verb+" as ")
			assert.Contains(t, html, tt.result.User)
			assert.Contains(t, html, tt.result.Message)

			// Verify the stylesheets and the logo are linked
			assert.Contains(t, html, `<link rel="stylesheet" href="/static/style.css">`)
			assert.Contains(t, html, "Roboto")
			assert.Contains(t, html, `class="md-typeset"`)
			assert.Contains(t, html, `<img class="headscale-logo" src="/static/headscale.svg" alt="Headscale">`)

			// Verify SVG elements are present
			assert.Contains(t, html, "<svg")
			assert.Contains(t, html, "id=\"checkbox\"")
		})
	}
}

// TestPageKindsFromHandlers: the handlers tell a template override which
// case it renders, through Kind, Code and AuthID.
func TestPageKindsFromHandlers(t *testing.T) {
	dir := t.TempDir()
	for name, text := range map[string]string{
		"auth_success.html": `{{.Page.Kind}}`,
		"auth_error.html":   `{{.Page.Code}}`,
		"auth_web.html":     `{{.Page.Kind}}:{{.Page.AuthID}}`,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600))
	}

	pages, err := templates.New(&types.Config{Web: types.WebConfig{TemplatesDir: dir}})
	require.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		oidc := &AuthProviderOIDC{h: &Headscale{pages: pages}}
		user := &types.User{Name: "alice"}
		node := (&types.Node{}).View()

		for reauth, want := range map[bool]string{false: templates.AuthSuccessRegister, true: templates.AuthSuccessReauth} {
			b, err := oidc.renderRegistrationSuccess(user, node, reauth)
			require.NoError(t, err)
			assert.Equal(t, want, string(b))
		}

		b, err := oidc.renderAuthSuccess(user)
		require.NoError(t, err)
		assert.Equal(t, templates.AuthSuccessSSH, string(b))
	})

	t.Run("error", func(t *testing.T) {
		rec := httptest.NewRecorder()
		httpUserError(rec, pages, NewHTTPError(http.StatusGone, "expired", nil))
		assert.Equal(t, http.StatusGone, rec.Code)
		assert.Equal(t, "410", rec.Body.String())
	})

	t.Run("auth web", func(t *testing.T) {
		authID, err := types.NewAuthID()
		require.NoError(t, err)

		web := NewAuthProviderWeb("https://hs.example.com", pages)
		for kind, handler := range map[string]http.HandlerFunc{
			templates.AuthWebRegister: web.RegisterHandler,
			templates.AuthWebAuth:     web.AuthHandler,
		} {
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("auth_id", authID.String())
			req := httptest.NewRequestWithContext(context.WithValue(t.Context(), chi.RouteCtxKey, rctx), http.MethodGet, "/", nil)

			rec := httptest.NewRecorder()
			handler(rec, req)
			assert.Equal(t, kind+":"+authID.String(), rec.Body.String())
		}
	})
}
