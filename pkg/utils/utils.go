package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	"sigs.k8s.io/yaml"
)

const (
	dirMode = os.ModePerm // 0755 - read/write/execute for owner, read/execute for group and others
)

// WriteFile writes content to a file
func WriteFile(path string, content string) error {
	// Ensure content ends with a newline
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Write file
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}

	return nil
}

// Create folder (clear its content if it exists)
func CreateFolder(path string, override bool) error {
	if override {
		if err := os.RemoveAll(path); err != nil {
			log.Debug().Str("path", path).Msgf("⚠️ Failed to delete folder: %s", err)
		}
	}
	err := os.MkdirAll(path, dirMode)
	if err != nil {
		log.Debug().Str("path", path).Msgf("❌ Failed to create folder: %s", err)
	}
	return err
}

var uniqueNumber uint64

func UniqueNumber() uint64 {
	return atomic.AddUint64(&uniqueNumber, 1)
}

func UniqueId() string {
	return fmt.Sprintf("uid-%d", UniqueNumber())
}

func SplitYAMLDocuments(manifest string) []string {
	docSeparatorRegex := regexp.MustCompile(`(?m)^---\s*(?:#.*)?$`)
	documents := docSeparatorRegex.Split(manifest, -1)

	// Filter out empty or whitespace-only documents
	var result []string
	for _, doc := range documents {
		if trimmed := strings.TrimSpace(doc); trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// SortManifestStrings sorts a slice of YAML manifest strings by their resource identity
// (kind, namespace, name) to ensure consistent ordering across runs
func SortManifestStrings(manifestStrings []string) {
	sort.Slice(manifestStrings, func(i, j int) bool {
		return getManifestSortKey(manifestStrings[i]) < getManifestSortKey(manifestStrings[j])
	})
}

// getManifestSortKey extracts a sort key from a YAML manifest string
// The key format is: "kind/namespace/name"
func getManifestSortKey(manifestStr string) string {
	var obj map[string]interface{}
	if err := yaml.Unmarshal([]byte(manifestStr), &obj); err != nil {
		// If we can't parse it, use the raw string for sorting
		return manifestStr
	}

	kind, _ := obj["kind"].(string)

	metadata, ok := obj["metadata"].(map[string]interface{})
	if !ok {
		return kind
	}

	namespace, _ := metadata["namespace"].(string)
	name, _ := metadata["name"].(string)

	// Create a sort key that ensures consistent ordering
	// Using "/" as separator and handling empty values
	if namespace == "" {
		namespace = "~" // Sort cluster-scoped resources last
	}

	return fmt.Sprintf("%s/%s/%s", kind, namespace, name)
}
