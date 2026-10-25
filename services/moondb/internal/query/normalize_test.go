package query

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"   \t ", ""},
		{"x := one()", "x := one()"},
		{"\t\tx  :=   one()  ", "x := one()"},
		{"x := one()\r", "x := one()"},
		{"a\tb", "a b"},
	}
	for _, tc := range tests {
		if got := normalize(tc.in); got != tc.want {
			t.Errorf("normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTrivial(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"", true},
		{"}", true},
		{"})", true},
		{"*/", true},
		{"return", true},
		{"end", true},
		{"} else {", true},
		{"if err != nil {", true},
		{"return nil, err", true},
		{"return true", true},
		{"import (", true},
		{`import "fmt"`, true},
		{"import os", true},
		{"from os import path", true},
		{"#include <stdio.h>", true},
		{"package query", true},
		{"return x", false},
		{"x := one()", false},
		{"if n > 0 {", false},
		{"0", false},
		{"from the start", false},
		{"important := true", false},
		{"packages := nil", false},
		{"Return Value", false},
	}
	for _, tc := range tests {
		if got := trivial(normalize(tc.line)); got != tc.want {
			t.Errorf("trivial(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}
