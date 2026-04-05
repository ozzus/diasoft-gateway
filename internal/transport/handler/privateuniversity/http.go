package privateuniversity

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/ssovich/diasoft-gateway/internal/privateapi"
	"github.com/ssovich/diasoft-gateway/internal/transport/handler/privateapiutil"
	transportmiddleware "github.com/ssovich/diasoft-gateway/internal/transport/middleware"
)

const maxUploadBytes = 64 << 20

type HTTPHandler struct {
	registry privateapi.RegistryClient
}

type revokeRequest struct {
	Reason string `json:"reason"`
}

func NewHTTPHandler(registry privateapi.RegistryClient) *HTTPHandler {
	return &HTTPHandler{registry: registry}
}

func (h *HTTPHandler) ListDiplomas(w http.ResponseWriter, r *http.Request) {
	claims, ok := transportmiddleware.PrivateClaimsFromContext(r.Context())
	if !ok {
		transportmiddleware.WritePrivateError(w, http.StatusUnauthorized, "Требуется авторизация")
		return
	}

	page := 1
	if rawPage := strings.TrimSpace(r.URL.Query().Get("page")); rawPage != "" {
		parsed, err := strconv.Atoi(rawPage)
		if err != nil || parsed < 1 {
			transportmiddleware.WritePrivateError(w, http.StatusBadRequest, "page must be a positive integer")
			return
		}
		page = parsed
	}

	response, err := h.registry.ListUniversityDiplomas(
		r.Context(),
		claims.UniversityID,
		r.URL.Query().Get("search"),
		r.URL.Query().Get("status"),
		page,
	)
	if err != nil {
		privateapiutil.HandleError(w, err)
		return
	}
	privateapiutil.WriteJSON(w, http.StatusOK, response)
}

func (h *HTTPHandler) UploadDiplomas(w http.ResponseWriter, r *http.Request) {
	claims, ok := transportmiddleware.PrivateClaimsFromContext(r.Context())
	if !ok {
		transportmiddleware.WritePrivateError(w, http.StatusUnauthorized, "Требуется авторизация")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		transportmiddleware.WritePrivateError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()
	if header.Size > maxUploadBytes {
		transportmiddleware.WritePrivateError(w, http.StatusBadRequest, "file is too large")
		return
	}

	content, err := io.ReadAll(io.LimitReader(file, maxUploadBytes))
	if err != nil {
		transportmiddleware.WritePrivateError(w, http.StatusBadRequest, "failed to read upload")
		return
	}

	response, err := h.registry.UploadUniversityDiplomas(r.Context(), claims.UniversityID, header.Filename, header.Header.Get("Content-Type"), content)
	if err != nil {
		privateapiutil.HandleError(w, err)
		return
	}
	privateapiutil.WriteJSON(w, http.StatusAccepted, response)
}

func (h *HTTPHandler) GetImport(w http.ResponseWriter, r *http.Request) {
	claims, ok := transportmiddleware.PrivateClaimsFromContext(r.Context())
	if !ok {
		transportmiddleware.WritePrivateError(w, http.StatusUnauthorized, "Требуется авторизация")
		return
	}

	response, err := h.registry.GetUniversityImport(r.Context(), claims.UniversityID, r.PathValue("jobId"))
	if err != nil {
		privateapiutil.HandleError(w, err)
		return
	}
	privateapiutil.WriteJSON(w, http.StatusOK, response)
}

func (h *HTTPHandler) GetImportErrors(w http.ResponseWriter, r *http.Request) {
	claims, ok := transportmiddleware.PrivateClaimsFromContext(r.Context())
	if !ok {
		transportmiddleware.WritePrivateError(w, http.StatusUnauthorized, "Требуется авторизация")
		return
	}

	response, err := h.registry.GetUniversityImportErrors(r.Context(), claims.UniversityID, r.PathValue("jobId"))
	if err != nil {
		privateapiutil.HandleError(w, err)
		return
	}
	privateapiutil.WriteJSON(w, http.StatusOK, map[string]any{"errors": response})
}

func (h *HTTPHandler) RevokeDiploma(w http.ResponseWriter, r *http.Request) {
	claims, ok := transportmiddleware.PrivateClaimsFromContext(r.Context())
	if !ok {
		transportmiddleware.WritePrivateError(w, http.StatusUnauthorized, "Требуется авторизация")
		return
	}

	var request revokeRequest
	if err := privateapiutil.DecodeJSONBody(w, r, &request, 4<<10); err != nil {
		transportmiddleware.WritePrivateError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(request.Reason) == "" {
		transportmiddleware.WritePrivateError(w, http.StatusBadRequest, "reason is required")
		return
	}

	if err := h.registry.RevokeUniversityDiploma(r.Context(), claims.UniversityID, r.PathValue("id"), request.Reason); err != nil {
		privateapiutil.HandleError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *HTTPHandler) GetQR(w http.ResponseWriter, r *http.Request) {
	claims, ok := transportmiddleware.PrivateClaimsFromContext(r.Context())
	if !ok {
		transportmiddleware.WritePrivateError(w, http.StatusUnauthorized, "Требуется авторизация")
		return
	}

	response, err := h.registry.GetUniversityQR(r.Context(), claims.UniversityID, r.PathValue("id"))
	if err != nil {
		privateapiutil.HandleError(w, err)
		return
	}
	privateapiutil.WriteJSON(w, http.StatusOK, response)
}
