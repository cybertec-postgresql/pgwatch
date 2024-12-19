package cmdopts

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourcePingCommand_Execute(t *testing.T) {

	f, err := os.CreateTemp(t.TempDir(), "sample.config.yaml")
	require.NoError(t, err)
	defer f.Close()

	_, err = f.WriteString(`
- name: test1
  kind: postgres
  conn_str: postgresql://foo@bar/baz`)

	require.NoError(t, err)

	os.Args = []string{0: "config_test", "--sources=" + f.Name(), "source", "ping"}
	_, err = New(nil)
	assert.NoError(t, err)

	os.Args = []string{0: "config_test", "--sources=" + f.Name(), "source", "ping", "test1"}
	_, err = New(nil)
	assert.NoError(t, err)
}
