package privateauth

import (
	"net/http"

	"github.com/ssovich/diasoft-gateway/internal/privateapi"
	"github.com/ssovich/diasoft-gateway/internal/transport/handler/privateapiutil"
	transportmiddleware "github.com/ssovich/diasoft-gateway/internal/transport/middleware"
)

const maxAuthBodyBytes = 4 << 10

type HTTPHandler struct {
	service *privateapi.Service
}

type loginRequest struct {
	Login    string          `json:"login"`
	Password string          `json:"password"`
	Role     privateapi.Role `json:"role"`
}

func NewHTTPHandler(service *privateapi.Service) *HTTPHandler {
	return &HTTPHandler{service: service}
}

func (h *HTTPHandler) Login(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	if err := privateapiutil.DecodeJSONBody(w, r, &request, maxAuthBodyBytes); err != nil {
		transportmiddleware.WritePrivateError(w, http.StatusBadRequest, err.Error())
		return
	}

	response, err := h.service.Login(r.Context(), request.Login, request.Password, request.Role)
	if err != nil {
		privateapiutil.HandleError(w, err)
		return
	}

	privateapiutil.WriteJSON(w, http.StatusOK, response)
}

func (h *HTTPHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := transportmiddleware.PrivateClaimsFromContext(r.Context())
	if !ok {
		transportmiddleware.WritePrivateError(w, http.StatusUnauthorized, "Требуется авторизация")
		return
	}

	response, err := h.service.Me(r.Context(), claims)
	if err != nil {
		privateapiutil.HandleError(w, err)
		return
	}

	privateapiutil.WriteJSON(w, http.StatusOK, response)
}
