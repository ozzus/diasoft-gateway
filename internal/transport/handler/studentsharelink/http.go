package studentsharelink

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/ssovich/diasoft-gateway/internal/application/usecase"
	domainverification "github.com/ssovich/diasoft-gateway/internal/domain/verification"
	transportmiddleware "github.com/ssovich/diasoft-gateway/internal/transport/middleware"
)

type HTTPHandler struct {
	resolveShareLink *usecase.ResolveShareLink
	audit            *usecase.RecordVerificationAudit
	ipResolver       *transportmiddleware.ClientIPResolver
}

type response struct {
	Verdict        string `json:"verdict,omitempty"`
	UniversityCode string `json:"universityCode,omitempty"`
	DiplomaNumber  string `json:"diplomaNumber,omitempty"`
	OwnerNameMask  string `json:"ownerNameMask,omitempty"`
	Program        string `json:"program,omitempty"`
}

const (
	maxShareTokenLength = 256
)

func NewHTTPHandler(resolveShareLink *usecase.ResolveShareLink, audit *usecase.RecordVerificationAudit, ipResolver *transportmiddleware.ClientIPResolver) *HTTPHandler {
	return &HTTPHandler{resolveShareLink: resolveShareLink, audit: audit, ipResolver: ipResolver}
}

func (h *HTTPHandler) ResolvePage(w http.ResponseWriter, r *http.Request) {
	shareToken := r.PathValue("shareToken")
	if !validToken(shareToken) {
		http.NotFound(w, r)
		return
	}
	result, err := h.resolveShareLink.Run(r.Context(), usecase.ResolveShareLinkCommand{Token: shareToken})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.recordAudit(r, shareToken, result.Verdict)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = shareLinkPageTemplate.Execute(w, struct {
		Title  string
		Result response
	}{
		Title:  pageTitle(result),
		Result: toVerificationResponse(result),
	})
}

func (h *HTTPHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	shareToken := r.PathValue("shareToken")
	if !validToken(shareToken) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid share token"})
		return
	}

	result, err := h.resolveShareLink.Run(r.Context(), usecase.ResolveShareLinkCommand{Token: shareToken})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	h.recordAudit(r, shareToken, result.Verdict)
	writeJSON(w, http.StatusOK, toVerificationResponse(result))
}

func (h *HTTPHandler) recordAudit(r *http.Request, shareToken string, verdict domainverification.Verdict) {
	if h.audit == nil {
		return
	}
	_ = h.audit.Run(r.Context(), usecase.RecordVerificationAuditCommand{
		RequestType: "share_link_page",
		Token:       shareToken,
		RemoteIP:    h.clientIP(r),
		Verdict:     verdict,
	})
}

func (h *HTTPHandler) clientIP(r *http.Request) string {
	if h.ipResolver != nil {
		return h.ipResolver.Resolve(r)
	}
	return transportmiddleware.ClientIP(r)
}

func toVerificationResponse(result domainverification.Result) response {
	return response{
		Verdict:        string(result.Verdict),
		UniversityCode: result.UniversityCode,
		DiplomaNumber:  result.DiplomaNumber,
		OwnerNameMask:  result.OwnerNameMask,
		Program:        result.Program,
	}
}

func pageTitle(result domainverification.Result) string {
	switch result.Verdict {
	case domainverification.VerdictValid:
		return "Share link verified"
	case domainverification.VerdictRevoked:
		return "Diploma revoked"
	case domainverification.VerdictExpired:
		return "Share link expired"
	default:
		return "Share link not found"
	}
}

func validToken(token string) bool {
	token = strings.TrimSpace(token)
	return token != "" && len(token) <= maxShareTokenLength
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

var shareLinkPageTemplate = template.Must(template.New("share-link-page").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{ .Title }}</title>
</head>
<body>
  <main>
    <h1>{{ .Title }}</h1>
    <p>Verdict: {{ .Result.Verdict }}</p>
    {{ if .Result.UniversityCode }}<p>University: {{ .Result.UniversityCode }}</p>{{ end }}
    {{ if .Result.DiplomaNumber }}<p>Diploma number: {{ .Result.DiplomaNumber }}</p>{{ end }}
    {{ if .Result.OwnerNameMask }}<p>Student: {{ .Result.OwnerNameMask }}</p>{{ end }}
    {{ if .Result.Program }}<p>Program: {{ .Result.Program }}</p>{{ end }}
  </main>
</body>
</html>`))
