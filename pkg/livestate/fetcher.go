package livestate

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/dag-andersen/argocd-diff-preview/pkg/argoapplication"
	"github.com/dag-andersen/argocd-diff-preview/pkg/utils"
)

// LiveStateResources holds the live state resources for a given application
type LiveStateResources struct {
	AppId      string
	AppName    string
	Resources  []unstructured.Unstructured
	Namespace  string
}

// FetchLiveStateForApps fetches the live cluster state for the given applications
func FetchLiveStateForApps(apps []argoapplication.ArgoResource, argocdNamespace string) ([]LiveStateResources, error) {
	if len(apps) == 0 {
		return []LiveStateResources{}, nil
	}

	// Get kubeconfig
	kubeConfigPath, exists := utils.GetKubeConfigPath()
	if !exists {
		return nil, fmt.Errorf("kubeconfig not found")
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		// Try in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
		}
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	var liveStates []LiveStateResources

	for _, app := range apps {
		log.Info().Msgf("🔍 Fetching live state for application: %s", app.Name)
		
		// Get the destination namespace from the application
		destNamespace, found, err := unstructured.NestedString(app.Yaml.Object, "spec", "destination", "namespace")
		if err != nil || !found {
			log.Warn().Msgf("⚠️ Could not find destination namespace for application %s, skipping", app.Name)
			continue
		}

		// Check if application exists in the cluster
		appGVR := schema.GroupVersionResource{
			Group:    "argoproj.io",
			Version:  "v1alpha1",
			Resource: "applications",
		}

		appObj, err := dynamicClient.Resource(appGVR).Namespace(argocdNamespace).Get(context.Background(), app.Name, metav1.GetOptions{})
		if err != nil {
			log.Warn().Msgf("⚠️ Application %s not found in cluster, skipping live state fetch", app.Name)
			continue
		}

		// Get managed resources from application status
		resources, err := getManagedResources(appObj, dynamicClient, destNamespace)
		if err != nil {
			log.Warn().Msgf("⚠️ Failed to get managed resources for %s: %v", app.Name, err)
			continue
		}

		liveStates = append(liveStates, LiveStateResources{
			AppId:     app.Id,
			AppName:   app.Name,
			Resources: resources,
			Namespace: destNamespace,
		})

		log.Info().Msgf("✅ Fetched %d resources for application: %s", len(resources), app.Name)
	}

	return liveStates, nil
}

// getManagedResources extracts the managed resources from an ArgoCD Application
func getManagedResources(app *unstructured.Unstructured, dynamicClient dynamic.Interface, namespace string) ([]unstructured.Unstructured, error) {
	var resources []unstructured.Unstructured

	// Get the resources from the application status
	statusResources, found, err := unstructured.NestedSlice(app.Object, "status", "resources")
	if err != nil || !found {
		return resources, fmt.Errorf("no resources found in application status")
	}

	for _, res := range statusResources {
		resMap, ok := res.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract resource metadata
		kind, _, _ := unstructured.NestedString(resMap, "kind")
		name, _, _ := unstructured.NestedString(resMap, "name")
		group, _, _ := unstructured.NestedString(resMap, "group")
		version, _, _ := unstructured.NestedString(resMap, "version")
		resNamespace, _, _ := unstructured.NestedString(resMap, "namespace")

		if name == "" || kind == "" {
			continue
		}

		// Use the resource's namespace if available, otherwise use the app's destination namespace
		if resNamespace == "" {
			resNamespace = namespace
		}

		// Construct GVR
		gvr := schema.GroupVersionResource{
			Group:    group,
			Version:  version,
			Resource: pluralizeKind(kind),
		}

		// Fetch the actual resource from the cluster
		var obj *unstructured.Unstructured
		if resNamespace != "" {
			obj, err = dynamicClient.Resource(gvr).Namespace(resNamespace).Get(context.Background(), name, metav1.GetOptions{})
		} else {
			// Cluster-scoped resource
			obj, err = dynamicClient.Resource(gvr).Get(context.Background(), name, metav1.GetOptions{})
		}

		if err != nil {
			log.Debug().Msgf("Could not fetch resource %s/%s: %v", kind, name, err)
			continue
		}

		resources = append(resources, *obj)
	}

	return resources, nil
}

// pluralizeKind converts a Kubernetes kind to its plural resource name
// This is a simple implementation that handles common cases
func pluralizeKind(kind string) string {
	// Convert to lowercase first
	lower := strings.ToLower(kind)
	
	// Common irregular plurals
	irregulars := map[string]string{
		"ingress":              "ingresses",
		"networkpolicy":        "networkpolicies",
		"podsecuritypolicy":    "podsecuritypolicies",
		"endpoints":            "endpoints",
		"componentstatus":      "componentstatuses",
		"customresourcedefinition": "customresourcedefinitions",
	}

	if plural, ok := irregulars[lower]; ok {
		return plural
	}
	
	// Handle 'y' ending
	if len(lower) > 1 && lower[len(lower)-1] == 'y' {
		return lower[:len(lower)-1] + "ies"
	}
	
	return lower + "s"
}
