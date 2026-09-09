package cmdopts

import (
	"errors"
	"os"
	"strings"
	"testing"

	flags "github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

// eeGroup is a stand-in for an embedder-specific flag group.
type eeGroup struct {
	Token string `long:"ee-token" description:"Enterprise token" env:"PW_EETOKEN"`
}

// eeCommand is a stand-in for an embedder-specific subcommand.
type eeCommand struct {
	opts  *Options
	Quiet bool `long:"quiet" description:"Say nothing"`
}

func (c *eeCommand) Execute([]string) error {
	c.opts.CompleteCommand(ExitCodeOK)
	return nil
}

// eeExtension registers both of them, the way an embedder would.
type eeExtension struct {
	group *eeGroup
	cmd   *eeCommand
}

func (e *eeExtension) Register(parser *flags.Parser, opts *Options) error {
	e.group = new(eeGroup)
	if _, err := parser.AddGroup("Enterprise", "", e.group); err != nil {
		return err
	}
	e.cmd = &eeCommand{opts: opts}
	_, err := parser.AddCommand("ee", "Enterprise commands", "", e.cmd)
	return err
}

func TestExtensionGroup(t *testing.T) {
	ext := new(eeExtension)
	os.Args = []string{0: "go-test", "--sources=sample.config.yaml", "--ee-token=secret"}
	opts, err := New(nil, ext)
	assert.NoError(t, err)
	assert.False(t, opts.CommandCompleted)
	assert.Equal(t, "secret", ext.group.Token)
}

func TestExtensionSubcommand(t *testing.T) {
	ext := new(eeExtension)
	os.Args = []string{0: "go-test", "ee", "--quiet"}
	opts, err := New(nil, ext)
	assert.NoError(t, err)
	assert.True(t, opts.CommandCompleted)
	assert.Equal(t, ExitCodeOK, opts.ExitCode)
	assert.True(t, ext.cmd.Quiet)
}

func TestExtensionRegisterError(t *testing.T) {
	boom := errors.New("boom")
	os.Args = []string{0: "go-test", "--sources=sample.config.yaml"}
	_, err := New(nil, ExtensionFunc(func(*flags.Parser, *Options) error { return boom }))
	assert.ErrorIs(t, err, boom)
}

func TestExtensionNilIgnored(t *testing.T) {
	os.Args = []string{0: "go-test", "--sources=sample.config.yaml"}
	_, err := New(nil, nil)
	assert.NoError(t, err)
}

// helpOutput returns the help text go-flags puts into the ErrHelp error.
func helpOutput(t *testing.T, exts ...Extension) string {
	t.Helper()
	os.Args = []string{0: "go-test", "--help"}
	opts, err := New(nil, exts...)
	assert.True(t, opts.Help)
	assert.Error(t, err)
	return err.Error()
}

// AC-002: the pgwatch binary registers no extension, so its --help output must
// stay exactly what it was; an extension only ever adds to it.
func TestHelpUnchangedWithoutExtension(t *testing.T) {
	plain := helpOutput(t)
	assert.NotContains(t, plain, "ee-token")
	assert.NotContains(t, plain, "Enterprise")
	assert.Equal(t, plain, helpOutput(t), "--help must be deterministic")

	extended := helpOutput(t, new(eeExtension))
	assert.Contains(t, extended, "--ee-token")
	assert.Contains(t, extended, "Enterprise commands")

	// Every line of the plain help must survive verbatim in the extended one --
	// the extension adds, it never rewrites. The usage line is the one
	// exception: it enumerates the commands, so a new command widens it.
	extendedLines := make(map[string]bool)
	for _, l := range strings.Split(extended, "\n") {
		extendedLines[l] = true
	}
	for _, l := range strings.Split(plain, "\n") {
		if strings.Contains(l, "[OPTIONS]") {
			// go-flags enumerates the commands here and collapses the list to
			// "[command]" past three of them, so only the prefix is stable.
			assert.Contains(t, extended, "go-test [OPTIONS]")
			continue
		}
		assert.True(t, extendedLines[l], "line dropped from --help: %q", l)
	}
}
