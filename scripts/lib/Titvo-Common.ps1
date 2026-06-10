# Titvo-Common.ps1
# Utilidades compartidas para scripts Titvo en PowerShell 5.1+
# Uso: . (Join-Path $PSScriptRoot 'lib\Titvo-Common.ps1')

Set-StrictMode -Version Latest

function Get-TitvoRepoRoot {
    param(
        [string]$ScriptRoot = $PSScriptRoot
    )
    if ($ScriptRoot -match '[\\/]lib$') {
        return (Split-Path (Split-Path $ScriptRoot -Parent) -Parent)
    }
    return (Split-Path $ScriptRoot -Parent)
}

function Import-TitvoDotEnv {
    param(
        [Parameter(Mandatory = $true)]
        [string]$EnvFile
    )

    if (-not (Test-Path -LiteralPath $EnvFile)) {
        return
    }

    Get-Content -LiteralPath $EnvFile -Encoding UTF8 | ForEach-Object {
        $line = $_.Trim()
        if ($line -eq '' -or $line.StartsWith('#')) { return }

        $eq = $line.IndexOf('=')
        if ($eq -lt 1) { return }

        $key = $line.Substring(0, $eq).Trim()
        $value = $line.Substring($eq + 1).Trim()

        if (($value.StartsWith('"') -and $value.EndsWith('"')) -or ($value.StartsWith("'") -and $value.EndsWith("'"))) {
            $value = $value.Substring(1, $value.Length - 2)
        }

        [Environment]::SetEnvironmentVariable($key, $value, 'Process')
        Set-Item -Path "Env:$key" -Value $value
    }
}

function Test-TitvoCommand {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name
    )

    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        Write-Host "Falta comando: $Name"
        exit 1
    }
}

function Write-TitvoLog {
    param(
        [Parameter(Mandatory = $true)]
        [ValidateSet('Info', 'Success', 'Warning', 'Error')]
        [string]$Level,

        [Parameter(Mandatory = $true)]
        [string]$Message
    )

    switch ($Level) {
        'Info'    { Write-Host "[INFO] $Message" -ForegroundColor Cyan }
        'Success' { Write-Host "[OK] $Message" -ForegroundColor Green }
        'Warning' { Write-Host "[WARN] $Message" -ForegroundColor Yellow }
        'Error'   { Write-Host "[ERROR] $Message" -ForegroundColor Red }
    }
}

function Confirm-TitvoAction {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Message,

        [ValidateSet('y', 'n')]
        [string]$Default = 'n'
    )

    if ($Default -eq 'y') {
        $prompt = "$Message (Y/n): "
        $response = Read-Host $prompt
        if ([string]::IsNullOrWhiteSpace($response)) { $response = 'y' }
    }
    else {
        $prompt = "$Message (y/N): "
        $response = Read-Host $prompt
        if ([string]::IsNullOrWhiteSpace($response)) { $response = 'n' }
    }

    return ($response -match '^[Yy]$')
}

function Invoke-TitvoExternalProcess {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Command,

        [Parameter(Mandatory = $true)]
        [string[]]$ArgumentList
    )

    $previousErrorAction = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $lines = @(& $Command @ArgumentList 2>&1)
        $exitCode = $LASTEXITCODE
    }
    finally {
        $ErrorActionPreference = $previousErrorAction
    }

    $output = @($lines | ForEach-Object {
            if ($_ -is [System.Management.Automation.ErrorRecord]) { $_.ToString() }
            elseif ($null -eq $_) { '' }
            else { "$_" }
        })

    return [pscustomobject]@{
        Output   = $output
        ExitCode = $exitCode
    }
}

function Invoke-TitvoAwsProcess {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$ArgumentList
    )

    return Invoke-TitvoExternalProcess -Command 'aws' -ArgumentList $ArgumentList
}

function Invoke-TitvoTerragrunt {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$ArgumentList
    )

    $args = @($ArgumentList)
    if ($args -notcontains '--terragrunt-no-color') {
        $args += '--terragrunt-no-color'
    }

    return Invoke-TitvoExternalProcess -Command 'terragrunt' -ArgumentList $args
}

function Invoke-TitvoAws {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$ArgumentList,

        [switch]$ReturnJson,
        [switch]$Quiet,
        [switch]$AllowFailure
    )

    $result = Invoke-TitvoAwsProcess -ArgumentList $ArgumentList
    $exitCode = $result.ExitCode
    $output = $result.Output

    if ($exitCode -ne 0 -and -not $AllowFailure) {
        throw "aws $($ArgumentList -join ' ') fallo (rc=$exitCode): $($output -join "`n")"
    }

    if ($Quiet) {
        return $null
    }

    $text = ($output -join "`n").Trim()
    if ($ReturnJson) {
        if ([string]::IsNullOrWhiteSpace($text)) {
            return $null
        }
        return ($text | ConvertFrom-Json)
    }

    return $text
}

function Invoke-TitvoAwsJson {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$ArgumentList,

        [switch]$AllowFailure
    )

    $argsWithJson = @($ArgumentList) + @('--output', 'json')
    try {
        return Invoke-TitvoAws -ArgumentList $argsWithJson -ReturnJson -AllowFailure:$AllowFailure
    }
    catch {
        if ($AllowFailure) { return $null }
        throw
    }
}

function Get-TitvoAwsJsonProperty {
    param(
        $Object,

        [Parameter(Mandatory = $true)]
        [string]$Name
    )

    if ($null -eq $Object) { return $null }

    $prop = $Object.PSObject.Properties[$Name]
    if ($prop) { return $prop.Value }
    return $null
}

function Test-TitvoAwsTagPrefixMatch {
    param(
        $Resource,
        $Tags,

        [Parameter(Mandatory = $true)]
        [string]$Prefix,

        [string]$TagPropertyName = 'Tags'
    )

    if ($null -eq $Tags) {
        if ($null -eq $Resource) { return $false }
        $Tags = Get-TitvoAwsJsonProperty -Object $Resource -Name $TagPropertyName
    }

    if (-not $Tags) { return $false }

    $prefixLower = $Prefix.ToLowerInvariant()
    foreach ($tag in @($Tags)) {
        if ($tag.Key -eq 'Name' -and $tag.Value.ToLowerInvariant().StartsWith($prefixLower)) {
            return $true
        }
        if ($tag.Key -eq 'Project' -and $tag.Value.ToLowerInvariant().Contains($prefixLower)) {
            return $true
        }
    }

    return $false
}

function Invoke-TitvoRun {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$ArgumentList,

        [switch]$Apply
    )

    $display = 'aws ' + ($ArgumentList -join ' ')
    if (-not $Apply) {
        Write-Host "[DRY-RUN] $display"
        return 0
    }

    Write-Host "[APPLY] $display"
    $result = Invoke-TitvoAwsProcess -ArgumentList $ArgumentList
    $exitCode = $result.ExitCode
    if ($exitCode -ne 0) {
        Write-Host "[WARN] comando fallo (rc=$exitCode): $display"
        if ($result.Output) { Write-Host ($result.Output -join "`n") }
    }
    return $exitCode
}

function Invoke-TitvoRunShell {
    param(
        [Parameter(Mandatory = $true)]
        [string]$CommandLine,

        [switch]$Apply
    )

    if (-not $Apply) {
        Write-Host "[DRY-RUN] $CommandLine"
        return 0
    }

    Write-Host "[APPLY] $CommandLine"
    $previousErrorAction = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try {
        $lines = @(Invoke-Expression $CommandLine 2>&1)
        $exitCode = $LASTEXITCODE
    }
    finally {
        $ErrorActionPreference = $previousErrorAction
    }

    $output = @($lines | ForEach-Object {
            if ($_ -is [System.Management.Automation.ErrorRecord]) { $_.ToString() }
            elseif ($null -eq $_) { '' }
            else { "$_" }
        })
    if ($exitCode -ne 0) {
        Write-Host "[WARN] comando fallo (rc=$exitCode): $CommandLine"
        if ($output) { Write-Host ($output -join "`n") }
    }
    return $exitCode
}

function Test-StartsWithPrefix {
    param(
        [string]$Value,
        [string]$Prefix
    )
    return $Value.StartsWith($Prefix)
}

function Remove-TitvoNetworkInterface {
    param(
        [Parameter(Mandatory = $true)]
        [string]$EniId,

        [Parameter(Mandatory = $true)]
        [string]$Region,

        [switch]$Apply
    )

    $detail = Invoke-TitvoAwsJson -ArgumentList @(
        'ec2', 'describe-network-interfaces',
        '--network-interface-ids', $EniId,
        '--region', $Region
    ) -AllowFailure

    $status = $null
    $attachmentId = $null
    if ($detail -and $detail.NetworkInterfaces -and $detail.NetworkInterfaces.Count -gt 0) {
        $eni = $detail.NetworkInterfaces[0]
        $status = $eni.Status
        $attachment = Get-TitvoAwsJsonProperty -Object $eni -Name 'Attachment'
        if ($attachment) {
            $attachmentId = Get-TitvoAwsJsonProperty -Object $attachment -Name 'AttachmentId'
        }
    }

    if ($attachmentId) {
        Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
            'ec2', 'detach-network-interface',
            '--attachment-id', $attachmentId,
            '--force',
            '--region', $Region
        ) | Out-Null
    }

    if ($status -eq 'available' -or $attachmentId) {
        Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
            'ec2', 'delete-network-interface',
            '--network-interface-id', $EniId,
            '--region', $Region
        ) | Out-Null
    }
    else {
        Write-Host "[INFO] ENI $EniId status=$status (no available, posiblemente gestionado por AWS); se intentara borrar igual"
        Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
            'ec2', 'delete-network-interface',
            '--network-interface-id', $EniId,
            '--region', $Region
        ) | Out-Null
    }
}

function Start-TitvoWait {
    param(
        [Parameter(Mandatory = $true)]
        [scriptblock]$Condition,

        [int]$MaxAttempts = 24,
        [int]$SleepSeconds = 5,
        [string]$WaitMessage = 'Esperando...',
        [string]$WarnMessage = 'Timeout en espera'
    )

    for ($attempt = 1; $attempt -le $MaxAttempts; $attempt++) {
        $result = & $Condition
        if ($result -eq $true) {
            return $true
        }

        Write-Host "[WAIT] $WaitMessage (intento $attempt/$MaxAttempts)"
        Start-Sleep -Seconds $SleepSeconds
    }

    Write-Host "[WARN] $WarnMessage"
    return $false
}

function Clear-TitvoS3Bucket {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Bucket,

        [Parameter(Mandatory = $true)]
        [string]$Region,

        [switch]$Apply
    )

    Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
        's3', 'rm', "s3://$Bucket", '--recursive', '--region', $Region
    ) | Out-Null

    $keyMarker = $null
    $versionMarker = $null

    while ($true) {
        $args = @('s3api', 'list-object-versions', '--bucket', $Bucket, '--region', $Region)
        if ($keyMarker) {
            $args += @('--key-marker', $keyMarker, '--version-id-marker', $versionMarker)
        }

        $versions = Invoke-TitvoAwsJson -ArgumentList $args -AllowFailure
        if (-not $versions) { $versions = [pscustomobject]@{} }

        $versionItems = Get-TitvoAwsJsonProperty -Object $versions -Name 'Versions'
        if ($versionItems) {
            foreach ($v in @($versionItems)) {
                Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                    's3api', 'delete-object',
                    '--bucket', $Bucket,
                    '--key', $v.Key,
                    '--version-id', $v.VersionId,
                    '--region', $Region
                ) | Out-Null
            }
        }

        $deleteMarkers = Get-TitvoAwsJsonProperty -Object $versions -Name 'DeleteMarkers'
        if ($deleteMarkers) {
            foreach ($dm in @($deleteMarkers)) {
                Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                    's3api', 'delete-object',
                    '--bucket', $Bucket,
                    '--key', $dm.Key,
                    '--version-id', $dm.VersionId,
                    '--region', $Region
                ) | Out-Null
            }
        }

        if ((Get-TitvoAwsJsonProperty -Object $versions -Name 'IsTruncated') -ne $true) { break }
        $keyMarker = Get-TitvoAwsJsonProperty -Object $versions -Name 'NextKeyMarker'
        $versionMarker = Get-TitvoAwsJsonProperty -Object $versions -Name 'NextVersionIdMarker'
    }

    $uploadsKeyMarker = $null
    $uploadsIdMarker = $null

    while ($true) {
        $args = @('s3api', 'list-multipart-uploads', '--bucket', $Bucket, '--region', $Region)
        if ($uploadsKeyMarker) {
            $args += @('--key-marker', $uploadsKeyMarker, '--upload-id-marker', $uploadsIdMarker)
        }

        $uploads = Invoke-TitvoAwsJson -ArgumentList $args -AllowFailure
        if (-not $uploads) { $uploads = [pscustomobject]@{} }

        $multipartUploads = Get-TitvoAwsJsonProperty -Object $uploads -Name 'Uploads'
        if ($multipartUploads) {
            foreach ($u in @($multipartUploads)) {
                Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                    's3api', 'abort-multipart-upload',
                    '--bucket', $Bucket,
                    '--key', $u.Key,
                    '--upload-id', $u.UploadId,
                    '--region', $Region
                ) | Out-Null
            }
        }

        if ((Get-TitvoAwsJsonProperty -Object $uploads -Name 'IsTruncated') -ne $true) { break }
        $uploadsKeyMarker = Get-TitvoAwsJsonProperty -Object $uploads -Name 'NextKeyMarker'
        $uploadsIdMarker = Get-TitvoAwsJsonProperty -Object $uploads -Name 'NextUploadIdMarker'
    }
}

function Remove-TitvoSsmParametersByPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,

        [Parameter(Mandatory = $true)]
        [string]$Region,

        [switch]$Apply
    )

    $nextToken = $null

    while ($true) {
        $args = @(
            'ssm', 'get-parameters-by-path',
            '--path', $Path,
            '--recursive',
            '--with-decryption',
            '--region', $Region
        )
        if ($nextToken) {
            $args += @('--next-token', $nextToken)
        }

        $params = Invoke-TitvoAwsJson -ArgumentList $args
        $names = @()
        $parameters = Get-TitvoAwsJsonProperty -Object $params -Name 'Parameters'
        if ($parameters) {
            $names = @($parameters | ForEach-Object { $_.Name })
        }

        for ($i = 0; $i -lt $names.Count; $i += 10) {
            $batch = @($names[$i..([Math]::Min($i + 9, $names.Count - 1))])
            if ($batch.Count -eq 0) { continue }

            $deleteArgs = @('ssm', 'delete-parameters', '--region', $Region, '--names') + $batch
            Invoke-TitvoRun -Apply:$Apply -ArgumentList $deleteArgs | Out-Null
        }

        $nextToken = Get-TitvoAwsJsonProperty -Object $params -Name 'NextToken'
        if ([string]::IsNullOrWhiteSpace([string]$nextToken)) { break }
    }
}

function Initialize-TitvoAwsEnvironment {
    $env:AWS_PAGER = ''
}

function Get-TitvoAwsTextLines {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$ArgumentList
    )

    $text = Invoke-TitvoAws -ArgumentList $ArgumentList -AllowFailure
    if ([string]::IsNullOrWhiteSpace($text)) {
        # Coma inicial: evita que PowerShell desempaquete el array al retornar.
        return ,@()
    }

    return ,@($text -split "`t|`n" | ForEach-Object { $_.Trim() } | Where-Object {
            $_ -and $_ -ne 'None'
        })
}

function Write-TitvoTempJsonFile {
    param(
        [Parameter(Mandatory = $true)]
        $Object,

        [string]$Prefix = 'titvo-aws'
    )

    $path = Join-Path $env:TEMP ("{0}-{1}.json" -f $Prefix, [Guid]::NewGuid().ToString('N'))
    $json = $Object | ConvertTo-Json -Depth 20 -Compress:$false
    Set-Content -LiteralPath $path -Value $json -Encoding UTF8
    return $path
}

function Remove-TitvoTempFile {
    param(
        [string]$Path
    )
    if ($Path -and (Test-Path -LiteralPath $Path)) {
        Remove-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
    }
}

function Remove-TitvoDirectoryTree {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    if (-not (Test-Path -LiteralPath $Path)) {
        return
    }

    $fullPath = (Get-Item -LiteralPath $Path).FullName.TrimEnd('\')

    # robocopy /MIR vacia el arbol; Remove-Item falla en rutas largas (.terragrunt-cache, .terraform).
    $empty = Join-Path $env:TEMP ("titvo-empty-{0}" -f [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $empty -Force | Out-Null
    try {
        & robocopy.exe $empty $fullPath /MIR /R:1 /W:1 /NFL /NDL /NJH /NJS /NC /NS /NP | Out-Null
        if ($LASTEXITCODE -ge 8) {
            throw "robocopy finalizo con codigo $LASTEXITCODE al vaciar $fullPath"
        }
    } finally {
        Remove-Item -LiteralPath $empty -Recurse -Force -ErrorAction SilentlyContinue
    }

    if (-not (Test-Path -LiteralPath $fullPath)) {
        return
    }

    $longPath = if ($fullPath.StartsWith('\\?\')) { $fullPath } else { "\\?\$fullPath" }
    if ([System.IO.Directory]::Exists($longPath)) {
        [System.IO.Directory]::Delete($longPath, $true)
        return
    }

    Remove-Item -LiteralPath $fullPath -Recurse -Force
}
