package hook

import "testing"

func TestBashReadTargets_HeadAndDelimiters(t *testing.T) {
	got := bashReadTargets("head -n 20 README.md && cat .env")
	wantHas := []string{"README.md", ".env"}
	for _, w := range wantHas {
		found := false
		for _, g := range got {
			if g == w {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("targets %#v missing expected %q", got, w)
		}
	}
}

func TestReSedInPlaceTargets_Variants(t *testing.T) {
	tests := []string{
		`sed -i 's/a/b/' file.txt`,
		`sed -i.bak 's/a/b/' file.txt`,
	}
	for _, cmd := range tests {
		got := reSedInPlaceTargets(cmd)
		if len(got) != 1 || got[0] != "file.txt" {
			t.Fatalf("reSedInPlaceTargets(%q) = %#v, want [file.txt]", cmd, got)
		}
	}
}
