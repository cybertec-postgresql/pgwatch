package metrics

import (
	"testing"
	"time"

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

func TestMeasurement(t *testing.T) {
	m := NewMeasurement(1234567890)
	assert.Equal(t, int64(1234567890), m.GetEpoch(), "epoch should be equal")
	m[EpochColumnName] = "wrong type"
	assert.True(t, time.Now().UnixNano()-m.GetEpoch() < int64(time.Second), "epoch should be close to now")
}

func TestMeasurements(t *testing.T) {
	m := Measurements{}
	assert.False(t, m.IsEpochSet(), "epoch should not be set")
	assert.True(t, time.Now().UnixNano()-m.GetEpoch() < 100, "epoch should be close to now")
	m = append(m, NewMeasurement(1234567890))
	assert.True(t, m.IsEpochSet(), "epoch should be set")
	assert.Equal(t, int64(1234567890), m.GetEpoch(), "epoch should be equal")
	m1 := m.DeepCopy()
	assert.Equal(t, m, m1, "deep copy should be equal")
	m1.Touch()
	assert.NotEqual(t, m, m1, "deep copy should be different")
	assert.True(t, time.Now().UnixNano()-m1.GetEpoch() < int64(time.Second), "epoch should be close to now")
}
