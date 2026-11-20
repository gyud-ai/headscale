package templates

import (
	"net/http"
	"time"

	"github.com/juanfont/headscale/hscontrol/types"
)

// The data each page gets as .Page. Templates in web.templates_dir read
// these fields by name, so renaming or removing one breaks them: add
// fields, and document them in docs/ref/branding.md.

// AuthWebData is the page that tells an administrator which command
// approves a registration or an auth request, without OpenID Connect.
type AuthWebData struct {
	// Title is the heading, e.g. "Node registration": the
	// web.text.auth_web title of Kind.
	Title string

	// Description introduces the command: the web.text.auth_web
	// description of Kind.
	Description string

	// Command is the headscale CLI command to run.
	Command string

	// Kind is what the command approves: AuthWebRegister or AuthWebAuth.
	Kind string

	// AuthID is the request's auth ID, the one Command names.
	AuthID string
}

// The values of AuthWebData.Kind.
const (
	// AuthWebRegister is /register/{id}: a new node waiting to be added.
	AuthWebRegister = "register"

	// AuthWebAuth is /auth/{id}: an auth request waiting for approval.
	AuthWebAuth = "auth"
)

// AuthErrorResult contains the text content for an error page shown
// to users in their browser when a browser-facing operation fails
// (OIDC callback, SSH check, registration confirmation, etc.), or when
// a browser asks for a page that does not exist.
type AuthErrorResult struct {
	// Title is the browser tab / page title, before the site name,
	// e.g. "Error": web.text.auth_error.title.
	Title string

	// Heading is the bold red text inside the error box,
	// e.g. "Access denied": the web.text.auth_error heading of Reason.
	Heading string

	// Message is the actionable user-facing message shown below
	// the heading, e.g. "You are not authorized. Please contact
	// your administrator.": the web.text.auth_error message of Reason.
	Message string

	// Code is the HTTP status code of the response, e.g. 410.
	Code int

	// Reason is what went wrong, one of the AuthError values below.
	// Several reasons share a code: 410 is both AuthErrorExpired and
	// AuthErrorLinkSpent.
	Reason string
}

// The values of AuthErrorResult.Reason.
const (
	// AuthErrorUnauthorized is a 401 or 403: the user may not do this.
	AuthErrorUnauthorized = "unauthorized"

	// AuthErrorExpired is a 410: the login session expired.
	AuthErrorExpired = "expired"

	// AuthErrorBadRequest is another 4xx: the request could not be used.
	AuthErrorBadRequest = "bad_request"

	// AuthErrorNotFound is a 404: there is no page at the address.
	AuthErrorNotFound = "not_found"

	// AuthErrorServerError is a 5xx: headscale failed.
	AuthErrorServerError = "server_error"

	// AuthErrorLinkSpent is a 410 for a registration link that was already
	// used, usually a reload after the user confirmed.
	AuthErrorLinkSpent = "link_spent"

	// AuthErrorNodeReportTooLarge is a 400 for a device whose report to
	// headscale is over the size limit.
	AuthErrorNodeReportTooLarge = "node_report_too_large"
)

// AuthErrorReason is the reason of an error that has only a status code.
func AuthErrorReason(code int) string {
	switch {
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return AuthErrorUnauthorized
	case code == http.StatusGone:
		return AuthErrorExpired
	case code == http.StatusNotFound:
		return AuthErrorNotFound
	case code >= 400 && code < 500:
		return AuthErrorBadRequest
	default:
		return AuthErrorServerError
	}
}

// AuthSuccessResult contains the text content for an authentication success page.
// Each field controls a distinct piece of user-facing text so that every auth
// flow (node registration, reauthentication, SSH check, …) can clearly
// communicate what just happened. Title, Heading, Verb and Message are the
// web.text.auth_success text of Kind.
type AuthSuccessResult struct {
	// Title is the browser tab / page title, before the site name,
	// e.g. "Node Registered".
	Title string

	// Heading is the bold green text inside the success box,
	// e.g. "Node registered".
	Heading string

	// Verb is the action prefix in the body text before "as <user>",
	// e.g. "Registered", "Reauthenticated", "Authorized".
	Verb string

	// User is the display name shown in bold in the body text,
	// e.g. "user@example.com".
	User string

	// Message is the follow-up instruction shown after the user name,
	// e.g. "You can now close this window."
	Message string

	// Kind is what succeeded: AuthSuccessRegister, AuthSuccessReauth or
	// AuthSuccessSSH.
	Kind string
}

// The values of AuthSuccessResult.Kind.
const (
	// AuthSuccessRegister is a new node registered.
	AuthSuccessRegister = "register"

	// AuthSuccessReauth is an existing node logged in again.
	AuthSuccessReauth = "reauth"

	// AuthSuccessSSH is an SSH session a check rule held, now approved.
	AuthSuccessSSH = "ssh"
)

// RegisterConfirmInfo carries the human-readable information shown on
// the registration confirmation interstitial that an OIDC-authenticated
// user must explicitly accept before a pending node is registered to
// their identity. The fields here intentionally include enough device
// detail (hostname, OS, machine-key fingerprint) for the user to
// recognise whether the device they are about to claim is in fact
// theirs.
// The form and user fields are also used for SSH check confirmations.
type RegisterConfirmInfo struct {
	// FormAction is the absolute or relative URL the confirm form
	// POSTs to. Typically /register/confirm/{auth_id}.
	FormAction string

	// CSRFTokenName is the name of the hidden form field carrying the
	// CSRF token. The corresponding cookie shares this name.
	CSRFTokenName string

	// CSRFToken is the per-session token that must match the value of
	// the cookie set by the OIDC callback before the POST is honoured.
	CSRFToken string

	// User is the OIDC-authenticated identity approving the request.
	User string

	// Hostname is the hostname the registering tailscaled instance
	// reported in its [tailcfg.RegisterRequest].
	Hostname string

	// OS is the operating system the registering tailscaled reported.
	// May be the empty string when the client did not send [tailcfg.Hostinfo].
	OS string

	// MachineKey is the short fingerprint of the registering machine
	// key. The full key is intentionally not shown.
	MachineKey string

	// ExistingNode is the stored node name, empty for a new device.
	ExistingNode string

	// Tags are the ownership tags requested by the registering client.
	Tags []string
}

// SSHCheckData asks the owner to approve the source and destination node
// pair of an SSH session held by a check rule. It reuses the form and
// user fields of [RegisterConfirmInfo].
type SSHCheckData struct {
	RegisterConfirmInfo

	// Source and Destination are the given names of the two nodes.
	Source      string
	Destination string
}

// pingLatencyPrecision is what the ping page rounds latency to.
const pingLatencyPrecision = time.Millisecond

// PingData is the /debug/ping page.
type PingData struct {
	// Query is the node the form asked for, shown back in the input.
	Query string

	// Result is nil until a ping has run.
	Result *PingResult

	// Nodes are the connected nodes, shown as quick-ping links.
	Nodes []ConnectedNode
}

// PingResult contains the outcome of a ping request.
type PingResult struct {
	// Status is "ok", "timeout", or "error".
	Status string

	// Latency is the round-trip time (only meaningful when [PingResult.Status] is "ok").
	Latency time.Duration

	// NodeID is the ID of the pinged node.
	NodeID types.NodeID

	// Message is a human-readable description of the result.
	Message string
}

// ConnectedNode is a node currently connected to the [mapper.Batcher],
// displayed as a quick-ping link on the debug ping page.
type ConnectedNode struct {
	ID       types.NodeID
	Hostname string
	IPs      []string
}

// Page names: the file in html/ or web.templates_dir that renders each
// page.
const (
	pageApple           = "apple.html"
	pageWindows         = "windows.html"
	pageAuthWeb         = "auth_web.html"
	pageAuthSuccess     = "auth_success.html"
	pageAuthError       = "auth_error.html"
	pageRegisterConfirm = "register_confirm.html"
	pageSSHCheckConfirm = "ssh_check_confirm.html"
	pagePing            = "ping.html"
	pageBlank           = "blank.html"
)

// componentsFile holds the small shared templates, such as the icons.
const componentsFile = "components.html"

// baseFiles are parsed into every page before the page itself.
var baseFiles = []string{"layout.html", componentsFile}

// samples is the data each page is rendered with when templates load, so
// a template that names a missing field fails then, not on a user's
// login. Pages with branches get one sample per branch.
var samples = map[string][]any{
	pageApple:   {nil},
	pageWindows: {nil},
	pageAuthWeb: {AuthWebData{
		Title:       "Node registration",
		Description: "Run the command below in the headscale server to add this node to your network:",
		Command:     "headscale auth register --auth-id sample --user USERNAME",
		Kind:        AuthWebRegister,
		AuthID:      "sample",
	}},
	pageAuthSuccess: {AuthSuccessResult{
		Title: "Node Registered", Heading: "Node registered", Verb: "Registered",
		User: "user@example.com", Message: "You can now close this window.",
		Kind: AuthSuccessRegister,
	}},
	pageAuthError: {AuthErrorResult{
		Title: "Error", Heading: "Forbidden", Message: "You are not authorized.",
		Code: 403, Reason: AuthErrorUnauthorized,
	}},
	pageRegisterConfirm: {
		sampleConfirm(),
		func() RegisterConfirmInfo {
			c := sampleConfirm()
			c.OS, c.ExistingNode, c.Tags = "", "", []string{"tag:server"}

			return c
		}(),
	},
	pageSSHCheckConfirm: {SSHCheckData{RegisterConfirmInfo: sampleConfirm(), Source: "laptop", Destination: "server"}},
	pagePing: {
		PingData{},
		PingData{
			Query:  "1",
			Result: &PingResult{Status: "ok", Latency: 12 * time.Millisecond, NodeID: 1},
			Nodes:  []ConnectedNode{{ID: 1, Hostname: "laptop", IPs: []string{"100.64.0.1", "fd7a:115c:a1e0::1"}}},
		},
		PingData{Query: "1", Result: &PingResult{Status: "timeout", NodeID: 1, Message: "No response."}},
		PingData{Query: "x", Result: &PingResult{Status: "error", Message: "No such node."}},
	},
	pageBlank: {nil},
}

func sampleConfirm() RegisterConfirmInfo {
	return RegisterConfirmInfo{
		FormAction:    "/register/confirm/sample",
		CSRFTokenName: "headscale_register_confirm",
		CSRFToken:     "sample",
		User:          "user@example.com",
		Hostname:      "laptop",
		OS:            "linux",
		MachineKey:    "[abcde]",
		ExistingNode:  "laptop",
	}
}
