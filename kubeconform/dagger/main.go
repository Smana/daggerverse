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
	// +default="v0.6.4"
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
	// +default="v0.6.4"
	version string,

	// Base directory to walk through in order to validate Kubernetes manifests.
	manifests *Directory,

	// kustomize if set to true it will look for kustomization.yaml files and validate them otherwise it will validate all the YAML files in the directory.
	// +optional
	kustomize bool,

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

exclude_dirs=()
exclude_files=()
kustomize=0
manifests_dir="."

options=$(getopt -o kd: --long kustomize,exclude-dirs:,exclude-files:,manifests-dir: -- "$@")
eval set -- "$options"

while true; do
  case $1 in
    --kustomize|-k)
      kustomize=1
      shift
      ;;
    --exclude-dirs)
      IFS=',' read -ra exclude_dirs <<< "$2"
      shift 2
      ;;
    --exclude-files)
      IFS=',' read -ra exclude_files <<< "$2"
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

generate_exclusions() {
  local dirs=("$@")
  local files=("${!#}")
  local exclusions=""

  for dir in "${dirs[@]}"; do
    exclusions+=" -o -path $dir"
  done
  for file in "${files[@]}"; do
    exclusions+=" -o -name $file"
  done

  exclusions=${exclusions:3}
  echo "$exclusions"
}

exclusions=$(generate_exclusions "${exclude_dirs[@]}" "${exclude_files[@]}")

ARGS=("-summary" "--strict" "-ignore-missing-schemas" "-schema-location" "default")
EXTRA_SCHEMAS_LOCATIONS=( $(if [ -d //schemas ]; then find /schemas -mindepth 1 -maxdepth 1 -type d; fi) )
for dir in "${EXTRA_SCHEMAS_LOCATIONS[@]}"; do
  ARGS+=("--schema-location" "$dir")
done

if [ $kustomize -eq 1 ]; then
  for file in $(find $manifests_dir -type f \( -name "kustomization.yaml" -o -name "kustomization.yml" \) -not \( $exclusions \)); do
    echo "Processing kustomization file: $file"
    kustomize build $(dirname $file) | /work/kubeconform ${ARGS[@]} -
    if [ $? -eq 0 ]; then
      echo "Validation successful for $file"
    else
      echo "Validation failed for $file"
      exit 1
    fi
  done
else
  for file in $(find $manifests_dir -type f \( -name "*.yaml" -o -name "*.yml" \) -not \( $exclusions \)); do
    echo "Processing file: $file"
    /work/kubeconform "${ARGS[@]}" $file
    if [ $? -ne 0 ]; then
      echo "Validation failed for $file"
      exit 1
    fi
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
	stdout, err = ctr.WithExec([]string{"bash", "/work/run_kubeconform.sh", "--exclude-dirs", "*/terraform,/.github"}).Stdout(ctx)
	if err != nil {
		return "", fmt.Errorf("validation failed: %v\n", err)
	}

	return stdout, nil
}
