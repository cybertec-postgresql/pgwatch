package config

type SourceKind string

const (
	SourcePostgres           SourceKind = "postgres"
	SourcePostgresContinuous SourceKind = "postgres-continuous-discovery"
	SourcePgBouncer          SourceKind = "pgbouncer"
	SourcePgPool             SourceKind = "pgpool"
	SourcePatroni            SourceKind = "patroni"
	SourcePatroniContinuous  SourceKind = "patroni-continuous-discovery"
	SourcePatroniNamespace   SourceKind = "patroni-namespace-discovery"
)
