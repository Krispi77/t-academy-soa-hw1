package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Krispi77/t-academy-soa-hw1/hw2/internal/api"
)

// FieldError — одно нарушение ограничения из спецификации.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, body api.ErrorResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeValidationError(w http.ResponseWriter, fields []FieldError) {
	writeError(w, http.StatusBadRequest, api.ErrorResponse{
		ErrorCode: api.ErrorCodeVALIDATIONERROR,
		Message:   "Request validation failed",
		Details:   &map[string]any{"fields": fields},
	})
}

// RequestErrorHandler вызывается сгенерированным кодом, если он не смог
// разобрать параметры или тело запроса. Обычно такие запросы отсекает
// валидатор раньше; это страховка, чтобы формат ошибки был тем же.
func RequestErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	writeValidationError(w, []FieldError{{Field: "request", Message: err.Error()}})
}

// ResponseErrorHandler вызывается, когда хендлер вернул ошибку
// (например, БД недоступна). Клиенту уходит INTERNAL_ERROR без деталей,
// а подробности — в лог.
func ResponseErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "request failed",
		slog.String("request_id", RequestIDFromContext(r.Context())),
		slog.String("error", err.Error()))
	writeError(w, http.StatusInternalServerError, api.ErrorResponse{
		ErrorCode: api.ErrorCodeINTERNALERROR,
		Message:   "Internal server error",
	})
}
