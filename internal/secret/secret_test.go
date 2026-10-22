package secret

import (
	"strings"
	"testing"
)

func TestPassword(t *testing.T) {
	h := HashPassword("correct horse battery")
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatal(h)
	}
	if !CheckPassword(h, "correct horse battery") || CheckPassword(h, "wrong") || CheckPassword("!", "x") {
		t.Fatal("password check wrong")
	}
	if HashPassword("same") == HashPassword("same") {
		t.Fatal("salt not random")
	}
}

func TestToken(t *testing.T) {
	p, h, l := NewToken()
	if !strings.HasPrefix(p, TokenPrefix) || len(p) < 60 || h != Hash(p) || !strings.HasSuffix(p, l) || len(l) != 4 {
		t.Fatalf("%s %s %s", p, h, l)
	}
	if p2, _, _ := NewToken(); p2 == p {
		t.Fatal("tokens repeat")
	}
}
