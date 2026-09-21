# Changelog

## 0.2.0

- Accept direct `namespace.key=value` input through repeatable `-flattened-tag` options and bulk JSON through `-flattened-tags`.
- Validate flattened references and require string values before making OCI API calls.
- Add Bash, PowerShell, and JSON examples plus unit tests for flattened tags.

## 0.1.0

- Add multiregion discovery for every subscribed OCI region in `READY` state.
- Apply free-form and defined tags to Compute instances, boot volumes, and block volumes.
- Preserve existing tags and use ETags for optimistic concurrency control.
- Add bounded region and resource worker pools with OCI SDK retry handling.
- Add dry-run behavior, unit tests, runnable examples, IAM policy samples, and an OCI-stencil architecture diagram.
