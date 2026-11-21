package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCardStyleFlag(t *testing.T) {
	cases := []struct {
		mode      string
		cmd       func() *cobra.Command
		wantStyle string
	}{
		{mode: "list", cmd: listCmd, wantStyle: "space"},
		{mode: "feed", cmd: feedCmd, wantStyle: "none"},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			flag := tc.cmd().Flags().Lookup("card-style")
			if flag == nil || flag.DefValue != tc.wantStyle {
				t.Fatalf("%s --card-style = %v, want a flag that defaults to %s", tc.mode, flag,
					tc.wantStyle)
			}

			c := tc.cmd()
			for _, style := range []string{"none", "space", "line", "box"} {
				if err := c.ParseFlags([]string{"--card-style", style}); err != nil {
					t.Errorf("%s --card-style %s error = %v", tc.mode, style, err)
				}
				if got := c.Flags().Lookup("card-style").Value.String(); got != style {
					t.Errorf("%s --card-style %s set %s", tc.mode, style, got)
				}
			}

			r := root()
			r.SetArgs([]string{tc.mode, "--card-style", "boxed"})
			if err := r.Execute(); !errors.Is(err, ErrUsage) {
				t.Errorf("%s --card-style boxed error = %v, want ErrUsage", tc.mode, err)
			}
		})
	}
}

func TestNewestFlag(t *testing.T) {
	for _, mode := range []struct {
		name string
		cmd  func() *cobra.Command
	}{{"list", listCmd}, {"feed", feedCmd}} {
		t.Run(mode.name, func(t *testing.T) {
			flag := mode.cmd().Flags().Lookup("newest")
			if flag == nil || flag.DefValue != "bottom" {
				t.Fatalf("%s --newest = %v, want a flag that defaults to bottom", mode.name, flag)
			}

			c := mode.cmd()
			for _, end := range []string{"top", "bottom"} {
				if err := c.ParseFlags([]string{"--newest", end}); err != nil {
					t.Errorf("%s --newest %s error = %v", mode.name, end, err)
				}
				if got := c.Flags().Lookup("newest").Value.String(); got != end {
					t.Errorf("%s --newest %s set %s", mode.name, end, got)
				}
			}

			r := root()
			r.SetArgs([]string{mode.name, "--newest", "middle"})
			err := r.Execute()
			if !errors.Is(err, ErrUsage) ||
				!strings.Contains(err.Error(), `invalid argument "middle"`) {
				t.Errorf("%s --newest middle error = %v, want a flag error wrapping ErrUsage",
					mode.name, err)
			}
		})
	}
}

func TestEscQuitsFlag(t *testing.T) {
	flag := listCmd().Flags().Lookup("esc-quits")
	if flag == nil || flag.DefValue != "false" {
		t.Fatalf("list --esc-quits = %v, want a flag that defaults to false", flag)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{args: nil, want: "false"},
		{args: []string{"--esc-quits"}, want: "true"},
		{args: []string{"--esc-quits=false"}, want: "false"},
	} {
		c := listCmd()
		if err := c.ParseFlags(tc.args); err != nil {
			t.Errorf("list %v error = %v", tc.args, err)
		}
		if got := c.Flags().Lookup("esc-quits").Value.String(); got != tc.want {
			t.Errorf("list %v set --esc-quits to %s, want %s", tc.args, got, tc.want)
		}
	}

	if feedCmd().Flags().Lookup("esc-quits") != nil {
		t.Error("feed has --esc-quits, want it in list mode only")
	}
	r := root()
	r.SetArgs([]string{"feed", "--esc-quits"})
	if err := r.Execute(); !errors.Is(err, ErrUsage) {
		t.Errorf("feed --esc-quits error = %v, want ErrUsage", err)
	}
}
