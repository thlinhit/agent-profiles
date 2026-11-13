package claude

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSetSkillDirs(t *testing.T) {
	current := []byte(`{"model":"sonnet","permissions":{"allow":["Edit"],"additionalDirectories":["/user/own","/old/skill"]}}`)
	out, err := Adapter{}.SetSkillDirs(current, []string{"/old/skill", "/kept/skill"}, []string{"/kept/skill", "/new/skill"})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Model       string
		Permissions struct {
			Allow                 []string
			AdditionalDirectories []string
		}
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	want := []string{"/user/own", "/kept/skill", "/new/skill"}
	if !reflect.DeepEqual(got.Permissions.AdditionalDirectories, want) {
		t.Fatalf("dirs = %v, want %v", got.Permissions.AdditionalDirectories, want)
	}
	if got.Model != "sonnet" || len(got.Permissions.Allow) != 1 {
		t.Fatalf("other settings changed: %s", out)
	}

	out, err = Adapter{}.SetSkillDirs([]byte(`{"permissions":{"additionalDirectories":["/a"]}}`), []string{"/a"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "{}\n" {
		t.Fatalf("empty reconcile = %s", out)
	}
}
