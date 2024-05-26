// This module checks the validity of Kubernetes manifests in a given directory using kubeconform.

package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
)

type Kubeconform struct {
	// Kubeconform version to use for validation.
	// +optional
	// +default="v0.6.6"
	Version string
}

// baseImage returns a container image with the required packages
func baseImage() (*Container, error) {
	ctr := dag.Apko().Wolfi([]string{"bash", "curl", "kustomize", "git", "yq"})
	return ctr, nil
}

// extractFileFromURL extracts a file from a given archive
func extractFileFromURL(archiveURL string, filePath string) (*File, error) {
	ctr, err := baseImage()
	if err != nil {
		return nil, err
	}

	dag.Apko().Wolfi([]string{})

	// retrieve basedir of the filePath into a variable binDir
	fileDir := filepath.Dir(filePath)

	return ctr.
		WithWorkdir("/usr/local/bin").
		WithFile("out.tgz", dag.HTTP(archiveURL)).
		WithExec([]string{"tar", "-xvzf", "out.tgz", "-C", fileDir}).
		File(filePath), nil
}

// Validate the Kubernetes manifests in the provided directory and optional source CRDs directories
func (m *Kubeconform) Validate(
	ctx context.Context,

	// Kubeconform version to use for validation.
	// +optional
	// +default="v0.6.6"
	version string,

	// Base directory to walk through in order to validate Kubernetes manifests.
	manifests *Directory,

	// kustomize if set to true it will look for kustomization.yaml files and validate them otherwise it will validate all the YAML files in the directory.
	// +optional
	kustomize bool,

	// exclude is string listing directories or files to exclude from the validation separated by commas (example: "./terraform,.gitignore").
	// +optional
	exclude string,

	// schemaDirs is a list of directories containing the CRDs to validate against.
	// +optional
	schemasDirs ...*Directory,
) (string, error) {
	if manifests == nil {
		manifests = dag.Directory().Directory(".")
	}

	// Download and extract the kubeconform binary given the version
	kubeconformBin, err := extractFileFromURL(fmt.Sprintf("https://github.com/yannh/kubeconform/releases/download/%s/kubeconform-linux-amd64.tar.gz", version), "/usr/local/bin/kubeconform")
	if err != nil {
		return "", fmt.Errorf("cannot extract kubeconform binary: %v", err)
	}

	ctr, err := baseImage()
	if err != nil {
		return "", err
	}

	// Mount all the CRDs schemas directories into the container
	for idx, dir := range schemasDirs {
		ctr = ctr.WithMountedDirectory(fmt.Sprintf("/schemas/%s", strconv.Itoa(idx)), dir)
	}

	// Mount the manifests and kubeconform binary
	ctr = ctr.WithMountedDirectory("/work", manifests).
		WithWorkdir("/work").
		WithFile("/work/kubeconform", kubeconformBin, ContainerWithFileOpts{Permissions: 0750})

	// Create the script
	scriptContent := `#!/bin/bash
set -e
set -o pipefail

kustomize=0
manifests_dir="."

options=$(getopt -o kd: --long kustomize,exclude:,manifests-dir: -- "$@")
eval set -- "$options"

while true; do
  case $1 in
    --kustomize|-k)
      kustomize=1
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

ARGS=("-summary" "--strict" "-ignore-missing-schemas" "-schema-location" "default")
EXTRA_SCHEMAS_LOCATIONS=( $(if [ -d //schemas ]; then find /schemas -mindepth 1 -maxdepth 1 -type d; fi) )
for dir in "${EXTRA_SCHEMAS_LOCATIONS[@]}"; do
  ARGS+=("--schema-location" "$dir")
done

if [ $kustomize -eq 1 ]; then
  for file in $(find_manifests "$manifests_dir" "kustomization.yaml,kustomization.yml" "$exclude"); do
    echo "Processing kustomization file: $file"
    if ! kustomize build $(dirname $file) | /work/kubeconform ${ARGS[@]} -; then
      echo "Validation failed for $file"
      exit 1
    fi
    echo "Validation successful for $file"
  done
else
  for file in $(find_manifests "$manifests_dir" "*.yaml,*.yml" "$exclude"); do
    echo "Processing file: $file"
    if ! /work/kubeconform "${ARGS[@]}" $file; then
      echo "Validation failed for $file"
      exit 1
    fi
    echo "Validation successful for $file"
  done
fi
`

	// Add the script content to the container
	ctr = ctr.WithNewFile("/work/run_kubeconform.sh", ContainerWithNewFileOpts{
		Permissions: 0750,
		Contents:    scriptContent,
	})

	// Verify the script exists before running it
	stdout, err := ctr.WithExec([]string{"ls", "-l", "/work/run_kubeconform.sh"}).Stdout(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to verify the script existence: %v", err)
	}

	// Execute the script
	kubeconform_command := []string{"bash", "/work/run_kubeconform.sh"}
	if kustomize {
		kubeconform_command = append(kubeconform_command, "--kustomize")
	}
	if exclude != "" {
		kubeconform_command = append(kubeconform_command, "--exclude", exclude)
	}
	stdout, err = ctr.WithExec(kubeconform_command).Stdout(ctx)
	if err != nil {
		return "", fmt.Errorf("validation failed: %v\n", err)
	}

	return stdout, nil
}
