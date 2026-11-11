package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/AGlElectronics/rally/backend/internal/authbase"
	"github.com/AGlElectronics/rally/backend/internal/httperror"
)

// maxJSONBody limits every JSON request body. Files never pass through the
// API (browsers upload straight to S3), so 1 MiB is far above any form.
const maxJSONBody = authbase.DefaultMaxBodyBytes

// writeJSON serializes body with the standard content-type. Errors on the
// writer path (e.g. client disconnected) are intentionally swallowed — the
// standard access log already records the status.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if body == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(body)
}

// decodeJSON strict-decodes the request body (at most maxJSONBody bytes)
// into dst and returns a typed error (400, or 413 for a body over the
// limit) that handlers hand straight to httperror.Render.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	err := authbase.DecodeJSON(w, r, dst, authbase.DecodeOptions{MaxBytes: maxJSONBody, DisallowUnknownFields: true})
	return decodeErr(err)
}

// decodeOptionalJSON is decodeJSON for endpoints whose body may be left
// out: an empty body leaves dst untouched.
func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	err := authbase.DecodeJSON(w, r, dst, authbase.DecodeOptions{MaxBytes: maxJSONBody, DisallowUnknownFields: true})
	if errors.Is(err, io.EOF) {
		return nil
	}
	return decodeErr(err)
}

func decodeErr(err error) error {
	if err == nil {
		return nil
	}
	var de *authbase.DecodeError
	if !errors.As(err, &de) {
		return httperror.BadRequest("invalid JSON body")
	}
	switch de.Status {
	case http.StatusRequestEntityTooLarge:
		return httperror.New(httperror.CodeRequestTooLarge, de.Message)
	case http.StatusUnsupportedMediaType:
		return httperror.New(httperror.CodeUnsupportedMediaType, de.Message)
	}
	return httperror.BadRequest(de.Message)
}

// parseUUIDParam reads a chi URL parameter and parses it as a UUID. The
// `name` is used verbatim in the error message, so pass a human-friendly
// label like "organization" rather than the raw URL fragment name.
func parseUUIDParam(r *http.Request, name string) (uuid.UUID, error) {
	raw := chi.URLParam(r, name)
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, httperror.BadRequest("invalid " + name + " id: " + raw)
	}
	return id, nil
}
