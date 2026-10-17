package integration

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/Jghh42/atuin-ai/tests/compatfixture"
)

func TestPhase8ManifestCoversCorpus(t *testing.T) {
	cases, e := compatfixture.Load("../../testdata/compat/phase8.json")
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile("../../testdata/compat/phase8-manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	var manifest struct {
		Sources map[string]struct {
			SHA     string `json:"upstream_commit"`
			License string `json:"license_spdx"`
		} `json:"sources"`
		Cases []struct {
			ID, Source  string
			Differences []string `json:"intentional_behavior_differences"`
		} `json:"differential_cases"`
	}
	if e = json.Unmarshal(b, &manifest); e != nil {
		t.Fatal(e)
	}
	wantSources := map[string]string{"cli": "4ca8b77c8a526b8c294eb864a2b74c95c685c983", "server": "4d582bc5ceea5b5edfdcf3abb49dc850400cda7c", "core": "565877e97ed3ff4840481b8d47e12eb2f9854039"}
	for id, sha := range wantSources {
		source := manifest.Sources[id]
		if source.SHA != sha || source.License == "" {
			t.Fatal("source provenance changed without review")
		}
	}
	entries := map[string][]string{}
	for _, entry := range manifest.Cases {
		if _, exists := entries[entry.ID]; exists {
			t.Fatal("duplicate provenance case")
		}
		if _, ok := manifest.Sources[entry.Source]; !ok {
			t.Fatal("missing upstream provenance")
		}
		if entry.Differences == nil {
			t.Fatal("explicit difference inventory required")
		}
		entries[entry.ID] = entry.Differences
	}
	if len(entries) != len(cases) {
		t.Fatal("fixture and provenance inventories differ")
	}
	for _, c := range cases {
		d, ok := entries[c.Name]
		if !ok || (len(d) > 0) != (c.Difference != "") {
			t.Fatalf("missing/inconsistent per-case provenance: %s", c.Name)
		}
	}
}
func TestTextSegmentationNormalization(t *testing.T) {
	one := []compatfixture.Event{{Event: "text", Data: map[string]any{"content": "hello world"}}, {Event: "done", Data: map[string]any{"session_id": "01900000-0000-7000-8000-000000000001", "usage": map[string]any{"input_tokens": 1}}}}
	split := []compatfixture.Event{{Event: "text", Data: map[string]any{"content": "hello "}}, {Event: "text", Data: map[string]any{"content": "world"}}, {Event: "done", Data: map[string]any{"session_id": "01900000-0000-7000-8000-000000000002", "usage": map[string]any{"input_tokens": 2}}}}
	if !reflect.DeepEqual(compatfixture.Normalize(one), compatfixture.Normalize(split)) {
		t.Fatal("documented nonbehavior normalization differs")
	}
	if split[0].Data["content"] != "hello " {
		t.Fatal("normalization mutated the original oracle event")
	}
}
