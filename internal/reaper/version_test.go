package reaper

import "testing"

func TestVersionToInt(t *testing.T) {
	tests := []struct {
		arg  string
		want int
	}{
		{"", 0},
		{"foo", 0},
		{"13", 13_00_00},
		{"3.0", 3_00_00},
		{"9.6.3", 9_06_03},
		{"v9.6-beta2", 9_06_00},
	}
	for _, tt := range tests {
		if got := VersionToInt(tt.arg); got != tt.want {
			t.Errorf("VersionToInt() = %v, want %v", got, tt.want)
		}
	}
}
