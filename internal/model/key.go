package model

import (
	"crypto/rand"
	"slices"
	"strings"
)

const keyDigits = "0123456789abcdefghijklmnopqrstuvwxyz"

func digit(s string, i int) int {
	if i >= len(s) {
		return 0
	}
	return strings.IndexByte(keyDigits, s[i])
}

func KeyBetween(a, b string) string {
	if b != "" {
		i := 0
		for i < len(b) && digit(a, i) == digit(b, i) {
			i++
		}
		if i > 0 {
			rest := ""
			if i < len(a) {
				rest = a[i:]
			}
			return b[:i] + KeyBetween(rest, b[i:])
		}
	}
	da, db := digit(a, 0), len(keyDigits)
	if b != "" {
		db = digit(b, 0)
	}
	if db-da > 1 {
		if b == "" {
			return string(keyDigits[da+1])
		}
		return string(keyDigits[(da+db)/2])
	}
	if b != "" && len(b) > 1 {
		return b[:1]
	}
	rest := ""
	if len(a) > 1 {
		rest = a[1:]
	}
	return string(keyDigits[da]) + KeyBetween(rest, "")
}

func KeysBetween(a, b string, n int) []string {
	keys := make([]string, n)
	for i := range keys {
		a = KeyBetween(a, b)
		keys[i] = a
	}
	return keys
}

func NewID(prefix string) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz234567"
	buf := make([]byte, 8)
	rand.Read(buf)
	for i := range buf {
		buf[i] = alphabet[buf[i]%32]
	}
	return prefix + string(buf)
}

func InsertAt(list []Block, idx int, add []Block) []Block {
	a, b := "", ""
	if idx > 0 {
		a = list[idx-1].Position
	}
	if idx < len(list) {
		b = list[idx].Position
	}
	keys := KeysBetween(a, b, len(add))
	for i := range add {
		add[i].Position = keys[i]
	}
	return slices.Insert(slices.Clone(list), idx, add...)
}
