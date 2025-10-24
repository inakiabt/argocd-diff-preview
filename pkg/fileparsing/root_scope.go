package fileparsing

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// BuildFileRegexFromRootApplication reads a root Application file and builds a regex
// that matches YAML files under the paths declared in spec.source.path or spec.sources[*].path.
// It honors the directory.recurse setting for each source.
func BuildFileRegexFromRootApplication(branchDir, appRelativePath string) (*string, error) {
	// Build absolute path to the root Application file
	appAbsPath := filepath.Join(branchDir, appRelativePath)

	log.Debug().Msgf("Reading root Application from: %s", appAbsPath)

	// Read the file
	data, err := os.ReadFile(appAbsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read root Application file '%s': %w", appAbsPath, err)
	}

	// Parse as unstructured
	var obj map[string]interface{}
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("failed to parse root Application YAML: %w", err)
	}

	u := &unstructured.Unstructured{Object: obj}

	// Validate kind == Application
	kind := u.GetKind()
	if kind != "Application" {
		return nil, fmt.Errorf("root Application file has kind '%s', expected 'Application'", kind)
	}

	// Collect source paths
	var sourcePaths []sourcePath

	// Try multi-source first (spec.sources)
	sources, found, err := unstructured.NestedSlice(u.Object, "spec", "sources")
	if err == nil && found && len(sources) > 0 {
		log.Debug().Msgf("Found %d sources in root Application", len(sources))
		for i, src := range sources {
			srcMap, ok := src.(map[string]interface{})
			if !ok {
				log.Warn().Msgf("Source %d is not a map, skipping", i)
				continue
			}

			path, pathFound, _ := unstructured.NestedString(srcMap, "path")
			if !pathFound || path == "" {
				log.Debug().Msgf("Source %d has no path, skipping", i)
				continue
			}

			// Read directory.recurse, default is false
			recurse := false
			if r, rFound, _ := unstructured.NestedBool(srcMap, "directory", "recurse"); rFound {
				recurse = r
			}

			sourcePaths = append(sourcePaths, sourcePath{
				path:    path,
				recurse: recurse,
			})
			log.Debug().Msgf("Source %d: path=%s, recurse=%t", i, path, recurse)
		}
	} else {
		// Fallback to single source (spec.source)
		path, pathFound, _ := unstructured.NestedString(u.Object, "spec", "source", "path")
		if pathFound && path != "" {
			// Read directory.recurse, default is false
			recurse := false
			if r, rFound, _ := unstructured.NestedBool(u.Object, "spec", "source", "directory", "recurse"); rFound {
				recurse = r
			}

			sourcePaths = append(sourcePaths, sourcePath{
				path:    path,
				recurse: recurse,
			})
			log.Debug().Msgf("Single source: path=%s, recurse=%t", path, recurse)
		}
	}

	if len(sourcePaths) == 0 {
		return nil, fmt.Errorf("no source paths found in root Application")
	}

	// Build regex patterns for each source path
	var patterns []string
	for _, sp := range sourcePaths {
		pattern := buildPatternForPath(sp.path, sp.recurse)
		patterns = append(patterns, pattern)
	}

	// Combine with |
	combined := strings.Join(patterns, "|")
	log.Info().Msgf("Built file regex from root Application: %s", combined)

	return &combined, nil
}

// sourcePath represents a source path and its recurse setting
type sourcePath struct {
	path    string
	recurse bool
}

// buildPatternForPath constructs a regex pattern for a given path and recurse setting
func buildPatternForPath(path string, recurse bool) string {
	// Clean the path to avoid issues with leading/trailing slashes
	path = strings.Trim(path, "/")

	if recurse {
		// Recursive: ^path/.*\.ya?ml$
		return fmt.Sprintf("^%s/.*\\.ya?ml$", regexp.QuoteMeta(path))
	}
	// Non-recursive: ^path/[^/]+\.ya?ml$
	return fmt.Sprintf("^%s/[^/]+\\.ya?ml$", regexp.QuoteMeta(path))
}
