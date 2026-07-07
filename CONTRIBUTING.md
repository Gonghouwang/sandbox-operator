# Contributing to Sandbox Operator

Thank you for your interest in contributing to Sandbox Operator! This document explains how to get started and what to expect when contributing.

## Getting Started

1. Fork the repository and clone your fork locally.
2. Make sure you have [Go](https://golang.org/dl/) 1.26 or later installed.
3. Have a Kubernetes cluster available for testing (kind, minikube, or a cloud cluster).

## Development Workflow

1. Create a feature branch from the latest `master` (or main) branch.

   ```bash
   git checkout -b feat/my-feature
   ```

2. Make your changes, including tests where appropriate.
3. Run the test suite and code checks:

   ```bash
   make test
   make vet
   # or run both formatting and vet checks
   make lint
   ```

4. Build the manager binary to verify compilation:

   ```bash
   make build
   ```

5. Build the container image if your change affects the Dockerfile or runtime behavior:

   ```bash
   make docker-build IMG=sandbox-operator:dev
   ```

6. Commit your changes with a clear commit message. We follow the [Conventional Commits](https://www.conventionalcommits.org/) style:

   ```text
   feat: add support for custom poll intervals
   fix: handle nil pointer in sandbox status update
   docs: update deployment guide with cert-manager example
   chore: bump controller-runtime to v0.24.2
   ```

## Pull Request Process

1. Ensure your branch is up to date with the upstream `master` branch.
2. Open a pull request with a clear title and description.
3. Fill out the pull request template if one is provided.
4. Link any related issues using keywords like `Closes #123`.
5. Keep the scope of a single PR focused. Split large changes into multiple PRs when possible.
6. Be responsive to review feedback.

## Code Style

* Format Go code with `gofmt`.
* Follow the existing package structure and naming conventions.
* Keep public APIs backward-compatible when possible; breaking changes require discussion.
* Add comments to exported types and functions in Go.
* Write user-facing documentation updates when behavior changes.

## Documentation

* User-facing documentation lives in `docs/en/` (primary) and `docs/zh-CN/` (Chinese).
* Update the English docs for any behavior change; Chinese docs may be updated in the same PR or a follow-up.
* Raw manifest README: `config/deploy/README.md` should stay in sync with `docs/en/deployment.md`.

## Testing

* Add unit tests for new controller, webhook, or client logic.
* Run `make test` before submitting a PR.
* For integration testing, deploy the operator in a test cluster and exercise the CR workflows.

## Reporting Issues

When reporting bugs, please include:

* A clear description of the problem.
* Steps to reproduce.
* Expected vs. actual behavior.
* Operator version, Kubernetes version, and relevant logs.
* A minimal YAML example if the issue involves CR processing.

## Community

* Be respectful and constructive. See [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md).
* For security issues, follow the process in [`SECURITY.md`](SECURITY.md).

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
