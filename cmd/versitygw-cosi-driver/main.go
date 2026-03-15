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

	"github.com/isac322/versitygw-cosi-driver/internal/driver"
	"github.com/isac322/versitygw-cosi-driver/internal/version"
	"github.com/isac322/versitygw-cosi-driver/internal/versitygw"
)

func main() {
	var (
		endpoint        string
		gwS3Endpoint    string
		gwAdminEndpoint string
		adminAccess     string
		adminSecret     string
		region          string
	)

	flag.StringVar(&endpoint, "endpoint", "/var/lib/cosi/cosi.sock", "Path to the COSI Unix socket")
	flag.StringVar(&gwS3Endpoint, "versitygw-s3-endpoint", envOrDefault("VERSITYGW_S3_ENDPOINT", "http://localhost:7070"), "versitygw S3 API endpoint URL")
	flag.StringVar(&gwAdminEndpoint, "versitygw-admin-endpoint", envOrDefault("VERSITYGW_ADMIN_ENDPOINT", "http://localhost:7071"), "versitygw Admin API endpoint URL")
	flag.StringVar(&adminAccess, "admin-access", envOrDefault("VERSITYGW_ADMIN_ACCESS", ""), "versitygw admin access key")
	flag.StringVar(&adminSecret, "admin-secret", envOrDefault("VERSITYGW_ADMIN_SECRET", ""), "versitygw admin secret key")
	flag.StringVar(&region, "region", envOrDefault("VERSITYGW_REGION", "us-east-1"), "S3 region")

	klog.InitFlags(nil)
	flag.Parse()

	if adminAccess == "" || adminSecret == "" {
		klog.Fatal("admin-access and admin-secret are required")
	}

	client := versitygw.NewClientWithRegion(gwS3Endpoint, gwAdminEndpoint, adminAccess, adminSecret, region)

	// Remove existing socket file if present
	socketPath := endpoint
	socketPath = strings.TrimPrefix(socketPath, "unix://")
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		klog.Fatalf("Failed to remove existing socket: %v", err)
	}

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", socketPath)
	if err != nil {
		klog.Fatalf("Failed to listen on %s: %v", socketPath, err)
	}

	server := grpc.NewServer()
	cosi.RegisterIdentityServer(server, &driver.IdentityServer{})
	cosi.RegisterProvisionerServer(server, driver.NewProvisionerServer(client, gwS3Endpoint, region))

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
		"socket", socketPath, "s3Endpoint", gwS3Endpoint, "adminEndpoint", gwAdminEndpoint)
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
