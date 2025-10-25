package extract

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dag-andersen/argocd-diff-preview/pkg/argoapplication"
	argocdPkg "github.com/dag-andersen/argocd-diff-preview/pkg/argocd"
	"github.com/dag-andersen/argocd-diff-preview/pkg/git"
	"github.com/rs/zerolog/log"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// DiscoverChildApplications deploys root applications and discovers all child applications created by ArgoCD
// This supports the App of Apps pattern where a root application generates child applications
func DiscoverChildApplications(
	argocd *argocdPkg.ArgoCDInstallation,
	rootApps []argoapplication.ArgoResource,
	branch git.BranchType,
	timeout uint64,
	prefix string,
) ([]argoapplication.ArgoResource, error) {
	if len(rootApps) == 0 {
		return []argoapplication.ArgoResource{}, nil
	}

	branchName := string(branch)
	log.Info().Msgf("🌳 Discovering child applications from %d root app(s) (branch: %s)", len(rootApps), branchName)

	// Deploy root applications and wait for them to sync
	deployedRootApps := make([]string, 0, len(rootApps))
	for _, app := range rootApps {
		// Add prefix to avoid conflicts
		if err := addApplicationPrefix(&app, prefix); err != nil {
			return nil, fmt.Errorf("failed to prefix application name: %w", err)
		}

		if err := labelApplicationWithRunID(&app, prefix); err != nil {
			return nil, fmt.Errorf("failed to label application with run ID: %w", err)
		}

		// Deploy the root application
		if err := argocd.K8sClient.ApplyManifest(app.Yaml, "string", argocd.Namespace); err != nil {
			return nil, fmt.Errorf("failed to apply root application %s: %w", app.GetLongName(), err)
		}

		log.Debug().Str("App", app.GetLongName()).Msg("Deployed root application")
		deployedRootApps = append(deployedRootApps, app.Id)
	}

	// Wait for root applications to sync
	log.Info().Msgf("⏳ Waiting for root applications to sync (timeout in %d seconds)", timeout)
	startTime := time.Now()
	
	for _, appName := range deployedRootApps {
		if err := waitForApplicationSync(argocd, appName, timeout, startTime); err != nil {
			return nil, fmt.Errorf("failed waiting for root app %s to sync: %w", appName, err)
		}
	}

	log.Info().Msgf("✅ Root applications synced in %s", time.Since(startTime).Round(time.Second))

	// Give ArgoCD a moment to create child applications
	time.Sleep(5 * time.Second)

	// Discover all applications in the namespace
	log.Info().Msgf("🔍 Discovering child applications from ArgoCD")
	allAppsJSON, err := argocd.K8sClient.GetArgoCDApplications(argocd.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to list applications: %w", err)
	}

	// Parse the JSON response
	var appList struct {
		Items []unstructured.Unstructured `json:"items"`
	}
	if err := json.Unmarshal([]byte(allAppsJSON), &appList); err != nil {
		return nil, fmt.Errorf("failed to parse applications list: %w", err)
	}

	// Filter applications to only include those created by our root apps
	// Look for applications with our run ID label or that are children of our roots
	childApps := make([]argoapplication.ArgoResource, 0)

	for _, item := range appList.Items {
		labels := item.GetLabels()
		
		// Skip if this doesn't have our run ID (not created by us)
		if labels == nil || labels["argocd-diff-preview.io/run-id"] != prefix {
			continue
		}

		appName := item.GetName()
		
		// Skip if this is one of the root applications we deployed
		isRoot := false
		for _, rootName := range deployedRootApps {
			if appName == rootName {
				isRoot = true
				break
			}
		}
		if isRoot {
			continue
		}

		// This is a child application - convert it to ArgoResource
		yamlBytes, err := yaml.Marshal(item.Object)
		if err != nil {
			log.Warn().Str("App", appName).Msg("Failed to marshal application to YAML, skipping")
			continue
		}

		var argoRes unstructured.Unstructured
		if err := yaml.Unmarshal(yamlBytes, &argoRes); err != nil {
			log.Warn().Str("App", appName).Msg("Failed to unmarshal application YAML, skipping")
			continue
		}

		// Extract metadata
		spec, ok := argoRes.Object["spec"].(map[string]interface{})
		if !ok {
			log.Warn().Str("App", appName).Msg("No spec found in application, skipping")
			continue
		}
		source, _ := spec["source"].(map[string]interface{})
		
		originalName := appName
		// Remove prefix if it was added
		if strings.HasPrefix(appName, prefix+"-") {
			originalName = strings.TrimPrefix(appName, prefix+"-")
		}

		sourcePath := ""
		if source != nil {
			if path, ok := source["path"].(string); ok {
				sourcePath = path
			}
		}

		childApp := argoapplication.ArgoResource{
			Id:       appName,
			Name:     originalName,
			FileName: fmt.Sprintf("discovered-from-argocd/%s.yaml", originalName),
			Branch:   branch,
			Yaml:     &argoRes,
			Kind:     argoapplication.Application,
		}

		childApps = append(childApps, childApp)
		log.Debug().Str("App", originalName).Str("Path", sourcePath).Msg("Discovered child application")
	}

	log.Info().Msgf("🌳 Discovered %d child application(s) from root apps (branch: %s)", len(childApps), branchName)

	return childApps, nil
}

// waitForApplicationSync waits for an application to reach Synced or OutOfSync status
func waitForApplicationSync(argocd *argocdPkg.ArgoCDInstallation, appName string, timeout uint64, startTime time.Time) error {
	for {
		// Check if we've exceeded timeout
		if time.Since(startTime).Seconds() > float64(timeout) {
			return fmt.Errorf("timed out waiting for application %s to sync", appName)
		}

		// Get application status
		output, err := argocd.K8sClient.GetArgoCDApplication(argocd.Namespace, appName)
		if err != nil {
			return fmt.Errorf("failed to get application %s: %w", appName, err)
		}

		var appStatus struct {
			Status struct {
				Sync struct {
					Status string `yaml:"status"`
				} `yaml:"sync"`
				Conditions []struct {
					Type    string `yaml:"type"`
					Message string `yaml:"message"`
				} `yaml:"conditions"`
			} `yaml:"status"`
		}

		if err := yaml.Unmarshal([]byte(output), &appStatus); err != nil {
			return fmt.Errorf("failed to parse application yaml for %s: %w", appName, err)
		}

		switch appStatus.Status.Sync.Status {
		case "OutOfSync", "Synced":
			log.Debug().Str("App", appName).Str("Status", appStatus.Status.Sync.Status).Msg("Application reached stable state")
			return nil
		case "Unknown":
			// Check for errors
			for _, condition := range appStatus.Status.Conditions {
				if isErrorCondition(condition.Type) {
					msg := condition.Message
					// If it's a timeout, refresh and continue
					if containsAny(msg, timeoutMessages) {
						log.Warn().Str("App", appName).Msg("⚠️ Application timed out, refreshing")
						if err := argocd.RefreshApp(appName); err != nil {
							log.Error().Err(err).Str("App", appName).Msg("⚠️ Failed to refresh application")
						}
					} else {
						return fmt.Errorf("application %s failed: %s", appName, msg)
					}
				}
			}
		}

		// Sleep before next iteration
		time.Sleep(5 * time.Second)
	}
}
