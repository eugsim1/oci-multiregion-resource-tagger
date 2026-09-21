# Examples

All OCIDs and tag values in this directory are placeholders. Start in dry-run mode and add `-apply` only after reviewing the complete scope.

## Bash

Run an example from the repository root and pass the target compartment OCID:

```bash
./examples/bash/freeform-dry-run.sh "ocid1.compartment.oc1..example"
./examples/bash/defined-tags-apply.sh "ocid1.compartment.oc1..example"
./examples/bash/combined-low-concurrency.sh "ocid1.compartment.oc1..example"
```

The combined example intentionally remains a dry run.

## PowerShell

```powershell
./examples/powershell/freeform-dry-run.ps1 -CompartmentId "ocid1.compartment.oc1..example"
./examples/powershell/defined-tags-apply.ps1 -CompartmentId "ocid1.compartment.oc1..example"
./examples/powershell/combined-low-concurrency.ps1 -CompartmentId "ocid1.compartment.oc1..example"
```

## Read tag JSON from files

Bash:

```bash
go run . \
  -compartment-id "ocid1.compartment.oc1..example" \
  -freeform-tags "$(<examples/tags/freeform-tags.json)" \
  -defined-tags "$(<examples/tags/defined-tags.json)"
```

PowerShell:

```powershell
$freeform = Get-Content ./examples/tags/freeform-tags.json -Raw
$defined = Get-Content ./examples/tags/defined-tags.json -Raw
go run . `
    -compartment-id "ocid1.compartment.oc1..example" `
    -freeform-tags $freeform `
    -defined-tags $defined
```

## IAM policies

- `iam/user-group-policy.txt` is a starting point for an OCI user group.
- `iam/dynamic-group-policy.txt` is a starting point for an instance-principal dynamic group.

Replace every group and compartment name. Restrict tag namespace access to the exact namespaces the run requires.
