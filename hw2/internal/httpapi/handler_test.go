package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/Krispi77/t-academy-soa-hw1/hw2/internal/product"
)

// fakeStore — хранилище в памяти; запоминает, вызывался ли он.
type fakeStore struct {
	called     bool
	lastFilter product.ListFilter
	items      map[uuid.UUID]product.Product
	err        error
}

func newFakeStore() *fakeStore {
	return &fakeStore{items: map[uuid.UUID]product.Product{}}
}

func (s *fakeStore) Create(_ context.Context, f product.Fields) (product.Product, error) {
	s.called = true
	if s.err != nil {
		return product.Product{}, s.err
	}
	now := time.Now().UTC()
	p := product.Product{ID: uuid.New(), Name: f.Name, Description: f.Description, Price: f.Price,
		Stock: f.Stock, Category: f.Category, Status: f.Status, CreatedAt: now, UpdatedAt: now}
	s.items[p.ID] = p
	return p, nil
}

func (s *fakeStore) Get(_ context.Context, id uuid.UUID) (product.Product, error) {
	s.called = true
	p, ok := s.items[id]
	if !ok {
		return product.Product{}, product.ErrNotFound
	}
	return p, nil
}

func (s *fakeStore) List(_ context.Context, f product.ListFilter) ([]product.Product, int64, error) {
	s.called = true
	s.lastFilter = f
	return nil, 0, s.err
}

func (s *fakeStore) Update(ctx context.Context, id uuid.UUID, f product.Fields) (product.Product, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return product.Product{}, err
	}
	p := s.items[id]
	p.Name, p.Description, p.Price, p.Stock, p.Category, p.Status = f.Name, f.Description, f.Price, f.Stock, f.Category, f.Status
	s.items[id] = p
	return p, nil
}

func (s *fakeStore) Archive(ctx context.Context, id uuid.UUID) (product.Product, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return product.Product{}, err
	}
	p := s.items[id]
	p.Status = product.StatusArchived
	s.items[id] = p
	return p, nil
}

type errorBody struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
	Details   struct {
		Fields []FieldError `json:"fields"`
	} `json:"details"`
}

func newTestHandler(t *testing.T, store ProductStore, logs io.Writer) http.Handler {
	t.Helper()
	if logs == nil {
		logs = io.Discard
	}
	h, err := NewHandler(NewJSONLogger(logs), store)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func do(h http.Handler, method, url, body string) *httptest.ResponseRecorder {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, url, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var e errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("response is not an ErrorResponse: %v: %s", err, rec.Body)
	}
	return e
}

func fieldNames(e errorBody) []string {
	var names []string
	for _, f := range e.Details.Fields {
		names = append(names, f.Field)
	}
	return names
}

const validBody = `{"name":"Phone","description":null,"price":19990.50,"stock":15,"category":"electronics","status":"ACTIVE"}`

func TestCreateProduct(t *testing.T) {
	store := newFakeStore()
	h := newTestHandler(t, store, nil)

	rec := do(h, http.MethodPost, "/products", validBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["price"] != 19990.5 {
		t.Errorf("price must be a JSON number, got %#v", got["price"])
	}
	if got["description"] != nil {
		t.Errorf("description = %#v, want null", got["description"])
	}
	if loc := rec.Header().Get("Location"); loc != "/products/"+got["id"].(string) {
		t.Errorf("Location = %q", loc)
	}
}

func TestValidationRejectsBeforeHandler(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		url        string
		body       string
		wantFields []string
	}{
		{
			name:       "constraints violated",
			method:     http.MethodPost,
			url:        "/products",
			body:       `{"name":"","price":0,"stock":-1,"category":"","status":"FOO"}`,
			wantFields: []string{"category", "name", "price", "status", "stock"},
		},
		{
			name:       "required fields missing",
			method:     http.MethodPost,
			url:        "/products",
			body:       `{}`,
			wantFields: []string{"category", "name", "price", "status", "stock"},
		},
		{
			name:       "server-managed field passed",
			method:     http.MethodPost,
			url:        "/products",
			body:       strings.Replace(validBody, `{`, `{"id":"3fa85f64-5717-4562-b3fc-2c963f66afa6",`, 1),
			wantFields: []string{"id"},
		},
		{
			name:       "wrong types",
			method:     http.MethodPost,
			url:        "/products",
			body:       `{"name":"a","price":"10","stock":1.5,"category":"c","status":"ACTIVE"}`,
			wantFields: []string{"price", "stock"},
		},
		{
			name:       "too long strings",
			method:     http.MethodPut,
			url:        "/products/" + uuid.NewString(),
			body:       `{"name":"` + strings.Repeat("a", 256) + `","description":"` + strings.Repeat("d", 4001) + `","price":1,"stock":0,"category":"` + strings.Repeat("c", 101) + `","status":"ACTIVE"}`,
			wantFields: []string{"category", "description", "name"},
		},
		{
			name:       "price below minimum",
			method:     http.MethodPost,
			url:        "/products",
			body:       strings.Replace(validBody, `19990.50`, `0.009`, 1),
			wantFields: []string{"price"},
		},
		{
			name:       "broken JSON",
			method:     http.MethodPost,
			url:        "/products",
			body:       `{"name":`,
			wantFields: []string{"body"},
		},
		{
			name:       "bad query params",
			method:     http.MethodGet,
			url:        "/products?page=-1&size=101&status=FOO&category=",
			wantFields: []string{"category", "page", "size", "status"},
		},
		{
			name:       "bad uuid",
			method:     http.MethodGet,
			url:        "/products/not-a-uuid",
			wantFields: []string{"id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			h := newTestHandler(t, store, nil)

			rec := do(h, tt.method, tt.url, tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
			}
			if store.called {
				t.Error("invalid request reached the handler")
			}
			e := decodeError(t, rec)
			if e.ErrorCode != "VALIDATION_ERROR" {
				t.Errorf("error_code = %q", e.ErrorCode)
			}
			if got := strings.Join(fieldNames(e), ","); got != strings.Join(tt.wantFields, ",") {
				t.Errorf("fields = %s, want %s (%s)", got, strings.Join(tt.wantFields, ","), rec.Body)
			}
		})
	}
}

func TestListPagination(t *testing.T) {
	store := newFakeStore()
	h := newTestHandler(t, store, nil)

	rec := do(h, http.MethodGet, "/products", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if store.lastFilter.Limit != 20 || store.lastFilter.Offset != 0 {
		t.Errorf("defaults: limit=%d offset=%d, want 20/0", store.lastFilter.Limit, store.lastFilter.Offset)
	}
	var page map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if page["page"] != 0.0 || page["size"] != 20.0 || page["totalElements"] != 0.0 {
		t.Errorf("page = %v", page)
	}
	if content, ok := page["content"].([]any); !ok || len(content) != 0 {
		t.Errorf("content must be an empty array, got %#v", page["content"])
	}

	do(h, http.MethodGet, "/products?page=2&size=5&status=ACTIVE&category=books", "")
	f := store.lastFilter
	if f.Limit != 5 || f.Offset != 10 || *f.Status != product.StatusActive || *f.Category != "books" {
		t.Errorf("filter = %+v", f)
	}
}

func TestNotFound(t *testing.T) {
	h := newTestHandler(t, newFakeStore(), nil)
	id := uuid.NewString()

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		rec := do(h, method, "/products/"+id, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d", method, rec.Code)
		}
		if e := decodeError(t, rec); e.ErrorCode != "PRODUCT_NOT_FOUND" {
			t.Errorf("%s: error_code = %q", method, e.ErrorCode)
		}
	}
}

func TestDeleteIsSoft(t *testing.T) {
	store := newFakeStore()
	h := newTestHandler(t, store, nil)
	p, _ := store.Create(context.Background(), product.Fields{Name: "a", Price: decimal.NewFromInt(1), Category: "c", Status: product.StatusActive})

	for i := 0; i < 2; i++ { // повторный DELETE тоже 200
		rec := do(h, http.MethodDelete, "/products/"+p.ID.String(), "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"ARCHIVED"`) {
			t.Fatalf("attempt %d: %d %s", i+1, rec.Code, rec.Body)
		}
	}
}

func TestInternalError(t *testing.T) {
	store := newFakeStore()
	store.err = errors.New("db is down")
	h := newTestHandler(t, store, nil)

	rec := do(h, http.MethodGet, "/products", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	e := decodeError(t, rec)
	if e.ErrorCode != "INTERNAL_ERROR" || strings.Contains(rec.Body.String(), "db is down") {
		t.Errorf("unexpected body: %s", rec.Body)
	}
}

func TestRequestLogging(t *testing.T) {
	var logs bytes.Buffer
	h := newTestHandler(t, newFakeStore(), &logs)

	body := strings.Replace(validBody, `"stock"`, `"password":"hunter2","stock"`, 1)
	rec := do(h, http.MethodPost, "/products", body) // 400: password — лишнее поле
	requestID := rec.Header().Get(RequestIDHeader)
	if _, err := uuid.Parse(requestID); err != nil {
		t.Fatalf("X-Request-Id = %q is not a UUID", requestID)
	}

	var entry map[string]any
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatalf("log line is not JSON: %v: %s", err, logs.String())
	}
	want := map[string]any{
		"request_id":  requestID,
		"method":      "POST",
		"endpoint":    "/products",
		"status_code": 400.0,
		"user_id":     nil,
	}
	for k, v := range want {
		if got, ok := entry[k]; !ok || got != v {
			t.Errorf("%s = %#v, want %#v", k, got, v)
		}
	}
	if _, ok := entry["duration_ms"].(float64); !ok {
		t.Errorf("duration_ms missing: %v", entry)
	}
	if ts, _ := entry["timestamp"].(string); ts == "" {
		t.Errorf("timestamp missing: %v", entry)
	} else if _, err := time.Parse(time.RFC3339Nano, ts); err != nil {
		t.Errorf("timestamp %q is not ISO 8601", ts)
	}
	reqBody, _ := entry["request_body"].(map[string]any)
	if reqBody["password"] != "***" || reqBody["name"] != "Phone" {
		t.Errorf("request_body = %v", entry["request_body"])
	}
	if strings.Contains(logs.String(), "hunter2") {
		t.Error("password leaked into logs")
	}

	logs.Reset()
	do(h, http.MethodGet, "/products", "")
	if strings.Contains(logs.String(), "request_body") {
		t.Errorf("GET must not log body: %s", logs.String())
	}
}
