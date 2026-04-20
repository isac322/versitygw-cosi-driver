package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// validConfig returns a Config with all required fields set to valid values.
func validConfig() Config {
	return Config{
		DriverName:     "versitygw.cosi.dev",
		S3Endpoint:     "https://s3.example.com",
		AdminEndpoint:  "https://admin.example.com",
		AdminAccessKey: "AKID",
		AdminSecretKey: "SECRET",
		Region:         "us-east-1",
	}
}

func TestConfigValidation(t *testing.T) {
	t.Parallel()

	t.Run("TC-U-001_valid_config_with_all_required_fields", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig()
		err := cfg.Validate()
		require.NoError(t, err)
	})

	t.Run("TC-U-002_missing_s3_endpoint", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig()
		cfg.S3Endpoint = ""
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "s3-endpoint")
	})

	t.Run("TC-U-003_missing_admin_endpoint", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig()
		cfg.AdminEndpoint = ""
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "admin-endpoint")
	})

	t.Run("TC-U-004_missing_access_key", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig()
		cfg.AdminAccessKey = ""
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "admin-access")
	})

	t.Run("TC-U-005_missing_secret_key", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig()
		cfg.AdminSecretKey = ""
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "admin-secret")
	})

	t.Run("TC-U-006_invalid_endpoint_url_format", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig()
		cfg.S3Endpoint = "not-a-url"
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "s3-endpoint")
	})

	t.Run("TC-U-007_default_region_applied", func(t *testing.T) {
		t.Parallel()

		cfg := validConfig()
		cfg.Region = ""
		cfg.ApplyDefaults()
		require.Equal(t, DefaultRegion, cfg.Region)
	})

	t.Run("TC-U-008_driver_name_required", func(t *testing.T) {
		t.Parallel()

		// Since 0.3.0, driver name is a required field with no default value.
		// The test spec mentions "default driver name applied", but the current
		// design requires driver-name explicitly. This test verifies that omitting
		// driver-name produces an error, which is the actual behavior.
		cfg := validConfig()
		cfg.DriverName = ""
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "driver-name")
	})
}

// TestApplyDefaultsPreservesExplicitRegion verifies that an explicitly set region
// is not overwritten by ApplyDefaults.
func TestApplyDefaultsPreservesExplicitRegion(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Region = "eu-west-1"
	cfg.ApplyDefaults()
	require.Equal(t, "eu-west-1", cfg.Region)
}
