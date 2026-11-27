package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicCatalogPageUsesEncodedByteBudget(t *testing.T) {
	rows := []providerView{}
	for _, id := range []string{"a", "b", "c", "d"} {
		rows = append(rows, providerView{ID: id, Name: strings.Repeat("<", 300000)})
	}
	value, err := publicPage(rows, false, func(v providerView) string { return v.ID })
	if err != nil {
		t.Fatal(err)
	}
	data, marshalErr := json.Marshal(value)
	if marshalErr != nil || len(data) > 7<<20 {
		t.Fatalf("响应未有界: %d %v", len(data), marshalErr)
	}
	var result struct {
		Items   []providerView `json:"items"`
		HasMore bool           `json:"has_more"`
		Cursor  string         `json:"next_after_id"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if !result.HasMore || len(result.Items) != 3 || result.Cursor != "c" {
		t.Fatalf("序列化分片未保留继续游标: %+v", result)
	}
}
