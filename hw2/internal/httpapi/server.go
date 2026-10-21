// Package httpapi реализует HTTP API из openapi/openapi.yaml поверх
// сгенерированного кода из пакета api.
package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/Krispi77/t-academy-soa-hw1/hw2/internal/api"
	"github.com/Krispi77/t-academy-soa-hw1/hw2/internal/product"
)

// Значения по умолчанию для пагинации — те же, что в спецификации.
// Генератор не подставляет default сам, поэтому это делает хендлер.
const (
	defaultPage = 0
	defaultSize = 20
)

// ProductStore — то, что хендлерам нужно от хранилища товаров.
type ProductStore interface {
	Create(ctx context.Context, f product.Fields) (product.Product, error)
	Get(ctx context.Context, id uuid.UUID) (product.Product, error)
	List(ctx context.Context, f product.ListFilter) ([]product.Product, int64, error)
	Update(ctx context.Context, id uuid.UUID, f product.Fields) (product.Product, error)
	Archive(ctx context.Context, id uuid.UUID) (product.Product, error)
}

// Server реализует сгенерированный интерфейс api.StrictServerInterface.
// Сюда приходят только запросы, уже прошедшие валидацию по спецификации.
type Server struct {
	products ProductStore
}

var _ api.StrictServerInterface = (*Server)(nil)

func NewServer(products ProductStore) *Server {
	return &Server{products: products}
}

func (s *Server) CreateProduct(ctx context.Context, req api.CreateProductRequestObject) (api.CreateProductResponseObject, error) {
	b := req.Body
	p, err := s.products.Create(ctx, product.Fields{
		Name:        b.Name,
		Description: b.Description,
		Price:       b.Price,
		Stock:       b.Stock,
		Category:    b.Category,
		Status:      product.Status(b.Status),
	})
	if err != nil {
		return nil, err
	}
	location := "/products/" + p.ID.String()
	return api.CreateProduct201JSONResponse{
		Body:    toResponse(p),
		Headers: api.CreateProduct201ResponseHeaders{Location: &location},
	}, nil
}

func (s *Server) GetProductById(ctx context.Context, req api.GetProductByIdRequestObject) (api.GetProductByIdResponseObject, error) {
	p, err := s.products.Get(ctx, req.Id)
	if errors.Is(err, product.ErrNotFound) {
		return api.GetProductById404JSONResponse{ProductNotFoundJSONResponse: notFound(req.Id)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.GetProductById200JSONResponse(toResponse(p)), nil
}

func (s *Server) ListProducts(ctx context.Context, req api.ListProductsRequestObject) (api.ListProductsResponseObject, error) {
	params := req.Params
	page, size := int32(defaultPage), int32(defaultSize)
	if params.Page != nil {
		page = *params.Page
	}
	if params.Size != nil {
		size = *params.Size
	}

	filter := product.ListFilter{
		Category: params.Category,
		Limit:    int64(size),
		Offset:   int64(page) * int64(size),
	}
	if params.Status != nil {
		st := product.Status(*params.Status)
		filter.Status = &st
	}

	products, total, err := s.products.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	content := make([]api.ProductResponse, 0, len(products))
	for _, p := range products {
		content = append(content, toResponse(p))
	}
	return api.ListProducts200JSONResponse{
		Content:       content,
		TotalElements: total,
		Page:          page,
		Size:          size,
	}, nil
}

func (s *Server) UpdateProduct(ctx context.Context, req api.UpdateProductRequestObject) (api.UpdateProductResponseObject, error) {
	b := req.Body
	p, err := s.products.Update(ctx, req.Id, product.Fields{
		Name:        b.Name,
		Description: b.Description,
		Price:       b.Price,
		Stock:       b.Stock,
		Category:    b.Category,
		Status:      product.Status(b.Status),
	})
	if errors.Is(err, product.ErrNotFound) {
		return api.UpdateProduct404JSONResponse{ProductNotFoundJSONResponse: notFound(req.Id)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.UpdateProduct200JSONResponse(toResponse(p)), nil
}

func (s *Server) DeleteProduct(ctx context.Context, req api.DeleteProductRequestObject) (api.DeleteProductResponseObject, error) {
	p, err := s.products.Archive(ctx, req.Id)
	if errors.Is(err, product.ErrNotFound) {
		return api.DeleteProduct404JSONResponse{ProductNotFoundJSONResponse: notFound(req.Id)}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.DeleteProduct200JSONResponse(toResponse(p)), nil
}

func toResponse(p product.Product) api.ProductResponse {
	return api.ProductResponse{
		Id:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Price:       p.Price,
		Stock:       p.Stock,
		Category:    p.Category,
		Status:      api.ProductStatus(p.Status),
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

func notFound(id uuid.UUID) api.ProductNotFoundJSONResponse {
	return api.ProductNotFoundJSONResponse{
		ErrorCode: api.ErrorCodePRODUCTNOTFOUND,
		Message:   fmt.Sprintf("Product with id %s not found", id),
		Details:   &map[string]any{"product_id": id.String()},
	}
}
