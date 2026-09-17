package log

// CmdOpts specifies the logging command-line options
type CmdOpts struct {
	LogLevel      string `short:"v" long:"log-level" mapstructure:"log-level" env:"PW_LOG_LEVEL" description:"Verbosity level for stdout and log file" choice:"debug" choice:"info" choice:"error" default:"info"`
	LogFile       string `long:"log-file" mapstructure:"log-file" env:"PW_LOG_FILE" description:"File name to store logs"`
	LogFileFormat string `long:"log-file-format" mapstructure:"log-file-format" env:"PW_LOG_FILE_FORMAT" description:"Format of file logs" choice:"json" choice:"text" default:"json"`
	LogFileRotate bool   `long:"log-file-rotate" mapstructure:"log-file-rotate" env:"PW_LOG_FILE_ROTATE" description:"Rotate log files"`
	LogFileSize   int    `long:"log-file-size" mapstructure:"log-file-size" env:"PW_LOG_FILE_SIZE" description:"Maximum size in MB of the log file before it gets rotated" default:"100"`
	LogFileAge    int    `long:"log-file-age" mapstructure:"log-file-age" env:"PW_LOG_FILE_AGE" description:"Number of days to retain old log files, 0 means forever" default:"0"`
	LogFileNumber int    `long:"log-file-number" mapstructure:"log-file-number" env:"PW_LOG_FILE_NUMBER" description:"Maximum number of old log files to retain, 0 to retain all" default:"0"`
}
