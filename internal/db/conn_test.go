package db_test

import (
	"reflect"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/db"
)

func TestMarshallParam(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want any
	}{
		{
			name: "nil",
			v:    nil,
			want: nil,
		},
		{
			name: "empty map",
			v:    map[string]string{},
			want: nil,
		},
		{
			name: "empty slice",
			v:    []string{},
			want: nil,
		},
		{
			name: "empty struct",
			v:    struct{}{},
			want: nil,
		},
		{
			name: "non-empty map",
			v:    map[string]string{"key": "value"},
			want: `{"key":"value"}`,
		},
		{
			name: "non-empty slice",
			v:    []string{"value"},
			want: `["value"]`,
		},
		{
			name: "non-empty struct",
			v:    struct{ Key string }{Key: "value"},
			want: `{"Key":"value"}`,
		},
		{
			name: "non-marshallable",
			v:    make(chan struct{}),
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := db.MarshallParamToJSONB(tt.v); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MarshallParamToJSONB() = %v, want %v", got, tt.want)
			}
		})
	}
}
