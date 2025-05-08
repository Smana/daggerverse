# Pre-Commit Terraform/Opentofu Dagger module

This Dagger module leverages [pre-commit-terraform](https://github.com/antonbabenko/pre-commit-terraform) to validate Terraform configurations.

## Requirements


Here's an example of the pre-commit configuration

```yaml
repos:
  - repo: https://github.com/antonbabenko/pre-commit-terraform.git
    rev: v1.99.0
    hooks:
      - id: terraform_fmt
      - id: terraform_docs
      - id: terraform_validate
      - id: terraform_tfsec
        args:
          - --args=--config-file=__GIT_WORKING_DIR__/.tfsec.yaml
      - id: terraform_tflint
        args:
          - --args=--config=__GIT_WORKING_DIR__/.tflint.hcl
```

## Usage

### Basic Usage

⚠️ Your tf-config path should contain the appropriate pre-commit config (see above)

```bash
dagger call run --dir /path/to/your/tf-config --tf-binary=tofu
```
