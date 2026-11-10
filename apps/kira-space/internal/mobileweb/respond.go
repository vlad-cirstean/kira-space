package mobileweb

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/kirathecat/kira-studio/internal/ipcerr"
	"github.com/kirathecat/kira-studio/packages/theme/src/i18n"
)

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Reason  string `json:"reason,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Debug("mobileweb: write response", "scope", "mobileweb", "err", err)
	}
}

// writeError uses the `{code, message}` shape ipcerr.Error marshals to, which the shared
// toCodedError parses on the phone.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Code: code, Message: message})
}

// writeErrorT answers with the catalog text for key.
func writeErrorT(w http.ResponseWriter, status int, code, key string) {
	writeError(w, status, code, i18n.T(key, nil))
}

func writeRateLimited(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "10")
	writeErrorT(w, http.StatusTooManyRequests, codeRateLimited, "backend.space.mobileweb.tooManyRequests.body")
}

// writeServiceError maps a bridge error to a status. Internal failures are logged and answered
// with a generic message: the phone never sees paths or driver text.
func writeServiceError(w http.ResponseWriter, err error) {
	var ie *ipcerr.Error
	if errors.As(err, &ie) {
		switch ie.Code {
		case string(ipcerr.CodeBadRequest), string(ipcerr.CodeContract):
			writeError(w, http.StatusBadRequest, ie.Code, ie.Message)
			return
		case string(ipcerr.CodeNotFound):
			writeError(w, http.StatusNotFound, ie.Code, ie.Message)
			return
		case string(ipcerr.CodeConflict), string(ipcerr.CodePreparing), string(ipcerr.CodeTaskNoRepo), string(ipcerr.CodeUnavailable):
			writeError(w, http.StatusConflict, ie.Code, ie.Message)
			return
		case string(ipcerr.CodeTimeout):
			writeError(w, http.StatusGatewayTimeout, ie.Code, ie.Message)
			return
		}
	}
	slog.Warn("mobileweb: service error", "scope", "mobileweb", "err", err)
	writeErrorT(w, http.StatusInternalServerError, string(ipcerr.CodeInternal), "backend.error.internal.body")
}
