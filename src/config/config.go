package config

import (
	"errors"
	"fmt"
	"io"
	"os"

	flags "github.com/jessevdk/go-flags"
)

// NewConfig returns a new instance of CmdOptions
func NewConfig(writer io.Writer) (*CmdOptions, error) {
	cmdOpts := new(CmdOptions)
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

func validateConfig(conf *CmdOptions) error {
	if conf.Connection.ServersRefreshLoopSeconds <= 1 {
		return errors.New("--servers-refresh-loop-seconds must be greater than 1")
	}
	if conf.MaxParallelConnectionsPerDb < 1 {
		return errors.New("--max-parallel-connections-per-db must be >= 1")
	}

	if err := validateAesGcmConfig(conf); err != nil {
		return err
	}

	if err := validateAdHocConfig(conf); err != nil {
		return err
	}
	// validate that input is boolean is set
	if conf.BatchingDelayMs < 0 || conf.BatchingDelayMs > 3600000 {
		return errors.New("--batching-delay-ms must be between 0 and 3600000")
	}

	return nil
}

func validateAesGcmConfig(conf *CmdOptions) error {
	if conf.AesGcmKeyphraseFile > "" {
		_, err := os.Stat(conf.AesGcmKeyphraseFile)
		if os.IsNotExist(err) {
			return fmt.Errorf("Failed to read aes_gcm_keyphrase_file at %s, thus cannot monitor hosts with encrypted passwords", conf.AesGcmKeyphraseFile)
		}
		keyBytes, err := os.ReadFile(conf.AesGcmKeyphraseFile)
		if err != nil {
			return err
		}
		if keyBytes[len(keyBytes)-1] == 10 {
			conf.AesGcmKeyphrase = string(keyBytes[:len(keyBytes)-1]) // remove line feed
		} else {
			conf.AesGcmKeyphrase = string(keyBytes)
		}
	}
	if conf.AesGcmPasswordToEncrypt > "" && conf.AesGcmKeyphrase == "" { // special flag - encrypt and exit
		return errors.New("--aes-gcm-password-to-encrypt requires --aes-gcm-keyphrase(-file)")
	}
	return nil
}

func validateAdHocConfig(conf *CmdOptions) error {
	if conf.AdHocConnString > "" || conf.AdHocConfig > "" {
		if len(conf.AdHocConnString)*len(conf.AdHocConfig) == 0 {
			return errors.New("--adhoc-conn-str and --adhoc-config params both need to be specified for Ad-hoc mode to work")
		}
		if conf.Config > "" {
			return errors.New("Conflicting flags! --adhoc-conn-str and --config cannot be both set")
		}
		if conf.Metric.MetricsFolder > "" && !checkFolderExistsAndReadable(conf.Metric.MetricsFolder) {
			return fmt.Errorf("--metrics-folder \"%s\" not readable, trying 1st default paths and then Config DB to fetch metric definitions", conf.Metric.MetricsFolder)
		}
		if conf.Connection.User > "" && conf.Connection.Password > "" {
			return errors.New("Conflicting flags! --adhoc-conn-str and --user/--password cannot be both set")
		}
		if conf.AdHocDBType != DbTypePg && conf.AdHocDBType != DbTypePgCont {
			return fmt.Errorf("--adhoc-dbtype can be of: [ %s (single DB) | %s (all non-template DB-s on an instance) ]. Default: %s", DbTypePg, DbTypePgCont, DbTypePg)
		}
	}
	return nil
}
