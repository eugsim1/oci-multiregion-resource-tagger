# OCI Multiregion Resource Tagger

Apply free-form tags, defined tags, or both to OCI Compute instances, boot volumes, and block volumes in one compartment across every subscribed region in `READY` state.

> This project is independent and is not affiliated with, endorsed by, or supported by Oracle or any Oracle product team.

The program is safe to inspect before use: it defaults to dry-run mode, preserves existing tags, skips terminating resources, uses ETags to protect concurrent changes, and limits API concurrency through configurable worker pools.

![OCI multiregion tagging architecture](docs/architecture/oci-multiregion-resource-tagger.svg)

## What it tags

For the specified compartment, the program discovers and processes these resources in every subscribed OCI region whose subscription status is `READY`:

| Resource | OCI SDK operations |
| --- | --- |
| Compute instances | `ListInstances`, `GetInstance`, `UpdateInstance` |
| Boot volumes | `ListBootVolumes`, `GetBootVolume`, `UpdateBootVolume` |
| Block volumes | `ListVolumes`, `GetVolume`, `UpdateVolume` |

The compartment itself is global, but the resources are regional. Child compartments are not traversed.

## Behavior

- Existing free-form and defined tags are retained.
- A supplied value replaces an existing value only when the namespace/key or free-form key matches.
- Defined-tag namespaces and tag definitions must already exist.
- The program does not create, rename, or delete tag namespaces or tag definitions.
- `TERMINATING` and `TERMINATED` resources are skipped.
- Dry-run is the default. The `-apply` flag is required to make changes.
- ETags are passed with updates, so an update fails instead of silently overwriting a resource changed after it was read.
- The OCI SDK default retry policy handles retryable throttling and transient service errors with backoff.

## Prerequisites

- Go 1.21 or later.
- An OCI tenancy and the OCID of the target compartment.
- An OCI SDK configuration in the standard `DEFAULT` profile, normally at:
  - Linux/macOS: `~/.oci/config`
  - Windows: `%USERPROFILE%\.oci\config`
- IAM permissions for region discovery, resource updates, and any defined-tag namespaces used.

Never place OCI private keys or configuration files inside this repository.

## Install and build

```bash
git clone https://github.com/eugsim1/oci-multiregion-resource-tagger.git
cd oci-multiregion-resource-tagger
go mod download
go test ./...
go build -o oci-multiregion-resource-tagger .
```

On Windows PowerShell:

```powershell
go mod download
go test ./...
go build -o oci-multiregion-resource-tagger.exe .
```

## IAM policies

Replace `ResourceTaggers` and `TargetCompartment` with your group and compartment names:

```text
Allow group ResourceTaggers to inspect tenancies in tenancy
Allow group ResourceTaggers to use instances in compartment TargetCompartment
Allow group ResourceTaggers to use volumes in compartment TargetCompartment
Allow group ResourceTaggers to use tag-namespaces in tenancy
```

The last statement is required for defined tags. Restrict it to approved namespaces where possible:

```text
Allow group ResourceTaggers to use tag-namespaces in tenancy where any {
  target.tag-namespace.name='Operations',
  target.tag-namespace.name='Security'
}
```

See the complete [user and dynamic-group policy examples](examples/iam/).

## Quick start

Start with a dry run:

```bash
go run . \
  -compartment-id "ocid1.compartment.oc1..example" \
  -freeform-tags '{"Environment":"Production","Owner":"FinOps"}'
```

Review every `DRY-RUN would update` line and the final summary. Apply the same change only after confirming the scope:

```bash
go run . \
  -compartment-id "ocid1.compartment.oc1..example" \
  -freeform-tags '{"Environment":"Production","Owner":"FinOps"}' \
  -apply
```

## Defined-tag examples

Apply one defined tag in dry-run mode:

```bash
go run . \
  -compartment-id "ocid1.compartment.oc1..example" \
  -defined-tags '{"Operations":{"CostCenter":"42"}}'
```

Apply multiple keys from multiple namespaces:

```bash
go run . \
  -compartment-id "ocid1.compartment.oc1..example" \
  -defined-tags '{"Operations":{"CostCenter":"42","Environment":"Production"},"Security":{"Classification":"Internal"}}' \
  -apply
```

Defined tag keys must already exist, and list-based tag definitions accept only one of their configured values.

## Combined tags

Free-form and defined tags can be applied in the same execution:

```bash
go run . \
  -compartment-id "ocid1.compartment.oc1..example" \
  -freeform-tags '{"Owner":"FinOps","ManagedBy":"GoTagger"}' \
  -defined-tags '{"Operations":{"CostCenter":"42"}}' \
  -apply
```

PowerShell, Bash, tag-file, and IAM examples are catalogued in [`examples/README.md`](examples/README.md).

## Concurrency

Two worker pools bound the number of concurrent operations:

- `-region-workers`: regions processed concurrently; default `3`.
- `-resource-workers`: resources processed concurrently inside each active region; default `5`.

The maximum number of simultaneous resource workers is approximately:

```text
region-workers × resource-workers
```

For example, `3 × 5` permits up to 15 resource workers. Use lower values if the tenancy experiences API throttling:

```bash
go run . \
  -compartment-id "ocid1.compartment.oc1..example" \
  -freeform-tags '{"Environment":"Production"}' \
  -region-workers 1 \
  -resource-workers 2
```

`-instance-workers` remains available as a deprecated alias for `-resource-workers`.

## Command reference

| Flag | Required | Default | Purpose |
| --- | --- | --- | --- |
| `-compartment-id` | Yes | — | Target compartment OCID. |
| `-freeform-tags` | One tag option required | `{}` | JSON object containing free-form key/value pairs. |
| `-defined-tags` | One tag option required | `{}` | JSON object containing namespace/key/value mappings. |
| `-apply` | No | `false` | Perform updates. Without it, the program is a dry run. |
| `-region-workers` | No | `3` | Maximum active regions. |
| `-resource-workers` | No | `5` | Maximum active resources per region. |

Display built-in help:

```bash
go run . -h
```

## Example output

Names, OCIDs, regions, and totals below are illustrative:

```text
READY regions (2): eu-frankfurt-1, eu-paris-1
DRY-RUN mode: no resource will be modified
[eu-frankfurt-1] DRY-RUN would update instance app-01 (ocid1.instance...)
[eu-frankfurt-1] DRY-RUN would update boot-volume app-01-boot (ocid1.bootvolume...)
[eu-paris-1] UNCHANGED block-volume shared-data
Finished: found=3 instances=1 boot-volumes=1 block-volumes=1 would-update=2 updated=0 unchanged=1 skipped=0 failed=0 region-failures=0
```

## Exit status

- `0`: all regional listings and resource operations completed successfully.
- `1`: at least one resource operation or regional listing failed, or input validation failed.

The program continues processing independent resources after an individual failure and reports the final failure counts.

## Troubleshooting

### `NotAuthorizedOrNotFound`

Verify the compartment OCID, the `use instances` and `use volumes` policies, and the active OCI profile. OCI deliberately uses this response for both missing resources and unauthorized access.

### Defined tag is not authorized or not found

Confirm the exact namespace and key spelling and grant `use tag-namespaces`. Namespace and key names are case-insensitive, while tag values are case-sensitive.

### `412 Precondition Failed`

The resource changed after the program read it. This is the ETag protection working as intended. Run a new dry run and retry after reviewing the latest state.

### `429 TooManyRequests`

The SDK retries retryable throttling responses. If failures persist, reduce `-region-workers` and `-resource-workers`.

### A region is missing

Only subscriptions in `READY` state are processed. Regions in `IN_PROGRESS` state are intentionally skipped.

### Child-compartment resources are missing

The program processes exactly one compartment OCID. Run it separately for child compartments.

## Architecture diagram

The editable Draw.io source and exported SVG are in [`docs/architecture`](docs/architecture/). The diagram uses shapes from Oracle's official [OCI Architecture Diagram Toolkit](https://docs.oracle.com/en-us/iaas/Content/General/Reference/graphicsfordiagrams.htm).

## Development

```bash
gofmt -w .
go test ./...
go vet ./...
go build -buildvcs=false ./...
```

The GitHub Actions workflow runs formatting checks, tests, vet, and build. Every successfully tested commit pushed to `main` receives one idempotent, commit-derived GitHub release whose title and notes come from the newest [`CHANGELOG.md`](CHANGELOG.md) section.

## Security considerations

- Begin with a dry run and review the complete scope.
- Prefer narrowly scoped IAM policies and defined-tag namespaces.
- Remember that tags can participate in IAM conditions. Changing a tag can change access behavior.
- Do not place secrets, customer identifiers, or confidential information in tag values.
- Do not commit OCI configuration files, private keys, tokens, or captured API responses.

## License

Released under the [MIT License](LICENSE).
