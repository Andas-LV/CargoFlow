// Package middleware contains the gateway's HTTP middleware: auth, rate limiting, logging.
package middleware

import (
	"encoding/json"
	"net/http"

	"google.golang.org/grpc/codes"
)

// writeError responds in the same JSON shape as grpc-gateway errors
// ({"code", "message", "details"}), so the frontend handles a single error format.
func writeError(w http.ResponseWriter, httpStatus int, code codes.Code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(errorBody{Code: int(code), Message: msg, Details: []any{}})
}

type errorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details []any  `json:"details"`
}
