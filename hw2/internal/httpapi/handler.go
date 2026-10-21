package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/Krispi77/t-academy-soa-hw1/hw2/internal/api"
)

// NewHandler собирает цепочку обработки запроса:
//
//	логирование → валидация по OpenAPI → сгенерированный роутер → хендлеры
func NewHandler(logger *slog.Logger, products ProductStore) (http.Handler, error) {
	spec, err := api.GetSwagger()
	if err != nil {
		return nil, fmt.Errorf("load embedded OpenAPI spec: %w", err)
	}
	validate, err := NewValidator(spec)
	if err != nil {
		return nil, err
	}

	strict := api.NewStrictHandlerWithOptions(NewServer(products), nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  RequestErrorHandler,
		ResponseErrorHandlerFunc: ResponseErrorHandler,
	})
	routes := api.HandlerWithOptions(strict, api.StdHTTPServerOptions{
		ErrorHandlerFunc: RequestErrorHandler,
	})

	return Logging(logger)(validate(routes)), nil
}
