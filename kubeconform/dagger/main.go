// This Dagger module is designed to validate Kubernetes manifests using a tool called kubeconform.
//
// kubeconform is a tool that validates Kubernetes resources against the Kubernetes OpenAPI specification. It checks if Kubernetes manifests (YAML or JSON files containing Kubernetes resources) conform to the specification.
//
// - Validates standalone YAML files and kustomization files.
//
// - Excludes specific directories from validation.
//
// - Converts CRDs into JSONSchemas.
//
// - Supports additional schemas from the Datree Catalog
//
// - Support Flux variables substitution.
//
// Refer to the Readme for more information on how to use this module: https://github.com/Smana/daggerverse/tree/main/kubeconform

package main

import (
	"context"
	"dagger/kubeconform/internal/dagger"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/mholt/archives"
)

type Kubeconform struct {
	// Kubeconform version to use for validation.
	// +optional
	// +default="v0.6.7"
	Version string
}

// kubeConformImage returns a container image with the required packages and tools to run kubeconform.
func kubeConformImage(kubeconform_version string, flux bool, fluxVersion string, env []string) (*dagger.Container, error) {
	ctr := dag.Container().From("alpine:latest").
		WithExec([]string{"apk", "add", "bash", "curl", "kustomize", "git", "python3", "py3-pip", "yq", "py3-yaml"})

	// Download the kubeconform archive and extract the binary into a dagger *File
	kubeconformBin := dag.Arc().
		Unarchive(dag.HTTP(fmt.Sprintf("https://github.com/yannh/kubeconform/releases/download/%s/kubeconform-linux-amd64.tar.gz", kubeconform_version)).
			WithName("kubeconform-linux-amd64.tar.gz")).File("kubeconform-linux-amd64/kubeconform")

	// Download the openapi2jsonschema.py script and return a dagger *File
	openapi2jsonschemaScript := dag.HTTP(fmt.Sprintf("https://raw.githubusercontent.com/yannh/kubeconform/%s/scripts/openapi2jsonschema.py", kubeconform_version))

	if flux {
		// Add the flux binary to the container
		fluxBin := dag.Arc().
			Unarchive(dag.HTTP(fmt.Sprintf("https://github.com/fluxcd/flux2/releases/download/v%s/flux_%s_linux_amd64.tar.gz", fluxVersion, fluxVersion)).
				WithName(fmt.Sprintf("flux_%s_linux_amd64.tar.gz", fluxVersion))).File("flux")

		ctr = ctr.WithFile("/bin/flux", fluxBin, dagger.ContainerWithFileOpts{Permissions: 0750})
	}

	// Add the environment variables to the container
	for _, e := range env {
		parts := strings.Split(e, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid flux variable format, must be in the form <key>:<value>: %s", e)
		}
		ctr = ctr.WithEnvVariable(parts[0], parts[1])
	}

	ctr = ctr.
		WithFile("/bin/kubeconform", kubeconformBin, dagger.ContainerWithFileOpts{Permissions: 0750}).
		WithFile("/bin/openapi2jsonschema.py", openapi2jsonschemaScript, dagger.ContainerWithFileOpts{Permissions: 0750})
	return ctr, nil
}

func isGitRepo(target_url string) (bool, error) {
	parsedURL, err := url.Parse(target_url)
	if err != nil {
		return false, err
	}

	if parsedURL.Hostname() != "github.com" {
		return false, nil
	}

	pathParts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
	if len(pathParts) >= 2 && pathParts[0] != "" && pathParts[1] != "" {
		if len(pathParts) > 2 && (pathParts[2] == "blob" || pathParts[2] == "releases") {
			return false, nil
		}
		return true, nil
	}

	return false, nil
}

// parseGitURL parses the GitHub URL to extract the repository URL, branch, and subdirectory
func parseGitURL(gitURL string) (string, string, string, error) {
	parsedURL, err := url.Parse(gitURL)
	if err != nil {
		return "", "", "", err
	}

	parts := strings.Split(parsedURL.Path, "/")
	if len(parts) < 5 {
		return "", "", "", fmt.Errorf("invalid URL format")
	}

	owner := parts[1]
	repo := parts[2]
	branch := parts[4]
	subdir := strings.Join(parts[5:], "/")

	repoURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)

	return repoURL, branch, subdir, nil
}

func isAnArchive(ctx context.Context, url string) (bool, error) {
	resp, err := http.Get(url)
	if err != nil {
		return false, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("received non-200 status: %s", resp.Status)
	}

	// Identify the archive format
	format, _, err := archives.Identify(ctx, url, resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to identify archive format: %w", err)
	}

	// Check if the identified format is an extractor
	if _, ok := format.(archives.Extractor); ok {
		return true, nil
	}

	return false, nil
}

// crdDirs creates a list of directories containing the CRDs schemas to validate against.
func crdDirs(ctx context.Context, crdURLs []string) ([]*dagger.Directory, error) {
	var dirs []*dagger.Directory
	for _, crdURL := range crdURLs {
		isRepo, err := isGitRepo(crdURL)
		if err != nil {
			return nil, fmt.Errorf("failed to check if the URL is a git repository: %v", err)
		}

		// Depending on the URL format it will create a different directory
		// If it is a git repository it will clone the repository and return the directory
		// If it is an archive it will download the archive extract it and return the directory
		// Otherwise, it will download the file and return the directory
		if isRepo {
			repoURL, branch, subdir, err := parseGitURL(crdURL)
			if err != nil {
				return nil, fmt.Errorf("failed to parse the git URL: %v", err)
			}
			dir := dag.Git(repoURL).Branch(branch).Tree()
			dirs = append(dirs, dir.Directory(subdir))
		} else {
			isArchive, err := isAnArchive(ctx, crdURL)
			if err != nil {
				return nil, fmt.Errorf("failed to check if the URL is an archive: %v", err)
			}
			if isArchive {
				dir := dag.Arc().Unarchive(dag.HTTP(crdURL).WithName(path.Base(crdURL)))
				dirs = append(dirs, dir)
			} else {
				dir := dag.Directory().WithFile(path.Base(crdURL), dag.HTTP(crdURL))
				dirs = append(dirs, dir)
			}
		}
	}
	return dirs, nil
}

// Validate the Kubernetes manifests in the provided directory and optional source CRDs directories
func (m *Kubeconform) Validate(
	ctx context.Context,

	// Kubeconform version to use for validation.
	// +optional
	// +default="v0.6.7"
	version string,

	// Base directory to walk through in order to validate Kubernetes manifests.
	manifests *dagger.Directory,

	// kustomize if set to true it will look for kustomization.yaml files and validate them otherwise it will validate all the YAML files in the directory.
	// +optional
	kustomize bool,

	// exclude is string listing directories or files to exclude from the validation separated by commas (example: "./terraform,.gitignore").
	// +optional
	exclude string,

	// catalog is a boolean that if set to true it will use the Datree catalog to validate the manifests. (ref: https://github.com/datreeio/CRDs-catalog)
	// +optional
	// +default=false
	catalog bool,

	// flux is a boolean that if set to true it will download the flux binary.
	// +optional
	// +default=false
	flux bool,

	// fluxVersion is the version of the flux binary to download.
	// +optional
	// +default="2.5.1"
	fluxVersion string,

	// crds is a list of URLs containing the CRDs to validate against.
	// +optional
	crds []string,

	// a list of environment variables, expected in (key:value) format
	// +optional
	env []string,
) (string, error) {
	if manifests == nil {
		manifests = dag.Directory().Directory(".")
	}

	ctr, err := kubeConformImage(version, flux, fluxVersion, env)
	if err != nil {
		return "", err
	}

	crdDirs, err := crdDirs(ctx, crds)
	if err != nil {
		return "", fmt.Errorf("failed to create the schemas directories: %v", err)
	}

	// Mount all the CRDs schemas directories into the container
	for idx, dir := range crdDirs {
		ctr = ctr.WithMountedDirectory(fmt.Sprintf("/crds/%s", strconv.Itoa(idx)), dir)
	}

	// Mount the manifests and kubeconform binary
	ctr = ctr.WithMountedDirectory("/work", manifests).
		WithWorkdir("/work")

	// Create the script
	scriptContent := `#!/bin/bash
set -e
set -o pipefail

kustomize=0
manifests_dir="."

options=$(getopt -o kd: --long kustomize,flux,catalog,exclude:,manifests-dir: -- "$@")
eval set -- "$options"

while true; do
  case $1 in
    --kustomize|-k)
      kustomize=1
      shift
      ;;
    --flux)
      flux=1
      shift
      ;;
    --catalog)
      catalog=1
      shift
      ;;
    --exclude)
      exclude=$2
      shift 2
      ;;
    --manifests-dir|-d)
      manifests_dir=$2
      shift 2
      ;;
    --)
      shift
      break
      ;;
    *)
      echo "Invalid option: $1" 1>&2
      exit 1
      ;;
  esac
done

find_manifests() {
  local dir=$1
  local search_patterns=$2
  local exclude_string=$3
  local IFS=','

  read -r -a pattern_array <<< "$search_patterns"
  read -r -a exclude_array <<< "$exclude_string"

  find_command="find $dir"

  for exclude in "${exclude_array[@]}"; do
    find_command+=" -path '${exclude// /}' -prune -o"
  done

  find_command+=" \("
  for pattern in "${pattern_array[@]}"; do
    find_command+=" -name '${pattern// /}' -o"
  done
  find_command="${find_command% -o} \) -type f -print"

  eval "$find_command"
}

# Convert all CRDs to JSON schemas
mkdir -p /schemas
if [ -d /crds ]; then
  find /crds -type f \( -name "*.yaml" -o -name "*.yml" \) -print0 | while IFS= read -r -d $'\0' file; do
    if yq e '.kind == "CustomResourceDefinition"' "$file"; then
      echo "Converting $file to JSON Schema"
      openapi2jsonschema.py "$file"
    fi
  done
  mv ./*.json "/schemas/"
fi

ARGS=("-summary" "--strict" "-ignore-missing-schemas" "-schema-location" "default")

# Add the schemas directories to the kubeconform arguments if they exist
if [ -n "$(find $1 -type f -print -quit)" ]; then
  ARGS+=("--schema-location" "/schemas/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json")
fi
# Add the Datree catalog if enabled
if [ $catalog -eq 1 ]; then
  ARGS+=("--schema-location" "https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json")
fi

if [ $kustomize -eq 1 ]; then
  for file in $(find_manifests "$manifests_dir" "kustomization.yaml,kustomization.yml" "$exclude"); do
    echo "Processing kustomization file: $file"
    if [ $flux -eq 1 ]; then
        if ! kustomize build $(dirname $file) | flux envsubst | kubeconform ${ARGS[@]} -; then
          echo "Validation failed for $file"
          exit 1
        fi
    else
        if ! kustomize build $(dirname $file) | kubeconform ${ARGS[@]} -; then
        echo "Validation failed for $file"
        exit 1
        fi
    fi
    echo "Validation successful for $file"
  done
else
  for file in $(find_manifests "$manifests_dir" "*.yaml,*.yml" "$exclude"); do
    echo "Processing file: $file"
    if ! kubeconform "${ARGS[@]}" $file; then
      echo "Validation failed for $file"
      exit 1
    fi
    echo "Validation successful for $file"
  done
fi
`

	// Add the manifests and the script to the container
	ctr = ctr.
		WithMountedDirectory("/work", manifests).
		WithNewFile("/work/run_kubeconform.sh", scriptContent, dagger.ContainerWithNewFileOpts{Permissions: 0750})
	// Execute the script
	kubeconform_command := []string{"bash", "/work/run_kubeconform.sh"}
	if kustomize {
		kubeconform_command = append(kubeconform_command, "--kustomize")
	}
	if flux {
		kubeconform_command = append(kubeconform_command, "--flux")
	}
	if catalog {
		kubeconform_command = append(kubeconform_command, "--catalog")
	}
	if exclude != "" {
		kubeconform_command = append(kubeconform_command, "--exclude", exclude)
	}
	stdout, err := ctr.WithExec(kubeconform_command).Stdout(ctx)
	if err != nil {
		return "", fmt.Errorf("validation failed: %v", err)
	}

	return stdout, nil
}
