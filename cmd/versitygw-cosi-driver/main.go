// Package main is the entry point for the versitygw COSI driver.
package main

import (
	"context"
	"flag"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	_ "github.com/KimMachineGun/automemlimit"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
	cosi "sigs.k8s.io/container-object-storage-interface-spec"

	"github.com/isac322/versitygw-cosi-driver/internal/config"
	"github.com/isac322/versitygw-cosi-driver/internal/driver"
	"github.com/isac322/versitygw-cosi-driver/internal/version"
	"github.com/isac322/versitygw-cosi-driver/internal/versitygw"
)

func main() {
	var cfg config.Config

	flag.StringVar(&cfg.Endpoint, "endpoint", "/var/lib/cosi/cosi.sock", "Path to the COSI Unix socket")
	flag.StringVar(&cfg.DriverName, "driver-name", envOrDefault("DRIVER_NAME", ""), "COSI driver name (required)")
	flag.StringVar(&cfg.S3Endpoint, "versitygw-s3-endpoint", envOrDefault("VERSITYGW_S3_ENDPOINT", "http://localhost:7070"), "versitygw S3 API endpoint URL")
	flag.StringVar(&cfg.AdminEndpoint, "versitygw-admin-endpoint", envOrDefault("VERSITYGW_ADMIN_ENDPOINT", "http://localhost:7071"), "versitygw Admin API endpoint URL")
	flag.StringVar(&cfg.AdminAccessKey, "admin-access", envOrDefault("VERSITYGW_ADMIN_ACCESS", ""), "versitygw admin access key")
	flag.StringVar(&cfg.AdminSecretKey, "admin-secret", envOrDefault("VERSITYGW_ADMIN_SECRET", ""), "versitygw admin secret key")
	flag.StringVar(&cfg.Region, "region", envOrDefault("VERSITYGW_REGION", ""), "S3 region")

	klog.InitFlags(nil)
	flag.Parse()

	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		klog.Fatalf("Invalid configuration: %v", err)
	}

	client := versitygw.NewClientWithRegion(cfg.S3Endpoint, cfg.AdminEndpoint, cfg.AdminAccessKey, cfg.AdminSecretKey, cfg.Region)

	// Remove existing socket file if present
	socketPath := strings.TrimPrefix(cfg.Endpoint, "unix://")
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		klog.Fatalf("Failed to remove existing socket: %v", err)
	}

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", socketPath)
	if err != nil {
		klog.Fatalf("Failed to listen on %s: %v", socketPath, err)
	}

	server := grpc.NewServer()
	cosi.RegisterIdentityServer(server, driver.NewIdentityServer(cfg.DriverName))
	cosi.RegisterProvisionerServer(server, driver.NewProvisionerServer(client, cfg.S3Endpoint, cfg.Region))

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		klog.InfoS("Received signal, shutting down", "signal", sig)
		server.GracefulStop()
	}()

	v := version.Get()
	klog.InfoS("Starting versitygw COSI driver",
		"version", v.Version, "go", v.GoVersion, "commit", v.Commit, "commitTime", v.CommitTime, "modified", v.Modified,
		"socket", socketPath, "s3Endpoint", cfg.S3Endpoint, "adminEndpoint", cfg.AdminEndpoint)
	if err := server.Serve(listener); err != nil {
		klog.Fatalf("Failed to serve: %v", err)
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
