// dconsole-acquia is a dconsole subprocess plugin that reaches Acquia
// Cloud environments through acli (https://github.com/acquia/cli).
//
// Why wrap acli instead of talking to Acquia Cloud directly: acli
// resolves <app>.<env> aliases into the current SSH URL (which rotates
// as Acquia moves environments between servers), handles auth tokens,
// and already knows the docroot layout. Re-implementing all that in
// Go for a thin plugin is pointless.
//
// What the plugin exposes to dconsole:
//
//	handler:
//	  type: acquia
//	  acquia:
//	    alias: myapp.prod        # required — the acli environment alias
//	    # Optional advanced overrides:
//	    # cmd_prefix: ["acli", "remote:drush"]   # default: ["acli", "remote:ssh"]
//
// Then `dconsole @site.env status` fans out through acli to the right
// Cloud server and runs drush there, with stdio streamed back.
//
// Build:   go build -o dconsole-acquia .
// Install: ln -s "$PWD/dconsole-acquia" ~/.dconsole/plugins/
//          (or `dconsole plugin install --path=./dconsole-acquia-*.tar.gz`)
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/dconsole/dconsole-acquia/internal/argv"
	"github.com/dconsole/dconsole/pkg/plugin"
)

const (
	pluginName    = "acquia"
	pluginVersion = "0.1.0"
)

// acliBin is the executable name we invoke. Exported-via-var so tests
// can swap in a stub binary; production code always sees "acli".
var acliBin = "acli"

func main() {
	if len(os.Args) < 2 {
		fail(plugin.ExitProtocolError, "dconsole-acquia: missing verb (got %d args)", len(os.Args))
	}
	switch os.Args[1] {
	case plugin.VerbPluginInfo:
		emitPluginInfo()
	case plugin.VerbAvailable:
		checkAvailable()
	case plugin.VerbExec:
		runExec()
	case plugin.VerbPipe:
		runPipe()
	case plugin.VerbShell:
		runShell()
	case plugin.VerbPreview:
		runPreview()
	default:
		// Unsupported verbs (DumpFor, LoadFor, SyncTo, …) are a legitimate
		// "I don't do this" signal — dconsole will fall through to the
		// generic flow. Use the dedicated exit code so dconsole can tell
		// us apart from a crash.
		fail(plugin.ExitVerbUnsupported, "dconsole-acquia: verb %q not implemented", os.Args[1])
	}
}

func emitPluginInfo() {
	info := plugin.PluginInfo{
		ProtocolVersion: plugin.ProtocolV1,
		Name:            pluginName,
		Version:         pluginVersion,
		Description:     "Reach Acquia Cloud environments through acli (ssh + drush).",
		Homepage:        "https://github.com/dconsole/dconsole-acquia",
		Transports: []plugin.TransportSpec{{
			Type:        "acquia",
			RequiredCLI: "acli",
			Description: "Acquia Cloud environment via acli remote:ssh",
		}},
	}
	body, err := json.Marshal(info)
	if err != nil {
		fail(plugin.ExitGeneric, "marshal plugin-info: %v", err)
	}
	os.Stdout.Write(body)
	os.Stdout.WriteString("\n")
}

func checkAvailable() {
	if _, err := exec.LookPath(acliBin); err != nil {
		// ExitGeneric with a one-line stderr is how dconsole surfaces
		// "transport is configured but can't run right now" to the
		// user, with the stderr body as the detail.
		fail(plugin.ExitGeneric, "acli not found on $PATH — install from https://github.com/acquia/cli")
	}
	os.Exit(plugin.ExitOK)
}

func runExec() {
	cfg, remoteCmd := parseConfigAndCmd()
	a := argv.Build(cfg.prefix(), cfg.Alias, remoteCmd)
	cmd := exec.Command(a[0], a[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	runAndExit(cmd)
}

func runPipe() {
	cfg, remoteCmd := parseConfigAndCmd()
	a := argv.Build(cfg.prefix(), cfg.Alias, remoteCmd)
	cmd := exec.Command(a[0], a[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	// Pipe keeps stderr on the user's terminal — mirrors what the
	// in-tree ssh transport does. Lets a sql dump stream through
	// stdout while acli / drush progress noise still shows.
	cmd.Stderr = os.Stderr
	runAndExit(cmd)
}

func runShell() {
	cfg, _ := parseConfigAndCmd()
	// acli remote:ssh with no ssh_command opens an interactive shell
	// at the environment's docroot — that's exactly what dconsole's
	// Shell verb wants, including handling TTY allocation on acli's
	// side.
	a := argv.Build(cfg.prefix(), cfg.Alias, nil)
	cmd := exec.Command(a[0], a[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	runAndExit(cmd)
}

func runPreview() {
	cfg, remoteCmd := parseConfigAndCmd()
	a := argv.Build(cfg.prefix(), cfg.Alias, remoteCmd)
	body, _ := json.Marshal(a)
	os.Stdout.Write(body)
	os.Stdout.WriteString("\n")
}

// config is the typed shape of the alias's `acquia:` config block.
type config struct {
	Alias     string   `yaml:"alias"      json:"alias"`
	CmdPrefix []string `yaml:"cmd_prefix" json:"cmd_prefix"`
}

func (c config) prefix() []string {
	if len(c.CmdPrefix) > 0 {
		return c.CmdPrefix
	}
	return argv.Default
}

// parseConfigAndCmd reads --alias-json, decodes the acquia transport
// config out of the envelope's Config map, and splits off the remote
// command following the literal "--" separator.
func parseConfigAndCmd() (config, []string) {
	envelope, remoteCmd := parseStandardArgs()
	cfg, err := extractConfig(envelope)
	if err != nil {
		fail(plugin.ExitProtocolError, "dconsole-acquia: %v", err)
	}
	if cfg.Alias == "" {
		fail(plugin.ExitProtocolError, "dconsole-acquia: alias is required — set `acquia.alias: <app>.<env>` in your handler config")
	}
	return cfg, remoteCmd
}

// extractConfig pulls the `acquia:` sub-block out of the alias
// envelope's Config map and decodes it into our typed struct. The
// envelope's Config carries whatever the user put under `handler.<type>:`,
// which for the acquia transport is `handler.acquia.*`.
func extractConfig(env plugin.AliasEnvelope) (config, error) {
	raw, ok := env.Config["acquia"]
	if !ok {
		return config{}, fmt.Errorf("missing `acquia:` config block in handler config for @%s.%s", env.Site, env.Env)
	}
	// Round-trip through JSON — raw is a map[string]any (yaml-decoded
	// into Go generic types), and json.Marshal / Unmarshal gives us
	// typed deserialisation without pulling in a yaml library.
	body, err := json.Marshal(raw)
	if err != nil {
		return config{}, fmt.Errorf("marshal acquia config: %w", err)
	}
	var cfg config
	if err := json.Unmarshal(body, &cfg); err != nil {
		return config{}, fmt.Errorf("decode acquia config: %w", err)
	}
	return cfg, nil
}

// parseStandardArgs reads --alias-json=<path>, loads the envelope,
// and returns it along with the remote command following "--".
// Identical shape to examples/dconsole-echo in the dconsole repo.
func parseStandardArgs() (plugin.AliasEnvelope, []string) {
	var (
		envelopePath string
		remoteCmd    []string
		sawSep       bool
	)
	for _, a := range os.Args[2:] {
		if sawSep {
			remoteCmd = append(remoteCmd, a)
			continue
		}
		if a == "--" {
			sawSep = true
			continue
		}
		if strings.HasPrefix(a, "--alias-json=") {
			envelopePath = strings.TrimPrefix(a, "--alias-json=")
		}
	}
	if envelopePath == "" {
		fail(plugin.ExitProtocolError, "dconsole-acquia: missing --alias-json")
	}
	body, err := os.ReadFile(envelopePath)
	if err != nil {
		fail(plugin.ExitProtocolError, "dconsole-acquia: read alias-json: %v", err)
	}
	var env plugin.AliasEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		fail(plugin.ExitProtocolError, "dconsole-acquia: parse alias-json: %v", err)
	}
	return env, remoteCmd
}

// runAndExit mirrors the child's exit code into our own exit code so
// a drush non-zero (missing module, bad command, etc.) bubbles up to
// dconsole's own exit code unmodified.
func runAndExit(cmd *exec.Cmd) {
	err := cmd.Run()
	if err == nil {
		os.Exit(plugin.ExitOK)
	}
	if ee, ok := err.(*exec.ExitError); ok {
		os.Exit(ee.ExitCode())
	}
	// Not an ExitError means we couldn't even spawn acli (missing
	// binary caught separately via the available verb, but defend
	// against mid-flight exec failures).
	fail(plugin.ExitGeneric, "dconsole-acquia: %v", err)
}

func fail(code int, format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(code)
}

