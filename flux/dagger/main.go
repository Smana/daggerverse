// This module checks the validity of Kubernetes manifests in a given directory using kubeconform.

package main

import (
	"context"
	"fmt"
	"path/filepath"
)

type Flux struct {
	// Base directory to walk through in order to validate Kubernetes manifests.
	// +default="."
	KustomizeDir *Directory

	// Kubeconform version to use for validation.
	// +default="v0.6.4"
	KubeconformVersion string
}

func New(

	// Base directory to walk through in order to validate Kubernetes manifests.
	// +optional
	kustomizeDir *Directory,

	// Kubeconform version to use for validation.
	// +optional
	// +default="v0.6.4"
	kubeconformVersion string,
) *Flux {
	if kustomizeDir == nil {
		kustomizeDir = dag.Directory().Directory(".")
	}
	if kubeconformVersion == "" {
		kubeconformVersion = "v0.6.4"
	}
	if kustomizeDir == nil {
		kustomizeDir = dag.Directory().Directory(".")
	}
	flux := &Flux{
		KustomizeDir:       kustomizeDir,
		KubeconformVersion: kubeconformVersion,
	}
	return flux
}

func containerWithRequirements() *Container {
	var packages = []string{"bash", "curl", "git", "kustomize", "yq"}
	return dag.Apko().Wolfi(packages)
}

// Extract file from a given archive
func extractFileFromURL(archiveURL string, filePath string) (*File, error) {
	ctr := containerWithRequirements()

	// retrieve basedir of the filePath into a variable binDir
	fileDir := filepath.Dir(filePath)

	return ctr.
		WithWorkdir("/usr/local/bin").
		WithFile("out.tgz", dag.HTTP(archiveURL)).
		WithExec([]string{"tar", "-xvzf", "out.tgz", "-C", fileDir}).
		WithExec([]string{"ls", "-l", "/usr/local/bin"}).
		File(filePath), nil
}

func extractToDirFromURL(archiveURL string, dirPath string) (*Directory, error) {
	ctr := containerWithRequirements()

	return ctr.
		WithWorkdir("/work").
		WithFile("out.tgz", dag.HTTP(archiveURL)).
		WithExec([]string{"mkdir", "-p", dirPath}).
		WithExec([]string{"tar", "-xvzf", "out.tgz", "-C", dirPath}).
		Directory(dirPath), nil
}

// Walk through a given directory and check that the manifests are valid
func (f *Flux) Validate(
	ctx context.Context,
	// +optional
	// +default="v0.6.4"
	kubeconformVersion string,

	kustomizeDir *Directory,

	clustersDir *Directory,
) (string, error) {

	kubeconformBin, err := extractFileFromURL(fmt.Sprintf("https://github.com/yannh/kubeconform/releases/download/%s/kubeconform-linux-amd64.tar.gz", kubeconformVersion), "/usr/local/bin/kubeconform")
	if err != nil {
		return "", fmt.Errorf("Cannot extract Kubeconform binary: %v", err)
	}

	fluxSchemasDir, err := extractToDirFromURL("https://github.com/fluxcd/flux2/releases/latest/download/crd-schemas.tar.gz", "/work/flux-crd-schemas/master-standalone-strict")
	if err != nil {
		return "", fmt.Errorf("Cannot extract Flux CRD schemas: %v", err)
	}
	ctr := containerWithRequirements()

	return ctr.
		WithWorkdir("/work").
		WithMountedDirectory("/kustomize", f.KustomizeDir).
		WithMountedDirectory("/clusters", clustersDir).
		WithFile("/work/kubeconform", kubeconformBin, ContainerWithFileOpts{Permissions: 0750}).
		WithMountedDirectory("/flux-crd-schemas/master-standalone-strict", fluxSchemasDir).
		WithNewFile("/work/run_kubeconform.sh", ContainerWithNewFileOpts{
			Permissions: 0750,
			Contents: `#!/bin/bash
# Process all YAML files in the given directory with kubeconform
set -e

# Define excluded directories and ignored files for find command
excluded_directories=("*/terraform/*" "*/.github/*")
ignored_files=(".tfsec.yaml" ".pre-commit-config.yaml")

process_file() {
  echo "Processing file: $1"
  /work/kubeconform -strict -summary -ignore-missing-schemas -schema-location default --schema-location /flux-crd-schemas $1
  if [ $? -ne 0 ]; then
    exit 1
  fi
}

export -f process_file

echo -e "\n\e[32m✔\e[0m Validating Flux clusters manifests with kubeconform"
for file in $(find /clusters -type f -name "*.y*ml" ! \( -path "${excluded_directories[0]}" -o -path "${excluded_directories[1]}" -o -name "${ignored_files[0]}" -o -name "${ignored_files[1]}" \)); do
  bash -c 'process_file "$0"' $file || exit 1
done

echo -e "\n\e[32m✔\e[0m Validating Kustomization manifests with kubeconform"
for file in $(find /kustomize -type f -name "kustomization.yaml" ! \( -path "${excluded_directories[0]}" -o -path "${excluded_directories[1]}" -o -name "${ignored_files[0]}" -o -name "${ignored_files[1]}" \)); do
  echo "Processing kustomization.yaml file: $file"
  kustomize build $(dirname $file) | /work/kubeconform -strict -summary -ignore-missing-schemas -schema-location default --schema-location /flux-crd-schemas -
  if [ $? -ne 0 ]; then
    exit 1
  fi
done
`,
		}).
		WithExec([]string{"bash", "run_kubeconform.sh", "."}).
		Stdout(ctx)
}
