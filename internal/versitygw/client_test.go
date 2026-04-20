package versitygw

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"
	"github.com/versity/versitygw/auth"
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	c := NewClient("http://localhost:7070", "http://localhost:7071", "access", "secret")

	require.NotNil(t, c)
	require.Equal(t, "http://localhost:7071", c.adminEndpoint)
	require.Equal(t, "us-east-1", c.region)
	require.Equal(t, "access", c.creds.AccessKeyID)
	require.Equal(t, "secret", c.creds.SecretAccessKey)
	require.NotNil(t, c.s3Client)
	require.NotNil(t, c.httpClient)
}

func TestNewClientWithRegion(t *testing.T) {
	t.Parallel()

	c := NewClientWithRegion("http://localhost:7070", "http://localhost:7071", "access", "secret", "ap-northeast-2")

	require.NotNil(t, c)
	require.Equal(t, "ap-northeast-2", c.region)
	require.Equal(t, "http://localhost:7071", c.adminEndpoint)
	require.Equal(t, "access", c.creds.AccessKeyID)
	require.Equal(t, "secret", c.creds.SecretAccessKey)
}

func TestNewClientWithRegion_TrimsTrailingSlash(t *testing.T) {
	t.Parallel()

	c := NewClientWithRegion("http://localhost:7070", "http://localhost:7071/", "access", "secret", "us-east-1")

	require.Equal(t, "http://localhost:7071", c.adminEndpoint)
}

func TestPolicyPrincipalUnmarshalJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    policyPrincipal
		wantErr bool
	}{
		{
			name:  "array_single",
			input: `{"AWS": ["user1"]}`,
			want:  policyPrincipal{"AWS": {"user1"}},
		},
		{
			name:  "array_multiple",
			input: `{"AWS": ["user1", "user2", "user3"]}`,
			want:  policyPrincipal{"AWS": {"user1", "user2", "user3"}},
		},
		{
			name:  "string_single",
			input: `{"AWS": "user1"}`,
			want:  policyPrincipal{"AWS": {"user1"}},
		},
		{
			name:  "string_arn",
			input: `{"AWS": "arn:aws:iam::123456789012:user/Alice"}`,
			want:  policyPrincipal{"AWS": {"arn:aws:iam::123456789012:user/Alice"}},
		},
		{
			name:  "multiple_types_array",
			input: `{"AWS": ["user1"], "Service": ["s3.amazonaws.com"]}`,
			want:  policyPrincipal{"AWS": {"user1"}, "Service": {"s3.amazonaws.com"}},
		},
		{
			name:  "multiple_types_string",
			input: `{"AWS": "user1", "Service": "s3.amazonaws.com"}`,
			want:  policyPrincipal{"AWS": {"user1"}, "Service": {"s3.amazonaws.com"}},
		},
		{
			name:  "empty_object",
			input: `{}`,
			want:  policyPrincipal{},
		},
		{
			name:  "empty_array",
			input: `{"AWS": []}`,
			want:  policyPrincipal{"AWS": {}},
		},
		{
			name:    "plain_string",
			input:   `"*"`,
			wantErr: true,
		},
		{
			name:    "number",
			input:   `123`,
			wantErr: true,
		},
		{
			name:    "array_of_numbers",
			input:   `[1, 2, 3]`,
			wantErr: true,
		},
		{
			name:    "nested_object",
			input:   `{"AWS": {"nested": "value"}}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var p policyPrincipal
			err := json.Unmarshal([]byte(tt.input), &p)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, p)
		})
	}
}

func TestPolicyPrincipalMarshalJSON(t *testing.T) {
	t.Parallel()

	p := policyPrincipal{"AWS": {"user1", "user2"}}
	data, err := json.Marshal(p)
	require.NoError(t, err)

	var roundtrip policyPrincipal
	err = json.Unmarshal(data, &roundtrip)
	require.NoError(t, err)
	require.Equal(t, p, roundtrip)
}

func TestBucketPolicyStmtRoundtrip(t *testing.T) {
	t.Parallel()

	stmt := BucketPolicyStmt{
		Effect:    "Allow",
		Principal: policyPrincipal{"AWS": {"user1", "user2"}},
		Action:    "s3:*",
		Resource:  []string{"arn:aws:s3:::bucket", "arn:aws:s3:::bucket/*"},
	}

	data, err := json.Marshal(stmt)
	require.NoError(t, err)

	var decoded BucketPolicyStmt
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Equal(t, stmt, decoded)
}

func TestBucketPolicyStmtUnmarshalSinglePrincipal(t *testing.T) {
	t.Parallel()

	// S3 may return a single principal as a string instead of an array
	input := `{"Effect":"Allow","Principal":{"AWS":"user1"},"Action":"s3:*","Resource":["arn:aws:s3:::bucket"]}`

	var stmt BucketPolicyStmt
	err := json.Unmarshal([]byte(input), &stmt)
	require.NoError(t, err)
	require.Equal(t, policyPrincipal{"AWS": {"user1"}}, stmt.Principal)
}

// TestBucketPolicyConstruction tests policy construction logic.
func TestBucketPolicyConstruction(t *testing.T) {
	t.Parallel()

	// Helper: build a new bucket policy as PutBucketPolicy does for a new bucket.
	newBucketPolicy := func(bucket, principal string) *BucketPolicy {
		return &BucketPolicy{
			Version: "2012-10-17",
			Statement: []BucketPolicyStmt{
				{
					Effect:    "Allow",
					Principal: policyPrincipal{"AWS": {principal}},
					Action:    "s3:*",
					Resource: []string{
						"arn:aws:s3:::" + bucket,
						fmt.Sprintf("arn:aws:s3:::%s/*", bucket),
					},
				},
			},
		}
	}

	// TC-U-040: New policy with read-write actions
	// The driver uses "s3:*" which covers all S3 actions including read-write.
	t.Run("TC-U-040_new_policy_with_readwrite_actions", func(t *testing.T) {
		t.Parallel()

		policy := newBucketPolicy("test-bucket", "AKID001")
		require.Equal(t, "2012-10-17", policy.Version)
		require.Len(t, policy.Statement, 1)

		stmt := policy.Statement[0]
		require.Equal(t, "Allow", stmt.Effect)
		require.Contains(t, stmt.Principal["AWS"], "AKID001")
		// The driver grants "s3:*" which includes all operations (read + write)
		require.Equal(t, "s3:*", stmt.Action)
		require.Contains(t, stmt.Resource, "arn:aws:s3:::test-bucket")
		require.Contains(t, stmt.Resource, "arn:aws:s3:::test-bucket/*")
	})

	// TC-U-041: Read-only actions subset check
	// The driver always uses "s3:*" for simplicity; this test documents that behavior.
	t.Run("TC-U-041_policy_action_covers_readonly", func(t *testing.T) {
		t.Parallel()

		policy := newBucketPolicy("test-bucket", "AKID001")
		// "s3:*" is a wildcard that matches s3:GetObject and s3:ListBucket
		require.Equal(t, "s3:*", policy.Statement[0].Action)
	})

	// TC-U-042: Write-only actions subset check
	// The driver always uses "s3:*" for simplicity; this test documents that behavior.
	t.Run("TC-U-042_policy_action_covers_writeonly", func(t *testing.T) {
		t.Parallel()

		policy := newBucketPolicy("test-bucket", "AKID001")
		// "s3:*" is a wildcard that matches s3:PutObject
		require.Equal(t, "s3:*", policy.Statement[0].Action)
	})

	// TC-U-043: Add principal to existing policy (merge)
	t.Run("TC-U-043_add_principal_to_existing_policy", func(t *testing.T) {
		t.Parallel()

		policy := newBucketPolicy("test-bucket", "AKID001")
		resource := policy.Statement[0].Resource

		// Simulate PutBucketPolicy merge logic
		merged := false
		for i, stmt := range policy.Statement {
			if stmt.Effect == "Allow" && stmt.Action == "s3:*" {
				if !slices.Contains(policy.Statement[i].Principal["AWS"], "AKID002") {
					policy.Statement[i].Principal["AWS"] = append(policy.Statement[i].Principal["AWS"], "AKID002")
				}
				merged = true
				break
			}
		}
		require.True(t, merged)
		require.Len(t, policy.Statement, 1, "should merge into existing statement, not add a new one")
		require.Contains(t, policy.Statement[0].Principal["AWS"], "AKID001")
		require.Contains(t, policy.Statement[0].Principal["AWS"], "AKID002")
		require.Equal(t, resource, policy.Statement[0].Resource)
	})

	// TC-U-044: Remove principal from multi-principal policy
	t.Run("TC-U-044_remove_principal_from_multi_principal_policy", func(t *testing.T) {
		t.Parallel()

		policy := newBucketPolicy("test-bucket", "AKID001")
		policy.Statement[0].Principal["AWS"] = append(policy.Statement[0].Principal["AWS"], "AKID002")

		// Simulate RemoveBucketPolicyPrincipal logic
		var remaining []BucketPolicyStmt
		for _, stmt := range policy.Statement {
			principals := stmt.Principal["AWS"]
			filtered := slices.DeleteFunc(slices.Clone(principals), func(p string) bool {
				return p == "AKID001"
			})
			if len(filtered) > 0 {
				stmt.Principal["AWS"] = filtered
				remaining = append(remaining, stmt)
			}
		}

		require.Len(t, remaining, 1)
		require.Equal(t, []string{"AKID002"}, remaining[0].Principal["AWS"])
	})

	// TC-U-045: Remove last principal from policy
	t.Run("TC-U-045_remove_last_principal_from_policy", func(t *testing.T) {
		t.Parallel()

		policy := newBucketPolicy("test-bucket", "AKID001")

		// Simulate RemoveBucketPolicyPrincipal logic
		var remaining []BucketPolicyStmt
		for _, stmt := range policy.Statement {
			principals := stmt.Principal["AWS"]
			filtered := slices.DeleteFunc(slices.Clone(principals), func(p string) bool {
				return p == "AKID001"
			})
			if len(filtered) > 0 {
				stmt.Principal["AWS"] = filtered
				remaining = append(remaining, stmt)
			}
		}

		// No remaining statements means the entire policy should be deleted
		require.Empty(t, remaining)
	})

	// TC-U-046: Principal uses access key ID, not ARN
	t.Run("TC-U-046_principal_uses_access_key_id_not_arn", func(t *testing.T) {
		t.Parallel()

		policy := newBucketPolicy("test-bucket", "AKIAIOSFODNN7EXAMPLE")
		principals := policy.Statement[0].Principal["AWS"]
		require.Len(t, principals, 1)
		// Must be raw access key ID, NOT an ARN
		require.Equal(t, "AKIAIOSFODNN7EXAMPLE", principals[0])
		require.NotContains(t, principals[0], "arn:aws:iam::")
	})

	// TC-U-047: Resource format is correct
	t.Run("TC-U-047_resource_format_is_correct", func(t *testing.T) {
		t.Parallel()

		policy := newBucketPolicy("my-bucket", "AKID001")
		resources := policy.Statement[0].Resource
		require.Len(t, resources, 2)
		require.Equal(t, "arn:aws:s3:::my-bucket", resources[0])
		require.Equal(t, "arn:aws:s3:::my-bucket/*", resources[1])
	})

	// TC-U-048: Actions map for each access mode
	// The driver uses "s3:*" for all grant operations (no per-mode differentiation).
	t.Run("TC-U-048_actions_map_for_all_access_modes", func(t *testing.T) {
		t.Parallel()

		policy := newBucketPolicy("test-bucket", "AKID001")
		// The driver always uses "s3:*" which covers ReadOnly, WriteOnly, and ReadWrite
		require.Equal(t, "s3:*", policy.Statement[0].Action)
	})
}

func TestParseAdminError(t *testing.T) {
	t.Parallel()

	c := &Client{}

	tests := []struct {
		name       string
		statusCode int
		body       string
		operation  string
		wantSub    string
	}{
		{
			name:       "xml_error_with_code_and_message",
			statusCode: http.StatusBadRequest,
			body:       `<Error><Code>InvalidBucketName</Code><Message>bucket name is invalid</Message></Error>`,
			operation:  "create bucket",
			wantSub:    "InvalidBucketName: bucket name is invalid",
		},
		{
			name:       "xml_error_includes_operation",
			statusCode: http.StatusConflict,
			body:       `<Error><Code>BucketAlreadyExists</Code><Message>exists</Message></Error>`,
			operation:  "create bucket",
			wantSub:    "create bucket: BucketAlreadyExists",
		},
		{
			name:       "non_xml_body_shows_status",
			statusCode: http.StatusInternalServerError,
			body:       "internal server error",
			operation:  "list users",
			wantSub:    "unexpected status 500",
		},
		{
			name:       "non_xml_body_includes_raw_body",
			statusCode: http.StatusInternalServerError,
			body:       "something went wrong",
			operation:  "list users",
			wantSub:    "something went wrong",
		},
		{
			name:       "empty_body",
			statusCode: http.StatusForbidden,
			body:       "",
			operation:  "delete user",
			wantSub:    "unexpected status 403",
		},
		{
			name:       "xml_without_code_falls_back",
			statusCode: http.StatusBadGateway,
			body:       `<Error><Message>no code field</Message></Error>`,
			operation:  "test op",
			wantSub:    "unexpected status 502",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp := &http.Response{
				StatusCode: tt.statusCode,
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}
			err := c.parseAdminError(resp, tt.operation)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantSub)
		})
	}
}

// TestAdminRequestConstruction tests Admin API request building.
func TestAdminRequestConstruction(t *testing.T) {
	t.Parallel()

	// TC-U-050: Create user request XML structure
	t.Run("TC-U-050_create_user_request_xml_structure", func(t *testing.T) {
		t.Parallel()

		account := auth.Account{
			Access: "NEWAKID",
			Secret: "NEWSECRET",
			Role:   "user",
		}
		body, err := xml.Marshal(account)
		require.NoError(t, err)

		xmlStr := string(body)
		require.Contains(t, xmlStr, "NEWAKID")
		require.Contains(t, xmlStr, "NEWSECRET")
		require.Contains(t, xmlStr, "user")
	})

	// TC-U-051: Create user request includes all required fields
	t.Run("TC-U-051_create_user_request_includes_required_fields", func(t *testing.T) {
		t.Parallel()

		account := auth.Account{
			Access: "AKID",
			Secret: "SECRET",
			Role:   "admin",
		}
		body, err := xml.Marshal(account)
		require.NoError(t, err)

		xmlStr := string(body)
		require.Contains(t, xmlStr, "AKID")
		require.Contains(t, xmlStr, "SECRET")
		require.Contains(t, xmlStr, "admin")
	})

	// TC-U-052: Delete user URL construction
	t.Run("TC-U-052_delete_user_url_construction", func(t *testing.T) {
		t.Parallel()

		var gotPath string
		var gotQuery string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotQuery = r.URL.RawQuery
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		c := newTestAdminClient(srv)
		resp, err := c.adminRequest(t.Context(), "/delete-user", map[string]string{"access": "AKID_TO_DELETE"}, nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, "/delete-user", gotPath)
		require.Contains(t, gotQuery, "access=AKID_TO_DELETE")
	})

	// TC-U-053: Parse list users XML response
	t.Run("TC-U-053_parse_list_users_xml_response", func(t *testing.T) {
		t.Parallel()

		xmlBody := `<ListUserAccountsResult>
			<Accounts>
				<Access>user1-key</Access>
				<Secret>user1-secret</Secret>
				<Role>user</Role>
			</Accounts>
			<Accounts>
				<Access>user2-key</Access>
				<Secret>user2-secret</Secret>
				<Role>admin</Role>
			</Accounts>
		</ListUserAccountsResult>`

		var result auth.ListUserAccountsResult
		err := xml.Unmarshal([]byte(xmlBody), &result)
		require.NoError(t, err)
		require.Len(t, result.Accounts, 2)
		require.Equal(t, "user1-key", result.Accounts[0].Access)
		require.Equal(t, "user1-secret", result.Accounts[0].Secret)
		require.Equal(t, auth.Role("user"), result.Accounts[0].Role)
		require.Equal(t, "user2-key", result.Accounts[1].Access)
		require.Equal(t, "user2-secret", result.Accounts[1].Secret)
		require.Equal(t, auth.Role("admin"), result.Accounts[1].Role)
	})
}

func TestAdminRequest(t *testing.T) {
	t.Parallel()

	t.Run("sends_patch_with_sigv4_headers", func(t *testing.T) {
		t.Parallel()
		var gotMethod string
		var gotHeaders http.Header
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotHeaders = r.Header.Clone()
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		c := newTestAdminClient(srv)
		resp, err := c.adminRequest(t.Context(), "/list-users", nil, nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.MethodPatch, gotMethod)
		require.NotEmpty(t, gotHeaders.Get("Authorization"))
		require.NotEmpty(t, gotHeaders.Get("X-Amz-Content-Sha256"))
		require.NotEmpty(t, gotHeaders.Get("X-Amz-Date"))
	})

	t.Run("includes_query_params", func(t *testing.T) {
		t.Parallel()
		var gotRawQuery string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotRawQuery = r.URL.RawQuery
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		c := newTestAdminClient(srv)
		resp, err := c.adminRequest(t.Context(), "/delete-user", map[string]string{"access": "testuser"}, nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Contains(t, gotRawQuery, "access=testuser")
	})

	t.Run("sends_body_with_content_type", func(t *testing.T) {
		t.Parallel()
		var gotContentType string
		var gotBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotContentType = r.Header.Get("Content-Type")
			gotBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		body := []byte("<Account><Access>test</Access></Account>")
		c := newTestAdminClient(srv)
		resp, err := c.adminRequest(t.Context(), "/create-user", nil, body)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, "application/xml", gotContentType)
		require.Equal(t, body, gotBody)
	})

	t.Run("nil_body_omits_content_type", func(t *testing.T) {
		t.Parallel()
		var gotContentType string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotContentType = r.Header.Get("Content-Type")
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		c := newTestAdminClient(srv)
		resp, err := c.adminRequest(t.Context(), "/list-buckets", nil, nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Empty(t, gotContentType)
	})
}

func newTestAdminClient(srv *httptest.Server) *Client {
	return &Client{
		adminEndpoint: srv.URL,
		creds:         aws.Credentials{AccessKeyID: "test", SecretAccessKey: "secret"},
		httpClient:    srv.Client(),
		region:        "us-east-1",
	}
}
