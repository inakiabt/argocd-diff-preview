package livestate

import (
	"fmt"
	"strings"

	"github.com/dag-andersen/argocd-diff-preview/pkg/extract"
	"github.com/rs/zerolog/log"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// ComparisonResult holds the result of comparing rendered manifests with live state
type ComparisonResult struct {
	AppId            string
	AppName          string
	OnlyInRendered   []string // Resources only in rendered manifests
	OnlyInLive       []string // Resources only in live state
	Different        []DiffDetail // Resources that differ between rendered and live
}

// DiffDetail holds details about a difference between rendered and live state
type DiffDetail struct {
	ResourceKind string
	ResourceName string
	Namespace    string
	Diff         string
}

// CompareWithLiveState compares rendered manifests with live cluster state
func CompareWithLiveState(
	renderedApps []extract.ExtractedApp,
	liveStates []LiveStateResources,
) []ComparisonResult {
	var results []ComparisonResult

	// Create a map of live states by app ID for quick lookup
	liveStateMap := make(map[string]LiveStateResources)
	for _, ls := range liveStates {
		liveStateMap[ls.AppId] = ls
	}

	for _, renderedApp := range renderedApps {
		liveState, exists := liveStateMap[renderedApp.Id]
		if !exists {
			log.Info().Msgf("📊 No live state found for application: %s (may not be deployed)", renderedApp.Name)
			// All rendered resources are "only in rendered"
			var onlyInRendered []string
			for _, manifest := range renderedApp.Manifest {
				onlyInRendered = append(onlyInRendered, getResourceIdentifier(&manifest))
			}
			results = append(results, ComparisonResult{
				AppId:          renderedApp.Id,
				AppName:        renderedApp.Name,
				OnlyInRendered: onlyInRendered,
			})
			continue
		}

		result := compareResources(renderedApp, liveState)
		results = append(results, result)
	}

	return results
}

// compareResources compares the resources of a single app
func compareResources(renderedApp extract.ExtractedApp, liveState LiveStateResources) ComparisonResult {
	result := ComparisonResult{
		AppId:   renderedApp.Id,
		AppName: renderedApp.Name,
	}

	// Create maps for quick lookup
	renderedMap := make(map[string]*unstructured.Unstructured)
	for i := range renderedApp.Manifest {
		key := getResourceIdentifier(&renderedApp.Manifest[i])
		renderedMap[key] = &renderedApp.Manifest[i]
	}

	liveMap := make(map[string]*unstructured.Unstructured)
	for i := range liveState.Resources {
		key := getResourceIdentifier(&liveState.Resources[i])
		liveMap[key] = &liveState.Resources[i]
	}

	// Find resources only in rendered
	for key := range renderedMap {
		if _, exists := liveMap[key]; !exists {
			result.OnlyInRendered = append(result.OnlyInRendered, key)
		}
	}

	// Find resources only in live
	for key := range liveMap {
		if _, exists := renderedMap[key]; !exists {
			result.OnlyInLive = append(result.OnlyInLive, key)
		}
	}

	// Find differences in resources that exist in both
	for key, renderedRes := range renderedMap {
		liveRes, exists := liveMap[key]
		if !exists {
			continue
		}

		// Compare the resources
		if diff := compareResourceContent(renderedRes, liveRes); diff != "" {
			result.Different = append(result.Different, DiffDetail{
				ResourceKind: renderedRes.GetKind(),
				ResourceName: renderedRes.GetName(),
				Namespace:    renderedRes.GetNamespace(),
				Diff:         diff,
			})
		}
	}

	return result
}

// getResourceIdentifier creates a unique identifier for a resource
func getResourceIdentifier(obj *unstructured.Unstructured) string {
	namespace := obj.GetNamespace()
	if namespace == "" {
		return fmt.Sprintf("%s/%s", obj.GetKind(), obj.GetName())
	}
	return fmt.Sprintf("%s/%s/%s", obj.GetKind(), namespace, obj.GetName())
}

// compareResourceContent compares the content of two resources and returns a diff string
func compareResourceContent(rendered, live *unstructured.Unstructured) string {
	// Remove metadata fields that are managed by Kubernetes and may differ
	renderedCopy := rendered.DeepCopy()
	liveCopy := live.DeepCopy()

	// Clean metadata for comparison
	cleanMetadata(renderedCopy)
	cleanMetadata(liveCopy)

	// Convert to YAML for comparison
	renderedYAML, err := yaml.Marshal(renderedCopy.Object)
	if err != nil {
		log.Warn().Msgf("Failed to marshal rendered resource: %v", err)
		return ""
	}

	liveYAML, err := yaml.Marshal(liveCopy.Object)
	if err != nil {
		log.Warn().Msgf("Failed to marshal live resource: %v", err)
		return ""
	}

	// Simple string comparison
	if string(renderedYAML) == string(liveYAML) {
		return ""
	}

	// Return a simple indication that they differ
	// A more sophisticated diff could be implemented here
	return fmt.Sprintf("Resource differs between rendered and live state")
}

// cleanMetadata removes fields from metadata that are managed by Kubernetes
func cleanMetadata(obj *unstructured.Unstructured) {
	metadata := obj.Object["metadata"].(map[string]interface{})
	
	// Remove fields that are managed by Kubernetes
	delete(metadata, "resourceVersion")
	delete(metadata, "uid")
	delete(metadata, "creationTimestamp")
	delete(metadata, "generation")
	delete(metadata, "managedFields")
	delete(metadata, "selfLink")
	
	// Remove status as it's managed by controllers
	delete(obj.Object, "status")
}

// FormatComparisonResults formats the comparison results as a string
func FormatComparisonResults(results []ComparisonResult) string {
	if len(results) == 0 {
		return "No live state comparisons performed.\n"
	}

	var sb strings.Builder
	sb.WriteString("## Live State Comparison\n\n")

	for _, result := range results {
		sb.WriteString(fmt.Sprintf("### Application: %s\n\n", result.AppName))

		hasChanges := len(result.OnlyInRendered) > 0 || len(result.OnlyInLive) > 0 || len(result.Different) > 0

		if !hasChanges {
			sb.WriteString("✅ Rendered manifests match live state\n\n")
			continue
		}

		if len(result.OnlyInRendered) > 0 {
			sb.WriteString("**Resources only in rendered manifests (will be created):**\n")
			for _, res := range result.OnlyInRendered {
				sb.WriteString(fmt.Sprintf("- %s\n", res))
			}
			sb.WriteString("\n")
		}

		if len(result.OnlyInLive) > 0 {
			sb.WriteString("**Resources only in live state (will be deleted):**\n")
			for _, res := range result.OnlyInLive {
				sb.WriteString(fmt.Sprintf("- %s\n", res))
			}
			sb.WriteString("\n")
		}

		if len(result.Different) > 0 {
			sb.WriteString("**Resources with differences (will be updated):**\n")
			for _, diff := range result.Different {
				sb.WriteString(fmt.Sprintf("- %s/%s in namespace %s\n", diff.ResourceKind, diff.ResourceName, diff.Namespace))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
