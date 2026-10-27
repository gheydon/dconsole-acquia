package argv

import (
	"reflect"
	"testing"
)

func TestShellQuote(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{"simple", []string{"drush", "cr"}, "'drush' 'cr'"},
		{"empty element", []string{"a", "", "b"}, "'a' '' 'b'"},
		{"spaces survive", []string{"drush", "config:set", "system.site name", "My Site"},
			"'drush' 'config:set' 'system.site name' 'My Site'"},
		{"contains single quote", []string{"echo", "it's"}, `'echo' 'it'\''s'`},
		{"shell metachars neutralised", []string{"echo", "$HOME", "`pwd`", "a&&b;c|d"},
			"'echo' '$HOME' '`pwd`' 'a&&b;c|d'"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ShellQuote(c.in)
			if got != c.want {
				t.Errorf("ShellQuote(%v)\n  got  %s\n  want %s", c.in, got, c.want)
			}
		})
	}
}

func TestBuild(t *testing.T) {
	cases := []struct {
		name   string
		prefix []string
		alias  string
		cmd    []string
		want   []string
	}{
		{
			"exec with drush command",
			Default, "myapp.prod", []string{"drush", "cr"},
			[]string{"acli", "remote:ssh", "myapp.prod", "--", "'drush' 'cr'"},
		},
		{
			"shell (no cmd) drops the -- separator so acli opens interactive shell",
			Default, "myapp.prod", nil,
			[]string{"acli", "remote:ssh", "myapp.prod"},
		},
		{
			"custom prefix for remote:drush fans",
			[]string{"acli", "remote:drush"}, "myapp.dev", []string{"cr"},
			[]string{"acli", "remote:drush", "myapp.dev", "--", "'cr'"},
		},
		{
			"wrapper-script prefix (CI etc.)",
			[]string{"/usr/local/bin/acli-wrapper", "run", "ssh"}, "x.y", []string{"ls"},
			[]string{"/usr/local/bin/acli-wrapper", "run", "ssh", "x.y", "--", "'ls'"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Build(c.prefix, c.alias, c.cmd)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("Build:\n  got  %v\n  want %v", got, c.want)
			}
		})
	}
}

// TestBuild_PrefixNotMutated — Build must not append to the caller's
// prefix slice. If it did, two concurrent Builds against Default
// would corrupt each other.
func TestBuild_PrefixNotMutated(t *testing.T) {
	before := append([]string{}, Default...)
	_ = Build(Default, "x.y", []string{"a"})
	if !reflect.DeepEqual(before, Default) {
		t.Errorf("Build mutated Default:\n  before %v\n  after  %v", before, Default)
	}
}
