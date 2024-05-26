# Kubeconform Dagger Module

This Dagger module provides a way to validate Kubernetes manifests using [kubeconform](https://github.com/yannh/kubeconform). Kubeconform is a tool that validates Kubernetes resources against the Kubernetes OpenAPI specification.

## Features

- Validates Kubernetes manifests against the Kubernetes OpenAPI specification.
- Supports both standalone YAML files and kustomization files.
- Allows excluding specific directories from validation.
- Supports additional schemas located in the `/schemas` directory.

## Usage

To use this module, you need to have `kubeconform` and `kustomize` (if using kustomization files) installed in your environment.

Run the kubeconform Dagger module and provide the directory containing the Flux CRD schemas:

```bash
dagger call validate --manifests /path/to/your/manifests  --exclude "./exclude/dir1,./exclude/file1"
```

In this command, replace `/path/to/your/manifests` with the path to your Kubernetes manifests.


## Options

* `--kustomize` or `-k`: Use `kustomize build` to process kustomization files.
* `--exclude` or `-e`: Exclude specific directories from validation. Directories should be separated by commas.
* `--manifests-dir` or `-d`: Specify the directory to search for manifests. Defaults to the current directory.

## Adding Additional Schemas

Kubeconform by default validates against the Kubernetes OpenAPI specification. However, you might have custom resources in your manifests that are not part of the standard Kubernetes API. In this case, you can add additional schemas for kubeconform to use during validation.

For example, if you are using Flux and want to validate Flux custom resources, you can download the Flux CRD schemas and provide them to kubeconform. Here's how you can do it:

1. Download the Flux CRD schemas and extract them to a directory:

    ```bash
    curl -sL https://github.com/fluxcd/flux2/releases/latest/download/crd-schemas.tar.gz | tar zxf - -C /tmp/schemas/flux-crd-schemas/master-standalone-strict/
    ```

2. Run the kubeconform Dagger module and provide the directory containing the Flux CRD schemas:

    ```bash
    dagger call validate --manifests /home/user/Sources/my-manifests --schemas-dirs /tmp/schemas/flux-crd-schemas --exclude "./infrastructure,./tooling" -vvv
    ```

In this command, replace `/home/user/Sources/my-manifests` with the path to your Kubernetes manifests, and `/tmp/schemas/flux-crd-schemas` with the path to the directory containing the Flux CRD schemas.

The `--schemas-dirs` option allows you to provide one or more directories containing additional schemas. Directories should be separated by commas.

This way, kubeconform will use the additional schemas during validation, allowing it to validate custom resources that are not part of the standard Kubernetes API.


## Error Handling
If `kubeconform` finds invalid resources, the module will print an error message and exit with a non-zero status. This allows the module to be used in scripts and CI/CD pipelines that need to fail when invalid resources are found.