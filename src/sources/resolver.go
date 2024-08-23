package sources

// This file contains the implemendation of Patroni and PostgrSQL resolvers for continuous monitoring.
// Patroni resolver will return the list of databases from the Patroni cluster.
// Postgres resolver will return the list of databases from the given Postgres instance.

import (
	"cmp"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"time"

	"github.com/cybertec-postgresql/pgwatch/db"
	"github.com/cybertec-postgresql/pgwatch/log"
	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	client "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

// ResolveDatabases() updates list of monitored objects from continuous monitoring sources, e.g. patroni
func (srcs Sources) ResolveDatabases() (_ MonitoredDatabases, err error) {
	resolvedDbs := make(MonitoredDatabases, 0, len(srcs))
	for _, s := range srcs {
		if !s.IsEnabled {
			continue
		}
		dbs, e := s.ResolveDatabases()
		err = errors.Join(err, e)
		resolvedDbs = append(resolvedDbs, dbs...)
	}
	return resolvedDbs, nil
}

// ResolveDatabases() return a slice of found databases for continuous monitoring sources, e.g. patroni
func (s Source) ResolveDatabases() (MonitoredDatabases, error) {
	switch s.Kind {
	case SourcePatroni, SourcePatroniContinuous, SourcePatroniNamespace:
		return ResolveDatabasesFromPatroni(s)
	case SourcePostgresContinuous:
		return ResolveDatabasesFromPostgres(s)
	}
	return MonitoredDatabases{&MonitoredDatabase{Source: *(&s).Clone()}}, nil
}

type PatroniClusterMember struct {
	Scope   string
	Name    string
	ConnURL string `yaml:"conn_url"`
	Role    string
}

var logger log.LoggerIface = log.FallbackLogger

var lastFoundClusterMembers = make(map[string][]PatroniClusterMember) // needed for cases where DCS is temporarily down
// don't want to immediately remove monitoring of DBs

func getConsulClusterMembers(Source) ([]PatroniClusterMember, error) {
	return nil, errors.ErrUnsupported
}

func getZookeeperClusterMembers(Source) ([]PatroniClusterMember, error) {
	return nil, errors.ErrUnsupported
}

func parseHostAndPortFromJdbcConnStr(connStr string) (string, string, error) {
	r := regexp.MustCompile(`postgres://(.*)+:([0-9]+)/`)
	matches := r.FindStringSubmatch(connStr)
	if len(matches) != 3 {
		logger.Errorf("Unexpected regex result groups:", matches)
		return "", "", fmt.Errorf("unexpected regex result groups: %v", matches)
	}
	return matches[1], matches[2], nil
}

func jsonTextToStringMap(jsonText string) (map[string]string, error) {
	retmap := make(map[string]string)
	if jsonText == "" {
		return retmap, nil
	}
	var iMap map[string]any
	if err := json.Unmarshal([]byte(jsonText), &iMap); err != nil {
		return nil, err
	}
	for k, v := range iMap {
		retmap[k] = fmt.Sprintf("%v", v)
	}
	return retmap, nil
}

func getTransport(conf HostConfigAttrs) (*tls.Config, error) {
	var caCertPool *x509.CertPool

	// create valid CertPool only if the ca certificate file exists
	if conf.CAFile != "" {
		caCert, err := os.ReadFile(conf.CAFile)
		if err != nil {
			return nil, fmt.Errorf("cannot load CA file: %s", err)
		}

		caCertPool = x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
	}

	var certificates []tls.Certificate

	// create valid []Certificate only if the client cert and key files exists
	if conf.CertFile != "" && conf.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(conf.CertFile, conf.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("cannot load client cert or key file: %s", err)
		}

		certificates = []tls.Certificate{cert}
	}

	tlsClientConfig := new(tls.Config)

	if caCertPool != nil {
		tlsClientConfig.RootCAs = caCertPool
		if certificates != nil {
			tlsClientConfig.Certificates = certificates
		}
	}

	return tlsClientConfig, nil
}

func getEtcdClusterMembers(s Source) ([]PatroniClusterMember, error) {
	var ret = make([]PatroniClusterMember, 0)
	var cfg client.Config

	if len(s.HostConfig.DcsEndpoints) == 0 {
		return ret, errors.New("Missing ETCD connect info, make sure host config has a 'dcs_endpoints' key")
	}

	tlsConfig, err := getTransport(s.HostConfig)
	if err != nil {
		return nil, err
	}
	cfg = client.Config{
		Endpoints:            s.HostConfig.DcsEndpoints,
		TLS:                  tlsConfig,
		DialKeepAliveTimeout: time.Second,
		Username:             s.HostConfig.Username,
		Password:             s.HostConfig.Password,
		DialTimeout:          5 * time.Second,
		Logger:               zap.NewNop(),
	}

	c, err := client.New(cfg)
	if err != nil {
		return ret, err
	}

	ctx, cancel := context.WithTimeoutCause(context.Background(), 5*time.Second, errors.New("etcd client timeout"))
	defer cancel()
	kapi := c.KV

	if s.Kind == SourcePatroniNamespace { // all scopes, all DBs (regex filtering applies if defined)
		if len(s.GetDatabaseName()) > 0 {
			return ret, fmt.Errorf("Skipping Patroni entry %s - cannot specify a DB name when monitoring all scopes (regex patterns are supported though)", s.Name)
		}
		if s.HostConfig.Namespace == "" {
			return ret, fmt.Errorf("Skipping Patroni entry %s - search 'namespace' not specified", s.Name)
		}
		resp, err := kapi.Get(ctx, s.HostConfig.Namespace)
		if err != nil {

			return ret, cmp.Or(context.Cause(ctx), err)
		}

		for _, node := range resp.Kvs {
			scope := path.Base(string(node.Key)) // Key="/service/batman"
			scopeMembers, err := extractEtcdScopeMembers(ctx, s, scope, kapi, true)
			if err != nil {
				continue
			}
			ret = append(ret, scopeMembers...)
		}
	} else {
		ret, err = extractEtcdScopeMembers(ctx, s, s.HostConfig.Scope, kapi, false)
		if err != nil {
			return ret, cmp.Or(context.Cause(ctx), err)
		}
	}
	lastFoundClusterMembers[s.Name] = ret
	return ret, nil
}

func extractEtcdScopeMembers(ctx context.Context, s Source, scope string, kapi client.KV, addScopeToName bool) ([]PatroniClusterMember, error) {
	var ret = make([]PatroniClusterMember, 0)
	var name string
	membersPath := path.Join(s.HostConfig.Namespace, scope, "members")

	resp, err := kapi.Get(ctx, membersPath)
	if err != nil {
		return nil, err
	}
	logger.Debugf("ETCD response for %s scope %s: %+v", s.Name, scope, resp)

	for _, node := range resp.Kvs {
		logger.Debugf("Found a cluster member from etcd [%s:%s]: %+v", s.Name, scope, node.Value)
		nodeData, err := jsonTextToStringMap(string(node.Value))
		if err != nil {
			logger.Errorf("Could not parse ETCD node data for node \"%s\": %s", node, err)
			continue
		}
		role := nodeData["role"]
		connURL := nodeData["conn_url"]
		if addScopeToName {
			name = scope + "_" + path.Base(string(node.Key))
		} else {
			name = path.Base(string(node.Key))
		}

		ret = append(ret, PatroniClusterMember{Scope: scope, ConnURL: connURL, Role: role, Name: name})
	}
	return ret, nil
}

const (
	dcsTypeEtcd      = "etcd"
	dcsTypeZookeeper = "zookeeper"
	dcsTypeConsul    = "consul"
)

func ResolveDatabasesFromPatroni(ce Source) ([]*MonitoredDatabase, error) {
	var mds []*MonitoredDatabase
	var clusterMembers []PatroniClusterMember
	var err error
	var ok bool
	var dbUnique string

	switch ce.HostConfig.DcsType {
	case dcsTypeEtcd:
		clusterMembers, err = getEtcdClusterMembers(ce)
	case dcsTypeZookeeper:
		clusterMembers, err = getZookeeperClusterMembers(ce)
	case dcsTypeConsul:
		clusterMembers, err = getConsulClusterMembers(ce)
	default:
		return nil, errors.New("unknown DCS")
	}
	if err != nil {
		logger.WithField("source", ce.Name).Debug("Failed to get info from DCS, using previous member info if any")
		if clusterMembers, ok = lastFoundClusterMembers[ce.Name]; ok { // mask error from main loop not to remove monitored DBs due to "jitter"
			err = nil
		}
	} else {
		lastFoundClusterMembers[ce.Name] = clusterMembers
	}
	if len(clusterMembers) == 0 {
		return mds, err
	}

	for _, m := range clusterMembers {
		logger.Infof("Processing Patroni cluster member [%s:%s]", ce.Name, m.Name)
		if ce.OnlyIfMaster && m.Role != "master" {
			logger.Infof("Skipping over Patroni cluster member [%s:%s] as not a master", ce.Name, m.Name)
			continue
		}
		host, port, err := parseHostAndPortFromJdbcConnStr(m.ConnURL)
		if err != nil {
			logger.Errorf("Could not parse Patroni conn str \"%s\" [%s:%s]: %v", m.ConnURL, ce.Name, m.Scope, err)
			continue
		}
		if ce.OnlyIfMaster {
			dbUnique = ce.Name
			if ce.Kind == SourcePatroniNamespace {
				dbUnique = ce.Name + "_" + m.Scope
			}
		} else {
			dbUnique = ce.Name + "_" + m.Name
		}
		if ce.GetDatabaseName() != "" {
			c := &MonitoredDatabase{Source: *ce.Clone()}
			c.Name = dbUnique
			mds = append(mds, c)
			continue
		}
		c, err := db.New(context.TODO(), ce.ConnStr,
			func(c *pgxpool.Config) error {
				c.ConnConfig.Host = host
				c.ConnConfig.Database = "template1"
				i, err := strconv.ParseUint(port, 10, 16)
				c.ConnConfig.Port = uint16(i)
				mds[len(mds)].ConnStr = c.ConnString()
				return err
			})
		if err != nil {
			logger.Errorf("Could not contact Patroni member [%s:%s]: %v", ce.Name, m.Scope, err)
			continue
		}
		defer c.Close()
		sql := `select datname::text as datname,
					quote_ident(datname)::text as datname_escaped
					from pg_database
					where not datistemplate
					and datallowconn
					and has_database_privilege (datname, 'CONNECT')
					and case when length(trim($1)) > 0 then datname ~ $1 else true end
					and case when length(trim($2)) > 0 then not datname ~ $2 else true end`

		rows, err := c.Query(context.TODO(), sql, ce.IncludePattern, ce.ExcludePattern)
		if err != nil {
			return nil, err
		}
		data, err := pgx.CollectRows(rows, pgx.RowToMap)
		if err != nil {
			logger.Errorf("Could not get DB name listing from Patroni member [%s:%s]: %v", ce.Name, m.Scope, err)
			continue
		}

		for _, d := range data {
			connURL, err := url.Parse(ce.ConnStr)
			if err != nil {
				continue
			}
			connURL.Host = host + ":" + port
			connURL.Path = d["datname"].(string)
			c := ce.Clone()
			c.Name = dbUnique + "_" + d["datname_escaped"].(string)
			c.ConnStr = connURL.String()
			mds = append(mds, &MonitoredDatabase{Source: *c})
		}

	}

	return mds, err
}

// "resolving" reads all the DB names from the given host/port, additionally matching/not matching specified regex patterns
func ResolveDatabasesFromPostgres(s Source) (resolvedDbs MonitoredDatabases, err error) {
	var (
		c      db.PgxPoolIface
		dbname string
		rows   pgx.Rows
	)
	c, err = db.New(context.TODO(), s.ConnStr)
	if err != nil {
		return
	}
	defer c.Close()

	sql := `select /* pgwatch3_generated */
		quote_ident(datname)::text as datname_escaped
		from pg_database
		where not datistemplate
		and datallowconn
		and has_database_privilege (datname, 'CONNECT')
		and case when length(trim($1)) > 0 then datname ~ $1 else true end
		and case when length(trim($2)) > 0 then not datname ~ $2 else true end`

	if rows, err = c.Query(context.TODO(), sql, s.IncludePattern, s.ExcludePattern); err != nil {
		return nil, err
	}
	for rows.Next() {
		if err = rows.Scan(&dbname); err != nil {
			return nil, err
		}
		rdb := &MonitoredDatabase{Source: *s.Clone()}
		rdb.Name += "_" + dbname
		rdb.SetDatabaseName(dbname)
		resolvedDbs = append(resolvedDbs, rdb)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return
}
