package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSQL(t *testing.T) {
	m := Metric{}
	m.SQLs = SQLs{
		1: "one",
		3: "three",
		5: "five",
		6: "six",
	}
	tests := map[int]string{
		2:  "one",
		3:  "three",
		4:  "three",
		5:  "five",
		6:  "six",
		10: "six",
		0:  "",
	}

	for i, tt := range tests {
		if got := m.GetSQL(i); got != tt {
			t.Errorf("VersionToInt() = %v, want %v", got, tt)
		}
	}
}
func TestPrimaryOnly(t *testing.T) {
	m := Metric{NodeStatus: "primary"}
	assert.True(t, m.PrimaryOnly())
	assert.False(t, m.StandbyOnly())
	m.NodeStatus = "standby"
	assert.False(t, m.PrimaryOnly())
	assert.True(t, m.StandbyOnly())
}
