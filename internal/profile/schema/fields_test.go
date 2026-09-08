package schema

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSharedFieldConditionContract(t *testing.T) {
	data, err := os.ReadFile("testdata/conditions.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Field, Op, Value string
		Valid            bool
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		if got := ValidateCondition(tc.Field, tc.Op, tc.Value) == nil; got != tc.Valid {
			t.Errorf("%+v: got %v", tc, got)
		}
	}
}

func TestProfileCoreFieldsValidateWithoutDroppingLegacyFields(t *testing.T) {
	for _, fields := range []map[string]string{{"score": "NaN"}, {"score": "101"}, {"device": "unknown"}} {
		if ValidateFields(fields) == nil {
			t.Fatalf("accepted %v", fields)
		}
	}
	if err := ValidateFields(map[string]string{"score": "88.5", "device": "android", "country": "CN"}); err != nil {
		t.Fatal(err)
	}
}

func TestAgeAndDictionaryProfileValues(t *testing.T) {
	for _, fields := range []map[string]string{{"age": "18.5"}, {"age": "121"}, {"member_level": "vip100"}, {"channel": "unknown"}, {"member_level": " gold "}} {
		if ValidateFields(fields) == nil {
			t.Fatalf("accepted invalid fields %v", fields)
		}
	}
	if err := ValidateFields(map[string]string{"age": "25", "member_level": "gold", "channel": "organic", "country": "CN"}); err != nil {
		t.Fatal(err)
	}
}
