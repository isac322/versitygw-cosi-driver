package deploy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	cosi "sigs.k8s.io/container-object-storage-interface/proto"
)

func TestManifests_useKnownCOSIAuthenticationTypeNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		path               string
		extract            func(t *testing.T, content string) string
		wantAuthentication cosi.AuthenticationType
	}{
		{
			name: "helm values bucket access class uses Key",
			path: "deploy/helm/versitygw-cosi-driver/values.yaml",
			extract: func(t *testing.T, content string) string {
				t.Helper()

				return extractYAMLFieldInBlock(t, content, "bucketAccessClass", "authenticationType")
			},
			wantAuthentication: cosi.AuthenticationType_Key,
		},
		{
			name: "kustomize bucket access class uses Key",
			path: "deploy/kustomize/components/bucketaccessclass/bucketaccessclass.yaml",
			extract: func(t *testing.T, content string) string {
				t.Helper()

				return extractTopLevelYAMLField(t, content, "authenticationType")
			},
			wantAuthentication: cosi.AuthenticationType_Key,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			content := readRepoFile(t, tt.path)
			authenticationType := tt.extract(t, content)

			got := resolveAuthenticationType(t, authenticationType)
			require.Equal(t, tt.wantAuthentication, got)
		})
	}
}

func TestHelmSchema_allowsKnownCOSIAuthenticationTypeNames(t *testing.T) {
	t.Parallel()

	content := readRepoFile(t, "deploy/helm/versitygw-cosi-driver/values.schema.json")

	authenticationTypes := extractHelmSchemaAuthenticationTypes(t, content)
	require.Equal(t, []string{"Key"}, authenticationTypes)

	for _, authenticationType := range authenticationTypes {
		got := resolveAuthenticationType(t, authenticationType)
		require.NotEqual(t, cosi.AuthenticationType_UnknownAuthenticationType, got)
	}
}

func TestKustomizeBase_usesCurrentDriverImage(t *testing.T) {
	t.Parallel()

	content := readRepoFile(t, "deploy/kustomize/base/deployment.yaml")

	require.Contains(t, content, "image: ghcr.io/isac322/versitygw-cosi-driver:0.5.1")
	require.NotContains(t, content, "image: ghcr.io/isac322/versitygw-cosi-driver:0.2.0")
}

func readRepoFile(t *testing.T, relativePath string) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)

	path := filepath.Join(filepath.Dir(filename), "..", "..", relativePath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(content)
}

func resolveAuthenticationType(t *testing.T, authenticationType string) cosi.AuthenticationType {
	t.Helper()

	value, ok := cosi.AuthenticationType_value[authenticationType]
	require.Truef(t, ok, "authenticationType %q must exactly match a COSI enum name", authenticationType)

	return cosi.AuthenticationType(value)
}

func extractYAMLFieldInBlock(t *testing.T, content, block, field string) string {
	t.Helper()

	inBlock := false
	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == block+":" {
			inBlock = true

			continue
		}
		if !inBlock {
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") {
			break
		}
		if value, ok := strings.CutPrefix(trimmed, field+":"); ok {
			return strings.TrimSpace(value)
		}
	}

	require.Failf(t, "missing YAML field", "field %q not found in block %q", field, block)

	return ""
}

func extractTopLevelYAMLField(t *testing.T, content, field string) string {
	t.Helper()

	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(line, " ") {
			continue
		}
		if value, ok := strings.CutPrefix(trimmed, field+":"); ok {
			return strings.TrimSpace(value)
		}
	}

	require.Failf(t, "missing YAML field", "top-level field %q not found", field)

	return ""
}

func extractHelmSchemaAuthenticationTypes(t *testing.T, content string) []string {
	t.Helper()

	var schema helmSchemaProperty
	require.NoError(t, json.Unmarshal([]byte(content), &schema))

	bucketAccessClass, ok := schema.Properties["bucketAccessClass"]
	require.True(t, ok)

	authenticationType, ok := bucketAccessClass.Properties["authenticationType"]
	require.True(t, ok)
	require.NotEmpty(t, authenticationType.Enum)

	return authenticationType.Enum
}

type helmSchemaProperty struct {
	Properties map[string]helmSchemaProperty `json:"properties"`
	Enum       []string                      `json:"enum"`
}
