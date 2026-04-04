package publicverify

import (
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"strings"

	"github.com/ssovich/diasoft-gateway/internal/application/usecase"
	domainverification "github.com/ssovich/diasoft-gateway/internal/domain/verification"
	transportmiddleware "github.com/ssovich/diasoft-gateway/internal/transport/middleware"
)

type HTTPHandler struct {
	verify     *usecase.Verify
	audit      *usecase.RecordVerificationAudit
	ipResolver *transportmiddleware.ClientIPResolver
}

type request struct {
	DiplomaNumber  string `json:"diplomaNumber"`
	UniversityCode string `json:"universityCode"`
}

type response struct {
	Verdict        string `json:"verdict"`
	UniversityCode string `json:"universityCode,omitempty"`
	DiplomaNumber  string `json:"diplomaNumber,omitempty"`
	OwnerNameMask  string `json:"ownerNameMask,omitempty"`
	Program        string `json:"program,omitempty"`
}

const (
	maxVerifyRequestBodyBytes = 4 << 10
	maxTokenLength            = 256
	maxLookupFieldLength      = 128
)

func NewHTTPHandler(verify *usecase.Verify, audit *usecase.RecordVerificationAudit, ipResolver *transportmiddleware.ClientIPResolver) *HTTPHandler {
	return &HTTPHandler{verify: verify, audit: audit, ipResolver: ipResolver}
}

func (h *HTTPHandler) Verify(w http.ResponseWriter, r *http.Request) {
	var req request
	if err := decodeJSONBody(w, r, &req, maxVerifyRequestBodyBytes); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	req.DiplomaNumber = strings.TrimSpace(req.DiplomaNumber)
	req.UniversityCode = strings.TrimSpace(req.UniversityCode)
	if req.DiplomaNumber == "" || req.UniversityCode == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "diplomaNumber and universityCode are required"})
		return
	}
	if len(req.DiplomaNumber) > maxLookupFieldLength || len(req.UniversityCode) > maxLookupFieldLength {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request fields are too long"})
		return
	}

	result, err := h.verify.Run(r.Context(), usecase.VerifyCommand{
		DiplomaNumber:  req.DiplomaNumber,
		UniversityCode: req.UniversityCode,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	h.recordAudit(r, "verify_lookup", "", req.DiplomaNumber, req.UniversityCode, result.Verdict)
	writeJSON(w, http.StatusOK, toResponse(result))
}

func (h *HTTPHandler) VerifyByToken(w http.ResponseWriter, r *http.Request) {
	verificationToken := r.PathValue("verificationToken")
	if !validToken(verificationToken) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid verification token"})
		return
	}
	result, err := h.verify.RunByToken(r.Context(), usecase.VerifyByTokenCommand{VerificationToken: verificationToken})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	h.recordAudit(r, "verify_token", verificationToken, "", "", result.Verdict)
	writeJSON(w, http.StatusOK, toResponse(result))
}

func (h *HTTPHandler) VerifyPage(w http.ResponseWriter, r *http.Request) {
	verificationToken := r.PathValue("verificationToken")
	if !validToken(verificationToken) {
		http.NotFound(w, r)
		return
	}
	result, err := h.verify.RunByToken(r.Context(), usecase.VerifyByTokenCommand{VerificationToken: verificationToken})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	h.recordAudit(r, "verify_page", verificationToken, "", "", result.Verdict)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = verificationPageTemplate.Execute(w, struct {
		Title  string
		Result response
	}{
		Title:  pageTitle(result),
		Result: toResponse(result),
	})
}

func (h *HTTPHandler) recordAudit(r *http.Request, requestType, token, diplomaNumber, universityCode string, verdict domainverification.Verdict) {
	if h.audit == nil {
		return
	}
	_ = h.audit.Run(r.Context(), usecase.RecordVerificationAuditCommand{
		RequestType:    requestType,
		Token:          token,
		DiplomaNumber:  diplomaNumber,
		UniversityCode: universityCode,
		RemoteIP:       h.clientIP(r),
		Verdict:        verdict,
	})
}

func (h *HTTPHandler) clientIP(r *http.Request) string {
	if h.ipResolver != nil {
		return h.ipResolver.Resolve(r)
	}
	return transportmiddleware.ClientIP(r)
}

func toResponse(result domainverification.Result) response {
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
		return "Diploma verified"
	case domainverification.VerdictRevoked:
		return "Diploma revoked"
	default:
		return "Diploma not found"
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return invalidRequestError(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return invalidRequestError(err)
	}
	return nil
}

func invalidRequestError(err error) error {
	if err == nil {
		return nil
	}
	return &requestError{message: "invalid request body"}
}

func validToken(token string) bool {
	token = strings.TrimSpace(token)
	return token != "" && len(token) <= maxTokenLength
}

type requestError struct {
	message string
}

func (e *requestError) Error() string {
	if e == nil || e.message == "" {
		return "invalid request"
	}
	return e.message
}

var verificationPageTemplate = template.Must(template.New("verify-page").Parse(`<!doctype html>
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
