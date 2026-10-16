package catalog

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Jghh42/atuin-ai/internal/config"
)

func TestPublicJSON(t *testing.T) {
	c := config.Config{Port: 8080, Models: []config.Model{
		{Alias: "z", Name: "Zed", Description: "", Model: "secret-upstream-z"},
		{Alias: "a", Name: "Alpha", Description: "A model", Model: "secret-upstream-a"},
	}}
	for _, tc := range []struct{ defaultAlias, want string }{{"", "z"}, {"a", "a"}} {
		c.DefaultModel = tc.defaultAlias
		r, err := New(c)
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		want := `{"default":"` + tc.want + `","models":[{"alias":"z","name":"Zed","description":""},{"alias":"a","name":"Alpha","description":"A model"}]}`
		if string(data) != want {
			t.Fatalf("got %s; want %s", data, want)
		}
		if c.DefaultModel != tc.defaultAlias {
			t.Fatal("mutated source default")
		}
	}
}

func TestIndependentSnapshot(t *testing.T) {
	c := config.Default()
	r, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	c.Models[0].Name = "changed config"
	if r.Models[0].Name == c.Models[0].Name {
		t.Fatal("source mutation affected response")
	}
	r.Models[0].Alias = "changed response"
	if c.Models[0].Alias == r.Models[0].Alias {
		t.Fatal("response mutation affected source")
	}
}

func TestInvalidConfig(t *testing.T) {
	for _, c := range []config.Config{
		{}, {Port: 8080},
		{Port: 8080, DefaultModel: "missing", Models: config.Default().Models},
	} {
		r, err := New(c)
		if err == nil || !reflect.DeepEqual(r, Response{}) {
			t.Fatalf("got %+v, %v", r, err)
		}
	}
}
