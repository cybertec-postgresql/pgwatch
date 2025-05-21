import yaml
import sys

yaml_file_path = "internal/metrics/metrics.yaml"
final_summary_str = "Error: Initial script error or YAML not processed." 

try:
    with open(yaml_file_path, 'r') as f:
        metrics_yaml_content = f.read()
    data = yaml.safe_load(metrics_yaml_content)
except FileNotFoundError:
    final_summary_str = "Error: YAML file not found at %s" % yaml_file_path
    sys.stdout.write(final_summary_str)
    sys.exit(1)
except yaml.YAMLError as e:
    final_summary_str = "Error parsing YAML: %s" % e
    sys.stdout.write(final_summary_str)
    sys.exit(1)
except Exception as e:
    final_summary_str = "An unexpected error occurred: %s" % e
    sys.stdout.write(final_summary_str)
    sys.exit(1)

if data and 'presets' in data and 'full' in data['presets'] and 'metrics' in data['presets']['full']:
    full_preset_metric_names = data['presets']['full']['metrics'].keys()
    full_preset_metrics_details = {}
    if 'metrics' in data:
        for metric_name in full_preset_metric_names:
            if metric_name in data['metrics']:
                full_preset_metrics_details[metric_name] = data['metrics'][metric_name]

    analysis_summary = {
        "large_result_sets": [],
        "resource_intensive_operations": [],
        "version_specific_issues": [],
        "general_observations": []
    }

    for metric_name, definition in full_preset_metrics_details.items():
        sqls = definition.get('sqls', {})
        if not sqls: 
            if metric_name.startswith("pgbouncer_") or metric_name.startswith("pgpool_"):
                 analysis_summary["general_observations"].append(
                    "Metric '%s' uses non-SQL commands (e.g., 'show stats'). Result size depends on pgbouncer/pgpool state. "
                    "Typically small, but 'show clients' or 'show pool_processes' can be larger with many clients/processes." % metric_name
                )
            continue

        for pg_version_key, sql_query in sqls.items():
            pg_version = str(pg_version_key) 
            if not sql_query.strip() or sql_query.strip().startswith("; --"):
                continue
            query_lower = sql_query.lower()

            # Large Result Sets
            if "pg_stat_statements" in query_lower and "limit" not in query_lower and "stat_statements_calls" != metric_name :
                analysis_summary["large_result_sets"].append(
                    "Metric '%s' (PG %s): Queries pg_stat_statements (up to ~600 rows via UNIONs of LIMIT 100). Contains 'query' column (varchar(8000))." % (metric_name, pg_version)
                )
            elif "pg_locks" in query_lower and "group by" not in query_lower and "count(*)" not in query_lower and "blocking_locks" != metric_name:
                 analysis_summary["large_result_sets"].append(
                    "Metric '%s' (PG %s): Queries pg_locks directly without aggregation (e.g., 'locks', 'locks_mode')." % (metric_name, pg_version)
                )
            if "pg_settings" in query_lower and "configuration_hashes" == metric_name :
                 analysis_summary["large_result_sets"].append(
                    "Metric '%s' (PG %s): Queries all rows from pg_settings." % (metric_name, pg_version)
                )
            if "pg_proc" in query_lower and "sproc_hashes" == metric_name:
                analysis_summary["large_result_sets"].append(
                    "Metric '%s' (PG %s): Queries all user-defined functions from pg_proc for hashing their source." % (metric_name, pg_version)
                )
            if "pg_class" in query_lower and "pg_namespace" in query_lower and ("table_hashes" == metric_name or "index_hashes" == metric_name):
                analysis_summary["large_result_sets"].append(
                    "Metric '%s' (PG %s): Queries all tables/indexes not in system schemas. 'table_hashes' aggregates column definitions." % (metric_name, pg_version)
                )
            if "information_schema.table_privileges" in query_lower and "privilege_changes" == metric_name: 
                 analysis_summary["large_result_sets"].append(
                    "Metric '%s' (PG %s): Queries information_schema for various privileges." % (metric_name, pg_version)
                )
            if "pg_stat_activity" in query_lower and "stat_activity_realtime" == metric_name:
                 analysis_summary["large_result_sets"].append(
                    "Metric '%s' (PG %s): Queries pg_stat_activity (LIMIT 25), includes query text (varchar(300))." % (metric_name, pg_version)
                )
            if "pg_show_plans" in query_lower and "show_plans_realtime" == metric_name:
                 analysis_summary["large_result_sets"].append(
                    "Metric '%s' (PG %s): Queries pg_show_plans (LIMIT 10), includes full plan and query text." % (metric_name, pg_version)
                )
            if "pgstattuple_approx" in query_lower and "table_bloat_approx_stattuple" == metric_name:
                 analysis_summary["large_result_sets"].append(
                    "Metric '%s' (PG %s): Calls pgstattuple_approx for tables >1MB." % (metric_name, pg_version)
                )
            if metric_name in ["table_stats", "table_io_stats", "table_stats_approx", "index_stats"]:
                unique_key_desc = "Queries table/index stats with LIMITs. 'index_stats' includes 'index_def'."
                is_present = any(metric_name in s and pg_version in s and unique_key_desc in s for s in analysis_summary["large_result_sets"])
                if not is_present:
                    analysis_summary["large_result_sets"].append(
                        "Metric '%s' (PG %s): %s" % (metric_name, pg_version, unique_key_desc)
                    )
            
            text_col_indicators = ["query::varchar(8000)", "ltrim(regexp_replace(b.query", "waiting_stm.query", "other_stm.query", "max(query)", "plan", "pg_get_indexdef", "pg_get_viewdef", "pg_get_functiondef", "prosrc"]
            if any(indicator in sql_query for indicator in text_col_indicators): 
                unique_key_desc = "Contains potentially large text columns."
                is_present = any(metric_name in s and pg_version in s and unique_key_desc in s for s in analysis_summary["large_result_sets"])
                if not is_present:
                     analysis_summary["large_result_sets"].append(
                        "Metric '%s' (PG %s): %s" % (metric_name, pg_version, unique_key_desc)
                    )

            # Resource-Intensive Operations (simplified unique additions)
            ops_desc_map = {
                "with recursive": "Uses 'WITH RECURSIVE'.",
                "blocking_locks_join": "Joins pg_locks with pg_stat_activity for blocking_locks.",
                "pgstattuple_approx": "Uses 'pgstattuple_approx'.",
                "pg_buffercache": "Queries 'pg_buffercache'.",
                "pg_qualstats_index_advisor": "Calls 'pg_qualstats_index_advisor()'.",
                "plpython": "Uses PL/Python helper functions.",
                "privilege_changes_complex": "Complex query on information_schema for privilege_changes.",
                "pg_ls_waldir": "Uses pg_ls_waldir() and sum(size).",
                "table_bloat_approx_summary_sql": "Complex CTE for bloat estimation.",
                "table_hashes_complex": "Builds and hashes column definitions from catalog tables."
            }
            if "with recursive" in query_lower and not any(metric_name in s and ops_desc_map["with recursive"] in s for s in analysis_summary["resource_intensive_operations"]):
                analysis_summary["resource_intensive_operations"].append(f"Metric '{metric_name}' (PG {pg_version}): {ops_desc_map['with recursive']}")
            if "pg_locks" in query_lower and "join" in query_lower and "pg_stat_activity" in query_lower and "blocking_locks" == metric_name and not any(metric_name in s and ops_desc_map["blocking_locks_join"] in s for s in analysis_summary["resource_intensive_operations"]):
                analysis_summary["resource_intensive_operations"].append(f"Metric '{metric_name}' (PG {pg_version}): {ops_desc_map['blocking_locks_join']}")
            # ... (apply similar unique checks for other ops if necessary, for brevity not all shown)
            if "pgstattuple_approx" in query_lower and not any(metric_name in s and ops_desc_map["pgstattuple_approx"] in s for s in analysis_summary["resource_intensive_operations"]):
                analysis_summary["resource_intensive_operations"].append(f"Metric '{metric_name}' (PG {pg_version}): {ops_desc_map['pgstattuple_approx']}")
            if "pg_buffercache" in query_lower and not any(metric_name in s and ops_desc_map["pg_buffercache"] in s for s in analysis_summary["resource_intensive_operations"]):
                analysis_summary["resource_intensive_operations"].append(f"Metric '{metric_name}' (PG {pg_version}): {ops_desc_map['pg_buffercache']}")
            if "pg_qualstats_index_advisor" in query_lower and not any(metric_name in s and ops_desc_map["pg_qualstats_index_advisor"] in s for s in analysis_summary["resource_intensive_operations"]):
                analysis_summary["resource_intensive_operations"].append(f"Metric '{metric_name}' (PG {pg_version}): {ops_desc_map['pg_qualstats_index_advisor']}")
            if ("plpython" in definition.get("init_sql", "").lower() or "language plpython" in query_lower) and not any(metric_name in s and ops_desc_map["plpython"] in s for s in analysis_summary["resource_intensive_operations"]):
                 analysis_summary["resource_intensive_operations"].append(f"Metric '{metric_name}' (PG {pg_version}): {ops_desc_map['plpython']}")
            if "information_schema" in query_lower and "union all" in query_lower and "privilege_changes" == metric_name and not any(metric_name in s and ops_desc_map["privilege_changes_complex"] in s for s in analysis_summary["resource_intensive_operations"]):
                analysis_summary["resource_intensive_operations"].append(f"Metric '{metric_name}' (PG {pg_version}): {ops_desc_map['privilege_changes_complex']}")
            if "pg_ls_waldir()" in query_lower and "wal_size" == metric_name and not any(metric_name in s and ops_desc_map["pg_ls_waldir"] in s for s in analysis_summary["resource_intensive_operations"]):
                analysis_summary["resource_intensive_operations"].append(f"Metric '{metric_name}' (PG {pg_version}): {ops_desc_map['pg_ls_waldir']}")
            if "table_bloat_approx_summary_sql" == metric_name and not any(metric_name in s and ops_desc_map["table_bloat_approx_summary_sql"] in s for s in analysis_summary["resource_intensive_operations"]):
                 analysis_summary["resource_intensive_operations"].append(f"Metric '{metric_name}' (PG {pg_version}): {ops_desc_map['table_bloat_approx_summary_sql']}")
            if "table_hashes" == metric_name and not any(metric_name in s and ops_desc_map["table_hashes_complex"] in s for s in analysis_summary["resource_intensive_operations"]):
                 analysis_summary["resource_intensive_operations"].append(f"Metric '{metric_name}' (PG {pg_version}): {ops_desc_map['table_hashes_complex']}")


            # Version-Specific Issues
            if metric_name == "stat_statements":
                sql_11 = sqls.get("11", sqls.get(11)) 
                sql_13 = sqls.get("13", sqls.get(13))
                if sql_11 and sql_13 and ("total_time" in sql_11 and "total_exec_time" in sql_13): 
                     analysis_summary["version_specific_issues"].append(
                        f"Metric '{metric_name}': SQL and columns differ significantly between PG11/12 and PG13+."
                    )
            if metric_name == "table_stats":
                 sql_11 = sqls.get("11", sqls.get(11))
                 sql_16 = sqls.get("16", sqls.get(16))
                 if sql_11 and sql_16 and "limit 1500" in sql_16.lower() and ("limit 1500" not in sql_11.lower()):
                    analysis_summary["version_specific_issues"].append(
                        f"Metric '{metric_name}': PG16 version has different internal LIMITing in CTEs compared to older versions."
                    )
            if metric_name == "bgwriter":
                sql_11 = sqls.get("11", sqls.get(11))
                sql_17 = sqls.get("17", sqls.get(17))
                if sql_11 and sql_17 and sql_11 != sql_17: 
                    analysis_summary["version_specific_issues"].append(
                        f"Metric '{metric_name}': PG17 separates checkpointer stats from bgwriter."
                    )
            
            version_specific_metrics_min_pg_version = {
                "wal_stats": 14, "replication_slot_stats": 14, "subscription_stats": 15, "stat_io": 16
            }
            numeric_keys = [int(k) for k in sqls.keys() if str(k).isdigit()]
            
            if numeric_keys: 
                min_defined_sql_version = min(numeric_keys) 
                for vm_name, min_v_required in version_specific_metrics_min_pg_version.items():
                    if metric_name == vm_name and min_defined_sql_version >= min_v_required :
                         unique_key_desc = "Only available from PG%d+." % min_v_required
                         is_present = any(metric_name in s and unique_key_desc in s for s in analysis_summary["version_specific_issues"])
                         if not is_present:
                            analysis_summary["version_specific_issues"].append(
                                "Metric '%s': %s" % (metric_name, unique_key_desc)
                            )

    impact_statement = \"\"\"
Large result sets from metrics like 'stat_statements' (with query texts), 'table_stats', 'index_stats' (with index definitions),
'blocking_locks' (with query texts), 'privilege_changes', 'table_hashes', 'index_hashes', 'configuration_hashes', 'table_bloat_approx_stattuple', and 'sproc_hashes'
can significantly impact pgwatch's memory.
When ~100 databases are monitored:
1. Each `dataRow` in `metrics.Measurements` will hold more data if there are many columns or large string values (e.g., query texts, definitions).
2. `MeasurementEnvelope.Data` (a slice of `dataRow`) will become larger. E.g., `stat_statements` can return ~600 rows; if each has 4KB query text, that's ~2.4MB per DB for this metric.
3. Buffering these larger `[]metrics.MeasurementEnvelope` slices in `measurementCh` (capacity 10,000) and `PostgresWriter.input` (capacity 512) consumes more memory.
4. The `PostgresWriter.poll.cache` (up to 512 envelopes) will also hold more data.
5. `InstanceMetricCache.cache` stores `metrics.Measurements`. Large datasets cached here, especially if not properly evicted (due to `Get` not deleting), will cause memory bloat.

Resource-intensive queries on monitored PostgreSQL servers can lead to:
- Increased CPU/memory usage on monitored databases.
- Longer query execution times, risking statement timeouts.
- Slower metric collection in pgwatch. If the sink is also slow, `measurementCh` can fill, blocking metric gathering, potentially leading to data loss in `PostgresWriter.Write` due to `highLoadTimeout`.
\"\"\"

    final_summary_str = "Analysis of 'full' preset metrics in metrics.yaml:\\n\\n"
    final_summary_str += "Metrics Potentially Returning Large Result Sets (many rows or large text columns):\\n"
    if analysis_summary["large_result_sets"]:
        unique_items = sorted(list(set(analysis_summary["large_result_sets"])))
        for item in unique_items: 
            final_summary_str += "- %s\\n" % item
    else:
        final_summary_str += "- None specifically identified beyond general catalog queries.\\n"

    final_summary_str += "\\nMetrics with Potentially Resource-Intensive Operations on PostgreSQL Server:\\n"
    if analysis_summary["resource_intensive_operations"]:
        unique_items = sorted(list(set(analysis_summary["resource_intensive_operations"])))
        for item in unique_items: 
            final_summary_str += "- %s\\n" % item
    else:
        final_summary_str += "- None specifically identified as highly resource-intensive beyond normal catalog queries.\\n"

    final_summary_str += "\\nVersion-Specific Issues Noted:\\n"
    if analysis_summary["version_specific_issues"]:
        unique_items = sorted(list(set(analysis_summary["version_specific_issues"])))
        for item in unique_items:
            final_summary_str += "- %s\\n" % item
    else:
        final_summary_str += "- No major version-specific issues noted that would drastically alter result size or intensity beyond typical feature additions.\\n"

    final_summary_str += "\\nGeneral Observations (e.g. non-SQL metrics):\\n"
    if analysis_summary["general_observations"]:
        unique_items = sorted(list(set(analysis_summary["general_observations"])))
        for item in unique_items: 
            final_summary_str += "- %s\\n" % item
    else:
        final_summary_str += "- None.\\n"

    final_summary_str += "\\nPotential Impact on pgwatch Memory and Performance:\\n"
    final_summary_str += impact_statement
else:
    final_summary_str = "Error: Could not find 'full' preset or its metrics in YAML structure."

sys.stdout.write(final_summary_str)
sys.stdout.flush()
