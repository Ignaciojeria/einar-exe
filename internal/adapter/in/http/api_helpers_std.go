package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-fuego/fuego"
)

// Helpers compartidos por handlers std (DeleteStd/PatchStd que no usan
// la magia de fuego para serializar respuestas/errores).
//
// Nota: fuego sí tiene helpers, pero los handlers std los pierden al
// trabajar directo con http.ResponseWriter. Re-implementamos con el
// shape consistente que usa el resto de la API.

func writeAPIJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeAPIError escribe un error con el shape de fuego.HTTPError:
// { "title", "status", "detail" }.
func writeAPIError(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"title":  title,
		"status": status,
		"detail": detail,
	})
}

// writeAPIErrorFromHTTPError extrae status/title/detail si el error
// es un fuego.HTTPError; sino lo trata como 500.
func writeAPIErrorFromHTTPError(w http.ResponseWriter, err error) {
	var he fuego.HTTPError
	if errors.As(err, &he) {
		writeAPIError(w, he.Status, he.Title, he.Detail)
		return
	}
	writeAPIError(w, http.StatusInternalServerError, "internal error", err.Error())
}

// decodeJSON parsea el body limitado (1 MB) en `dst`.
func decodeJSON(r *http.Request, dst any) error {
	const maxBody = 1 << 20 // 1 MB
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
