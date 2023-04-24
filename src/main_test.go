package main

import "testing"

func TestVersionToInt(t *testing.T) {
	tests := []struct {
		arg  string
		want uint
	}{
		{"", 0},
		{"foo", 0},
		{"13", 1300},
		{"3.0", 300},
		{"9.6.3", 906},
		{"v9.6-beta2", 906},
	}
	for _, tt := range tests {
		if got := VersionToInt(tt.arg); got != tt.want {
			t.Errorf("VersionToInt() = %v, want %v", got, tt.want)
		}
	}
}
