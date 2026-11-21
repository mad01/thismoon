package order

import (
	"slices"
	"strings"
	"testing"
)

func TestNewestText(t *testing.T) {
	if got, want := Names(), []string{"bottom", "top"}; !slices.Equal(got, want) {
		t.Errorf("Names() = %q, want %q", got, want)
	}
	var zero Newest
	if zero != Bottom {
		t.Errorf("zero Newest = %v, want bottom", zero)
	}
	for _, n := range []Newest{Bottom, Top} {
		text, err := n.MarshalText()
		if err != nil {
			t.Fatalf("%v.MarshalText() error = %v", n, err)
		}
		var back Newest
		if err := back.UnmarshalText(text); err != nil || back != n {
			t.Errorf("UnmarshalText(%q) = %v, %v, want %v", text, back, err, n)
		}
	}

	back := Top
	err := back.UnmarshalText([]byte("middle"))
	if err == nil || !strings.Contains(err.Error(), "bottom, top") {
		t.Errorf("UnmarshalText(middle) error = %v, want one naming the ends", err)
	}
	if back != Top {
		t.Errorf("UnmarshalText(middle) set %v, want it left as top", back)
	}
}
