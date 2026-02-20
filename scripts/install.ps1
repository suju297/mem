Param(
    [Parameter(Mandatory = $true)]
    [string]$Repo,
    [string]$Version = "latest",
    [string]$InstallDir = "$env:USERPROFILE\bin",
    [string]$BinName = "mem",
    [switch]$SkipChecksums
)

$ErrorActionPreference = "Stop"

function Resolve-Arch {
    if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { return "arm64" }
    if ($env:PROCESSOR_ARCHITECTURE -eq "AMD64") { return "amd64" }
    if ($env:PROCESSOR_ARCHITEW6432 -eq "ARM64") { return "arm64" }
    if ($env:PROCESSOR_ARCHITEW6432 -eq "AMD64") { return "amd64" }
    throw "Unsupported Windows architecture: $env:PROCESSOR_ARCHITECTURE"
}

$arch = Resolve-Arch
$asset = "mempack_windows_${arch}.zip"

if ($Version -eq "latest") {
    $base = "https://github.com/$Repo/releases/latest/download"
} else {
    $base = "https://github.com/$Repo/releases/download/$Version"
}

$assetUrl = "$base/$asset"
$checksumsUrl = "$base/checksums.txt"

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("mempack-install-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
    $archive = Join-Path $tmp $asset
    $downloadedRelease = $true
    try {
        Invoke-WebRequest -Uri $assetUrl -OutFile $archive
    } catch {
        $downloadedRelease = $false
        Write-Warning "Release asset not found ($asset). Falling back to source build."
    }

    $binFile = "$BinName.exe"
    $candidate = $null

    if ($downloadedRelease) {
        if (-not $SkipChecksums) {
            $checksumsPath = Join-Path $tmp "checksums.txt"
            try {
                Invoke-WebRequest -Uri $checksumsUrl -OutFile $checksumsPath
                $line = Select-String -Path $checksumsPath -Pattern " $asset$" | Select-Object -First 1
                if ($line) {
                    $expected = ($line.Line -split "\s+")[0]
                    $actual = (Get-FileHash -Algorithm SHA256 -Path $archive).Hash.ToLowerInvariant()
                    if ($actual -ne $expected.ToLowerInvariant()) {
                        throw "Checksum mismatch for $asset"
                    }
                } else {
                    Write-Warning "No checksum entry found for $asset; continuing"
                }
            } catch {
                Write-Warning "Checksum verification skipped: $($_.Exception.Message)"
            }
        }

        $extractDir = Join-Path $tmp "extract"
        Expand-Archive -Path $archive -DestinationPath $extractDir -Force

        $candidate = Join-Path $extractDir $binFile
        if (-not (Test-Path $candidate)) {
            $found = Get-ChildItem -Path $extractDir -Recurse -File | Where-Object { $_.Name -eq $binFile } | Select-Object -First 1
            if (-not $found) {
                throw "Binary $binFile not found in archive"
            }
            $candidate = $found.FullName
        }
    } else {
        $go = Get-Command go -ErrorAction SilentlyContinue
        if (-not $go) {
            throw "Release asset unavailable and Go toolchain not found for source build fallback."
        }

        if ($Version -eq "latest") {
            $sourceRef = "heads/main"
        } else {
            $sourceRef = "tags/$Version"
        }
        $sourceUrl = "https://github.com/$Repo/archive/refs/$sourceRef.tar.gz"
        $sourceArchive = Join-Path $tmp "source.tar.gz"
        Invoke-WebRequest -Uri $sourceUrl -OutFile $sourceArchive

        $sourceExtract = Join-Path $tmp "source"
        New-Item -ItemType Directory -Path $sourceExtract | Out-Null
        tar -xzf $sourceArchive -C $sourceExtract

        $mainGo = Get-ChildItem -Path $sourceExtract -Recurse -File |
            Where-Object { $_.FullName -match "[\\\\/]cmd[\\\\/]mem[\\\\/]main\\.go$" } |
            Select-Object -First 1
        if (-not $mainGo) {
            throw "Source archive missing cmd/mem/main.go"
        }

        $srcRoot = Split-Path -Path (Split-Path -Path (Split-Path -Path $mainGo.FullName -Parent) -Parent) -Parent
        $candidate = Join-Path $tmp $binFile
        Write-Host "Building $binFile from source ($sourceRef)..."
        Push-Location $srcRoot
        try {
            & go build -trimpath -o $candidate ./cmd/mem
        } finally {
            Pop-Location
        }
        if (-not (Test-Path $candidate)) {
            throw "Built binary $binFile was not created"
        }
    }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $dest = Join-Path $InstallDir $binFile
    Copy-Item -Path $candidate -Destination $dest -Force

    Write-Host "Installed $binFile to $dest"
    Write-Host "If needed, add $InstallDir to your user PATH."
} finally {
    if (Test-Path $tmp) {
        Remove-Item -Path $tmp -Recurse -Force
    }
}
