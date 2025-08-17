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
	if len(gaps) > 0 {
		t.Fatalf("found %d unresolved gap(s); update reports/gaps.* after fixing", len(gaps))
	}
}
