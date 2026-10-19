package worker

import (
	"fmt"
	"log/slog"
	"os"
)

// Logger adapts asynq's logging interface to the application's slog logger.
type Logger struct{ Log *slog.Logger }

func (l Logger) Debug(args ...any) { l.Log.Debug(fmt.Sprint(args...)) }
func (l Logger) Info(args ...any)  { l.Log.Info(fmt.Sprint(args...)) }
func (l Logger) Warn(args ...any)  { l.Log.Warn(fmt.Sprint(args...)) }
func (l Logger) Error(args ...any) { l.Log.Error(fmt.Sprint(args...)) }
func (l Logger) Fatal(args ...any) { l.Log.Error(fmt.Sprint(args...)); os.Exit(1) }
