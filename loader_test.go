package dsr

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dpopsuev/origami/schematics/toolkit"
)

func testdataPath(name string) string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(f), "testdata", name)
}

func TestLoadFromPath_NewFormat_YAML(t *testing.T) {
	cat, err := LoadFromPath(testdataPath("catalog.yaml"))
	if err != nil {
		t.Fatalf("LoadFromPath: %v", err)
	}
	if len(cat.Items) != 4 {
		t.Fatalf("want 4 sources, got %d", len(cat.Items))
	}
	s := cat.Items[0]
	if s.Name != "ptp-operator" || s.Kind != toolkit.SourceKindRepo {
		t.Errorf("first source: got %+v", s)
	}
	if s.Tags["component"] != "ptp" {
		t.Errorf("tags: got %v", s.Tags)
	}
	if cat.Items[1].Kind != toolkit.SourceKindSpec {
		t.Errorf("second source kind: got %q", cat.Items[1].Kind)
	}
	if cat.Items[2].Kind != toolkit.SourceKindDoc {
		t.Errorf("third source kind: got %q", cat.Items[2].Kind)
	}
}

func TestLoadFromPath_NewFormat_JSON(t *testing.T) {
	cat, err := LoadFromPath(testdataPath("catalog.json"))
	if err != nil {
		t.Fatalf("LoadFromPath: %v", err)
	}
	if len(cat.Items) != 3 {
		t.Fatalf("want 3 sources, got %d", len(cat.Items))
	}
	if cat.Items[0].Tags["team"] != "platform" {
		t.Errorf("tags: got %v", cat.Items[0].Tags)
	}
}

func TestLoad_DetectJSON(t *testing.T) {
	data := []byte(`{"sources":[{"name":"a","kind":"repo","uri":"/a"}]}`)
	cat, err := Load(data, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cat.Items) != 1 || cat.Items[0].Name != "a" {
		t.Errorf("got %+v", cat)
	}
}

func TestLoad_DetectYAML(t *testing.T) {
	data := []byte("sources:\n  - name: x\n    kind: doc\n    uri: /x\n")
	cat, err := Load(data, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cat.Items) != 1 || cat.Items[0].Kind != toolkit.SourceKindDoc {
		t.Errorf("got %+v", cat)
	}
}

func TestLoad_EmptyCatalog(t *testing.T) {
	data := []byte("{}")
	cat, err := Load(data, ".json")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cat.Items) != 0 {
		t.Errorf("expected empty, got %d", len(cat.Items))
	}
}
