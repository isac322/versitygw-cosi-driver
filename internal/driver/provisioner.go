package driver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/versity/versitygw/auth"
	"k8s.io/klog/v2"
	cosi "sigs.k8s.io/container-object-storage-interface-spec"

	"github.com/isac322/versitygw-cosi-driver/internal/versitygw"
)

// ProvisionerServer implements the COSI ProvisionerServer interface.
type ProvisionerServer struct {
	client     *versitygw.Client
	s3Endpoint string // S3 endpoint returned in credentials for bucket consumers
	region     string
}

// NewProvisionerServer creates a new ProvisionerServer.
// s3Endpoint is the S3 API URL that bucket consumers will use.
func NewProvisionerServer(client *versitygw.Client, s3Endpoint, region string) *ProvisionerServer {
	return &ProvisionerServer{
		client:     client,
		s3Endpoint: s3Endpoint,
		region:     region,
	}
}

func (s *ProvisionerServer) DriverCreateBucket(ctx context.Context, req *cosi.DriverCreateBucketRequest) (*cosi.DriverCreateBucketResponse, error) {
	klog.V(4).InfoS("DriverCreateBucket", "name", req.GetName())

	if err := s.client.CreateBucket(ctx, req.GetName()); err != nil {
		return nil, fmt.Errorf("create bucket: %w", err)
	}

	return &cosi.DriverCreateBucketResponse{
		BucketId: req.GetName(),
	}, nil
}

func (s *ProvisionerServer) DriverDeleteBucket(ctx context.Context, req *cosi.DriverDeleteBucketRequest) (*cosi.DriverDeleteBucketResponse, error) {
	klog.V(4).InfoS("DriverDeleteBucket", "bucketId", req.GetBucketId())

	if err := s.client.DeleteBucket(ctx, req.GetBucketId()); err != nil {
		return nil, fmt.Errorf("delete bucket: %w", err)
	}

	return &cosi.DriverDeleteBucketResponse{}, nil
}

func (s *ProvisionerServer) DriverGrantBucketAccess(ctx context.Context, req *cosi.DriverGrantBucketAccessRequest) (*cosi.DriverGrantBucketAccessResponse, error) {
	bucketID := req.GetBucketId()
	klog.V(4).InfoS("DriverGrantBucketAccess", "bucketId", bucketID, "name", req.GetName())

	accountName := fmt.Sprintf("ba-%s", uuid.New().String()[:8])
	secret, err := generateSecretKey()
	if err != nil {
		return nil, fmt.Errorf("generate secret key: %w", err)
	}

	// Create user on versitygw
	err = s.client.CreateUser(ctx, accountName, secret, string(auth.RoleUser))
	if err != nil && !errors.Is(err, auth.ErrUserExists) {
		return nil, fmt.Errorf("create user %q: %w", accountName, err)
	}

	// Grant access via bucket policy
	if err := s.client.PutBucketPolicy(ctx, bucketID, accountName); err != nil {
		// Best-effort cleanup: delete the user we just created
		_ = s.client.DeleteUser(ctx, accountName)
		return nil, fmt.Errorf("put bucket policy: %w", err)
	}

	return &cosi.DriverGrantBucketAccessResponse{
		AccountId: accountName,
		Credentials: map[string]*cosi.CredentialDetails{
			"s3": {
				Secrets: map[string]string{
					"accessKeyID":     accountName,
					"accessSecretKey": secret,
					"endpoint":        s.s3Endpoint,
					"region":          s.region,
				},
			},
		},
	}, nil
}

func (s *ProvisionerServer) DriverRevokeBucketAccess(ctx context.Context, req *cosi.DriverRevokeBucketAccessRequest) (*cosi.DriverRevokeBucketAccessResponse, error) {
	bucketID := req.GetBucketId()
	accountID := req.GetAccountId()
	klog.V(4).InfoS("DriverRevokeBucketAccess", "bucketId", bucketID, "accountId", accountID)

	// Remove bucket policy
	if err := s.client.DeleteBucketPolicy(ctx, bucketID); err != nil {
		return nil, fmt.Errorf("delete bucket policy: %w", err)
	}

	// Delete user (idempotent: ignore not-found)
	if err := s.client.DeleteUser(ctx, accountID); err != nil && !errors.Is(err, auth.ErrNoSuchUser) {
		return nil, fmt.Errorf("delete user %q: %w", accountID, err)
	}

	return &cosi.DriverRevokeBucketAccessResponse{}, nil
}

func generateSecretKey() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
