param(
    [Parameter(Mandatory = $true)]
    [string]$CompartmentId
)

go run . `
    -compartment-id $CompartmentId `
    -freeform-tags '{"Environment":"Production","Owner":"FinOps"}'
