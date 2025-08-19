package reports

import (
	"encoding/json"
	"os"
	"testing"
)

func TestNoUnresolvedGaps(t *testing.T) {
	data, err := os.ReadFile("gaps.json")
	if err != nil {
		t.Fatalf("read gaps.json: %v", err)
	}
	var gaps []map[string]any
	if err := json.Unmarshal(data, &gaps); err != nil {
		t.Fatalf("parse gaps.json: %v", err)
	}
	unresolved := 0
	for _, g := range gaps {
		if s, ok := g["status"].(string); ok && s == "closed" {
			continue
		}
		unresolved++
	}
	if unresolved > 0 {
		t.Fatalf("found %d unresolved gap(s); update reports/gaps.* after fixing", unresolved)
	}
}
