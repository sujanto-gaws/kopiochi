package logger

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func Init(level, format string) zerolog.Logger {
	var lvl zerolog.Level
	switch level {
	case "debug":
		lvl = zerolog.DebugLevel
	case "info":
		lvl = zerolog.InfoLevel
	case "warn":
		lvl = zerolog.WarnLevel
	case "error":
		lvl = zerolog.ErrorLevel
	default:
		lvl = zerolog.InfoLevel
	}

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	if format == "console" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	} else {
		log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	}
	zerolog.SetGlobalLevel(lvl)
	return log.Logger
}
