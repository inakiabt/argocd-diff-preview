package argoapplication

import (
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/dag-andersen/argocd-diff-preview/pkg/argocd"
	"github.com/dag-andersen/argocd-diff-preview/pkg/git"
	"github.com/dag-andersen/argocd-diff-preview/pkg/utils"
	"sigs.k8s.io/yaml"
)

func ConvertAppSetsToAppsInBothBranches(
	argocd *argocd.ArgoCDInstallation,
	baseApps []ArgoResource,
	targetApps []ArgoResource,
	baseBranch *git.Branch,
	targetBranch *git.Branch,
	repo string,
	tempFolder string,
	redirectRevisions []string,
	debug bool,
	baseFilterOptions FilterOptions,
	targetFilterOptions FilterOptions,
) ([]ArgoResource, []ArgoResource, time.Duration, error) {
	startTime := time.Now()
	defer func() {
		log.Info().Msgf("🤖 Converting ApplicationSets to Applications in both branches took %s", time.Since(startTime))
	}()

	log.Info().Msg("🤖 Converting ApplicationSets to Applications in both branches")

	baseTempFolder := fmt.Sprintf("%s/%s", tempFolder, git.Base)
	targetTempFolder := fmt.Sprintf("%s/%s", tempFolder, git.Target)

	baseApps, err := processAppSets(
		argocd,
		baseApps,
		baseBranch,
		baseTempFolder,
		debug,
		baseFilterOptions,
		repo,
		redirectRevisions,
	)

	if err != nil {
		log.Error().Str("branch", baseBranch.Name).Msg("❌ Failed to generate base apps")
		return nil, nil, time.Since(startTime), err
	}

	targetApps, err = processAppSets(
		argocd,
		targetApps,
		targetBranch,
		targetTempFolder,
		debug,
		targetFilterOptions,
		repo,
		redirectRevisions,
	)
	if err != nil {
		log.Error().Str("branch", targetBranch.Name).Msg("❌ Failed to generate target apps")
		return nil, nil, time.Since(startTime), err
	}

	return baseApps, targetApps, time.Since(startTime), nil
}

func processAppSets(
	argocd *argocd.ArgoCDInstallation,
	appSets []ArgoResource,
	branch *git.Branch,
	tempFolder string,
	debug bool,
	filterOptions FilterOptions,
	repo string,
	redirectRevisions []string,
) ([]ArgoResource, error) {

	appSetTempFolder := fmt.Sprintf("%s/app-sets", tempFolder)
	if err := utils.CreateFolder(appSetTempFolder, true); err != nil {
		log.Error().Msgf("❌ Failed to create temp folder: %s", appSetTempFolder)
		return nil, err
	}

	apps, err := convertAppSetsToApps(
		argocd,
		appSets,
		branch,
		appSetTempFolder,
		debug,
	)
	if err != nil {
		log.Error().Str("branch", branch.Name).Msg("❌ Failed to generate apps")
		return nil, err
	}

	if len(apps) == 0 {
		return apps, nil
	}

	log.Info().Str("branch", branch.Name).Msgf("🤖 Filtering %d Applications", len(apps))
	apps = FilterAll(apps, filterOptions)

	if len(apps) == 0 {
		log.Info().Str("branch", branch.Name).Msg("🤖 No applications left after filtering")
		return apps, nil
	}

	log.Info().Str("branch", branch.Name).Msgf("🤖 Patching %d Applications", len(apps))
	apps, err = patchApplications(
		argocd.Namespace,
		apps,
		branch,
		repo,
		redirectRevisions,
	)
	if err != nil {
		log.Error().Str("branch", branch.Name).Msgf("❌ Failed to patch Applications on branch: %s", branch.Name)
		return nil, err
	}

	if debug {
		appTempFolder := fmt.Sprintf("%s/apps", tempFolder)
		if err := utils.CreateFolder(appTempFolder, true); err != nil {
			log.Error().Msgf("❌ Failed to create temp folder: %s", appTempFolder)
		}

		for _, app := range apps {
			if _, err := app.WriteToFolder(appTempFolder); err != nil {
				log.Error().Err(err).Str("branch", branch.Name).Str(app.Kind.ShortName(), app.GetLongName()).Msgf("❌ Failed to write Application to file")
				break
			}
		}
	}

	return apps, nil
}

func convertAppSetsToApps(
	argocd *argocd.ArgoCDInstallation,
	appSets []ArgoResource,
	branch *git.Branch,
	tempFolder string,
	debug bool,
) ([]ArgoResource, error) {
	var appsNew []ArgoResource
	appSetCounter := 0
	generatedAppsCounter := 0

	log.Debug().Str("branch", branch.Name).Msg("🤖 Generating Applications from ApplicationSets")

	if debug {
		if err := argocd.EnsureArgoCdIsReady(); err != nil {
			return nil, fmt.Errorf("failed to wait for deployments to be ready: %w", err)
		}
	}

	for _, appSet := range appSets {
		// Skip non-ApplicationSets
		if appSet.Kind != ApplicationSet {
			appsNew = append(appsNew, appSet)
			continue
		}

		appSetCounter++
		localGeneratedAppsCounter := 0

		randomFileName, err := appSet.WriteToFolder(tempFolder)
		if err != nil {
			log.Error().Err(err).Str("branch", branch.Name).Str(appSet.Kind.ShortName(), appSet.GetLongName()).Msgf("❌ Failed to write ApplicationSet to file")
			continue
		}

		// Generate applications using argocd appset generate
		retryCount := 5
		output, err := argocd.AppsetGenerateWithRetry(randomFileName, retryCount)
		if err != nil {
			log.Error().Err(err).Str("branch", branch.Name).Str(appSet.Kind.ShortName(), appSet.GetLongName()).Msg("❌ Failed to generate applications from ApplicationSet")
			return nil, err
		}

		// check if output is empty / null
		if strings.TrimSpace(output) == "" || strings.TrimSpace(output) == "null" {
			log.Warn().Str("branch", branch.Name).Str(appSet.Kind.ShortName(), appSet.GetLongName()).Msgf("⚠️ ApplicationSet generated empty output")
			continue
		}

		// check if output is list of applications
		isList := strings.HasPrefix(output, "-")

		var yamlData []unstructured.Unstructured
		if isList {
			var yamlOutput []unstructured.Unstructured
			if err := yaml.Unmarshal([]byte(output), &yamlOutput); err != nil {
				log.Error().Str("branch", branch.Name).Str(appSet.Kind.ShortName(), appSet.GetLongName()).Msg("❌ Failed to read output from ApplicationSet")
				log.Error().Err(err)
				continue
			}
			yamlData = yamlOutput
		} else {
			var yamlOutput unstructured.Unstructured
			if err := yaml.Unmarshal([]byte(output), &yamlOutput); err != nil {
				log.Error().Str("branch", branch.Name).Str(appSet.Kind.ShortName(), appSet.GetLongName()).Msg("❌ Failed to read output from ApplicationSet")
				log.Error().Err(err)
				continue
			}
			yamlData = []unstructured.Unstructured{yamlOutput}
		}

		if len(yamlData) == 0 {
			log.Error().Str("branch", branch.Name).Str(appSet.Kind.ShortName(), appSet.GetLongName()).Msg("❌ No applications found in ApplicationSet")
			continue
		}

		// Convert each document to ArgoResource
		for _, doc := range yamlData {
			kind := doc.GetKind()
			if kind == "" {
				log.Error().
					Str(appSet.Kind.ShortName(), appSet.GetLongName()).
					Msg("❌ Output from ApplicationSet contains no kind")
				continue
			}
			if kind != "Application" {
				log.Error().
					Str(appSet.Kind.ShortName(), appSet.GetLongName()).
					Msg("❌ Output from ApplicationSet contains non-Application resources")
				continue
			}

			name := doc.GetName()
			if name == "" {
				log.Error().Str(appSet.Kind.ShortName(), appSet.GetLongName()).Msg("❌ Generated Application missing name")
				continue
			}

			// Create a deep copy of the YAML node to avoid reference issues
			docCopy := doc.DeepCopy()

			app := NewArgoResource(docCopy, Application, name, name, appSet.FileName, branch.Type())

			localGeneratedAppsCounter++
			generatedAppsCounter++
			appsNew = append(appsNew, *app)
		}

		log.Debug().Str("branch", branch.Name).Str(appSet.Kind.ShortName(), appSet.GetLongName()).Msgf(
			"Generated %d Applications from ApplicationSet",
			localGeneratedAppsCounter,
		)
	}

	if appSetCounter > 0 {
		log.Info().Str("branch", branch.Name).Msgf(
			"🤖 Generated %d applications from %d ApplicationSets for branch: %s",
			generatedAppsCounter, appSetCounter, branch.Name,
		)
	} else {
		log.Info().Msgf("🤖 No ApplicationSets found for branch: %s", branch.Name)
	}

	return appsNew, nil
}
