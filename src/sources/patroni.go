package sources

import (
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

	"github.com/cybertec-postgresql/pgwatch3/db"
	"github.com/cybertec-postgresql/pgwatch3/log"
	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	client "go.etcd.io/etcd/client/v3"
)

type PatroniClusterMember struct {
	Scope   string
	Name    string
	ConnURL string `yaml:"conn_url"`
	Role    string
}

var logger log.LoggerIface = log.FallbackLogger

var lastFoundClusterMembers = make(map[string][]PatroniClusterMember) // needed for cases where DCS is temporarily down
// don't want to immediately remove monitoring of DBs

func getConsulClusterMembers(*MonitoredDatabase) ([]PatroniClusterMember, error) {
	return nil, errors.ErrUnsupported
}

func getZookeeperClusterMembers(*MonitoredDatabase) ([]PatroniClusterMember, error) {
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

func getEtcdClusterMembers(database *MonitoredDatabase) ([]PatroniClusterMember, error) {
	var ret = make([]PatroniClusterMember, 0)
	var cfg client.Config

	if len(database.HostConfig.DcsEndpoints) == 0 {
		return ret, errors.New("Missing ETCD connect info, make sure host config has a 'dcs_endpoints' key")
	}

	tlsConfig, err := getTransport(database.HostConfig)
	if err != nil {
		return nil, err
	}
	cfg = client.Config{
		Endpoints:            database.HostConfig.DcsEndpoints,
		TLS:                  tlsConfig,
		DialKeepAliveTimeout: time.Second,
		Username:             database.HostConfig.Username,
		Password:             database.HostConfig.Password,
	}

	c, err := client.New(cfg)
	if err != nil {
		logger.Errorf("[%s ]Could not connect to ETCD: %v", database.DBUniqueName, err)
		return ret, err
	}
	kapi := c.KV

	if database.Kind == SourcePatroniNamespace { // all scopes, all DBs (regex filtering applies if defined)
		if len(database.GetDatabaseName()) > 0 {
			return ret, fmt.Errorf("Skipping Patroni entry %s - cannot specify a DB name when monitoring all scopes (regex patterns are supported though)", database.DBUniqueName)
		}
		if database.HostConfig.Namespace == "" {
			return ret, fmt.Errorf("Skipping Patroni entry %s - search 'namespace' not specified", database.DBUniqueName)
		}
		resp, err := kapi.Get(context.Background(), database.HostConfig.Namespace)
		if err != nil {
			return ret, err
		}

		for _, node := range resp.Kvs {
			scope := path.Base(string(node.Key)) // Key="/service/batman"
			scopeMembers, err := extractEtcdScopeMembers(database, scope, kapi, true)
			if err != nil {
				continue
			}
			ret = append(ret, scopeMembers...)
		}
	} else {
		ret, err = extractEtcdScopeMembers(database, database.HostConfig.Scope, kapi, false)
		if err != nil {
			return ret, err
		}
	}
	lastFoundClusterMembers[database.DBUniqueName] = ret
	return ret, nil
}

func extractEtcdScopeMembers(database *MonitoredDatabase, scope string, kapi client.KV, addScopeToName bool) ([]PatroniClusterMember, error) {
	var ret = make([]PatroniClusterMember, 0)
	var name string
	membersPath := path.Join(database.HostConfig.Namespace, scope, "members")

	resp, err := kapi.Get(context.Background(), membersPath)
	if err != nil {
		return nil, err
	}
	logger.Debugf("ETCD response for %s scope %s: %+v", database.DBUniqueName, scope, resp)

	for _, node := range resp.Kvs {
		logger.Debugf("Found a cluster member from etcd [%s:%s]: %+v", database.DBUniqueName, scope, node.Value)
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

func ResolveDatabasesFromPatroni(ce *MonitoredDatabase) ([]*MonitoredDatabase, error) {
	var md []*MonitoredDatabase
	var cm []PatroniClusterMember
	var err error
	var ok bool
	var dbUnique string

	if ce.Kind == SourcePatroniNamespace && ce.HostConfig.DcsType != dcsTypeEtcd {
		logger.Warningf("Skipping Patroni monitoring entry \"%s\" as currently only ETCD namespace scanning is supported...", ce.DBUniqueName)
		return md, nil
	}
	logger.Debugf("Resolving Patroni nodes for \"%s\" from HostConfig: %+v", ce.DBUniqueName, ce.HostConfig)
	if ce.HostConfig.DcsType == dcsTypeEtcd {
		cm, err = getEtcdClusterMembers(ce)
	} else if ce.HostConfig.DcsType == dcsTypeZookeeper {
		cm, err = getZookeeperClusterMembers(ce)
	} else if ce.HostConfig.DcsType == dcsTypeConsul {
		cm, err = getConsulClusterMembers(ce)
	} else {
		logger.Error("unknown DCS", ce.HostConfig.DcsType)
		return md, errors.New("unknown DCS")
	}
	if err != nil {
		logger.Warningf("Failed to get info from DCS for %s, using previous member info if any", ce.DBUniqueName)
		cm, ok = lastFoundClusterMembers[ce.DBUniqueName]
		if ok { // mask error from main loop not to remove monitored DBs due to "jitter"
			err = nil
		}
	} else {
		lastFoundClusterMembers[ce.DBUniqueName] = cm
	}
	if len(cm) == 0 {
		logger.Warningf("No Patroni cluster members found for cluster [%s:%s]", ce.DBUniqueName, ce.HostConfig.Scope)
		return md, nil
	}
	logger.Infof("Found %d Patroni members for entry %s", len(cm), ce.DBUniqueName)

	for _, m := range cm {
		logger.Infof("Processing Patroni cluster member [%s:%s]", ce.DBUniqueName, m.Name)
		if ce.OnlyIfMaster && m.Role != "master" {
			logger.Infof("Skipping over Patroni cluster member [%s:%s] as not a master", ce.DBUniqueName, m.Name)
			continue
		}
		host, port, err := parseHostAndPortFromJdbcConnStr(m.ConnURL)
		if err != nil {
			logger.Errorf("Could not parse Patroni conn str \"%s\" [%s:%s]: %v", m.ConnURL, ce.DBUniqueName, m.Scope, err)
			continue
		}
		if ce.OnlyIfMaster {
			dbUnique = ce.DBUniqueName
			if ce.Kind == SourcePatroniNamespace {
				dbUnique = ce.DBUniqueName + "_" + m.Scope
			}
		} else {
			dbUnique = ce.DBUniqueName + "_" + m.Name
		}
		if ce.GetDatabaseName() != "" {
			md = append(md, &MonitoredDatabase{
				DBUniqueName:     dbUnique,
				DBUniqueNameOrig: ce.DBUniqueName,
				ConnStr:          ce.ConnStr,
				Metrics:          ce.Metrics,
				PresetMetrics:    ce.PresetMetrics,
				IsSuperuser:      ce.IsSuperuser,
				CustomTags:       ce.CustomTags,
				HostConfig:       ce.HostConfig,
				Kind:             "postgres"})
			continue
		}
		c, err := db.New(context.TODO(), ce.ConnStr,
			func(c *pgxpool.Config) error {
				c.ConnConfig.Host = host
				c.ConnConfig.Database = "template1"
				i, err := strconv.ParseUint(port, 10, 16)
				c.ConnConfig.Port = uint16(i)
				md[len(md)].ConnStr = c.ConnString()
				return err
			})
		if err != nil {
			logger.Errorf("Could not contact Patroni member [%s:%s]: %v", ce.DBUniqueName, m.Scope, err)
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
			logger.Errorf("Could not get DB name listing from Patroni member [%s:%s]: %v", ce.DBUniqueName, m.Scope, err)
			continue
		}

		for _, d := range data {
			connURL, err := url.Parse(ce.ConnStr)
			if err != nil {
				continue
			}
			connURL.Host = host + ":" + port
			connURL.Path = d["datname"].(string)
			md = append(md, &MonitoredDatabase{
				DBUniqueName:     dbUnique + "_" + d["datname_escaped"].(string),
				DBUniqueNameOrig: dbUnique,
				ConnStr:          connURL.String(),
				Metrics:          ce.Metrics,
				PresetMetrics:    ce.PresetMetrics,
				IsSuperuser:      ce.IsSuperuser,
				CustomTags:       ce.CustomTags,
				HostConfig:       ce.HostConfig,
				Kind:             "postgres"})
		}

	}

	return md, err
}
