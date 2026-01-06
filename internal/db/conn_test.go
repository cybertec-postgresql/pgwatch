package db_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/cybertec-postgresql/pgwatch/v5/internal/db"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
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

func TestIsClientOnSameHost(t *testing.T) {
	// Create a pgxmock pool
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create pgxmock pool: %v", err)
	}
	defer mock.Close()
	dataDir := t.TempDir()
	pgControl := filepath.Join(dataDir, "global")
	require.NoError(t, os.MkdirAll(pgControl, 0755))
	file, err := os.OpenFile(filepath.Join(pgControl, "pg_control"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	require.NoError(t, err)
	err = binary.Write(file, binary.LittleEndian, uint64(12345))
	require.NoError(t, err)
	require.NoError(t, file.Close())

	// Test cases
	tests := []struct {
		name      string
		setupMock func()
		want      bool
		wantErr   bool
	}{
		{
			name: "UNIX socket connection",
			setupMock: func() {
				mock.ExpectQuery(`SELECT COALESCE`).WillReturnRows(
					pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(true),
				)
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "Matching system identifier",
			setupMock: func() {
				mock.ExpectQuery(`SELECT COALESCE`).WillReturnRows(
					pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(false),
				)
				mock.ExpectQuery(`SHOW`).WillReturnRows(
					pgxmock.NewRows([]string{"data_directory"}).AddRow(dataDir),
				)
				mock.ExpectQuery(`SELECT`).WillReturnRows(
					pgxmock.NewRows([]string{"system_identifier"}).AddRow(uint64(12345)),
				)
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "Non-matching system identifier",
			setupMock: func() {
				mock.ExpectQuery(`SELECT COALESCE`).WillReturnRows(
					pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(false),
				)
				mock.ExpectQuery(`SHOW`).WillReturnRows(
					pgxmock.NewRows([]string{"data_directory"}).AddRow(dataDir),
				)
				mock.ExpectQuery(`SELECT`).WillReturnRows(
					pgxmock.NewRows([]string{"system_identifier"}).AddRow(uint64(42)),
				)
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "Error on COALESCE query",
			setupMock: func() {
				mock.ExpectQuery(`SELECT COALESCE`).WillReturnError(os.ErrInvalid)
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "Error on SHOW query",
			setupMock: func() {
				mock.ExpectQuery(`SELECT COALESCE`).WillReturnRows(
					pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(false),
				)
				mock.ExpectQuery(`SHOW`).WillReturnError(os.ErrInvalid)
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "Error on SELECT system_identifier query",
			setupMock: func() {
				mock.ExpectQuery(`SELECT COALESCE`).WillReturnRows(
					pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(false),
				)
				mock.ExpectQuery(`SHOW`).WillReturnRows(
					pgxmock.NewRows([]string{"data_directory"}).AddRow(dataDir),
				)
				mock.ExpectQuery(`SELECT`).WillReturnError(os.ErrInvalid)
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "Error on os.Open",
			setupMock: func() {
				mock.ExpectQuery(`SELECT COALESCE`).WillReturnRows(
					pgxmock.NewRows([]string{"is_unix_socket"}).AddRow(false),
				)
				mock.ExpectQuery(`SHOW`).WillReturnRows(
					pgxmock.NewRows([]string{"data_directory"}).AddRow("invalid/path"),
				)
				mock.ExpectQuery(`SELECT`).WillReturnRows(
					pgxmock.NewRows([]string{"system_identifier"}).AddRow(uint64(12345)),
				)
			},
			want:    false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()
			got, err := db.IsClientOnSameHost(mock)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsClientOnSameHost() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("IsClientOnSameHost() = %v, want %v", got, tt.want)
			}
		})
	}
}
