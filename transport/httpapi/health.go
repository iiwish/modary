package httpapi

import (
	"fmt"
	"net/http"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/appkit"
)

type healthResponse struct {
	Status      string          `json:"status"`
	Application appkit.Metadata `json:"application"`
}

// NewHealth returns an explicitly mounted readiness endpoint. Its response is
// intentionally limited to public application metadata and readiness state.
func NewHealth(application *appkit.Application) (http.Handler, error) {
	if application == nil {
		return nil, fmt.Errorf("http health application is required")
	}
	metadata := application.Metadata()
	if metadata.ID == "" || metadata.Name == "" || metadata.Version == "" {
		return nil, fmt.Errorf("http health application is unavailable")
	}
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := prepareResponse(writer, request)
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			writer.Header().Set("Allow", "GET, HEAD")
			writePublicError(writer, http.StatusMethodNotAllowed, requestID, action.CodeValidationFailed, "method is not allowed")
			return
		}
		if !acceptsJSON(request) {
			writePublicError(writer, http.StatusNotAcceptable, requestID, action.CodeValidationFailed, "response must be accepted as application/json")
			return
		}
		if !requestHasNoBody(request) || request.URL.RawQuery != "" {
			writePublicError(writer, http.StatusBadRequest, requestID, action.CodeValidationFailed, "health request must not contain a body or query")
			return
		}
		if !application.Ready() {
			writeJSONMethod(writer, request.Method, http.StatusServiceUnavailable, healthResponse{Status: "unavailable", Application: metadata})
			return
		}
		writeJSONMethod(writer, request.Method, http.StatusOK, healthResponse{Status: "ready", Application: metadata})
	})
	return handler, nil
}
