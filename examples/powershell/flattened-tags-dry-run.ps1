param(
    [Parameter(Mandatory = $true)]
    [string]$CompartmentId
)

go run . `
    -compartment-id $CompartmentId `
    -flattened-tag "Operations.CostCent=42" `
    -flattened-tag "Operations.Environment=Production" `
    -flattened-tag "Security.Classification=Internal"
