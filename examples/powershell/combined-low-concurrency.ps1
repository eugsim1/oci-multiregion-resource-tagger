param(
    [Parameter(Mandatory = $true)]
    [string]$CompartmentId
)

go run . `
    -compartment-id $CompartmentId `
    -freeform-tags '{"Owner":"FinOps","ManagedBy":"GoTagger"}' `
    -defined-tags '{"Operations":{"CostCenter":"42"}}' `
    -region-workers 1 `
    -resource-workers 2
