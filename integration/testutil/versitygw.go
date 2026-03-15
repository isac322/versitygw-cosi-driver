// Package testutil provides helpers for integration tests.
package testutil

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// VersityGWInstance represents a running versitygw process for testing.
type VersityGWInstance struct {
	Endpoint      string // S3 API endpoint (http://127.0.0.1:<port>)
	AdminEndpoint string // Admin API endpoint (http://127.0.0.1:<port>)
	AccessKey     string
	SecretKey     string
	DataDir       string
	cmd           *exec.Cmd
}

// StartVersityGW launches an isolated versitygw instance for testing.
// Each call creates a unique temp directory, random ports, and unique credentials.
// The process and temp dir are automatically cleaned up via t.Cleanup().
func StartVersityGW(t *testing.T) *VersityGWInstance {
	t.Helper()

	versitygwBin, err := exec.LookPath("versitygw")
	if err != nil {
		t.Skip("versitygw binary not found in PATH; skipping integration test")
	}

	dataDir := t.TempDir()
	s3Port := getFreePort(t)
	adminPort := getFreePort(t)
	accessKey := fmt.Sprintf("admin-%d", s3Port)
	secretKey := fmt.Sprintf("secret-%d", s3Port)

	// Global options must come before the subcommand.
	cmd := exec.Command(
		versitygwBin,
		"--access", accessKey,
		"--secret", secretKey,
		"--port", fmt.Sprintf(":%d", s3Port),
		"--admin-port", fmt.Sprintf(":%d", adminPort),
		"--iam-dir", dataDir,
		"posix",
		dataDir,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start versitygw: %v", err)
	}

	inst := &VersityGWInstance{
		Endpoint:      fmt.Sprintf("http://127.0.0.1:%d", s3Port),
		AdminEndpoint: fmt.Sprintf("http://127.0.0.1:%d", adminPort),
		AccessKey:     accessKey,
		SecretKey:     secretKey,
		DataDir:       dataDir,
		cmd:           cmd,
	}

	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	waitForReady(t, inst)

	return inst
}

// getFreePort finds an available TCP port.
func getFreePort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	tcpAddr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("listener address is not *net.TCPAddr")
	}
	port := tcpAddr.Port
	l.Close()
	return port
}

// waitForReady polls versitygw until it responds or the timeout expires.
func waitForReady(t *testing.T, inst *VersityGWInstance) {
	t.Helper()

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(inst.AccessKey, inst.SecretKey, "")),
	)
	if err != nil {
		t.Fatalf("failed to load AWS config: %v", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(inst.Endpoint)
		o.UsePathStyle = true
	})

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		_, err := client.ListBuckets(ctx, &s3.ListBucketsInput{})
		cancel()
		if err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("versitygw did not become ready within 10 seconds at %s", inst.Endpoint)
}
