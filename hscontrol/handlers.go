package hscontrol

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/juanfont/headscale/hscontrol/templates"
	"github.com/juanfont/headscale/hscontrol/types"
	"github.com/juanfont/headscale/hscontrol/util/zlog/zf"
	"github.com/rs/zerolog/log"
	"tailscale.com/tailcfg"
	"tailscale.com/util/truncate"
)

const (
	reservedResponseHeaderSize = 4
)

// logTextLimit bounds the client-chosen text copied into one log field.
// Inside the Noise tunnel, header compression makes resending a long
// request target nearly free for the client.
const logTextLimit = 512

// clipForLog cuts client-chosen text to [logTextLimit] for a log field.
func clipForLog(s string) string {
	if len(s) <= logTextLimit {
		return s
	}

	return truncate.String(s, logTextLimit) + "..."
}

// errForLog clips an error's text for logging, as parse errors quote the
// request. What the caller handles and the client is sent is unchanged.
func errForLog(err error) error {
	if err == nil || len(err.Error()) <= logTextLimit {
		return err
	}

	return errors.New(clipForLog(err.Error())) //nolint:err113 // log-only copy
}

// httpError logs an error and sends an HTTP error response with the given.
func httpError(w http.ResponseWriter, err error) {
	if herr, ok := errors.AsType[HTTPError](err); ok {
		http.Error(w, herr.Msg, herr.Code)
		log.Error().Err(errForLog(herr.Err)).Int(zf.Code, herr.Code).
			Msgf("user msg: %s", herr.Msg)
	} else {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Error().Err(errForLog(err)).
			Int(zf.Code, http.StatusInternalServerError).
			Msg("http internal server error")
	}
}

// httpUserError logs an error and sends a styled HTML error page,
// rendered by pages (nil for the built-in one). Use this for
// browser-facing error paths (OIDC, registration confirm) where the user
// should see a branded page instead of plain text. Technical details go
// to the server log; the HTML page only shows an actionable message for
// the error's reason, or one derived from the HTTP status code.
func httpUserError(w http.ResponseWriter, pages *templates.Renderer, err error) {
	code := http.StatusInternalServerError
	reason := ""

	if herr, ok := errors.AsType[HTTPError](err); ok {
		if herr.Code != 0 {
			code = herr.Code
		}

		reason = herr.Reason

		log.Error().Err(errForLog(herr.Err)).Int(zf.Code, code).
			Msgf("user msg: %s", herr.Msg)
	} else {
		log.Error().Err(errForLog(err)).Int(zf.Code, code).
			Msg("http internal server error")
	}

	if reason == "" {
		reason = templates.AuthErrorReason(code)
	}

	writePage(w, code)(pages.AuthError(templates.AuthErrorResult{
		Title:   "Error",
		Heading: http.StatusText(code),
		Message: userMessages[reason],
		Code:    code,
		Reason:  reason,
	}))
}

// writePage returns a function that writes a rendered page with status,
// or a plain 500 when rendering failed: writePage(w, 200)(pages.Apple()).
// Headers set before it are kept.
func writePage(w http.ResponseWriter, status int) func([]byte, error) {
	return func(page []byte, err error) {
		if err != nil {
			log.Error().Err(err).Msg("rendering web page")
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)

		_, err = w.Write(page) //nolint:gosec // G705: html/template escaped every value in page
		if err != nil {
			log.Error().Err(err).Msg("writing web page")
		}
	}
}

// userMessages is the message of each error page reason.
var userMessages = map[string]string{
	templates.AuthErrorUnauthorized: "You are not authorized. Please contact your administrator.",
	templates.AuthErrorExpired:      "Your session has expired. Please try again.",
	templates.AuthErrorBadRequest:   "The request could not be processed. Please try again.",
	templates.AuthErrorServerError:  "Something went wrong. Please try again later.",
	templates.AuthErrorLinkSpent: "This link has already been used or has expired. " +
		"If your device is connected you are done; otherwise start the login again.",
	templates.AuthErrorNodeReportTooLarge: "This device reports too much data. " +
		"Ask your administrator to reduce its reported metadata, then sign in again.",
}

// HTTPError represents an error that is surfaced to the user via web.
type HTTPError struct {
	Code   int    // HTTP response code to send to client; 0 means 500
	Msg    string // Response body to send to non-browser clients
	Err    error  // Detailed error to log on the server
	Reason string // Error page reason, a templates.AuthError value; empty derives it from Code
}

func (e HTTPError) Error() string { return fmt.Sprintf("http error[%d]: %s, %s", e.Code, e.Msg, e.Err) }
func (e HTTPError) Unwrap() error { return e.Err }

// NewHTTPError returns an HTTPError containing the given information.
func NewHTTPError(code int, msg string, err error) HTTPError {
	return HTTPError{Code: code, Msg: msg, Err: err}
}

// newHTTPUserError is an HTTPError whose error page shows the message of
// reason, a templates.AuthError value, instead of the one of code.
func newHTTPUserError(code int, msg, reason string, err error) HTTPError {
	return HTTPError{Code: code, Msg: msg, Err: err, Reason: reason}
}

var errMethodNotAllowed = NewHTTPError(http.StatusMethodNotAllowed, "method not allowed", nil)

var ErrRegisterMethodCLIDoesNotSupportExpire = errors.New(
	"machines registered with CLI do not support expiry",
)

func parseCapabilityVersion(req *http.Request) (tailcfg.CapabilityVersion, error) {
	clientCapabilityStr := req.URL.Query().Get("v")

	if clientCapabilityStr == "" {
		return 0, NewHTTPError(http.StatusBadRequest, "capability version must be set", nil)
	}

	clientCapabilityVersion, err := strconv.Atoi(clientCapabilityStr)
	if err != nil {
		return 0, NewHTTPError(http.StatusBadRequest, "invalid capability version", fmt.Errorf("parsing capability version: %w", err))
	}

	return tailcfg.CapabilityVersion(clientCapabilityVersion), nil
}

// verifyBodyLimit caps the request body for /verify. The DERP verify
// protocol payload ([tailcfg.DERPAdmitClientRequest]) is a few hundred
// bytes; 4 KiB is generous and prevents an unauthenticated client from
// OOMing the public router with arbitrarily large POSTs.
const verifyBodyLimit int64 = 4 * 1024

func (h *Headscale) handleVerifyRequest(
	req *http.Request,
	writer io.Writer,
) error {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return NewHTTPError(http.StatusRequestEntityTooLarge, "request body too large", fmt.Errorf("reading request body: %w", err))
	}

	var derpAdmitClientRequest tailcfg.DERPAdmitClientRequest
	if err := json.Unmarshal(body, &derpAdmitClientRequest); err != nil { //nolint:noinlineerr
		return NewHTTPError(http.StatusBadRequest, "Bad Request: invalid JSON", fmt.Errorf("parsing DERP client request: %w", err))
	}

	// Every DERP connect lands here, unauthenticated, so use the NodeKey
	// index rather than scanning every node.
	nv, ok := h.state.GetNodeByNodeKey(derpAdmitClientRequest.NodePublic)

	resp := &tailcfg.DERPAdmitClientResponse{
		Allow: ok && nv.Valid(),
	}

	return json.NewEncoder(writer).Encode(resp)
}

// VerifyHandler answers a DERP server's client-verification POST
// ([tailcfg.DERPAdmitClientRequest]). A client is admitted while its NodeKey
// belongs to a registered node; expiry, tags and ephemerality do not gate it.
func (h *Headscale) VerifyHandler(
	writer http.ResponseWriter,
	req *http.Request,
) {
	if req.Method != http.MethodPost {
		httpError(writer, errMethodNotAllowed)
		return
	}

	req.Body = http.MaxBytesReader(writer, req.Body, verifyBodyLimit)

	// Set the Content-Type before any body byte is written. The first
	// Write in handleVerifyRequest triggers an implicit WriteHeader that
	// snapshots the header map, so setting it afterwards is a no-op. The
	// error path resets the Content-Type via http.Error, so error
	// responses remain text/plain.
	writer.Header().Set("Content-Type", "application/json")

	err := h.handleVerifyRequest(req, writer)
	if err != nil {
		httpError(writer, err)
		return
	}
}

// KeyHandler provides the Headscale pub key
// Listens in /key.
func (h *Headscale) KeyHandler(
	writer http.ResponseWriter,
	req *http.Request,
) {
	// New Tailscale clients send a 'v' parameter to indicate the CurrentCapabilityVersion
	capVer, err := parseCapabilityVersion(req)
	if err != nil {
		httpError(writer, err)
		return
	}

	// Only disclose the Noise public key to clients this server can
	// actually complete a handshake with. Gating on the same floor the
	// Noise handshake enforces (capver.MinSupportedCapabilityVersion, see
	// isSupportedVersion in noise.go) keeps /key consistent with /ts2021:
	// versions the handshake would reject get a clear rejection here
	// instead of a key that only serves as a version-boundary oracle.
	// See https://github.com/juanfont/headscale/issues/3380.
	if !isSupportedVersion(capVer) {
		httpError(writer, NewHTTPError(http.StatusBadRequest, "unsupported client version", unsupportedClientError(capVer)))
		return
	}

	resp := tailcfg.OverTLSPublicKeyResponse{
		PublicKey: h.noisePrivateKey.Public(),
	}

	writer.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(writer).Encode(resp)
	if err != nil {
		log.Error().Err(err).Msg("failed to encode public key response")
	}
}

func (h *Headscale) HealthHandler(
	writer http.ResponseWriter,
	req *http.Request,
) {
	respond := func(err error) {
		writer.Header().Set("Content-Type", "application/health+json; charset=utf-8")

		res := struct {
			Status string `json:"status"`
		}{
			Status: "pass",
		}

		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)

			res.Status = "fail"
		}

		encErr := json.NewEncoder(writer).Encode(res)
		if encErr != nil {
			log.Error().Err(encErr).Msg("failed to encode health response")
		}
	}

	err := h.state.PingDB(req.Context())
	if err != nil {
		respond(err)

		return
	}

	respond(nil)
}

func (h *Headscale) RobotsHandler(
	writer http.ResponseWriter,
	req *http.Request,
) {
	writer.Header().Set("Content-Type", "text/plain")
	writer.WriteHeader(http.StatusOK)

	_, err := writer.Write([]byte("User-agent: *\nDisallow: /"))
	if err != nil {
		log.Error().
			Caller().
			Err(err).
			Msg("Failed to write HTTP response")
	}
}

// VersionHandler returns version information about the Headscale server
// Listens in /version.
func (h *Headscale) VersionHandler(
	writer http.ResponseWriter,
	req *http.Request,
) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)

	versionInfo := types.GetVersionInfo()

	err := json.NewEncoder(writer).Encode(versionInfo)
	if err != nil {
		log.Error().
			Caller().
			Err(err).
			Msg("Failed to write version response")
	}
}

type AuthProviderWeb struct {
	serverURL string
	pages     *templates.Renderer
}

// NewAuthProviderWeb returns the auth provider without OpenID Connect.
// Its pages are rendered by the given renderer, nil for the built-in ones.
func NewAuthProviderWeb(serverURL string, pages *templates.Renderer) *AuthProviderWeb {
	return &AuthProviderWeb{
		serverURL: serverURL,
		pages:     pages,
	}
}

// authPathURL builds an auth-flow URL of the form
// "<serverURL>/<kind>/<id>".
func authPathURL(serverURL, kind string, authID types.AuthID) string {
	return fmt.Sprintf("%s/%s/%s", serverURL, kind, authID.String())
}

func (a *AuthProviderWeb) RegisterURL(authID types.AuthID) string {
	return authPathURL(a.serverURL, "register", authID)
}

func (a *AuthProviderWeb) AuthURL(authID types.AuthID) string {
	return authPathURL(a.serverURL, "auth", authID)
}

func (a *AuthProviderWeb) AuthHandler(
	writer http.ResponseWriter,
	req *http.Request,
) {
	authID, err := authIDFromRequest(req)
	if err != nil {
		httpError(writer, err)
		return
	}

	writePage(writer, http.StatusOK)(a.pages.AuthWeb(templates.AuthWebData{
		Title:       "Authentication check",
		Description: "Run the command below in the headscale server to approve this authentication request:",
		Command:     "headscale auth approve --auth-id " + authID.String(),
		Kind:        templates.AuthWebAuth,
		AuthID:      authID.String(),
	}))
}

func authIDFromRequest(req *http.Request) (types.AuthID, error) {
	raw, err := stringParam(req, "auth_id")
	if err != nil {
		return "", NewHTTPError(http.StatusBadRequest, "invalid auth id", fmt.Errorf("parsing auth_id from URL: %w", err))
	}

	authId, err := types.AuthIDFromString(raw)
	if err != nil {
		return "", NewHTTPError(http.StatusBadRequest, "invalid auth id", fmt.Errorf("parsing auth_id from URL: %w", err))
	}

	return authId, nil
}

// RegisterHandler shows a simple message in the browser to point to the CLI
// Listens in /register/:registration_id.
//
// This is not part of the Tailscale control API, as we could send whatever URL
// in the [tailcfg.RegisterResponse.AuthURL] field.
func (a *AuthProviderWeb) RegisterHandler(
	writer http.ResponseWriter,
	req *http.Request,
) {
	authId, err := authIDFromRequest(req)
	if err != nil {
		httpError(writer, err)
		return
	}

	writePage(writer, http.StatusOK)(a.pages.AuthWeb(templates.AuthWebData{
		Title:       "Node registration",
		Description: "Run the command below in the headscale server to add this node to your network:",
		Command:     fmt.Sprintf("headscale auth register --auth-id %s --user USERNAME", authId.String()),
		Kind:        templates.AuthWebRegister,
		AuthID:      authId.String(),
	}))
}

// BlankHandler serves /: by default a blank page that only sets the
// favicon, or blank.html of web.templates_dir.
func (h *Headscale) BlankHandler(writer http.ResponseWriter, _ *http.Request) {
	writePage(writer, http.StatusOK)(h.pages.Blank())
}
