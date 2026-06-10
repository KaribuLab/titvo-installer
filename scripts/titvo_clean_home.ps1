# titvo_clean_home.ps1
# Elimina ~/.titvo (USERPROFILE\.titvo) con soporte para rutas largas en Windows.
#
# Uso:
#   cd scripts
#   .\titvo_clean_home.ps1
#
#   powershell -NoProfile -ExecutionPolicy Bypass -File .\titvo_clean_home.ps1

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $ScriptDir 'lib\Titvo-Common.ps1')

$titvoHome = Join-Path $env:USERPROFILE '.titvo'

if (-not (Test-Path -LiteralPath $titvoHome)) {
    Write-TitvoLog -Level Info -Message "No existe $titvoHome; nada que borrar."
    exit 0
}

Write-TitvoLog -Level Info -Message "Eliminando $titvoHome ..."
Remove-TitvoDirectoryTree -Path $titvoHome
Write-TitvoLog -Level Success -Message "Eliminado $titvoHome"
