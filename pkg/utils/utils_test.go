package utils

import (
	"testing"
)

func TestSplitYAMLDocuments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single document",
			input:    "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test",
			expected: []string{"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test"},
		},
		{
			name: "two documents with clean separators",
			input: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test1
---
apiVersion: v1
kind: Secret
metadata:
  name: test2`,
			expected: []string{
				"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test1",
				"apiVersion: v1\nkind: Secret\nmetadata:\n  name: test2",
			},
		},
		{
			name: "documents with whitespace after ---",
			input: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test1
---   
apiVersion: v1
kind: Secret
metadata:
  name: test2
---		# with tabs
apiVersion: v1
kind: Service
metadata:
  name: test3`,
			expected: []string{
				"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test1",
				"apiVersion: v1\nkind: Secret\nmetadata:\n  name: test2",
				"apiVersion: v1\nkind: Service\nmetadata:\n  name: test3",
			},
		},
		{
			name: "documents with --- in content should not split",
			input: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test
  description: "This contains --- in the middle"
  data:
    key: "value with --- separator"
---
apiVersion: v1
kind: Secret
metadata:
  name: test2`,
			expected: []string{
				`apiVersion: v1
kind: ConfigMap
metadata:
  name: test
  description: "This contains --- in the middle"
  data:
    key: "value with --- separator"`,
				"apiVersion: v1\nkind: Secret\nmetadata:\n  name: test2",
			},
		},
		{
			name: "documents with indented --- should not split",
			input: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test
  data:
    key: |
      some content
      ---
      more content
---
apiVersion: v1
kind: Secret
metadata:
  name: test2`,
			expected: []string{
				`apiVersion: v1
kind: ConfigMap
metadata:
  name: test
  data:
    key: |
      some content
      ---
      more content`,
				"apiVersion: v1\nkind: Secret\nmetadata:\n  name: test2",
			},
		},
		{
			name: "documents with --- at end of line should not split",
			input: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test
  description: "This ends with ---"
---
apiVersion: v1
kind: Secret
metadata:
  name: test2`,
			expected: []string{
				`apiVersion: v1
kind: ConfigMap
metadata:
  name: test
  description: "This ends with ---"`,
				"apiVersion: v1\nkind: Secret\nmetadata:\n  name: test2",
			},
		},
		{
			name: "empty documents should be filtered out",
			input: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test1
---
---
apiVersion: v1
kind: Secret
metadata:
  name: test2
---

apiVersion: v1
kind: Service
metadata:
  name: test3`,
			expected: []string{
				"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test1",
				"apiVersion: v1\nkind: Secret\nmetadata:\n  name: test2",
				"apiVersion: v1\nkind: Service\nmetadata:\n  name: test3",
			},
		},
		{
			name: "documents with mixed whitespace after ---",
			input: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test1
---  ` + "\t" + `  
apiVersion: v1
kind: Secret
metadata:
  name: test2
---		# comment with tabs
apiVersion: v1
kind: Service
metadata:
  name: test3`,
			expected: []string{
				"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test1",
				"apiVersion: v1\nkind: Secret\nmetadata:\n  name: test2",
				"apiVersion: v1\nkind: Service\nmetadata:\n  name: test3",
			},
		},
		{
			name: "single document with --- in description",
			input: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test
  description: |
    This is a description that contains ---
    multiple lines with --- separators
    but should not be split`,
			expected: []string{`apiVersion: v1
kind: ConfigMap
metadata:
  name: test
  description: |
    This is a description that contains ---
    multiple lines with --- separators
    but should not be split`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitYAMLDocuments(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d documents, got %d", len(tt.expected), len(result))
				return
			}

			for i, doc := range result {
				if doc != tt.expected[i] {
					t.Errorf("Document %d mismatch:\nExpected:\n%s\n\nGot:\n%s", i, tt.expected[i], doc)
				}
			}
		})
	}
}

func TestSplitYAMLDocumentsEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "only separators",
			input:    "---\n---\n---",
			expected: []string{},
		},
		{
			name:     "separators with whitespace",
			input:    "---\n   \n---\n\t\n---",
			expected: []string{},
		},
		{
			name:     "single separator",
			input:    "---",
			expected: []string{},
		},
		{
			name:     "separator with whitespace",
			input:    "---   ",
			expected: []string{},
		},
		{
			name: "document with only whitespace",
			input: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test
---
   
---
apiVersion: v1
kind: Secret
metadata:
  name: test2`,
			expected: []string{
				"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test",
				"apiVersion: v1\nkind: Secret\nmetadata:\n  name: test2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitYAMLDocuments(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d documents, got %d", len(tt.expected), len(result))
				return
			}

			for i, doc := range result {
				if doc != tt.expected[i] {
					t.Errorf("Document %d mismatch:\nExpected:\n%s\n\nGot:\n%s", i, tt.expected[i], doc)
				}
			}
		})
	}
}

func TestSortManifestStrings(t *testing.T) {
tests := []struct {
name     string
input    []string
expected []string
}{
{
name:     "empty slice",
input:    []string{},
expected: []string{},
},
{
name: "single manifest",
input: []string{
"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: default",
},
expected: []string{
"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: default",
},
},
{
name: "two manifests already sorted",
input: []string{
"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test1\n  namespace: default",
"apiVersion: v1\nkind: Secret\nmetadata:\n  name: test2\n  namespace: default",
},
expected: []string{
"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test1\n  namespace: default",
"apiVersion: v1\nkind: Secret\nmetadata:\n  name: test2\n  namespace: default",
},
},
{
name: "two manifests need sorting by kind",
input: []string{
"apiVersion: v1\nkind: Service\nmetadata:\n  name: test\n  namespace: default",
"apiVersion: v1\nkind: Deployment\nmetadata:\n  name: test\n  namespace: default",
},
expected: []string{
"apiVersion: v1\nkind: Deployment\nmetadata:\n  name: test\n  namespace: default",
"apiVersion: v1\nkind: Service\nmetadata:\n  name: test\n  namespace: default",
},
},
{
name: "two manifests need sorting by name",
input: []string{
"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: z-config\n  namespace: default",
"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a-config\n  namespace: default",
},
expected: []string{
"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a-config\n  namespace: default",
"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: z-config\n  namespace: default",
},
},
{
name: "manifests with different namespaces",
input: []string{
"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: prod",
"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: dev",
},
expected: []string{
"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: dev",
"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: prod",
},
},
{
name: "cluster-scoped vs namespaced resources",
input: []string{
"apiVersion: v1\nkind: Namespace\nmetadata:\n  name: test",
"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: default",
},
expected: []string{
"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: default",
"apiVersion: v1\nkind: Namespace\nmetadata:\n  name: test",
},
},
{
name: "complex sorting scenario",
input: []string{
"apiVersion: v1\nkind: Service\nmetadata:\n  name: app\n  namespace: prod",
"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: app\n  namespace: dev",
"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: config\n  namespace: prod",
"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: app\n  namespace: prod",
},
expected: []string{
"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: config\n  namespace: prod",
"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: app\n  namespace: dev",
"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: app\n  namespace: prod",
"apiVersion: v1\nkind: Service\nmetadata:\n  name: app\n  namespace: prod",
},
},
}

for _, tt := range tests {
t.Run(tt.name, func(t *testing.T) {
// Make a copy to avoid modifying the input
input := make([]string, len(tt.input))
copy(input, tt.input)

SortManifestStrings(input)

if len(input) != len(tt.expected) {
t.Errorf("Expected %d manifests, got %d", len(tt.expected), len(input))
return
}

for i := range input {
if input[i] != tt.expected[i] {
t.Errorf("Manifest %d mismatch:\nExpected:\n%s\n\nGot:\n%s", i, tt.expected[i], input[i])
}
}
})
}
}

func TestGetManifestSortKey(t *testing.T) {
tests := []struct {
name     string
manifest string
expected string
}{
{
name:     "standard manifest with namespace",
manifest: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: test\n  namespace: default",
expected: "ConfigMap/default/test",
},
{
name:     "cluster-scoped resource",
manifest: "apiVersion: v1\nkind: Namespace\nmetadata:\n  name: test",
expected: "Namespace/~/test",
},
{
name:     "invalid YAML",
manifest: "not valid yaml {{{",
expected: "not valid yaml {{{",
},
{
name:     "missing metadata",
manifest: "apiVersion: v1\nkind: ConfigMap",
expected: "ConfigMap",
},
}

for _, tt := range tests {
t.Run(tt.name, func(t *testing.T) {
result := getManifestSortKey(tt.manifest)
if result != tt.expected {
t.Errorf("Expected %q, got %q", tt.expected, result)
}
})
}
}
