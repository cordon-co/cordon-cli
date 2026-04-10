package store

import "testing"

func TestMatchPathPattern_DoubleStarCharacterClass(t *testing.T) {
	if !matchPathPattern("**/file[0-9].env", "a/b/file7.env") {
		t.Fatal("expected **/file[0-9].env to match a/b/file7.env")
	}
}

func TestMatchPathPattern_DoubleStarUnterminatedClassLiteral(t *testing.T) {
	if !matchPathPattern("**/file[.env", "a/b/file[.env") {
		t.Fatal("expected unterminated class pattern to match literal '[' path")
	}
}
