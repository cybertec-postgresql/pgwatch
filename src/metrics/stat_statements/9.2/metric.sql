WITH q_data AS (
    SELECT
        (regexp_replace(md5(query::varchar(1000)), E'\\D', '', 'g'))::varchar(10)::text as tag_queryid,
        max(query::varchar(8000)) AS query,
        /*
         NB! if security conscious about exposing query texts replace the below expression with a dash ('-') OR
         use the stat_statements_no_query_text metric instead, created specifically for this use case.
         */
        array_to_string(array_agg(DISTINCT quote_ident(pg_get_userbyid(userid))), ',') AS users,
        sum(s.calls)::int8 AS calls,
        round(sum(s.total_time)::numeric, 3)::double precision AS total_time,
        sum(shared_blks_hit)::int8 AS shared_blks_hit,
        sum(shared_blks_read)::int8 AS shared_blks_read,
        sum(shared_blks_written)::int8 AS shared_blks_written,
        sum(shared_blks_dirtied)::int8 AS shared_blks_dirtied,
        sum(temp_blks_read)::int8 AS temp_blks_read,
        sum(temp_blks_written)::int8 AS temp_blks_written,
        round(sum(blk_read_time)::numeric, 3)::double precision AS blk_read_time,
        round(sum(blk_write_time)::numeric, 3)::double precision AS blk_write_time
    FROM
        get_stat_statements() s
    WHERE
        calls > 5
        AND total_time > 5
        AND dbid = (
            SELECT
                oid
            FROM
                pg_database
            WHERE
                datname = current_database())
            AND NOT upper(s.query::varchar(50))
            LIKE ANY (ARRAY['DEALLOCATE%',
                'SET %',
                'RESET %',
                'BEGIN%',
                'BEGIN;',
                'COMMIT%',
                'END%',
                'ROLLBACK%',
                'SHOW%'])
        GROUP BY
            tag_queryid
)
SELECT (EXTRACT(epoch FROM now()) * 1e9)::int8 AS epoch_ns,
       b.tag_queryid,
       b.users,
       b.calls,
       b.total_time,
       b.shared_blks_hit,
       b.shared_blks_read,
       b.shared_blks_written,
       b.shared_blks_dirtied,
       b.temp_blks_read,
       b.temp_blks_written,
       b.blk_read_time,
       b.blk_write_time,
       ltrim(regexp_replace(b.query, E'[ \\t\\n\\r]+', ' ', 'g')) tag_query
FROM (
    SELECT
        *
    FROM (
        SELECT
            *
        FROM
            q_data
        WHERE
            total_time > 0
        ORDER BY
            total_time DESC
        LIMIT 100) a
UNION
select /* pgwatch3_generated */
    *
FROM (
    SELECT
        *
    FROM
        q_data
    ORDER BY
        calls DESC
    LIMIT 100) a
UNION
select /* pgwatch3_generated */
    *
FROM (
    SELECT
        *
    FROM
        q_data
    WHERE
        shared_blks_read > 0
    ORDER BY
        shared_blks_read DESC
    LIMIT 100) a
UNION
select /* pgwatch3_generated */
    *
FROM (
    SELECT
        *
    FROM
        q_data
    WHERE
        shared_blks_written > 0
    ORDER BY
        shared_blks_written DESC
    LIMIT 100) a
UNION
select /* pgwatch3_generated */
    *
FROM (
    SELECT
        *
    FROM
        q_data
    WHERE
        temp_blks_read > 0
    ORDER BY
        temp_blks_read DESC
    LIMIT 100) a
UNION
select /* pgwatch3_generated */
    *
FROM (
    SELECT
        *
    FROM
        q_data
    WHERE
        temp_blks_written > 0
    ORDER BY
        temp_blks_written DESC
    LIMIT 100) a) b;
