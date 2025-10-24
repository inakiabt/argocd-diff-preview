package fileparsing

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildFileRegexFromRootApplication(t *testing.T) {
	tests := []struct {
		name          string
		appContent    string
		expectError   bool
		expectedRegex string
		matchFiles    []string
		noMatchFiles  []string
	}{
		{
			name: "single source with non-recursive",
			appContent: `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: root-app
spec:
  source:
    path: apps
    directory:
      recurse: false
`,
			expectError:   false,
			expectedRegex: `^apps/[^/]+\.ya?ml$`,
			matchFiles:    []string{"apps/app1.yaml", "apps/app2.yml"},
			noMatchFiles:  []string{"apps/subdir/app.yaml", "other/app.yaml", "apps/app.txt"},
		},
		{
			name: "single source with recursive",
			appContent: `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: root-app
spec:
  source:
    path: apps
    directory:
      recurse: true
`,
			expectError:   false,
			expectedRegex: `^apps/.*\.ya?ml$`,
			matchFiles:    []string{"apps/app1.yaml", "apps/subdir/app2.yml", "apps/a/b/c/deep.yaml"},
			noMatchFiles:  []string{"other/app.yaml", "apps.yaml"},
		},
		{
			name: "single source without directory.recurse (default false)",
			appContent: `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: root-app
spec:
  source:
    path: manifests
`,
			expectError:   false,
			expectedRegex: `^manifests/[^/]+\.ya?ml$`,
			matchFiles:    []string{"manifests/app.yaml", "manifests/service.yml"},
			noMatchFiles:  []string{"manifests/subdir/app.yaml"},
		},
		{
			name: "multi-source with mixed recurse",
			appContent: `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: root-app
spec:
  sources:
    - path: apps
      directory:
        recurse: false
    - path: charts
      directory:
        recurse: true
`,
			expectError:   false,
			expectedRegex: `^apps/[^/]+\.ya?ml$|^charts/.*\.ya?ml$`,
			matchFiles: []string{
				"apps/app1.yaml",
				"charts/chart1.yaml",
				"charts/subdir/chart2.yml",
			},
			noMatchFiles: []string{
				"apps/subdir/app.yaml",
				"other/app.yaml",
			},
		},
		{
			name: "multi-source all non-recursive",
			appContent: `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: root-app
spec:
  sources:
    - path: apps
      directory:
        recurse: false
    - path: infra
      directory:
        recurse: false
`,
			expectError:   false,
			expectedRegex: `^apps/[^/]+\.ya?ml$|^infra/[^/]+\.ya?ml$`,
			matchFiles: []string{
				"apps/app1.yaml",
				"infra/network.yml",
			},
			noMatchFiles: []string{
				"apps/subdir/app.yaml",
				"infra/k8s/deploy.yaml",
			},
		},
		{
			name: "multi-source all recursive",
			appContent: `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: root-app
spec:
  sources:
    - path: cluster1
      directory:
        recurse: true
    - path: cluster2
      directory:
        recurse: true
`,
			expectError:   false,
			expectedRegex: `^cluster1/.*\.ya?ml$|^cluster2/.*\.ya?ml$`,
			matchFiles: []string{
				"cluster1/app.yaml",
				"cluster1/a/b/c/deep.yaml",
				"cluster2/service.yml",
				"cluster2/x/y/z/file.yaml",
			},
			noMatchFiles: []string{
				"other/app.yaml",
			},
		},
		{
			name: "path with special regex characters",
			appContent: `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: root-app
spec:
  source:
    path: my-apps.v2
    directory:
      recurse: false
`,
			expectError:   false,
			expectedRegex: `^my-apps\.v2/[^/]+\.ya?ml$`,
			matchFiles:    []string{"my-apps.v2/app.yaml"},
			noMatchFiles:  []string{"myXappsXv2/app.yaml"},
		},
		{
			name: "invalid kind",
			appContent: `apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: not-an-app
spec:
  generators: []
`,
			expectError: true,
		},
		{
			name: "no source paths",
			appContent: `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: root-app
spec:
  destination:
    server: https://kubernetes.default.svc
`,
			expectError: true,
		},
		{
			name: "invalid yaml",
			appContent: `this is not valid yaml
  indentation: broken
metadata
`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory
			tempDir, err := os.MkdirTemp("", "root-scope-test")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			// Write the app file
			appPath := "root-app.yaml"
			appFullPath := filepath.Join(tempDir, appPath)
			err = os.WriteFile(appFullPath, []byte(tt.appContent), 0644)
			require.NoError(t, err)

			// Call the function
			result, err := BuildFileRegexFromRootApplication(tempDir, appPath)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
				return
			}

			// Should not error
			require.NoError(t, err)
			require.NotNil(t, result)

			// Check the regex matches expected
			assert.Equal(t, tt.expectedRegex, *result)

			// Compile the regex to ensure it's valid
			compiledRegex, err := regexp.Compile(*result)
			require.NoError(t, err)

			// Test files that should match
			for _, file := range tt.matchFiles {
				assert.True(t, compiledRegex.MatchString(file),
					"expected regex to match file: %s", file)
			}

			// Test files that should NOT match
			for _, file := range tt.noMatchFiles {
				assert.False(t, compiledRegex.MatchString(file),
					"expected regex to NOT match file: %s", file)
			}
		})
	}
}

func TestBuildFileRegexFromRootApplication_FileNotFound(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "root-scope-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	result, err := BuildFileRegexFromRootApplication(tempDir, "nonexistent.yaml")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to read root Application file")
}

func TestBuildPatternForPath(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		recurse        bool
		expected       string
		shouldMatch    []string
		shouldNotMatch []string
	}{
		{
			name:           "simple non-recursive",
			path:           "apps",
			recurse:        false,
			expected:       `^apps/[^/]+\.ya?ml$`,
			shouldMatch:    []string{"apps/app.yaml", "apps/svc.yml"},
			shouldNotMatch: []string{"apps/sub/app.yaml"},
		},
		{
			name:           "simple recursive",
			path:           "apps",
			recurse:        true,
			expected:       `^apps/.*\.ya?ml$`,
			shouldMatch:    []string{"apps/app.yaml", "apps/a/b/c.yml"},
			shouldNotMatch: []string{"other/app.yaml"},
		},
		{
			name:           "path with leading slash",
			path:           "/apps",
			recurse:        false,
			expected:       `^apps/[^/]+\.ya?ml$`,
			shouldMatch:    []string{"apps/app.yaml"},
			shouldNotMatch: []string{},
		},
		{
			name:           "path with trailing slash",
			path:           "apps/",
			recurse:        true,
			expected:       `^apps/.*\.ya?ml$`,
			shouldMatch:    []string{"apps/x/y.yaml"},
			shouldNotMatch: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildPatternForPath(tt.path, tt.recurse)
			assert.Equal(t, tt.expected, result)

			// Compile and test
			re, err := regexp.Compile(result)
			require.NoError(t, err)

			for _, file := range tt.shouldMatch {
				assert.True(t, re.MatchString(file),
					"expected pattern to match: %s", file)
			}

			for _, file := range tt.shouldNotMatch {
				assert.False(t, re.MatchString(file),
					"expected pattern NOT to match: %s", file)
			}
		})
	}
}
