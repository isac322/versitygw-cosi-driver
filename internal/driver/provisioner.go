package driver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	smithy "github.com/aws/smithy-go"
	"github.com/google/uuid"
	"github.com/versity/versitygw/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func mapToGRPCError(err error, msg string) error {
	if errors.Is(err, auth.ErrUserExists) {
		return status.Errorf(codes.AlreadyExists, "%s: %v", msg, err)
	}
	if errors.Is(err, auth.ErrNoSuchUser) {
		return status.Errorf(codes.NotFound, "%s: %v", msg, err)
	}

	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "BucketAlreadyExists", "BucketAlreadyOwnedByYou":
			return status.Errorf(codes.AlreadyExists, "%s: %v", msg, err)
		case "NoSuchBucket":
			return status.Errorf(codes.NotFound, "%s: %v", msg, err)
		case "InvalidBucketName":
			return status.Errorf(codes.InvalidArgument, "%s: %v", msg, err)
		case "BucketNotEmpty":
			return status.Errorf(codes.FailedPrecondition, "%s: %v", msg, err)
		}
	}

	if errors.Is(err, context.Canceled) {
		return status.Errorf(codes.Canceled, "%s: %v", msg, err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Errorf(codes.DeadlineExceeded, "%s: %v", msg, err)
	}

	return status.Errorf(codes.Internal, "%s: %v", msg, err)
}

func validateBucketName(name string) error {
	if len(name) < 3 || len(name) > 63 {
		return status.Errorf(codes.InvalidArgument, "bucket name must be between 3 and 63 characters, got %d", len(name))
	}
	for _, c := range name {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return status.Errorf(codes.InvalidArgument, "bucket name contains invalid character %q", c)
		}
	}
	first, last := name[0], name[len(name)-1]
	if (first < 'a' || first > 'z') && (first < '0' || first > '9') {
		return status.Errorf(codes.InvalidArgument, "bucket name must start with a lowercase letter or number")
	}
	if (last < 'a' || last > 'z') && (last < '0' || last > '9') {
		return status.Errorf(codes.InvalidArgument, "bucket name must end with a lowercase letter or number")
	}
	return nil
}

// DriverCreateBucket creates a new bucket on versitygw.
func (s *ProvisionerServer) DriverCreateBucket(ctx context.Context, req *cosi.DriverCreateBucketRequest) (*cosi.DriverCreateBucketResponse, error) {
	klog.V(4).InfoS("DriverCreateBucket", "name", req.GetName())

	if err := validateBucketName(req.GetName()); err != nil {
		return nil, err
	}

	if err := s.client.CreateBucket(ctx, req.GetName()); err != nil {
		return nil, mapToGRPCError(err, "create bucket")
	}

	return &cosi.DriverCreateBucketResponse{
		BucketId: req.GetName(),
	}, nil
}

// DriverDeleteBucket deletes a bucket from versitygw.
func (s *ProvisionerServer) DriverDeleteBucket(ctx context.Context, req *cosi.DriverDeleteBucketRequest) (*cosi.DriverDeleteBucketResponse, error) {
	klog.V(4).InfoS("DriverDeleteBucket", "bucketId", req.GetBucketId())

	if err := s.client.DeleteBucket(ctx, req.GetBucketId()); err != nil {
		return nil, mapToGRPCError(err, "delete bucket")
	}

	return &cosi.DriverDeleteBucketResponse{}, nil
}

// DriverGrantBucketAccess creates a user and grants it access to the bucket.
func (s *ProvisionerServer) DriverGrantBucketAccess(ctx context.Context, req *cosi.DriverGrantBucketAccessRequest) (*cosi.DriverGrantBucketAccessResponse, error) {
	bucketID := req.GetBucketId()
	klog.V(4).InfoS("DriverGrantBucketAccess", "bucketId", bucketID, "name", req.GetName())

	accountName := "ba-" + uuid.New().String()[:8]
	secret, err := generateSecretKey()
	if err != nil {
		return nil, mapToGRPCError(err, "generate secret key")
	}

	// Create user on versitygw
	err = s.client.CreateUser(ctx, accountName, secret, string(auth.RoleUser))
	if err != nil && !errors.Is(err, auth.ErrUserExists) {
		return nil, mapToGRPCError(err, fmt.Sprintf("create user %q", accountName))
	}

	// Grant access via bucket policy
	if err := s.client.PutBucketPolicy(ctx, bucketID, accountName); err != nil {
		// Best-effort cleanup: delete the user we just created
		_ = s.client.DeleteUser(ctx, accountName)
		return nil, mapToGRPCError(err, "put bucket policy")
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

// DriverRevokeBucketAccess removes the user's principal from the bucket policy and deletes the user.
func (s *ProvisionerServer) DriverRevokeBucketAccess(ctx context.Context, req *cosi.DriverRevokeBucketAccessRequest) (*cosi.DriverRevokeBucketAccessResponse, error) {
	bucketID := req.GetBucketId()
	accountID := req.GetAccountId()
	klog.V(4).InfoS("DriverRevokeBucketAccess", "bucketId", bucketID, "accountId", accountID)

	// Remove this user's principal from the bucket policy
	if err := s.client.RemoveBucketPolicyPrincipal(ctx, bucketID, accountID); err != nil {
		return nil, mapToGRPCError(err, "remove bucket policy principal")
	}

	// Delete user (idempotent: ignore not-found)
	if err := s.client.DeleteUser(ctx, accountID); err != nil && !errors.Is(err, auth.ErrNoSuchUser) {
		return nil, mapToGRPCError(err, fmt.Sprintf("delete user %q", accountID))
	}

	return &cosi.DriverRevokeBucketAccessResponse{}, nil
}

func generateSecretKey() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}
