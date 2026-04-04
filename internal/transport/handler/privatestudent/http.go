package privatestudent

import (
	"net/http"

	"github.com/ssovich/diasoft-gateway/internal/privateapi"
	"github.com/ssovich/diasoft-gateway/internal/transport/handler/privateapiutil"
	transportmiddleware "github.com/ssovich/diasoft-gateway/internal/transport/middleware"
)

type HTTPHandler struct {
	registry privateapi.RegistryClient
}

type shareLinkRequest struct {
	TTLSeconds int `json:"ttlSeconds"`
}

func NewHTTPHandler(registry privateapi.RegistryClient) *HTTPHandler {
	return &HTTPHandler{registry: registry}
}

func (h *HTTPHandler) GetDiploma(w http.ResponseWriter, r *http.Request) {
	claims, ok := transportmiddleware.PrivateClaimsFromContext(r.Context())
	if !ok {
		transportmiddleware.WritePrivateError(w, http.StatusUnauthorized, "Требуется авторизация")
		return
	}

	response, err := h.registry.GetStudentDiploma(r.Context(), claims.DiplomaID)
	if err != nil {
		privateapiutil.HandleError(w, err)
		return
	}
	privateapiutil.WriteJSON(w, http.StatusOK, response)
}

func (h *HTTPHandler) CreateShareLink(w http.ResponseWriter, r *http.Request) {
	claims, ok := transportmiddleware.PrivateClaimsFromContext(r.Context())
	if !ok {
		transportmiddleware.WritePrivateError(w, http.StatusUnauthorized, "Требуется авторизация")
		return
	}

	var request shareLinkRequest
	if err := privateapiutil.DecodeJSONBody(w, r, &request, 4<<10); err != nil {
		transportmiddleware.WritePrivateError(w, http.StatusBadRequest, err.Error())
		return
	}

	response, err := h.registry.CreateStudentShareLink(r.Context(), claims.DiplomaID, request.TTLSeconds)
	if err != nil {
		privateapiutil.HandleError(w, err)
		return
	}
	privateapiutil.WriteJSON(w, http.StatusOK, response)
}

func (h *HTTPHandler) DeleteShareLink(w http.ResponseWriter, r *http.Request) {
	claims, ok := transportmiddleware.PrivateClaimsFromContext(r.Context())
	if !ok {
		transportmiddleware.WritePrivateError(w, http.StatusUnauthorized, "Требуется авторизация")
		return
	}

	if err := h.registry.DeleteStudentShareLink(r.Context(), claims.DiplomaID, r.PathValue("token")); err != nil {
		privateapiutil.HandleError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}
