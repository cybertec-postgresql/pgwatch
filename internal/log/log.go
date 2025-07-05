package log

import (
	"context"
	"os"

	"github.com/jackc/pgx/v5/tracelog"
	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

type (
	// Logger is the interface used by all components
	Logger logrus.FieldLogger
	//LoggerHooker adds AddHook method to LoggerIface for database logging hook
	LoggerHooker interface {
		Logger
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

func getLogFileWriter(opts CmdOpts) any {
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

const (
	disableColors = true
	enableColors  = false
)

func getLogFileFormatter(opts CmdOpts) logrus.Formatter {
	if opts.LogFileFormat == "text" {
		return newFormatter(disableColors)
	}
	return &logrus.JSONFormatter{}
}

// Init creates logging facilities for the application
func Init(opts CmdOpts) LoggerHooker {
	var err error
	l := logger{logrus.New(), NewBrokerHook(context.Background(), opts.LogLevel)}
	l.AddHook(l.BrokerHook)
	l.Out = os.Stdout
	if opts.LogFile > "" {
		l.AddHook(lfshook.NewHook(getLogFileWriter(opts), getLogFileFormatter(opts)))
	}
	l.Level, err = logrus.ParseLevel(opts.LogLevel)
	if err != nil {
		l.Level = logrus.InfoLevel
	}
	l.SetFormatter(newFormatter(enableColors))
	l.SetBrokerFormatter(newFormatter(disableColors))
	l.SetReportCaller(l.Level > logrus.InfoLevel)
	return l
}

// PgxLogger is the struct used to log using pgx postgres driver
type PgxLogger struct {
	l Logger
}

// NewPgxLogger returns a new instance of PgxLogger
func NewPgxLogger(l Logger) *PgxLogger {
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
func WithLogger(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// FallbackLogger is an alias for the standard logger
var FallbackLogger = Init(CmdOpts{})

// GetLogger retrieves the current logger from the context. If no logger is
// available, the default logger is returned
func GetLogger(ctx context.Context) Logger {
	logger := ctx.Value(loggerKey{})
	if logger == nil {
		return FallbackLogger
	}
	return logger.(Logger)
}

func NewNoopLogger() Logger {
	l := logrus.New()
	l.SetLevel(logrus.PanicLevel) // Noop logger should not output anything
	return l
}
