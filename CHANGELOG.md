# Changelog

## 0.4.0

- Add explicit `-vcns`, `-subnets`, and `-security-lists` selection flags and tag updates across READY regions, while keeping the existing no-flag resource scope.
- Preserve existing network tags and use ETags; omit non-tag network settings and security rules from update requests.
- Document network resource examples, IAM policies, and manual rollback guidance.

## 0.3.0

- Add `-compute`, `-boot-volumes`, and `-block-volumes` flags to select which resource types are discovered and tagged; preserve all-types behavior when no selection flag is supplied.
- Document separate dry-run and apply examples for compute instances, boot volumes, and block volumes.

## 0.2.1

- Document a complete tagging example for compute instances, boot volumes, and block volumes.
- Clarify preservation of existing tags and the manual rollback procedure.

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
