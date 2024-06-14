# Kubeconform Dagger Module

This Dagger module validates Kubernetes manifests using [kubeconform](https://github.com/yannh/kubeconform), a tool that checks Kubernetes resources against the Kubernetes OpenAPI specification.

## Features

- Validates standalone YAML files and kustomization files.
- Excludes specific directories from validation.
- Converts CRDs into JSONSchemas.
- Supports additional schemas from the [Datree Catalog](https://github.com/datreeio/CRDs-catalog)


## Usage

### Basic Usage

```bash
dagger call -m github.com/Smana/daggerverse/kubeconform@v0.0.4 validate --manifests /path/to/your/manifests --exclude "./exclude/dir1,./exclude/file1"
```

Replace /path/to/your/manifests with the path to your Kubernetes manifests.


### Advanced Usage

If you're using [**flux**](https://github.com/fluxcd/flux2) variables substitution, you can add environment variables:

```bash
dagger call -m github.com/Smana/daggerverse/kubeconform@v0.0.4 validate --manifests /path/to/your/manifests \
--crds https://github.com/kubernetes-sigs/gateway-api/tree/main/config/crd,https://github.com/external-secrets/external-secrets/tree/main/config/crds/bases \
--exclude ".github/*" --kustomize --env "domain_name:cluster.local,cluster_name:foobar,region:eu-west-3" --flux
```

You can also use the Datree catalog, which contains commonly used custom resources JSONSchemas:

```bash
dagger call -m github.com/Smana/daggerverse/kubeconform@v0.0.4 validate --manifests /path/to/your/manifests --catalog
```


This module handles **any CRDs**. You can provide an HTTP URL pointing to a git directory, a tarball, or a direct link. The module will **convert** these CRDs to JSONSchemas:

```bash
dagger call -m github.com/Smana/daggerverse/kubeconform@v0.0.4 validate --manifests /path/to/your/manifests \
--crds https://github.com/kubernetes-sigs/gateway-api/tree/main/config/crd,http://another.crd.url
```

In this command, replace `/path/to/your/manifests` with the path to your Kubernetes manifests.


## Options

* `--kustomize` or `-k`: Processes kustomization files using `kustomize build`.
* `--exclude` or `-e`: Excludes specific directories from validation. Separate directories with commas.
* `--manifests-dir` or `-d`: Specifies the directory to search for manifests. Defaults to the current directory.
* `--flux`: Installs flux to handle variable substitutions.
* `--catalog`: Uses the Datree Catalog to validate Kubernetes manifests.
* `--env` or `-e`: Adds a list of environment variables to the running container. Useful for flux variables substitution.
