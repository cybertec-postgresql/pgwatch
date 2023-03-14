package config

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	flags "github.com/jessevdk/go-flags"
	"github.com/spf13/viper"
)

type cmdArg struct {
	fullName string
	*flags.Option
}

func (a cmdArg) HasChanged() bool {
	return a.IsSet() && !a.IsSetDefault()
}

func (a cmdArg) Name() string {
	return a.fullName
}

func (a cmdArg) ValueString() string {
	return fmt.Sprintf("%v", a.Value())
}

func (a cmdArg) ValueType() string {
	return a.Field().Type.Name()
}

type cmdArgSet struct {
	*flags.Parser
}

func eachGroup(g *flags.Group, f func(*flags.Group)) {
	f(g)
	for _, gg := range g.Groups() {
		eachGroup(gg, f)
	}
}

func eachOption(g *flags.Group, f func(*flags.Group, *flags.Option)) {
	eachGroup(g, func(g *flags.Group) {
		for _, option := range g.Options() {
			f(g, option)
		}
	})
}

// VisitAll will execute fn() for all options found in command line.
// Since we have only two level of nesting it's enough to use simplified group-prefixed name.
func (cmdSet cmdArgSet) VisitAll(fn func(viper.FlagValue)) {
	root := cmdSet.Parser.Group.Find("Application Options")
	eachOption(root, func(g *flags.Group, o *flags.Option) {
		name := o.LongName
		if g != root {
			name = g.ShortDescription + cmdSet.Parser.NamespaceDelimiter + name
		}
		fn(cmdArg{name, o})
	})
}

func (cmdSet cmdArgSet) setDefaults(v *viper.Viper) {
	eachOption(cmdSet.Parser.Group, func(g *flags.Group, o *flags.Option) {
		if o.Default != nil && o.IsSetDefault() {
			name := o.LongName
			if g != cmdSet.Parser.Group {
				name = g.ShortDescription + cmdSet.Parser.NamespaceDelimiter + name
			}
			v.SetDefault(name, o.Value())
		}
	})
}

// NewConfig returns a new instance of CmdOptions
func NewConfig(writer io.Writer) (*CmdOptions, error) {
	v := viper.New()
	p, err := Parse(writer)
	if err != nil {
		return nil, err
	}
	flagSet := cmdArgSet{p}
	if err = v.BindFlagValues(flagSet); err != nil {
		return nil, fmt.Errorf("cannot bind command-line flag values with viper: %w", err)
	}
	flagSet.setDefaults(v)
	if v.IsSet("config") {
		v.SetConfigFile(v.GetString("config"))
		err := v.ReadInConfig() // Find and read the config file
		if err != nil {         // Handle errors reading the config file
			return nil, fmt.Errorf("Fatal error reading config file: %w", err)
		}
	}
	conf := &CmdOptions{}
	if err = v.Unmarshal(conf); err != nil {
		return nil, fmt.Errorf("Fatal error unmarshalling config file: %w", err)
	}

	return conf, validateConfig(conf)
}

func checkFolderExistsAndReadable(path string) bool {
	_, err := ioutil.ReadDir(path)
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

	if conf.TestdataDays > 0 || conf.TestdataMultiplier > 0 {
		if conf.AdHocConnString == "" {
			return errors.New("Test mode requires --adhoc-conn-str!")
		}
		if conf.TestdataMultiplier == 0 {
			return errors.New("Test mode requires --testdata-multiplier!")
		}
		if conf.TestdataDays == 0 {
			return errors.New("Test mode requires --testdata-days!")
		}
	}
	if conf.AddRealDbname && conf.RealDbnameField == "" {
		return errors.New("--real-dbname-field cannot be empty when --add-real-dbname enabled")
	}
	if conf.AddSystemIdentifier && conf.SystemIdentifierField == "" {
		return errors.New("--system-identifier-field cannot be empty when --add-system-identifier enabled")
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
		} else {
			keyBytes, err := ioutil.ReadFile(conf.AesGcmKeyphraseFile)
			if err != nil {
				return err
			}
			if keyBytes[len(keyBytes)-1] == 10 {
				conf.AesGcmKeyphrase = string(keyBytes[:len(keyBytes)-1]) // remove line feed
			} else {
				conf.AesGcmKeyphrase = string(keyBytes)
			}
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
			return fmt.Errorf("--metrics-folder \"%s\" not readable, trying 1st default paths and then Config DB to fetch metric definitions...", conf.Metric.MetricsFolder)
		}
		if conf.Connection.User > "" && conf.Connection.Password > "" {
			return errors.New("Conflicting flags! --adhoc-conn-str and --user/--password cannot be both set")
		}
		if conf.AdHocDBType != DBTYPE_PG && conf.AdHocDBType != DBTYPE_PG_CONT {
			return fmt.Errorf("--adhoc-dbtype can be of: [ %s (single DB) | %s (all non-template DB-s on an instance) ]. Default: %s", DBTYPE_PG, DBTYPE_PG_CONT, DBTYPE_PG)
		}
	}
	return nil
}
