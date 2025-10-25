package livestate

import (
	"testing"

	"github.com/dag-andersen/argocd-diff-preview/pkg/extract"
	"github.com/dag-andersen/argocd-diff-preview/pkg/git"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCompareWithLiveState(t *testing.T) {
	// Create a rendered app with some resources
	renderedApp := extract.ExtractedApp{
		Id:   "app1",
		Name: "test-app",
		Manifest: []unstructured.Unstructured{
			createTestResource("Deployment", "test-deployment", "default"),
			createTestResource("Service", "test-service", "default"),
		},
		Branch: git.Target,
	}

	// Create live state with some differences
	liveState := LiveStateResources{
		AppId:   "app1",
		AppName: "test-app",
		Resources: []unstructured.Unstructured{
			createTestResource("Deployment", "test-deployment", "default"),
			createTestResource("ConfigMap", "test-config", "default"),
		},
		Namespace: "default",
	}

	results := CompareWithLiveState([]extract.ExtractedApp{renderedApp}, []LiveStateResources{liveState})

	assert.Len(t, results, 1, "Should have one comparison result")
	result := results[0]

	assert.Equal(t, "app1", result.AppId)
	assert.Equal(t, "test-app", result.AppName)
	
	// Service should be only in rendered (will be created)
	assert.Contains(t, result.OnlyInRendered, "Service/default/test-service")
	
	// ConfigMap should be only in live (will be deleted)
	assert.Contains(t, result.OnlyInLive, "ConfigMap/default/test-config")
}

func TestCompareWithLiveStateNoLiveState(t *testing.T) {
	// Create a rendered app
	renderedApp := extract.ExtractedApp{
		Id:   "app1",
		Name: "test-app",
		Manifest: []unstructured.Unstructured{
			createTestResource("Deployment", "test-deployment", "default"),
		},
		Branch: git.Target,
	}

	// No live state
	results := CompareWithLiveState([]extract.ExtractedApp{renderedApp}, []LiveStateResources{})

	assert.Len(t, results, 1)
	result := results[0]

	// All resources should be only in rendered
	assert.Contains(t, result.OnlyInRendered, "Deployment/default/test-deployment")
	assert.Empty(t, result.OnlyInLive)
	assert.Empty(t, result.Different)
}

func TestGetResourceIdentifier(t *testing.T) {
	// Test with namespace
	resource := createTestResource("Deployment", "test-deployment", "default")
	identifier := getResourceIdentifier(&resource)
	assert.Equal(t, "Deployment/default/test-deployment", identifier)

	// Test without namespace (cluster-scoped)
	clusterResource := createTestResource("ClusterRole", "test-role", "")
	identifier = getResourceIdentifier(&clusterResource)
	assert.Equal(t, "ClusterRole/test-role", identifier)
}

func TestFormatComparisonResults(t *testing.T) {
	results := []ComparisonResult{
		{
			AppId:   "app1",
			AppName: "test-app",
			OnlyInRendered: []string{"Service/default/test-service"},
			OnlyInLive:     []string{"ConfigMap/default/test-config"},
		},
	}

	formatted := FormatComparisonResults(results)
	
	assert.Contains(t, formatted, "## Live State Comparison")
	assert.Contains(t, formatted, "### Application: test-app")
	assert.Contains(t, formatted, "Resources only in rendered manifests")
	assert.Contains(t, formatted, "Resources only in live state")
	assert.Contains(t, formatted, "Service/default/test-service")
	assert.Contains(t, formatted, "ConfigMap/default/test-config")
}

func TestFormatComparisonResultsNoChanges(t *testing.T) {
	results := []ComparisonResult{
		{
			AppId:   "app1",
			AppName: "test-app",
			// No differences
		},
	}

	formatted := FormatComparisonResults(results)
	
	assert.Contains(t, formatted, "✅ Rendered manifests match live state")
}

// Helper function to create a test resource
func createTestResource(kind, name, namespace string) unstructured.Unstructured {
	resource := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}
	
	if namespace != "" {
		if metadata, ok := resource.Object["metadata"].(map[string]interface{}); ok {
			metadata["namespace"] = namespace
		}
	}
	
	return resource
}
