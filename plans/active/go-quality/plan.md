# Go quality setup

Implement root AGENTS.md focused on agent execution, explicit broad Go linting,
Linux GitHub Actions, robust package documentation and examples, and Apache-2.0.
No contributor policy documents. Preserve library behavior and standard-library
runtime dependencies. CI must not use Jev credentials or make live API calls.

Acceptance: patched Go 1.26 and stable tests/builds pass; format, configured lint,
workflow, Markdown, vulnerability, and module tidiness checks pass. All exported
APIs are documented and executable examples remain offline. Hosted CI execution
awaits a remote and push; this task does not publish the repository.
