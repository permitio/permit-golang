package config

import (
	"github.com/permitio/permit-golang/pkg/log"
	"go.uber.org/zap"
)

type zapLoggerAdapter struct {
	zapLogger *zap.Logger
}

func (z zapLoggerAdapter) Debug(msg string, params ...interface{}) {
	zFields := make([]zap.Field, 0, len(params))
	for i := 0; i < len(params); i += 2 {
		zFields = append(zFields, zap.Any(params[i].(string), params[i+1]))
	}
	z.zapLogger.Debug(msg, zFields...)
}

func (z zapLoggerAdapter) Info(msg string, params ...interface{}) {
	zFields := make([]zap.Field, 0, len(params))
	for i := 0; i < len(params); i += 2 {
		zFields = append(zFields, zap.Any(params[i].(string), params[i+1]))
	}
	z.zapLogger.Info(msg, zFields...)
}

func (z zapLoggerAdapter) Warn(msg string, params ...interface{}) {
	zFields := make([]zap.Field, 0, len(params))
	for i := 0; i < len(params); i += 2 {
		zFields = append(zFields, zap.Any(params[i].(string), params[i+1]))
	}
	z.zapLogger.Warn(msg, zFields...)
}

func (z zapLoggerAdapter) Error(msg string, err error, params ...interface{}) {
	zFields := make([]zap.Field, 0, len(params)+1)
	zFields = append(zFields, zap.Error(err))
	for i := 0; i < len(params); i += 2 {
		zFields = append(zFields, zap.Any(params[i].(string), params[i+1]))
	}
	z.zapLogger.Error(msg, zFields...)
}

func newLoggerFromZap(zapLogger *zap.Logger) log.Logger {
	return &zapLoggerAdapter{zapLogger: zapLogger}
}
