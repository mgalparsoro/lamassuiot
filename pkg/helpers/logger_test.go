package helpers

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/lamassuiot/lamassuiot/v2/pkg/config"
	"github.com/lamassuiot/lamassuiot/v2/pkg/models"
	"github.com/sirupsen/logrus"
)

func TestConfigureLoggerWithRequestID(t *testing.T) {
	// Test case 1: Logger level is not TraceLevel
	logger := logrus.NewEntry(logrus.New())
	logger.Level = logrus.InfoLevel
	ctx := context.Background()

	result := ConfigureLoggerWithRequestID(ctx, logger)

	// Verify that the returned logger is the same as the input logger
	if result != logger {
		t.Error("ConfigureLoggerWithRequestID returned a different logger when level is not TraceLevel")
	}

	logger = logrus.NewEntry(logrus.New())
	logger.Level = logrus.TraceLevel

	result = ConfigureLoggerWithRequestID(ctx, logger)

	// Verify that the returned logger is not the same as the input logger
	if result == logger {
		t.Error("ConfigureLoggerWithRequestID returned a different logger when level is not TraceLevel")
	}

	// Test case 2: Request ID exists in the context
	reqID := "12345"
	ctx = context.WithValue(ctx, HTTPRequestID, reqID)

	result = ConfigureLoggerWithRequestID(ctx, logger)

	// Verify that the returned logger has the correct request ID field
	if result.Data["req-id"] != reqID {
		t.Errorf("ConfigureLoggerWithRequestID returned logger with incorrect request ID field. Expected: %s, Got: %v", reqID, result.Data["req-id"])
	}

	// Test case 3: Request ID does not exist in the context
	ctx = context.Background()

	result = ConfigureLoggerWithRequestID(ctx, logger)

	// Verify that the returned logger has a generated request ID field
	if _, ok := result.Data["req-id"]; !ok {
		t.Error("ConfigureLoggerWithRequestID returned logger without request ID field")
	}

	// Verify that the generated request ID field starts with "internal."
	if reqID, ok := result.Data["req-id"].(string); ok {
		if !startsWith(reqID, "internal.") {
			t.Errorf("ConfigureLoggerWithRequestID returned logger with incorrect generated request ID field. Expected: %s, Got: %s", "internal.", reqID)
		}
	} else {
		t.Error("ConfigureLoggerWithRequestID returned logger with incorrect generated request ID field type")
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func TestConfigureContextWithRequest(t *testing.T) {
	ctx := context.Background()
	headers := http.Header{}
	headers.Set("x-request-id", "12345")
	headers.Set("x-lms-source", "test-source")
	headers.Set("x-ignored", "ignored")

	ctx = ConfigureContextWithRequest(ctx, headers)

	// Verify that the request ID is correctly set in the context
	reqID := ctx.Value(HTTPRequestID)
	if reqID != "12345" {
		t.Errorf("ConfigureContextWithRequest did not set the correct request ID in the context. Expected: %s, Got: %v", "12345", reqID)
	}

	// Verify that the source is correctly set in the context
	source := ctx.Value(models.ContextSourceKey)
	if source != "test-source" {
		t.Errorf("ConfigureContextWithRequest did not set the correct source in the context. Expected: %s, Got: %v", "test-source", source)
	}

	// Verify that the source is correctly set in the context
	ignored := ctx.Value("x-ignored")
	if ignored != nil {
		t.Errorf("ConfigureContextWithRequest should not have set the ignored header in the context. Got: %v", ignored)
	}
}

func TestConfigureLogger(t *testing.T) {
	// Test case 1: currentLevel is config.None
	currentLevel := config.None
	subsystem := "test-subsystem"

	logger := ConfigureLogger(currentLevel, subsystem)

	// Verify that the logger output is set to io.Discard
	if logger.Logger.Out != io.Discard {
		t.Error("ConfigureLogger did not set logger output to io.Discard when currentLevel is config.None")
	}

	// Test case 2: currentLevel is valid
	currentLevel = config.Info
	subsystem = "test-subsystem"

	logger = ConfigureLogger(currentLevel, subsystem)

	// Verify that the logger level is set correctly
	if logger.Logger.Level != logrus.InfoLevel {
		t.Errorf("ConfigureLogger did not set logger level correctly. Expected: %v, Got: %v", logrus.InfoLevel, logger.Logger.Level)
	}

	// Test case 3: currentLevel is invalid
	currentLevel = "invalid-level"
	subsystem = "test-subsystem"

	logger = ConfigureLogger(currentLevel, subsystem)

	// Verify that the logger level is set to the default level
	if logger.Logger.GetLevel() != logrus.GetLevel() {
		t.Errorf("ConfigureLogger did not set logger level to default when currentLevel is invalid. Expected: %v, Got: %v", logrus.GetLevel(), logger.Logger.GetLevel())
	}
}
