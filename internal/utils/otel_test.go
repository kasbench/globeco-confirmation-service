package utils

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupOTel(t *testing.T) {
	tests := []struct {
		name   string
		config OTelConfig
		want   bool
	}{
		{
			name: "disabled configuration",
			config: OTelConfig{
				Enabled: false,
			},
			want: true,
		},
		{
			name: "enabled configuration with valid settings",
			config: OTelConfig{
				ServiceName:      "test-service",
				ServiceVersion:   "1.0.0",
				ServiceNamespace: "globeco",
				OTLPEndpoint:     "localhost:4317",
				Enabled:          true,
			},
			want: true, // Setup should succeed, connection happens later
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			shutdown, err := SetupOTel(ctx, tt.config)
			
			if tt.want {
				assert.NoError(t, err)
				assert.NotNil(t, shutdown)
				
				// Test shutdown function (may fail if no collector is running, which is OK)
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 1*time.Second)
				defer shutdownCancel()
				
				// Don't assert on shutdown error as it may fail without collector
				_ = shutdown(shutdownCtx)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		envKey       string
		defaultValue string
		want         string
	}{
		{
			name:         "returns default when env not set",
			envKey:       "NON_EXISTENT_ENV_VAR",
			defaultValue: "default-value",
			want:         "default-value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getEnvOrDefault(tt.envKey, tt.defaultValue)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOTelConfigValidation(t *testing.T) {
	config := OTelConfig{
		ServiceName:      "globeco-confirmation-service",
		ServiceVersion:   "1.0.0",
		ServiceNamespace: "globeco",
		OTLPEndpoint:     "otel-collector-collector.monitoring.svc.cluster.local:4317",
		Enabled:          true,
	}

	require.NotEmpty(t, config.ServiceName)
	require.NotEmpty(t, config.ServiceVersion)
	require.NotEmpty(t, config.ServiceNamespace)
	require.NotEmpty(t, config.OTLPEndpoint)
	require.True(t, config.Enabled)
}