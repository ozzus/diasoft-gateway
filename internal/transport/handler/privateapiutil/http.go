package privateapiutil

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/ssovich/diasoft-gateway/internal/privateapi"
	transportmiddleware "github.com/ssovich/diasoft-gateway/internal/transport/middleware"
)

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func DecodeJSONBody(w http.ResponseWriter, r *http.Request, dst any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return errors.New("invalid request body")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("invalid request body")
	}
	return nil
}

func HandleError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	var apiErr *privateapi.APIError
	if errors.As(err, &apiErr) {
		transportmiddleware.WritePrivateError(w, apiErr.StatusCode, apiErr.Message)
		return
	}
	transportmiddleware.WritePrivateError(w, http.StatusInternalServerError, "internal error")
}
