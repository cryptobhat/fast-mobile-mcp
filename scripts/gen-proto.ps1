$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent $PSScriptRoot
$ProtoDir = Join-Path $Root "proto"
$OutGo = Join-Path $ProtoDir "gen/go"
$OutTs = Join-Path $Root "gateway-mcp/src/gen"

New-Item -ItemType Directory -Force -Path $OutGo | Out-Null
New-Item -ItemType Directory -Force -Path $OutTs | Out-Null

# Ensure protoc-gen-go binaries are discoverable for protoc.
$goBinCandidates = @(
  (Join-Path $env:USERPROFILE "go/bin"),
  (Join-Path $env:USERPROFILE "tools/go1.26.0/bin")
)
foreach ($candidate in $goBinCandidates) {
  if ((Test-Path $candidate) -and -not ($env:Path -split ';' | Where-Object { $_ -eq $candidate })) {
    $env:Path = "$candidate;$env:Path"
  }
}

$protocCommand = Get-Command protoc -ErrorAction SilentlyContinue
if ($protocCommand) {
  $protocExe = $protocCommand.Source
} else {
  $fallback = Join-Path $env:USERPROFILE "tools/protoc-33.5/bin/protoc.exe"
  if (-not (Test-Path $fallback)) {
    throw "protoc not found. Install protoc or place it at $fallback"
  }
  $protocExe = $fallback
}

& $protocExe `
  --proto_path="$ProtoDir" `
  --go_out="$OutGo" --go_opt=module=github.com/fast-mobile-mcp/proto/gen/go `
  --go-grpc_out="$OutGo" --go-grpc_opt=module=github.com/fast-mobile-mcp/proto/gen/go `
  "$ProtoDir/mobile.proto"

& $protocExe `
  --proto_path="$ProtoDir" `
  --plugin="protoc-gen-ts_proto=$Root/gateway-mcp/node_modules/.bin/protoc-gen-ts_proto.cmd" `
  --ts_proto_out="$OutTs" `
  --ts_proto_opt=outputServices=grpc-js,esModuleInterop=true,forceLong=string,importSuffix=.js `
  "$ProtoDir/mobile.proto"
