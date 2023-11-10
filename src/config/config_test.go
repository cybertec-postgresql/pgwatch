package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
	os.Args = []string{0: "config_test", "--config=sample.config.yaml"}
	_, err := NewConfig(nil)
	assert.NoError(t, err)

	os.Args = []string{0: "config_test", "--unknown"}
	_, err = NewConfig(nil)
	assert.Error(t, err)

	os.Args = []string{0: "config_test"} // clientname arg is missing, but set PGTT_CLIENTNAME
	assert.NoError(t, os.Setenv("PW3_PG_METRIC_STORE_CONN_STR", "postgresql://foo:baz@bar/test"))
	_, err = NewConfig(nil)
	assert.NoError(t, err)
}
