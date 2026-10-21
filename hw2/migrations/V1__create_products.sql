-- Таблица товаров. Применяется Flyway при старте docker-compose.

CREATE TYPE product_status AS ENUM ('ACTIVE', 'INACTIVE', 'ARCHIVED');

CREATE TABLE products (
    id          UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255)   NOT NULL CHECK (char_length(name) >= 1),
    description VARCHAR(4000),
    price       NUMERIC(12, 2) NOT NULL CHECK (price > 0),
    stock       INTEGER        NOT NULL CHECK (stock >= 0),
    category    VARCHAR(100)   NOT NULL CHECK (char_length(category) >= 1),
    status      product_status NOT NULL,
    created_at  TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ    NOT NULL DEFAULT now()
);

-- Индекс для фильтра GET /products?status=...
CREATE INDEX idx_products_status ON products (status);

-- Индекс для фильтра по категории и сортировки списка.
CREATE INDEX idx_products_category ON products (category);
CREATE INDEX idx_products_created_at ON products (created_at DESC, id);

-- updated_at обновляется автоматически при любом UPDATE.
CREATE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_products_updated_at
    BEFORE UPDATE ON products
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
