package skills

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLinksClassification(t *testing.T) {
	e := selectFixture(t)
	source, err := filepath.EvalSymlinks(filepath.Join(e.poolA, "code-review"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "..", "..", "missing"), filepath.Join(e.dir, "relative")); err != nil {
		t.Fatal(err)
	}
	got := map[string]Link{}
	for link, err := range Links(e.dir, map[string]string{source: e.poolA}) {
		if err != nil {
			t.Fatal(err)
		}
		got[link.Name] = link
	}
	for name, kind := range map[string]LinkKind{
		"code-review": Pool, "review": Alias, "old-tool": External,
		"stale": Broken, "relative": Broken, "notes": NonLink,
	} {
		if got[name].Kind != kind {
			t.Errorf("%s kind = %v, want %v", name, got[name].Kind, kind)
		}
	}
	if got["review"].Source != source || got["review"].Pool != e.poolA || got["review"].Target != filepath.Join(e.poolA, "code-review") {
		t.Errorf("alias = %+v", got["review"])
	}
	if got["stale"].Source != "" || got["relative"].Target != filepath.Join("..", "..", "..", "..", "missing") {
		t.Errorf("broken links = %+v, %+v", got["stale"], got["relative"])
	}
}

func TestLinksMissingDirectoryAndEarlyStop(t *testing.T) {
	e := selectFixture(t)
	for _, pools := range []map[string]string{nil, {}} {
		for link, err := range Links(filepath.Join(e.dir, "absent"), pools) {
			t.Fatalf("missing directory yielded %+v, %v", link, err)
		}
	}
	var names []string
	for link, err := range Links(e.dir, nil) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, link.Name)
		break
	}
	if !reflect.DeepEqual(names, []string{"code-review"}) {
		t.Fatalf("early stop = %v", names)
	}
}
