package sources

// This file contains the implemendation of Patroni and PostgrSQL resolvers for continuous monitoring.
// Patroni resolver will return the list of databases from the Patroni cluster.
// Postgres resolver will return the list of databases from the given Postgres instance.

import (
	"cmp"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"

	"github.com/cybertec-postgresql/pgwatch/v3/internal/db"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	pgx "github.com/jackc/pgx/v5"
	client "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

// ResolveDatabases() updates list of monitored objects from continuous monitoring sources, e.g. patroni
func (srcs Sources) ResolveDatabases() (_ SourceConns, err error) {
	resolvedDbs := make(SourceConns, 0, len(srcs))
	for _, s := range srcs {
		if !s.IsEnabled {
			continue
		}
		dbs, e := s.ResolveDatabases()
		err = errors.Join(err, e)
		resolvedDbs = append(resolvedDbs, dbs...)
	}
	return resolvedDbs, err
}

// ResolveDatabases() return a slice of found databases for continuous monitoring sources, e.g. patroni
func (s Source) ResolveDatabases() (SourceConns, error) {
	switch s.Kind {
	case SourcePatroni:
		return ResolveDatabasesFromPatroni(s)
	case SourcePostgresContinuous:
		return ResolveDatabasesFromPostgres(s)
	}
	return SourceConns{&SourceConn{Source: s}}, nil
}

type PatroniClusterMember struct {
	Scope   string
	Name    string
	ConnURL string `yaml:"conn_url"`
	Role    string
}

var logger log.Logger = log.FallbackLogger

var lastFoundClusterMembers = make(map[string][]PatroniClusterMember) // needed for cases where DCS is temporarily down
// don't want to immediately remove monitoring of DBs

func getConsulClusterMembers(Source) ([]PatroniClusterMember, error) {
	return nil, errors.ErrUnsupported
}

func getZookeeperClusterMembers(Source, HostConfig) ([]PatroniClusterMember, error) {
	return nil, errors.ErrUnsupported
}

func jsonTextToStringMap(jsonText string) (map[string]string, error) {
	retmap := make(map[string]string)
	if jsonText == "" {
		return retmap, nil
	}
	var iMap map[string]any
	if err := jsoniter.ConfigFastest.Unmarshal([]byte(jsonText), &iMap); err != nil {
		return nil, err
	}
	for k, v := range iMap {
		retmap[k] = fmt.Sprintf("%v", v)
	}
	return retmap, nil
}

func getTransport(conf HostConfig) (*tls.Config, error) {
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

func getEtcdClusterMembers(s Source, hc HostConfig) ([]PatroniClusterMember, error) {
	var ret = make([]PatroniClusterMember, 0)
	var cfg client.Config

	if len(hc.DcsEndpoints) == 0 {
		return ret, errors.New("missing ETCD connect info, make sure host config has a 'dcs_endpoints' key")
	}

	tlsConfig, err := getTransport(hc)
	if err != nil {
		return nil, err
	}
	cfg = client.Config{
		Endpoints:            hc.DcsEndpoints,
		TLS:                  tlsConfig,
		DialKeepAliveTimeout: time.Second,
		Username:             hc.Username,
		Password:             hc.Password,
		DialTimeout:          5 * time.Second,
		Logger:               zap.NewNop(),
	}

	c, err := client.New(cfg)
	if err != nil {
		return ret, err
	}
	defer c.Close()

	ctx, cancel := context.WithTimeoutCause(context.Background(), 5*time.Second, errors.New("etcd client timeout"))
	defer cancel()

	// etcd3 does not have a dir node.
	// Key="/namespace/scope/leader", e.g. "/service/batman/leader"
	// Key="/namespace/scope/members/node", e.g. "/service/batman/members/pg1"

	resp, err := c.Get(ctx, hc.Path, client.WithPrefix())
	if err != nil {
		return ret, cmp.Or(context.Cause(ctx), err)
	}

	for _, node := range resp.Kvs {
		nodeData, err := jsonTextToStringMap(string(node.Value))
		if err != nil {
			logger.Errorf("Could not parse ETCD node data for node \"%s\": %s", node, err)
			continue
		}
		// remove leading slash and split by "/"
		parts := strings.Split(strings.TrimPrefix(string(node.Key), "/"), "/")
		if len(parts) < 3 {
			return nil, errors.New("invalid ETCD key format")
		}
		role := nodeData["role"]
		connURL := nodeData["conn_url"]
		scope := parts[1]
		name := parts[3]
		ret = append(ret, PatroniClusterMember{Scope: scope, ConnURL: connURL, Role: role, Name: name})
	}

	lastFoundClusterMembers[s.Name] = ret
	return ret, nil
}

const (
	dcsTypeEtcd      = "etcd"
	dcsTypeZookeeper = "zookeeper"
	dcsTypeConsul    = "consul"
)

type HostConfig struct {
	DcsType      string   `yaml:"dcs_type"`
	DcsEndpoints []string `yaml:"dcs_endpoints"`
	Path         string
	Username     string
	Password     string
	CAFile       string `yaml:"ca_file"`
	CertFile     string `yaml:"cert_file"`
	KeyFile      string `yaml:"key_file"`
}

func (hc HostConfig) IsScopeSpecified() bool {
	// Path is usually "/namespace/scope"
	// so we check if it has at least 2 slashes
	return strings.Count(hc.Path, "/") >= 2
}

func NewHostConfig(URI string) (hc HostConfig, err error) {
	var url *url.URL
	url, err = url.Parse(URI)
	if err != nil {
		return
	}

	switch url.Scheme {
	case dcsTypeEtcd:
		hc.DcsType = dcsTypeEtcd
		for h := range strings.SplitSeq(url.Host, ",") {
			hc.DcsEndpoints = append(hc.DcsEndpoints, "http://"+h)
		}
	case dcsTypeZookeeper:
		hc.DcsType = dcsTypeZookeeper
		hc.DcsEndpoints = []string{url.Host} // Zookeeper usually has a
		// single endpoint, but can be a list of hosts separated by commas
	case dcsTypeConsul:
		hc.DcsType = dcsTypeConsul
		hc.DcsEndpoints = strings.Split(url.Host, ",")
	default:
		return hc, fmt.Errorf("unsupported DCS type: %s", url.Scheme)
	}

	hc.Path = url.Path
	hc.Username = url.User.Username()
	hc.Password, _ = url.User.Password() // password is optional, so we ignore the error
	hc.CAFile = url.Query().Get("ca_file")
	hc.CertFile = url.Query().Get("cert_file")
	hc.KeyFile = url.Query().Get("key_file")

	return hc, nil
}

func ResolveDatabasesFromPatroni(source Source) (SourceConns, error) {
	var mds []*SourceConn
	var clusterMembers []PatroniClusterMember
	var err error
	var ok bool

	hostConfig, err := NewHostConfig(source.ConnStr)
	if err != nil {
		return nil, err
	}

	switch hostConfig.DcsType {
	case dcsTypeEtcd:
		clusterMembers, err = getEtcdClusterMembers(source, hostConfig)
	case dcsTypeZookeeper:
		clusterMembers, err = getZookeeperClusterMembers(source, hostConfig)
	case dcsTypeConsul:
		clusterMembers, err = getConsulClusterMembers(source)
	default:
		return nil, errors.New("unknown DCS")
	}
	logger := logger.WithField("sorce", source.Name)
	if err != nil {
		if errors.Is(err, errors.ErrUnsupported) {
			return nil, err
		}
		logger.Debug("Failed to get info from DCS, using previous member info if any")
		if clusterMembers, ok = lastFoundClusterMembers[source.Name]; ok { // mask error from main loop not to remove monitored DBs due to "jitter"
			err = nil
		}
	} else {
		lastFoundClusterMembers[source.Name] = clusterMembers
	}
	if len(clusterMembers) == 0 {
		return mds, err
	}

	for _, patroniMember := range clusterMembers {
		logger.Info("Processing Patroni cluster member: ", patroniMember.Name)
		if source.OnlyIfMaster && patroniMember.Role != "master" {
			continue
		}
		src := *source.Clone()
		src.ConnStr = patroniMember.ConnURL
		if hostConfig.IsScopeSpecified() {
			src.Name += "_" + patroniMember.Scope
		}
		src.Name += "_" + patroniMember.Name
		if dbs, err := ResolveDatabasesFromPostgres(src); err == nil {
			mds = append(mds, dbs...)
		} else {
			logger.WithError(err).Error("Failed to resolve databases for Patroni member: ", patroniMember.Name)
		}
	}
	return mds, err
}

// ResolveDatabasesFromPostgres reads all the databases from the given cluster,
// additionally matching/not matching specified regex patterns
func ResolveDatabasesFromPostgres(s Source) (resolvedDbs SourceConns, err error) {
	var (
		c      db.PgxPoolIface
		dbname string
		rows   pgx.Rows
	)
	c, err = NewConn(context.TODO(), s.ConnStr)
	if err != nil {
		return
	}
	defer c.Close()

	sql := `select /* pgwatch_generated */
	datname
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
		rdb := &SourceConn{Source: *s.Clone()}
		rdb.Name += "_" + dbname
		rdb.SetDatabaseName(dbname)
		resolvedDbs = append(resolvedDbs, rdb)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return
}
