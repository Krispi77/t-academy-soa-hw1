package api

import "github.com/shopspring/decimal"

// По контракту price — JSON-число (19990.5), а не строка ("19990.5"),
// поэтому decimal.Decimal сериализуется без кавычек.
func init() {
	decimal.MarshalJSONWithoutQuotes = true
}
