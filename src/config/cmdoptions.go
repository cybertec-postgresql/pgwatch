package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	flags "github.com/jessevdk/go-flags"
	"golang.org/x/crypto/pbkdf2"
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
	if _, err := pgx.ParseConfig(c.Source.Config); err == nil {
		return Kind(ConfigPgURL), nil
	}
	var fi os.FileInfo
	if fi, err = os.Stat(c.Source.Config); err == nil {
		if fi.IsDir() {
			return Kind(ConfigFolder), nil
		}
		return Kind(ConfigFile), nil
	}
	return Kind(ConfigError), err
}

func validateConfig(c *Options) error {
	if c.Source.Refresh <= 1 {
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
	if c.Source.AesGcmKeyphraseFile > "" {
		_, err := os.Stat(c.Source.AesGcmKeyphraseFile)
		if os.IsNotExist(err) {
			return fmt.Errorf("Failed to read aes_gcm_keyphrase_file at %s, thus cannot monitor hosts with encrypted passwords", c.Source.AesGcmKeyphraseFile)
		}
		keyBytes, err := os.ReadFile(c.Source.AesGcmKeyphraseFile)
		if err != nil {
			return err
		}
		if keyBytes[len(keyBytes)-1] == 10 {
			c.Source.AesGcmKeyphrase = string(keyBytes[:len(keyBytes)-1]) // remove line feed
		} else {
			c.Source.AesGcmKeyphrase = string(keyBytes)
		}
	}
	if c.Source.AesGcmPasswordToEncrypt > "" && c.Source.AesGcmKeyphrase == "" { // special flag - encrypt and exit
		return errors.New("--aes-gcm-password-to-encrypt requires --aes-gcm-keyphrase(-file)")
	}
	return nil
}

func validateAdHocConfig(c *Options) error {
	if c.AdHocConnString > "" || c.AdHocConfig > "" {
		if len(c.AdHocConnString)*len(c.AdHocConfig) == 0 {
			return errors.New("--adhoc-conn-str and --adhoc-config params both need to be specified for Ad-hoc mode to work")
		}
		if len(c.Source.Config) > 0 {
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
		if !strings.HasPrefix(c.AdHocSrcType, "postgres") {
			return fmt.Errorf("--adhoc-type can be of: [ postgres (single DB) | postgres-continuous-discovery (all non-template DB-s on an instance) ]. Default: postgres-continuous-discovery")
		}
	}
	return nil
}

func (c Options) Encrypt() string { // called when --password-to-encrypt set
	passphrase, plaintext := c.Source.AesGcmKeyphrase, c.Source.AesGcmPasswordToEncrypt
	key, salt := deriveKey(passphrase, nil)
	iv := make([]byte, 12)
	_, _ = rand.Read(iv)
	b, _ := aes.NewCipher(key)
	aesgcm, _ := cipher.NewGCM(b)
	data := aesgcm.Seal(nil, iv, []byte(plaintext), nil)
	return hex.EncodeToString(salt) + "-" + hex.EncodeToString(iv) + "-" + hex.EncodeToString(data)
}

func deriveKey(passphrase string, salt []byte) ([]byte, []byte) {
	if salt == nil {
		salt = make([]byte, 8)
		_, _ = rand.Read(salt)
	}
	return pbkdf2.Key([]byte(passphrase), salt, 1000, 32, sha256.New), salt
}

func (c Options) Decrypt(ciphertext string) string {
	arr := strings.Split(ciphertext, "-")
	if len(arr) != 3 {
		// Warning("Aes-gcm-256 encrypted password for \"%s\" should consist of 3 parts - using 'as is'", dbUnique)
		return ciphertext
	}
	salt, _ := hex.DecodeString(arr[0])
	iv, _ := hex.DecodeString(arr[1])
	data, _ := hex.DecodeString(arr[2])
	key, _ := deriveKey(c.Source.AesGcmKeyphrase, salt)
	b, _ := aes.NewCipher(key)
	aesgcm, _ := cipher.NewGCM(b)
	data, _ = aesgcm.Open(nil, iv, data, nil)
	//log.Debug("decoded", string(data))
	return string(data)
}
