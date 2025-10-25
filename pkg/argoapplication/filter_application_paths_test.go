package argoapplication

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterByApplicationPaths(t *testing.T) {
	// Create test applications using existing helper
	app1 := createTestApp("app1", "apps/prod/app1.yaml")
	app2 := createTestApp("app2", "apps/dev/app2.yaml")
	app3 := createTestApp("app3", "apps/staging/app3.yaml")

	apps := []ArgoResource{app1, app2, app3}

	// Test filtering with application paths
	filterOptions := FilterOptions{
		ApplicationPaths: []string{"apps/prod/app1.yaml", "apps/dev/app2.yaml"},
	}

	filteredApps := FilterAll(apps, filterOptions)

	assert.Equal(t, 2, len(filteredApps), "Should filter to 2 apps")
	assert.Equal(t, "app1", filteredApps[0].Name)
	assert.Equal(t, "app2", filteredApps[1].Name)
}

func TestFilterByApplicationPathsEmpty(t *testing.T) {
	// Create test applications using existing helper
	app1 := createTestApp("app1", "apps/prod/app1.yaml")
	app2 := createTestApp("app2", "apps/dev/app2.yaml")

	apps := []ArgoResource{app1, app2}

	// Test with no application paths - should return all apps
	filterOptions := FilterOptions{
		ApplicationPaths: []string{},
	}

	filteredApps := FilterAll(apps, filterOptions)

	assert.Equal(t, 2, len(filteredApps), "Should return all apps when no paths specified")
}

func TestFilterByApplicationPathsNoMatch(t *testing.T) {
	// Create test applications using existing helper
	app1 := createTestApp("app1", "apps/prod/app1.yaml")
	app2 := createTestApp("app2", "apps/dev/app2.yaml")

	apps := []ArgoResource{app1, app2}

	// Test with paths that don't match any apps
	filterOptions := FilterOptions{
		ApplicationPaths: []string{"apps/test/app3.yaml"},
	}

	filteredApps := FilterAll(apps, filterOptions)

	assert.Equal(t, 0, len(filteredApps), "Should return no apps when paths don't match")
}
