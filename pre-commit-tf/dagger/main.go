// This module is responsible for running pre-commit-terraform on a given directory
// It uses the official pre-commit-terraform Docker image to run the checks
// The module supports specifying the version of pre-commit-terraform to run, the directory to run it in, and the Terraform/Opentofu binary to use
// It also supports caching Terraform plugins to speed up the execution
// The module returns the output of pre-commit-terraform as a string

package main

import (
	"context"
	"fmt"
)

const (
	PreCommitTfImage = "ghcr.io/antonbabenko/pre-commit-terraform"
)

type PreCommitTf struct{}

// Run runs pre-commit-terraform on the given directory
func (m *PreCommitTf) Run(
	ctx context.Context,

	// Version of pre-commit-terraform to run
	// +optional
	// +default="v1.92.0"
	version string,

	// Directory to run pre-commit-terraform in
	dir *Directory,

	// Choose between "terraform" or "tofu"
	// +optional
	// +default="terraform"
	tfBinary string,

	// cache is a directory to use as a cache for Terraform plugins
	// +optional
	cache_dir *Directory,
) (string, error) {

	ctr := dag.Container().
		From(fmt.Sprintf("%s:%s", PreCommitTfImage, version))

	if cache_dir != nil {
		ctr = ctr.
			WithMountedDirectory("/tf_cache", cache_dir).
			WithEnvVariable("TF_PLUGIN_CACHE_DIR", "/tf_cache")
	}

	return ctr.WithEnvVariable("PCT_TFPATH=", tfBinary).
		WithMountedDirectory("/mnt", dir).
		WithWorkdir("/mnt").
		WithExec([]string{"run", "-a"}).
		Stdout(ctx)
}
