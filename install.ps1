$ErrorActionPreference = "Stop"
$Repo = "V12KLT/authris-cli"
$Dest = if ($env:AUTHRIS_DEST) { $env:AUTHRIS_DEST } else { Join-Path $env:LOCALAPPDATA "authris\bin" }

$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
  "ARM64" { "arm64" }
  default { "amd64" }
}

$Tag = $env:AUTHRIS_VERSION
if (-not $Tag) {
  $Tag = (Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest").tag_name
}
if (-not $Tag) { throw "could not resolve latest release" }

$Tmp = Join-Path ([IO.Path]::GetTempPath()) ("authris-" + [Guid]::NewGuid().ToString() + ".exe")
try {
  Invoke-WebRequest -Uri "https://github.com/$Repo/releases/download/$Tag/authris-windows-$Arch.exe" -OutFile $Tmp -UseBasicParsing
  New-Item -ItemType Directory -Force -Path $Dest | Out-Null
  Move-Item -Force -Path $Tmp -Destination (Join-Path $Dest "authris.exe")
} finally {
  if (Test-Path $Tmp) { Remove-Item -Force $Tmp }
}

$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$Dest*") {
  [Environment]::SetEnvironmentVariable("Path", ($UserPath.TrimEnd(";") + ";" + $Dest), "User")
  $env:Path += ";" + $Dest
}

Write-Output "installed authris $Tag to $Dest\authris.exe"
& (Join-Path $Dest "authris.exe") | Select-Object -First 2
