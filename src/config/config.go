package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	flags "github.com/jessevdk/go-flags"
)

// NewConfig returns a new instance of CmdOptions
func NewConfig(writer io.Writer) (*Options, error) {
	cmdOpts := new(Options)
	parser := flags.NewParser(cmdOpts, flags.PrintErrors)
	var err error
	if _, err = parser.Parse(); err != nil {
		if !flags.WroteHelp(err) {
			parser.WriteHelp(writer)
			return nil, err
		}
	}
	return cmdOpts, validateConfig(cmdOpts)
}

func checkFolderExistsAndReadable(path string) bool {
	_, err := os.ReadDir(path)
	return err == nil
}

const (
	defaultMetricsDefinitionPathPkg    = "/etc/pgwatch3/metrics" // prebuilt packages / Docker default location
	defaultMetricsDefinitionPathDocker = "/pgwatch3/metrics"     // prebuilt packages / Docker default location
)

// Verbose returns true if the debug log is enabled
func (c Options) Verbose() bool {
	return c.Logging.LogLevel == "debug"
}

func (c Options) IsAdHocMode() bool {
	return len(c.AdHocConnString)+len(c.AdHocConfig) > 0
}

// VersionOnly returns true if the `--version` is the only argument
func (c Options) VersionOnly() bool {
	return len(os.Args) == 2 && c.Version
}

func (c Options) GetConfigKind() (_ Kind, err error) {
	if _, err := pgx.ParseConfig(c.Connection.Config); err == nil {
		return Kind(ConfigPgURL), nil
	}
	var fi os.FileInfo
	if fi, err = os.Stat(c.Connection.Config); err == nil {
		if fi.IsDir() {
			return Kind(ConfigFolder), nil
		}
		return Kind(ConfigFile), nil
	}
	return Kind(ConfigError), err
}

func validateConfig(c *Options) error {
	if c.Connection.ServersRefreshLoopSeconds <= 1 {
		return errors.New("--servers-refresh-loop-seconds must be greater than 1")
	}
	if c.MaxParallelConnectionsPerDb < 1 {
		return errors.New("--max-parallel-connections-per-db must be >= 1")
	}

	if c.Metric.MetricsFolder > "" && !checkFolderExistsAndReadable(c.Metric.MetricsFolder) {
		return fmt.Errorf("Could not read --metrics-folder path %s", c.Metric.MetricsFolder)
	}

	if err := validateAesGcmConfig(c); err != nil {
		return err
	}

	if err := validateAdHocConfig(c); err != nil {
		return err
	}
	// validate that input is boolean is set
	if c.BatchingDelay < 0 || c.BatchingDelay > time.Hour {
		return errors.New("--batching-delay-ms must be between 0 and 3600000")
	}

	return nil
}

func validateAesGcmConfig(c *Options) error {
	if c.AesGcmKeyphraseFile > "" {
		_, err := os.Stat(c.AesGcmKeyphraseFile)
		if os.IsNotExist(err) {
			return fmt.Errorf("Failed to read aes_gcm_keyphrase_file at %s, thus cannot monitor hosts with encrypted passwords", c.AesGcmKeyphraseFile)
		}
		keyBytes, err := os.ReadFile(c.AesGcmKeyphraseFile)
		if err != nil {
			return err
		}
		if keyBytes[len(keyBytes)-1] == 10 {
			c.AesGcmKeyphrase = string(keyBytes[:len(keyBytes)-1]) // remove line feed
		} else {
			c.AesGcmKeyphrase = string(keyBytes)
		}
	}
	if c.AesGcmPasswordToEncrypt > "" && c.AesGcmKeyphrase == "" { // special flag - encrypt and exit
		return errors.New("--aes-gcm-password-to-encrypt requires --aes-gcm-keyphrase(-file)")
	}
	return nil
}

func validateAdHocConfig(c *Options) error {
	if c.AdHocConnString > "" || c.AdHocConfig > "" {
		if len(c.AdHocConnString)*len(c.AdHocConfig) == 0 {
			return errors.New("--adhoc-conn-str and --adhoc-config params both need to be specified for Ad-hoc mode to work")
		}
		if len(c.Connection.Config) > 0 {
			return errors.New("Conflicting flags! --adhoc-conn-str and --config cannot be both set")
		}
		if c.Metric.MetricsFolder == "" {
			if checkFolderExistsAndReadable(defaultMetricsDefinitionPathPkg) {
				c.Metric.MetricsFolder = defaultMetricsDefinitionPathPkg
			} else if checkFolderExistsAndReadable(defaultMetricsDefinitionPathDocker) {
				c.Metric.MetricsFolder = defaultMetricsDefinitionPathDocker
			} else {
				return errors.New("--adhoc-conn-str requires --metrics-folder")
			}
		}
		if c.AdHocSrcType != SourcePostgres && c.AdHocSrcType != SourcePostgresContinuous {
			return fmt.Errorf("--adhoc-type can be of: [ %s (single DB) | %s (all non-template DB-s on an instance) ]. Default: %s", SourcePostgres, SourcePostgresContinuous, SourcePostgres)
		}
	}
	return nil
}
