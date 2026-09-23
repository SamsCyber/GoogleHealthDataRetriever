# Generates Go gRPC stubs for the Google Health API v4 into ./gen/healthpb
$ErrorActionPreference = "Stop"

# Read the module name from go.mod so this always matches `go mod init`
if (-not (Test-Path go.mod)) { throw "go.mod not found - run 'go mod init GoogleHealthDataRetriever' first" }
$module   = (Select-String -Path go.mod -Pattern '^module\s+(\S+)').Matches[0].Groups[1].Value
$outPkg   = "$module/gen/healthpb"
$protoDir = "google/devicesandservices/health/v4"

if (-not (Test-Path googleapis)) {
    git clone --depth 1 https://github.com/googleapis/googleapis.git
}

# Every .proto in the v4 package (PowerShell doesn't glob for protoc, so list them)
$files = Get-ChildItem "googleapis/$protoDir/*.proto" | ForEach-Object { "$protoDir/$($_.Name)" }

# Map each health proto into our own Go package, so we don't depend on
# whatever go_package option the upstream files declare.
$mappings = $files | ForEach-Object { "M$_=$outPkg" }

New-Item -ItemType Directory -Force -Path gen/healthpb | Out-Null

$protocArgs = @(
    "--proto_path=googleapis",
    "--go_out=.",       "--go_opt=module=$module",
    "--go-grpc_out=.",  "--go-grpc_opt=module=$module"
)
$protocArgs += $mappings | ForEach-Object { "--go_opt=$_" }
$protocArgs += $mappings | ForEach-Object { "--go-grpc_opt=$_" }
$protocArgs += $files

& protoc @protocArgs
if ($LASTEXITCODE -ne 0) { throw "protoc failed" }

Write-Host "Generated stubs in gen/healthpb"
