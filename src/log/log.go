package log

import (
	"context"
	"os"

	"github.com/cybertec-postgresql/pgwatch3/config"
	"github.com/jackc/pgx/v5/tracelog"
	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

type (
	// LoggerIface is the interface used by all components
	LoggerIface logrus.FieldLogger
	//LoggerHookerIface adds AddHook method to LoggerIface for database logging hook
	LoggerHookerIface interface {
		LoggerIface
		AddHook(hook logrus.Hook)
		AddSubscriber(msgCh MessageChanType)
		RemoveSubscriber(msgCh MessageChanType)
	}

	loggerKey struct{}
)

type logger struct {
	*logrus.Logger
	*BrokerHook
}

func getLogFileWriter(opts config.LoggingOpts) any {
	if opts.LogFileRotate {
		return &lumberjack.Logger{
			Filename:   opts.LogFile,
			MaxSize:    opts.LogFileSize,
			MaxBackups: opts.LogFileNumber,
			MaxAge:     opts.LogFileAge,
		}
	}
	return opts.LogFile
}

func getLogFileFormatter(opts config.LoggingOpts) logrus.Formatter {
	if opts.LogFileFormat == "text" {
		return &logrus.TextFormatter{}
	}
	return &logrus.JSONFormatter{}
}

// Init creates logging facilities for the application
func Init(opts config.LoggingOpts) LoggerHookerIface {
	var err error
	l := logger{logrus.New(), NewHook(context.Background(), opts.LogLevel)}
	l.AddHook(l.BrokerHook)
	l.Out = os.Stdout
	if opts.LogFile > "" {
		l.AddHook(lfshook.NewHook(getLogFileWriter(opts), getLogFileFormatter(opts)))
	}
	l.Level, err = logrus.ParseLevel(opts.LogLevel)
	if err != nil {
		l.Level = logrus.InfoLevel
	}
	l.SetFormatter(&Formatter{
		HideKeys:        false,
		FieldsOrder:     []string{"database", "metric", "sql", "params"},
		TimestampFormat: "2006-01-02 15:04:05.000",
		ShowFullLevel:   true,
	})
	l.SetBrokerFormatter(l.Formatter)
	l.SetReportCaller(l.Level > logrus.InfoLevel)
	return l
}

// PgxLogger is the struct used to log using pgx postgres driver
type PgxLogger struct {
	l LoggerIface
}

// NewPgxLogger returns a new instance of PgxLogger
func NewPgxLogger(l LoggerIface) *PgxLogger {
	return &PgxLogger{l}
}

// Log transforms logging calls from pgx to logrus
func (pgxlogger *PgxLogger) Log(ctx context.Context, level tracelog.LogLevel, msg string, data map[string]any) {
	logger := GetLogger(ctx)
	if logger == FallbackLogger { //switch from standard to specified
		logger = pgxlogger.l
	}
	if data != nil {
		logger = logger.WithFields(data)
	}
	switch level {
	case tracelog.LogLevelTrace:
		logger.WithField("PGX_LOG_LEVEL", level).Debug(msg)
	case tracelog.LogLevelDebug, tracelog.LogLevelInfo: //pgx is way too chatty on INFO level
		logger.Debug(msg)
	case tracelog.LogLevelWarn:
		logger.Warn(msg)
	case tracelog.LogLevelError:
		logger.Error(msg)
	default:
		logger.WithField("INVALID_PGX_LOG_LEVEL", level).Error(msg)
	}
}

// WithLogger returns a new context with the provided logger. Use in
// combination with logger.WithField(s) for great effect
func WithLogger(ctx context.Context, logger LoggerIface) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// FallbackLogger is an alias for the standard logger
var FallbackLogger = logrus.StandardLogger()

// GetLogger retrieves the current logger from the context. If no logger is
// available, the default logger is returned
func GetLogger(ctx context.Context) LoggerIface {
	logger := ctx.Value(loggerKey{})
	if logger == nil {
		return FallbackLogger
	}
	return logger.(LoggerIface)
}
