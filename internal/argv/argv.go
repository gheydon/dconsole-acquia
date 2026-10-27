// Package argv builds the acli command lines the dconsole-acquia
// plugin issues. Factored out so the quoting + layout logic can be
// unit-tested without actually exec'ing acli.
package argv

import "strings"

// Default is the acli subcommand the plugin wraps when the alias
// config doesn't override it. remote:ssh accepts an arbitrary command
// string, which gives us one path for drush, drupal, cat, whatever —
// the alternative (remote:drush) would need us to strip a "drush"
// prefix out of remoteCmd and would fail silently on non-drush
// invocations.
//
// acli joins ssh_command elements with a single space and runs the
// result inside the environment's docroot (see
// https://github.com/acquia/cli/blob/main/src/Command/Remote/SshCommand.php),
// so we must shell-quote the dconsole-supplied remoteCmd into ONE
// string argument — otherwise args containing spaces or shell
// metacharacters get mangled.
var Default = []string{"acli", "remote:ssh"}

// Build returns the full argv the plugin passes to exec.Command. The
// shape is:
//
//	<prefix> <alias> -- <quoted-command>
//
// `prefix` is normally ["acli", "remote:ssh"] but config can override
// it (useful for `remote:drush`, or for a wrapper script in CI).
// `alias` is the Acquia environment ID (e.g. "myapp.prod"). `cmd` is
// the remote command dconsole wants to run on the environment; the
// slice is joined into a single shell-quoted argument so acli hands
// the remote shell a correctly-quoted command string.
//
// If cmd is empty the "--" and command portions are omitted — acli
// with no ssh_command drops the user into an interactive shell at
// the docroot, which is what dconsole's Shell verb wants.
func Build(prefix []string, alias string, cmd []string) []string {
	out := append([]string{}, prefix...)
	out = append(out, alias)
	if len(cmd) == 0 {
		return out
	}
	out = append(out, "--", ShellQuote(cmd))
	return out
}

// ShellQuote joins cmd into a single POSIX-shell-safe string. Each
// element is single-quoted (and any contained single quote escaped
// as the standard '"'"' incantation). Chosen over double-quote
// escaping because single quotes suppress every form of shell
// expansion — no $VAR surprises, no backtick execution, no backslash
// re-interpretation.
func ShellQuote(cmd []string) string {
	parts := make([]string, len(cmd))
	for i, c := range cmd {
		parts[i] = singleQuote(c)
	}
	return strings.Join(parts, " ")
}

func singleQuote(s string) string {
	if s == "" {
		return "''"
	}
	// '\'' closes the single-quoted run, inserts an escaped quote,
	// and reopens. Standard POSIX trick.
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
