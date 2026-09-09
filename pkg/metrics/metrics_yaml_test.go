package metrics

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

// knownMetrics is the set of top-level metric names that the embedded
// metrics.yaml is expected to ship. If a metric is renamed or removed
// intentionally, update this list alongside the YAML change.
var knownMetrics = []string{
	"archiver",
	"autovacuum_scores",
	"backends",
	"backup_age_pgbackrest",
	"backup_age_walg",
	"bgwriter",
	"blocking_locks",
	"buffercache_by_db",
	"buffercache_by_type",
	"change_events",
	"checkpointer",
	"configuration_hashes",
	"cpu_load",
	"database_conflicts",
	"db_size",
	"db_size_approx",
	"db_stats",
	"index_hashes",
	"index_stats",
	"instance_up",
	"invalid_indexes",
	"kpi",
	"locks",
	"locks_mode",
	"logical_subscriptions",
	"pgbouncer_stats",
	"pgbouncer_clients",
	"pgpool_processes",
	"pgpool_stats",
	"privilege_changes",
	"psutil_cpu",
	"psutil_disk",
	"psutil_disk_io_total",
	"psutil_mem",
	"reco_add_index",
	"reco_default_public_schema",
	"reco_disabled_triggers",
	"reco_drop_index",
	"reco_nested_views",
	"reco_partial_index_candidates",
	"reco_sprocs_wo_search_path",
	"reco_superusers",
	"recovery",
	"replication",
	"replication_slot_stats",
	"replication_slots",
	"sequence_health",
	"server_log_event_counts",
	"settings",
	"smart_health_per_disk",
	"sproc_hashes",
	"sproc_stats",
	"stat_activity",
	"stat_lock",
	"stat_io",
	"stat_ssl",
	"stat_statements",
	"stat_statements_calls",
	"stat_statements_no_query_text",
	"subscription_stats",
	"table_bloat_approx_stattuple",
	"table_bloat_approx_summary",
	"table_bloat_approx_summary_sql",
	"table_hashes",
	"table_io_stats",
	"table_stats",
	"table_stats_approx",
	"unused_indexes",
	"vmstat",
	"wait_events",
	"wal",
	"wal_receiver",
	"wal_size",
	"wal_stats",
	"datfrozenxid",
	"postgres_role",
	"archiver_pending_count",
}

// metricsRoot parses the embedded metrics.yaml into the raw yaml.v3 node
// tree. Going via the AST (rather than the typed Metrics struct) is what
// lets the structural guards below catch orphan-scalar bleed: Go struct
// unmarshalling silently absorbs misaligned continuation lines into the
// surrounding block scalar, but the AST preserves indentation and
// sequence structure.
func metricsRoot(t *testing.T) *yaml.Node {
	t.Helper()
	var root yaml.Node
	assert.NoError(t, yaml.Unmarshal(defaultMetricsYAML, &root),
		"metrics.yaml must be valid YAML")
	assert.Equal(t, yaml.DocumentNode, root.Kind, "metrics.yaml root must be a document")
	assert.Len(t, root.Content, 1, "metrics.yaml document must contain exactly one node")
	top := root.Content[0]
	assert.Equal(t, yaml.MappingNode, top.Kind, "metrics.yaml top-level must be a mapping")
	return &root
}

// metricsBlock returns the inner mapping node of the `metrics:` block.
func metricsBlock(t *testing.T, root *yaml.Node) *yaml.Node {
	t.Helper()
	top := root.Content[0]
	for i := 0; i < len(top.Content); i += 2 {
		if top.Content[i].Value == "metrics" {
			return top.Content[i+1]
		}
	}
	t.Fatalf("metrics.yaml: top-level 'metrics:' key not found")
	return nil
}

// SQL clause-classification regexes, anchored to start-of-line (after
// optional whitespace). The "non-terminal" set (WHERE/GROUP BY/HAVING)
// must appear strictly before every "terminal" set (LIMIT/ORDER BY).
// A `|-` SQL scalar that has absorbed a sibling metric's body will show
// WHERE/GROUP BY/HAVING clauses appearing AFTER a LIMIT/ORDER BY —
// structurally impossible in any single valid SELECT.
var (
	sqlNonTerminal = regexp.MustCompile(`(?im)^\s*(where|group\s+by|having)\b`)
	sqlTerminal    = regexp.MustCompile(`(?im)^\s*(limit|order\s+by)\b`)
)

// clauseOrderViolation returns the line number of the first non-terminal
// clause (WHERE/GROUP BY/HAVING) that appears AFTER a terminal clause
// (LIMIT/ORDER BY) at parenthesis depth 0 — i.e. in the outer SELECT body,
// not inside a CTE definition or subquery. Returns 0 when the SQL is
// well-ordered.
func clauseOrderViolation(sql string) int {
	lastTerminal := 0
	depth := 0
	for i, raw := range strings.Split(sql, "\n") {
		line := raw
		// Track paren depth using the line's raw text before classifying.
		for _, r := range line {
			switch r {
			case '(':
				depth++
			case ')':
				if depth > 0 {
					depth--
				}
			}
		}
		if depth != 0 {
			continue
		}
		if sqlNonTerminal.MatchString(line) {
			if lastTerminal > 0 {
				return i + 1
			}
		} else if sqlTerminal.MatchString(line) {
			if i+1 > lastTerminal {
				lastTerminal = i + 1
			}
		}
	}
	return 0
}

// TestAllKnownMetricsPresent guards against silent top-level key
// deletions during future edits to metrics.yaml. The presence check runs
// against the raw yaml.v3 AST, so a metric that disappears mid-edit (even
// if its body is later re-pasted into a sibling block) is still caught.
//
// If a metric is renamed or removed intentionally, update knownMetrics in
// this test.
func TestAllKnownMetricsPresent(t *testing.T) {
	a := assert.New(t)
	block := metricsBlock(t, metricsRoot(t))
	a.Equal(yaml.MappingNode, block.Kind, "'metrics:' block must be a mapping")

	got := make(map[string]bool, len(block.Content)/2)
	for i := 0; i < len(block.Content); i += 2 {
		got[block.Content[i].Value] = true
	}

	for _, name := range knownMetrics {
		a.True(got[name],
			"metric %q missing from metrics.yaml top-level (was it silently removed by an edit?)", name)
	}
}

// TestSQLScalarsAreStructurallyIntact guards against scalar-tail bleed
// (orphan lines from a sibling metric absorbed into a `|-` block scalar).
//
// Go struct unmarshalling accepts continuation lines indented to look like
// block-scalar content, folding them silently into the scalar value. This
// guard detects the resulting structural impossibility: WHERE / GROUP BY /
// HAVING must never appear after LIMIT / ORDER BY at parenthesis depth 0
// in any single SELECT.
//
// The companion TestSQLScalarBleedRegression test feeds a corrupted
// scalar through the same logic to prove the guard actually fires.
func TestSQLScalarsAreStructurallyIntact(t *testing.T) {
	a := assert.New(t)
	block := metricsBlock(t, metricsRoot(t))

	for i := 0; i < len(block.Content); i += 2 {
		name := block.Content[i].Value
		def := block.Content[i+1]
		if def.Kind != yaml.MappingNode {
			continue
		}
		var sqlsNode *yaml.Node
		for j := 0; j < len(def.Content); j += 2 {
			if def.Content[j].Value == "sqls" {
				sqlsNode = def.Content[j+1]
				break
			}
		}
		if sqlsNode == nil || sqlsNode.Kind != yaml.MappingNode {
			continue
		}

		for k := 0; k < len(sqlsNode.Content); k += 2 {
			version := sqlsNode.Content[k].Value
			scalar := sqlsNode.Content[k+1]
			if scalar.Kind != yaml.ScalarNode {
				a.Failf("expected literal block scalar",
					"%s sqls[%s]: expected a literal block scalar, got kind=%d (%q)",
					name, version, scalar.Kind, scalar.Tag)
				continue
			}
			if line := clauseOrderViolation(scalar.Value); line > 0 {
				a.Failf("scalar-tail bleed detected",
					"%s sqls[%s]: terminal clause precedes non-terminal clause at line %d. Tail: %q",
					name, version, line, tail(scalar.Value, 200))
			}
		}
	}
}

// TestSQLScalarBleedRegression is a self-contained reproducer that proves
// TestSQLScalarsAreStructurallyIntact catches the class of bug it is
// written for: a `|-` SQL scalar that has absorbed orphan lines from a
// neighbouring metric. It feeds a small synthetic YAML containing a
// canonical bleed through the same parsing pipeline and asserts that the
// guard fires on the corrupted scalar while passing on the clean one.
func TestSQLScalarBleedRegression(t *testing.T) {
	a := assert.New(t)
	const fixture = `
metrics:
    metric_a:
        sqls:
            14: |-
                select /* pgwatch_generated */
                  1::int as a_value
                from
                  some_table
                LIMIT 300
            19: |-
                select /* pgwatch_generated */
                  1::int as a_value
                from
                  some_table
                LIMIT 300
                where s.datname = current_database()
                  and s.state = 'active'
                group by s.query
    metric_b:
        sqls:
            14: |-
                select /* pgwatch_generated */
                  1::int as count
                from some_view s
                where s.datname = current_database()
                  and s.state = 'active'
                group by s.query
`

	var root yaml.Node
	a.NoError(yaml.Unmarshal([]byte(fixture), &root),
		"synthetic YAML must still parse (Go absorbs the bleed silently)")

	block := metricsBlock(t, &root)
	metricA := findMetricNode(t, block, "metric_a")
	sqls := findChildMapping(t, metricA, "sqls")

	v14 := sqlScalarValue(t, sqls, "14")
	a.Zero(clauseOrderViolation(v14),
		"clean v14 should pass the invariant but failed:\n%s", v14)

	v19 := sqlScalarValue(t, sqls, "19")
	a.Contains(v19, "where s.datname",
		"fixture is wrong — bleed lines missing from v19 scalar:\n%s", v19)
	a.NotZero(clauseOrderViolation(v19),
		"guard did NOT catch the bleed — v19 scalar still looks OK:\n%s", v19)
}

func findMetricNode(t *testing.T, block *yaml.Node, name string) *yaml.Node {
	t.Helper()
	if block.Kind != yaml.MappingNode {
		t.Fatalf("metrics block is not a mapping (kind=%d)", block.Kind)
	}
	for i := 0; i < len(block.Content); i += 2 {
		if block.Content[i].Value == name {
			return block.Content[i+1]
		}
	}
	t.Fatalf("metric %q not found", name)
	return nil
}

func findChildMapping(t *testing.T, parent *yaml.Node, key string) *yaml.Node {
	t.Helper()
	if parent.Kind != yaml.MappingNode {
		t.Fatalf("parent is not a mapping (kind=%d)", parent.Kind)
	}
	for i := 0; i < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			if parent.Content[i+1].Kind != yaml.MappingNode {
				t.Fatalf("%q child is not a mapping (kind=%d)", key, parent.Content[i+1].Kind)
			}
			return parent.Content[i+1]
		}
	}
	t.Fatalf("child %q not found", key)
	return nil
}

func sqlScalarValue(t *testing.T, sqls *yaml.Node, version string) string {
	t.Helper()
	for i := 0; i < len(sqls.Content); i += 2 {
		if sqls.Content[i].Value == version {
			return sqls.Content[i+1].Value
		}
	}
	t.Fatalf("sqls[%s] not found", version)
	return ""
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}
