package main

import (
	"encoding/json"
	"fmt"
	"github.com/zhanghaiyang/adflow/internal/decision/domain"
	"github.com/zhanghaiyang/adflow/internal/profile/schema"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		panic("Usage: go run ./tests/plan-sample-contract <samples.json>")
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	var cases []struct {
		Rule    domain.TargetingRule
		Profile struct {
			UserID string
			Tags   []string
			Fields map[string]string
		}
		Expected bool
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		panic(err)
	}
	for i, c := range cases {
		if err := schema.ValidateFields(c.Profile.Fields); err != nil {
			panic(fmt.Sprintf("case %d invalid fields: %v", i, err))
		}
		actual := (domain.Evaluator{}).Match(domain.NewProfile(c.Profile.UserID, c.Profile.Tags, c.Profile.Fields), c.Rule)
		if actual != c.Expected {
			panic(fmt.Sprintf("case %d: got %v expected %v", i, actual, c.Expected))
		}
	}
	fmt.Printf("PASS: %d generated samples agree with Go Evaluator and field schema; no API requests or database writes\n", len(cases))
}
