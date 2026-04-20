// Package config provides configuration parsing and validation for the COSI driver.
package config

import (
	"errors"
	"net/url"
)

// DefaultRegion is the default AWS region used when none is specified.
const DefaultRegion = "us-east-1"

// Config holds the driver configuration parsed from flags and environment variables.
type Config struct {
	Endpoint       string
	DriverName     string
	S3Endpoint     string
	AdminEndpoint  string
	AdminAccessKey string
	AdminSecretKey string
	Region         string
}

// ApplyDefaults fills in default values for optional fields.
func (c *Config) ApplyDefaults() {
	if c.Region == "" {
		c.Region = DefaultRegion
	}
}

// Validate checks that all required configuration fields are set.
// It returns an error describing the first missing required field.
func (c *Config) Validate() error {
	if c.DriverName == "" {
		return errors.New("driver-name is required")
	}
	if c.S3Endpoint == "" {
		return errors.New("versitygw-s3-endpoint is required")
	}
	if u, err := url.Parse(c.S3Endpoint); err != nil || u.Scheme == "" || u.Host == "" {
		return errors.New("versitygw-s3-endpoint must be a valid URL with scheme and host")
	}
	if c.AdminEndpoint == "" {
		return errors.New("versitygw-admin-endpoint is required")
	}
	if u, err := url.Parse(c.AdminEndpoint); err != nil || u.Scheme == "" || u.Host == "" {
		return errors.New("versitygw-admin-endpoint must be a valid URL with scheme and host")
	}
	if c.AdminAccessKey == "" {
		return errors.New("admin-access is required")
	}
	if c.AdminSecretKey == "" {
		return errors.New("admin-secret is required")
	}
	return nil
}
