package main

import (
	"fmt"
	"log/slog"
	"time"
)

// Defer the returned function directly so panics are logged and propagated to
// the template engine without changing its error handling.
func logOperation(name string, fields ...interface{}) func() {
	start := time.Now()
	fields = append([]interface{}{"operation", name}, fields...)
	slog.Info("Operation started", fields...)
	return func() {
		fields = append(fields, "duration", time.Since(start))
		if failure := recover(); failure != nil {
			slog.Error("Operation failed", append(fields, "panic_type", fmt.Sprintf("%T", failure))...)
			panic(failure)
		}
		slog.Info("Operation completed", fields...)
	}
}
