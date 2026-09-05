# Read incoming JSON payload from stdin
$inputJson = [Console]::In.ReadToEnd() | ConvertFrom-Json
$cmd = $inputJson.toolCall.args.CommandLine

# Define regex patterns for destructive commands
$destructivePatterns = @(
    '\b(rm|del|Remove-Item)\s+.*(-r|-Recurse|-Force|\/s|\/q)',
    'git\s+reset\s+--hard',
    'git\s+clean\s+.*-f',
    'format\s+[a-zA-Z]:',
    'drop\s+database'
)

$isDestructive = $false
foreach ($pattern in $destructivePatterns) {
    if ($cmd -match $pattern) {
        $isDestructive = $true
        break
    }
}

if ($isDestructive) {
    # Force confirmation prompt before executing
    @{
        decision = "force_ask"
        reason   = "Destructive command pattern detected: $cmd"
    } | ConvertTo-Json -Compress
} else {
    # Allow execution automatically
    @{
        decision = "allow"
    } | ConvertTo-Json -Compress
}