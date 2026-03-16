// Package versitygw provides a client for the versitygw S3 and Admin APIs.
package versitygw

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"
	"github.com/versity/versitygw/auth"
)

// Client communicates with versitygw via S3 API and Admin API.
type Client struct {
	s3Client      *s3.Client
	adminEndpoint string
	creds         aws.Credentials
	httpClient    *http.Client
	region        string
}

// BucketInfo holds bucket metadata returned by the Admin API.
type BucketInfo struct {
	Name  string `xml:"Name"`
	Owner string `xml:"Owner"`
}

// listBucketsResult is the XML response from the admin list-buckets endpoint.
type listBucketsResult struct {
	XMLName xml.Name     `xml:"ListBucketsResult"`
	Buckets []BucketInfo `xml:"Buckets"`
}

// apiErrorResponse represents an S3-style XML error response.
type apiErrorResponse struct {
	XMLName xml.Name `xml:"Error"`
	Code    string   `xml:"Code"`
	Message string   `xml:"Message"`
}

// bucketPolicy is a JSON-serializable bucket policy structure.
type bucketPolicy struct {
	Version   string             `json:"Version"`
	Statement []bucketPolicyStmt `json:"Statement"`
}

type bucketPolicyStmt struct {
	Effect    string          `json:"Effect"`
	Principal policyPrincipal `json:"Principal"`
	Action    string          `json:"Action"`
	Resource  []string        `json:"Resource"`
}

// policyPrincipal is a map of principal type to principal identifiers.
// S3 may represent a single principal as a string or multiple as an array;
// UnmarshalJSON normalizes both forms into []string.
type policyPrincipal map[string][]string

func (p *policyPrincipal) UnmarshalJSON(data []byte) error {
	var multi map[string][]string
	if json.Unmarshal(data, &multi) == nil {
		*p = multi
		return nil
	}
	var single map[string]string
	if json.Unmarshal(data, &single) == nil {
		result := make(policyPrincipal, len(single))
		for k, v := range single {
			result[k] = []string{v}
		}
		*p = result
		return nil
	}
	return fmt.Errorf("cannot unmarshal principal: %s", string(data))
}

// NewClient creates a new versitygw client.
// s3Endpoint is the S3 API endpoint, adminEndpoint is the Admin API endpoint.
func NewClient(s3Endpoint, adminEndpoint, accessKey, secretKey string) *Client {
	return NewClientWithRegion(s3Endpoint, adminEndpoint, accessKey, secretKey, "us-east-1")
}

// NewClientWithRegion creates a new versitygw client with a specified region.
func NewClientWithRegion(s3Endpoint, adminEndpoint, accessKey, secretKey, region string) *Client {
	creds := aws.Credentials{
		AccessKeyID:     accessKey,
		SecretAccessKey: secretKey,
	}

	cfg, _ := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(s3Endpoint)
		o.UsePathStyle = true
	})

	return &Client{
		s3Client:      s3Client,
		adminEndpoint: strings.TrimRight(adminEndpoint, "/"),
		creds:         creds,
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		region:        region,
	}
}

// CreateBucket creates a new S3 bucket.
func (c *Client) CreateBucket(ctx context.Context, name string) error {
	_, err := c.s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(name),
	})
	if err != nil {
		// Idempotent: bucket already owned by us is OK
		var owned *s3types.BucketAlreadyOwnedByYou
		var exists *s3types.BucketAlreadyExists
		if errors.As(err, &owned) || errors.As(err, &exists) {
			return nil
		}
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			code := apiErr.ErrorCode()
			if code == "BucketAlreadyOwnedByYou" || code == "BucketAlreadyExists" {
				return nil
			}
		}
		return fmt.Errorf("create bucket %q: %w", name, err)
	}
	return nil
}

// DeleteBucket deletes an S3 bucket.
func (c *Client) DeleteBucket(ctx context.Context, name string) error {
	_, err := c.s3Client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(name),
	})
	if err != nil {
		// Check both typed error and generic API error code
		var nsk *s3types.NoSuchBucket
		if errors.As(err, &nsk) {
			return nil
		}
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchBucket" {
			return nil
		}
		return fmt.Errorf("delete bucket %q: %w", name, err)
	}
	return nil
}

// GetBucketPolicy retrieves and parses the bucket policy.
// Returns nil, nil if no policy exists.
func (c *Client) GetBucketPolicy(ctx context.Context, bucket string) (*bucketPolicy, error) {
	resp, err := c.s3Client.GetBucketPolicy(ctx, &s3.GetBucketPolicyInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchBucketPolicy" {
			return nil, nil
		}
		return nil, fmt.Errorf("get bucket policy for %q: %w", bucket, err)
	}

	var policy bucketPolicy
	if err := json.Unmarshal([]byte(aws.ToString(resp.Policy)), &policy); err != nil {
		return nil, fmt.Errorf("unmarshal bucket policy for %q: %w", bucket, err)
	}
	return &policy, nil
}

// PutBucketPolicy grants the specified principal full S3 access to the bucket.
// If a policy already exists, the principal is merged into the existing statement.
func (c *Client) PutBucketPolicy(ctx context.Context, bucket, principal string) error {
	resource := []string{
		"arn:aws:s3:::" + bucket,
		fmt.Sprintf("arn:aws:s3:::%s/*", bucket),
	}

	existing, err := c.GetBucketPolicy(ctx, bucket)
	if err != nil {
		return err
	}

	if existing != nil {
		merged := false
		for i, stmt := range existing.Statement {
			if stmt.Effect == "Allow" && stmt.Action == "s3:*" && slices.Equal(stmt.Resource, resource) {
				if !slices.Contains(existing.Statement[i].Principal["AWS"], principal) {
					existing.Statement[i].Principal["AWS"] = append(existing.Statement[i].Principal["AWS"], principal)
				}
				merged = true
				break
			}
		}
		if !merged {
			existing.Statement = append(existing.Statement, bucketPolicyStmt{
				Effect:    "Allow",
				Principal: policyPrincipal{"AWS": {principal}},
				Action:    "s3:*",
				Resource:  resource,
			})
		}
	} else {
		existing = &bucketPolicy{
			Version: "2012-10-17",
			Statement: []bucketPolicyStmt{
				{
					Effect:    "Allow",
					Principal: policyPrincipal{"AWS": {principal}},
					Action:    "s3:*",
					Resource:  resource,
				},
			},
		}
	}

	policyJSON, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshal bucket policy: %w", err)
	}

	_, err = c.s3Client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
		Bucket: aws.String(bucket),
		Policy: aws.String(string(policyJSON)),
	})
	if err != nil {
		return fmt.Errorf("put bucket policy for %q: %w", bucket, err)
	}
	return nil
}

// DeleteBucketPolicy removes the bucket policy.
func (c *Client) DeleteBucketPolicy(ctx context.Context, bucket string) error {
	_, err := c.s3Client.DeleteBucketPolicy(ctx, &s3.DeleteBucketPolicyInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return fmt.Errorf("delete bucket policy for %q: %w", bucket, err)
	}
	return nil
}

// RemoveBucketPolicyPrincipal removes a single principal from the bucket policy.
// If no principals remain, the entire policy is deleted.
// Returns nil if no policy exists (idempotent).
func (c *Client) RemoveBucketPolicyPrincipal(ctx context.Context, bucket, principal string) error {
	existing, err := c.GetBucketPolicy(ctx, bucket)
	if err != nil {
		return err
	}
	if existing == nil {
		return nil
	}

	var remaining []bucketPolicyStmt
	for _, stmt := range existing.Statement {
		principals := stmt.Principal["AWS"]
		filtered := slices.DeleteFunc(slices.Clone(principals), func(p string) bool {
			return p == principal
		})
		if len(filtered) > 0 {
			stmt.Principal["AWS"] = filtered
			remaining = append(remaining, stmt)
		}
	}

	if len(remaining) == 0 {
		return c.DeleteBucketPolicy(ctx, bucket)
	}

	existing.Statement = remaining
	policyJSON, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshal bucket policy: %w", err)
	}

	_, err = c.s3Client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
		Bucket: aws.String(bucket),
		Policy: aws.String(string(policyJSON)),
	})
	if err != nil {
		return fmt.Errorf("update bucket policy for %q: %w", bucket, err)
	}
	return nil
}

// CreateUser creates a new user via the versitygw Admin API.
func (c *Client) CreateUser(ctx context.Context, access, secret, role string) error {
	account := auth.Account{
		Access: access,
		Secret: secret,
		Role:   auth.Role(role),
	}

	body, err := xml.Marshal(account)
	if err != nil {
		return fmt.Errorf("marshal account: %w", err)
	}

	resp, err := c.adminRequest(ctx, "/create-user", nil, body)
	if err != nil {
		return fmt.Errorf("create user %q: %w", access, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return auth.ErrUserExists
	}
	if resp.StatusCode != http.StatusCreated {
		return c.parseAdminError(resp, "create user")
	}
	return nil
}

// DeleteUser deletes a user via the versitygw Admin API.
// Returns nil if the user does not exist (idempotent).
func (c *Client) DeleteUser(ctx context.Context, access string) error {
	params := map[string]string{"access": access}

	resp, err := c.adminRequest(ctx, "/delete-user", params, nil)
	if err != nil {
		return fmt.Errorf("delete user %q: %w", access, err)
	}
	defer resp.Body.Close()

	// versitygw returns 200 OK even for non-existent users (idempotent behavior).
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusOK {
		return nil
	}
	return c.parseAdminError(resp, "delete user")
}

// ListUsers lists all users via the versitygw Admin API.
func (c *Client) ListUsers(ctx context.Context) ([]auth.Account, error) {
	resp, err := c.adminRequest(ctx, "/list-users", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseAdminError(resp, "list users")
	}

	var result auth.ListUserAccountsResult
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode list users response: %w", err)
	}
	return result.Accounts, nil
}

// ChangeBucketOwner changes the owner of a bucket via the versitygw Admin API.
func (c *Client) ChangeBucketOwner(ctx context.Context, bucket, owner string) error {
	params := map[string]string{
		"bucket": bucket,
		"owner":  owner,
	}

	resp, err := c.adminRequest(ctx, "/change-bucket-owner", params, nil)
	if err != nil {
		return fmt.Errorf("change bucket owner: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.parseAdminError(resp, "change bucket owner")
	}
	return nil
}

// ListBuckets lists all buckets with owner info via the versitygw Admin API.
func (c *Client) ListBuckets(ctx context.Context) ([]BucketInfo, error) {
	resp, err := c.adminRequest(ctx, "/list-buckets", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseAdminError(resp, "list buckets")
	}

	var result listBucketsResult
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode list buckets response: %w", err)
	}
	return result.Buckets, nil
}

// adminRequest sends a PATCH request to the versitygw Admin API with SigV4 signing.
func (c *Client) adminRequest(ctx context.Context, path string, queryParams map[string]string, body []byte) (*http.Response, error) {
	url := c.adminEndpoint + path
	if len(queryParams) > 0 {
		parts := make([]string, 0, len(queryParams))
		for k, v := range queryParams {
			parts = append(parts, k+"="+v)
		}
		url += "?" + strings.Join(parts, "&")
	}

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/xml")
	}

	// Compute payload hash
	var payloadHash string
	if body != nil {
		h := sha256.Sum256(body)
		payloadHash = hex.EncodeToString(h[:])
	} else {
		h := sha256.Sum256([]byte{})
		payloadHash = hex.EncodeToString(h[:])
	}
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	// Sign with SigV4
	signer := v4.NewSigner()
	if err := signer.SignHTTP(ctx, c.creds, req, payloadHash, "s3", c.region, time.Now()); err != nil {
		return nil, fmt.Errorf("sign request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	return resp, nil
}

// parseAdminError reads an XML error response from the Admin API.
func (c *Client) parseAdminError(resp *http.Response, operation string) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s: status %d, failed to read body: %w", operation, resp.StatusCode, err)
	}

	var apiErr apiErrorResponse
	if xml.Unmarshal(body, &apiErr) == nil && apiErr.Code != "" {
		return fmt.Errorf("%s: %s: %s", operation, apiErr.Code, apiErr.Message)
	}

	return fmt.Errorf("%s: unexpected status %d: %s", operation, resp.StatusCode, string(body))
}
