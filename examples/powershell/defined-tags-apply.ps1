param(
    [Parameter(Mandatory = $true)]
    [string]$CompartmentId
)

go run . `
    -compartment-id $CompartmentId `
    -defined-tags '{"Operations":{"CostCenter":"42","Environment":"Production"},"Security":{"Classification":"Internal"}}' `
    -apply
