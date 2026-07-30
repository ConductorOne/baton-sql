package studio

import (
	"encoding/json"
	"testing"
)

func TestSpec_JSONRoundTrip(t *testing.T) {
	in := Spec{
		AppName: "Finance DB",
		Connect: ConnectConfig{Scheme: "mysql", Host: "db", Port: "3306", Database: "finance"},
		ResourceTypes: []ResourceTypeSpec{{
			ID: "users", Name: "Users", Trait: "user",
			List: ListSpec{Query: "SELECT id FROM employees", Fields: []FieldMapping{{Field: "id", Column: "id"}}},
		}},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Spec
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.AppName != in.AppName || out.ResourceTypes[0].List.Fields[0].Field != "id" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}
