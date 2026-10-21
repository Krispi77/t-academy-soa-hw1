package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// RequestIDHeader — заголовок ответа с идентификатором запроса.
const RequestIDHeader = "X-Request-Id"

// maxLoggedBody — сколько байт тела запроса попадает в лог.
const maxLoggedBody = 8 << 10

// sensitiveKeys — поля, значения которых маскируются в логах.
// Сравнение без учёта регистра и по вхождению: "password",
// "new_password", "refreshToken" и т.п.
var sensitiveKeys = []string{"password", "token", "secret"}

const masked = "***"

type ctxKey struct{}

// RequestIDFromContext возвращает request_id текущего запроса.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// Logging пишет одну JSON-строку на каждый запрос и проставляет
// заголовок X-Request-Id. Должно стоять первым в цепочке middleware,
// чтобы в лог попадали и запросы, отклонённые валидатором.
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			requestID := uuid.NewString()
			w.Header().Set(RequestIDHeader, requestID)
			r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, requestID))

			var body any
			if isMutating(r.Method) {
				body = readBodyForLog(r)
			}

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			// Время записи — момент начала запроса (поле timestamp).
			record := slog.NewRecord(start, slog.LevelInfo, "http request", 0)
			record.AddAttrs(
				slog.String("request_id", requestID),
				slog.String("method", r.Method),
				slog.String("endpoint", r.URL.Path),
				slog.Int("status_code", rec.status),
				slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
				slog.Any("user_id", nil), // авторизации пока нет
			)
			if isMutating(r.Method) {
				record.AddAttrs(slog.Any("request_body", body))
			}
			_ = logger.Handler().Handle(r.Context(), record)
		})
	}
}

// NewJSONLogger создаёт slog-логгер в формате JSON, где время записи
// выводится в поле timestamp в формате ISO 8601 (UTC).
func NewJSONLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.String("timestamp", a.Value.Time().UTC().Format(time.RFC3339Nano))
			}
			return a
		},
	}))
}

func isMutating(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete
}

// readBodyForLog читает тело, возвращает его обратно в запрос и отдаёт
// версию для лога: JSON с замаскированными чувствительными полями,
// строку для не-JSON или nil для пустого тела.
func readBodyForLog(r *http.Request) any {
	if r.Body == nil || r.Body == http.NoBody {
		return nil
	}
	// Читаем не больше maxLoggedBody+1 байт; остаток тела остаётся
	// в запросе и дочитывается хендлером.
	data, err := io.ReadAll(io.LimitReader(r.Body, maxLoggedBody+1))
	r.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(data), r.Body), r.Body}
	if err != nil || len(data) == 0 {
		return nil
	}
	if len(data) > maxLoggedBody {
		// Обрезанный JSON не разобрать и не замаскировать — не логируем.
		return "<body too large to log>"
	}

	var parsed any
	if json.Unmarshal(data, &parsed) == nil {
		return maskSensitive(parsed)
	}
	return string(data)
}

func maskSensitive(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if isSensitive(k) {
				t[k] = masked
			} else {
				t[k] = maskSensitive(val)
			}
		}
	case []any:
		for i := range t {
			t[i] = maskSensitive(t[i])
		}
	}
	return v
}

func isSensitive(key string) bool {
	key = strings.ToLower(key)
	for _, s := range sensitiveKeys {
		if strings.Contains(key, s) {
			return true
		}
	}
	return false
}

// statusRecorder запоминает HTTP-код ответа.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wroteHeader = true
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
