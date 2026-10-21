package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

// maxBodyBytes ограничивает размер тела запроса. Самое длинное поле
// (description) — 4000 символов, так что 1 МБ хватает с большим запасом.
const maxBodyBytes = 1 << 20

// uuidPattern — UUID любой версии в каноническом виде 8-4-4-4-12.
const uuidPattern = `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`

// NewValidator возвращает middleware, которое проверяет каждый запрос
// по OpenAPI-спецификации: параметры пути и query, тело, типы, enum,
// minLength/maxLength, minimum/maximum, required, additionalProperties.
// Невалидный запрос получает 400 VALIDATION_ERROR и до хендлера не доходит.
func NewValidator(spec *openapi3.T) (func(http.Handler) http.Handler, error) {
	// kin-openapi по умолчанию не проверяет format: uuid — включаем.
	openapi3.DefineStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(uuidPattern))

	if err := spec.Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("invalid OpenAPI spec: %w", err)
	}
	// servers в спецификации содержит http://localhost:8080; без этой
	// строки роутер не узнавал бы запросы, пришедшие на другой хост.
	spec.Servers = nil
	router, err := gorillamux.NewRouter(spec)
	if err != nil {
		return nil, fmt.Errorf("build OpenAPI router: %w", err)
	}

	options := &openapi3filter.Options{
		MultiError:         true, // собрать все нарушения, а не только первое
		AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route, pathParams, err := router.FindRoute(r)
			if err != nil {
				// Путь или метод не описан в спецификации — пусть
				// ServeMux ответит 404/405.
				next.ServeHTTP(w, r)
				return
			}

			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
			input := &openapi3filter.RequestValidationInput{
				Request:    r,
				PathParams: pathParams,
				Route:      route,
				Options:    options,
			}
			if err := openapi3filter.ValidateRequest(r.Context(), input); err != nil {
				writeValidationError(w, fieldErrors(err))
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

// fieldErrors превращает ошибки kin-openapi в список {field, message}.
func fieldErrors(err error) []FieldError {
	var out []FieldError
	collect(err, "", &out)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out
}

var unsupportedProperty = regexp.MustCompile(`^property "(.+)" is unsupported$`)

// collect рекурсивно обходит дерево ошибок. Здесь нужен именно type switch,
// а не errors.As: errors.As «проваливается» внутрь RequestError, и тогда
// теряется имя параметра, к которому относится ошибка.
func collect(err error, field string, out *[]FieldError) {
	switch e := err.(type) {
	case openapi3.MultiError:
		for _, inner := range e {
			collect(inner, field, out)
		}

	case *openapi3filter.RequestError:
		if e.Parameter != nil {
			field = e.Parameter.Name
		}
		if e.Err == nil {
			*out = append(*out, FieldError{Field: fieldOrBody(field), Message: e.Reason})
			return
		}
		collect(e.Err, field, out)

	case *openapi3.SchemaError:
		path := e.JSONPointer()
		// Для лишнего поля kin-openapi указывает на объект, а не на поле.
		if m := unsupportedProperty.FindStringSubmatch(e.Reason); m != nil {
			path = append(path, m[1])
		}
		*out = append(*out, FieldError{Field: joinField(field, path), Message: e.Reason})

	case *openapi3filter.ParseError:
		*out = append(*out, FieldError{Field: fieldOrBody(field), Message: e.Error()})

	default:
		*out = append(*out, FieldError{Field: fieldOrBody(field), Message: err.Error()})
	}
}

func joinField(field string, path []string) string {
	parts := path
	if field != "" {
		parts = append([]string{field}, path...)
	}
	return fieldOrBody(strings.Join(parts, "."))
}

func fieldOrBody(field string) string {
	if field == "" {
		return "body"
	}
	return field
}
