# Build and install the SDK from a Visual Studio developer shell.
param([string]$Generator = 'Visual Studio 17 2022')
$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
$buildPath = $env:PATH
Push-Location $repo
try {
  go mod download github.com/aperturerobotics/protobuf github.com/aperturerobotics/abseil-cpp
  if ($LASTEXITCODE -ne 0) { throw 'Go dependency resolution failed' }
  cmake -S . -B build-win -G $Generator -A x64
  if ($LASTEXITCODE -ne 0) { throw 'SDK configuration failed' }
  cmake --build build-win --config Release --parallel
  if ($LASTEXITCODE -ne 0) { throw 'SDK build failed' }
  cmake --install build-win --config Release --prefix "$repo/build-win/sdk"
  if ($LASTEXITCODE -ne 0) { throw 'SDK installation failed' }
  $env:PATH = "$repo/build-win/sdk/bin;$env:PATH"
  ctest --test-dir build-win -C Release --output-on-failure --interactive-debug-mode 0 --timeout 120
  if ($LASTEXITCODE -ne 0) { throw 'SDK tests failed' }
} finally {
  $env:PATH = $buildPath
  Pop-Location
}
