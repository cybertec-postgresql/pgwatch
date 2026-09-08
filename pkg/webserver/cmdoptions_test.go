package webserver

import (
	"os"
	"testing"

	flags "github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
)

func TestWebDisableOpt(t *testing.T) {
	a := assert.New(t)
	testCases := []struct {
		args        []string
		expected    string
		expectError bool
	}{
		{[]string{0: "config_test"}, "", false},
		{[]string{0: "config_test", "--web-disable"}, WebDisableAll, false},
		{[]string{0: "config_test", "--web-disable=all"}, WebDisableAll, false},
		{[]string{0: "config_test", "--web-disable=ui"}, WebDisableUI, false},
		{[]string{0: "config_test", "--web-disable=foo"}, "", true},
	}

	for _, tc := range testCases {
		opts := new(CmdOpts)
		os.Args = tc.args
		_, err := flags.NewParser(opts, flags.HelpFlag).Parse()

		if tc.expectError {
			a.Error(err)
			a.Empty(opts.WebDisable)
		} else {
			a.NoError(err)
			a.Equal(tc.expected, opts.WebDisable)
		}
	}

}
