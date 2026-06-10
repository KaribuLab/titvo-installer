# titvo_destroy_infra.ps1
# Destruye infraestructura TITVO en AWS (port de titvo_destroy_infra.sh)
#
# Requisitos: PowerShell 5.1+, aws CLI, terragrunt, terraform en PATH
# Uso:
#   cd scripts
#   .\titvo_destroy_infra.ps1
#
# Si falla la ejecucion por politica:
#   powershell -NoProfile -ExecutionPolicy Bypass -File .\titvo_destroy_infra.ps1

[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $ScriptDir 'lib\Titvo-Common.ps1')

Initialize-TitvoAwsEnvironment

$RepoRoot = Get-TitvoRepoRoot -ScriptRoot $ScriptDir
$EnvFile = Join-Path $RepoRoot '.env'

if (-not (Test-Path -LiteralPath $EnvFile)) {
    Write-TitvoLog -Level Error -Message "Archivo .env no encontrado en $EnvFile"
    exit 1
}

Import-TitvoDotEnv -EnvFile $EnvFile
Test-TitvoCommand -Name 'aws'
Test-TitvoCommand -Name 'terragrunt'

$script:Errors = 0

function Add-TitvoError {
    $script:Errors++
}

Write-Host '=================================================='
Write-Host '  TITVO Infrastructure Destroyer'
Write-Host '=================================================='
Write-Host ''

$requiredVars = @('AWS_STAGE', 'AWS_REGION', 'AWS_ACCOUNT_ID')
foreach ($var in $requiredVars) {
    $val = [Environment]::GetEnvironmentVariable($var, 'Process')
    if ([string]::IsNullOrWhiteSpace($val)) {
        Write-TitvoLog -Level Error -Message "Variable $var no esta definida en .env"
        exit 1
    }
}

$AwsStage = $env:AWS_STAGE
$AwsRegion = $env:AWS_REGION
$AwsAccountId = $env:AWS_ACCOUNT_ID

Write-TitvoLog -Level Info -Message "Cuenta AWS: $AwsAccountId"
Write-TitvoLog -Level Info -Message "Stage: $AwsStage"
Write-TitvoLog -Level Info -Message "Region: $AwsRegion"
Write-Host ''

Write-TitvoLog -Level Warning -Message 'Esta accion destruira TODA la infraestructura de TITVO'
if (-not (Confirm-TitvoAction -Message 'Estas seguro de continuar?' -Default 'n')) {
    Write-TitvoLog -Level Info -Message 'Operacion cancelada'
    exit 0
}

Write-Host ''
Write-TitvoLog -Level Info -Message 'Iniciando proceso de destruccion...'
Write-Host ''

$ProjectName = 'titvo-security-scan'
$CliFilesBucketName = "$ProjectName-reports-$AwsStage"
if ($AwsAccountId) {
    $CliFilesBucketName = "$CliFilesBucketName-$AwsAccountId"
}
$InfraDir = Join-Path $env:USERPROFILE '.titvo\infra'

function Test-TitvoEcrRepositoryName {
    param([string]$RepositoryName)
    $lower = $RepositoryName.ToLowerInvariant()
    return ($lower.StartsWith('tvo') -or $lower.StartsWith('titvo'))
}

function Clear-TitvoEcrRepositoryImages {
    param([string]$RepositoryName)

    Write-TitvoLog -Level Info -Message "Vaciando repositorio ECR '$RepositoryName'..."
    $totalDeleted = 0
    $nextToken = $null

    do {
        $listArgs = @(
            'ecr', 'list-images',
            '--repository-name', $RepositoryName,
            '--region', $AwsRegion,
            '--max-results', '1000',
            '--output', 'json'
        )
        if ($nextToken) { $listArgs += @('--next-token', $nextToken) }

        $response = Invoke-TitvoAwsJson -ArgumentList $listArgs -AllowFailure
        if (-not $response) { break }

        $imageIds = @()
        $responseImageIds = Get-TitvoAwsJsonProperty -Object $response -Name 'imageIds'
        if ($responseImageIds) { $imageIds = @($responseImageIds) }

        for ($i = 0; $i -lt $imageIds.Count; $i += 100) {
            $end = [Math]::Min($i + 99, $imageIds.Count - 1)
            $batch = @($imageIds[$i..$end])
            if ($batch.Count -eq 0) { continue }

            $tmpFile = Write-TitvoTempJsonFile -Object $batch -Prefix 'ecr-images'
            try {
                $fileUri = 'file://' + ($tmpFile -replace '\\', '/')
                Invoke-TitvoAws -ArgumentList @(
                    'ecr', 'batch-delete-image',
                    '--repository-name', $RepositoryName,
                    '--region', $AwsRegion,
                    '--image-ids', $fileUri
                ) -Quiet -AllowFailure | Out-Null
                $totalDeleted += $batch.Count
            }
            catch {
                Write-TitvoLog -Level Warning -Message "No se pudieron eliminar algunas imagenes de '$RepositoryName'"
                Add-TitvoError
            }
            finally {
                Remove-TitvoTempFile -Path $tmpFile
            }
        }

        $nextToken = Get-TitvoAwsJsonProperty -Object $response -Name 'nextToken'
        if ([string]::IsNullOrWhiteSpace([string]$nextToken)) { $nextToken = $null }
    } while ($nextToken)

    if ($totalDeleted -gt 0) {
        Write-TitvoLog -Level Success -Message "Eliminadas $totalDeleted imagenes de '$RepositoryName'"
    }
    else {
        Write-TitvoLog -Level Info -Message "Repositorio ECR '$RepositoryName' ya esta vacio"
    }
}

function Clear-TitvoEcrRepositories {
    $reposResponse = Invoke-TitvoAwsJson -ArgumentList @(
        'ecr', 'describe-repositories', '--region', $AwsRegion
    ) -AllowFailure

    if (-not $reposResponse -or -not $reposResponse.repositories) {
        Write-TitvoLog -Level Info -Message 'No se encontraron repositorios ECR en la region'
        return
    }

    foreach ($repo in @($reposResponse.repositories)) {
        if (-not (Test-TitvoEcrRepositoryName -RepositoryName $repo.repositoryName)) { continue }
        Clear-TitvoEcrRepositoryImages -RepositoryName $repo.repositoryName
    }
}

function Disable-TitvoEcsClusters {
    Write-TitvoLog -Level Info -Message "Paso 2.5/5: Deshabilitando clusters ECS con prefijo 'tvo'"

    $clustersResponse = Invoke-TitvoAwsJson -ArgumentList @(
        'ecs', 'list-clusters', '--region', $AwsRegion
    ) -AllowFailure

    $clusters = @()
    if ($clustersResponse -and $clustersResponse.clusterArns) {
        $clusters = @($clustersResponse.clusterArns)
    }

    if ($clusters.Count -eq 0) {
        Write-TitvoLog -Level Info -Message 'No se encontraron clusters ECS'
        Write-Host ''
        return
    }

    $foundTvo = $false

    foreach ($clusterArn in $clusters) {
        $clusterName = ($clusterArn -split '/')[-1]
        if (-not $clusterName.StartsWith('tvo')) { continue }

        $foundTvo = $true
        Write-TitvoLog -Level Info -Message "Procesando cluster ECS: $clusterName"

        $serviceArns = Get-TitvoAwsTextLines -ArgumentList @(
            'ecs', 'list-services',
            '--cluster', $clusterArn,
            '--region', $AwsRegion,
            '--query', 'serviceArns[]',
            '--output', 'text'
        )

        foreach ($serviceArn in $serviceArns) {
            $serviceName = ($serviceArn -split '/')[-1]
            Write-TitvoLog -Level Info -Message "Escalando servicio a 0: $serviceName"

            try {
                Invoke-TitvoAws -ArgumentList @(
                    'ecs', 'update-service',
                    '--cluster', $clusterArn,
                    '--service', $serviceArn,
                    '--desired-count', '0',
                    '--region', $AwsRegion
                ) -Quiet | Out-Null
            }
            catch {
                Write-TitvoLog -Level Warning -Message "No se pudo escalar $serviceName a 0"
                Add-TitvoError
            }

            try {
                Invoke-TitvoAws -ArgumentList @(
                    'ecs', 'wait', 'services-stable',
                    '--cluster', $clusterArn,
                    '--services', $serviceArn,
                    '--region', $AwsRegion
                ) -Quiet -AllowFailure | Out-Null
            }
            catch {
                Write-TitvoLog -Level Warning -Message "Timeout esperando estabilidad de $serviceName"
            }

            Write-TitvoLog -Level Info -Message "Eliminando servicio ECS: $serviceName"
            try {
                Invoke-TitvoAws -ArgumentList @(
                    'ecs', 'delete-service',
                    '--cluster', $clusterArn,
                    '--service', $serviceArn,
                    '--force',
                    '--region', $AwsRegion
                ) -Quiet | Out-Null
            }
            catch {
                Write-TitvoLog -Level Warning -Message "No se pudo eliminar servicio $serviceName"
                Add-TitvoError
            }
        }

        $taskArns = Get-TitvoAwsTextLines -ArgumentList @(
            'ecs', 'list-tasks',
            '--cluster', $clusterArn,
            '--region', $AwsRegion,
            '--query', 'taskArns[]',
            '--output', 'text'
        )

        foreach ($taskArn in $taskArns) {
            $taskId = ($taskArn -split '/')[-1]
            Write-TitvoLog -Level Info -Message "Deteniendo task ECS: $taskId"
            try {
                Invoke-TitvoAws -ArgumentList @(
                    'ecs', 'stop-task',
                    '--cluster', $clusterArn,
                    '--task', $taskArn,
                    '--reason', 'titvo destroy pre-drain',
                    '--region', $AwsRegion
                ) -Quiet | Out-Null
            }
            catch {
                Write-TitvoLog -Level Warning -Message "No se pudo detener task $taskId"
                Add-TitvoError
            }
        }

        Write-TitvoLog -Level Info -Message "Intentando eliminar cluster ECS: $clusterName"
        try {
            Invoke-TitvoAws -ArgumentList @(
                'ecs', 'delete-cluster',
                '--cluster', $clusterArn,
                '--region', $AwsRegion
            ) -Quiet | Out-Null
        }
        catch {
            Write-TitvoLog -Level Warning -Message "No se pudo eliminar cluster $clusterName (Terraform deberia intentar destruirlo)"
        }
    }

    if (-not $foundTvo) {
        Write-TitvoLog -Level Info -Message "No se encontraron clusters ECS con prefijo 'tvo'"
    }
    else {
        Write-TitvoLog -Level Success -Message 'Finalizo pre-proceso ECS'
    }
    Write-Host ''
}

function Remove-TitvoCloudMapNamespaceServices {
    $namespaceName = 'internal.titvo.com'
    Write-TitvoLog -Level Info -Message "Paso 2.6/5: Eliminando servicios Cloud Map del namespace '$namespaceName'"

    $namespaceId = Invoke-TitvoAws -ArgumentList @(
        'servicediscovery', 'list-namespaces',
        '--region', $AwsRegion,
        "--query", "Namespaces[?Name=='$namespaceName'].Id | [0]",
        '--output', 'text'
    ) -AllowFailure

    if ([string]::IsNullOrWhiteSpace($namespaceId) -or $namespaceId -eq 'None') {
        Write-TitvoLog -Level Info -Message "Namespace '$namespaceName' no encontrado"
        Write-Host ''
        return
    }

    $serviceIds = Get-TitvoAwsTextLines -ArgumentList @(
        'servicediscovery', 'list-services',
        '--region', $AwsRegion,
        '--filters', "Name=NAMESPACE_ID,Values=$namespaceId,Condition=EQ",
        '--query', 'Services[].Id',
        '--output', 'text'
    )

    if (@($serviceIds).Count -eq 0) {
        Write-TitvoLog -Level Info -Message "Namespace '$namespaceName' no tiene servicios asociados"
        Write-Host ''
        return
    }

    foreach ($serviceId in $serviceIds) {
        Write-TitvoLog -Level Info -Message "Eliminando Cloud Map service: $serviceId"
        try {
            Invoke-TitvoAws -ArgumentList @(
                'servicediscovery', 'delete-service',
                '--id', $serviceId,
                '--region', $AwsRegion
            ) -Quiet | Out-Null
        }
        catch {
            Write-TitvoLog -Level Warning -Message "No se pudo eliminar Cloud Map service $serviceId"
            Add-TitvoError
        }
    }

    Write-TitvoLog -Level Success -Message "Finalizo limpieza de servicios Cloud Map para '$namespaceName'"
    Write-Host ''
}

# Paso 1: ECR
Write-TitvoLog -Level Info -Message 'Paso 1/5: Limpiando repositorios ECR con prefijo tvo/titvo'
Clear-TitvoEcrRepositories
Write-Host ''

# Paso 2: S3
Write-TitvoLog -Level Info -Message 'Paso 2/5: Limpiando bucket S3'
$headOk = $true
try {
    Invoke-TitvoAws -ArgumentList @('s3api', 'head-bucket', '--bucket', $CliFilesBucketName) -Quiet | Out-Null
}
catch {
    $headOk = $false
}

if ($headOk) {
    Write-TitvoLog -Level Info -Message "Bucket '$CliFilesBucketName' encontrado"

    $objectCountText = Invoke-TitvoAws -ArgumentList @(
        's3', 'ls', "s3://$CliFilesBucketName", '--recursive'
    ) -AllowFailure
    $objectCount = 0
    if ($objectCountText) {
        $objectCount = @($objectCountText -split "`n" | Where-Object { $_.Trim() -ne '' }).Count
    }

    if ($objectCount -gt 0) {
        Write-TitvoLog -Level Info -Message "Eliminando $objectCount objetos del bucket..."

        $versioning = Invoke-TitvoAws -ArgumentList @(
            's3api', 'get-bucket-versioning',
            '--bucket', $CliFilesBucketName,
            '--query', 'Status',
            '--output', 'text'
        ) -AllowFailure
        if ([string]::IsNullOrWhiteSpace($versioning)) { $versioning = 'None' }

        if ($versioning -eq 'Enabled' -or $versioning -eq 'Suspended') {
            Write-TitvoLog -Level Warning -Message 'Bucket tiene versionado habilitado, eliminando todas las versiones...'
            $tmpFile = Join-Path $env:TEMP ("s3_versions_{0}.json" -f [Guid]::NewGuid().ToString('N'))

            try {
                Invoke-TitvoAws -ArgumentList @(
                    's3api', 'list-object-versions',
                    '--bucket', $CliFilesBucketName,
                    '--output', 'json',
                    '--query', '{Objects: Versions[].{Key:Key,VersionId:VersionId}}'
                ) -AllowFailure | Set-Content -LiteralPath $tmpFile -Encoding UTF8

                $versionsPayload = Get-Content -LiteralPath $tmpFile -Raw | ConvertFrom-Json
                $objCount = 0
                if ($versionsPayload.Objects) { $objCount = @($versionsPayload.Objects).Count }

                if ($objCount -gt 0) {
                    $fileUri = 'file://' + ($tmpFile -replace '\\', '/')
                    Invoke-TitvoAws -ArgumentList @(
                        's3api', 'delete-objects',
                        '--bucket', $CliFilesBucketName,
                        '--delete', $fileUri,
                        '--quiet'
                    ) -AllowFailure -Quiet | Out-Null
                }

                Invoke-TitvoAws -ArgumentList @(
                    's3api', 'list-object-versions',
                    '--bucket', $CliFilesBucketName,
                    '--output', 'json',
                    '--query', '{Objects: DeleteMarkers[].{Key:Key,VersionId:VersionId}}'
                ) -AllowFailure | Set-Content -LiteralPath $tmpFile -Encoding UTF8

                $markersPayload = Get-Content -LiteralPath $tmpFile -Raw | ConvertFrom-Json
                $markerCount = 0
                if ($markersPayload.Objects) { $markerCount = @($markersPayload.Objects).Count }

                if ($markerCount -gt 0) {
                    $fileUri = 'file://' + ($tmpFile -replace '\\', '/')
                    Invoke-TitvoAws -ArgumentList @(
                        's3api', 'delete-objects',
                        '--bucket', $CliFilesBucketName,
                        '--delete', $fileUri,
                        '--quiet'
                    ) -AllowFailure -Quiet | Out-Null
                }
            }
            finally {
                Remove-TitvoTempFile -Path $tmpFile
            }
        }
        else {
            Invoke-TitvoAws -ArgumentList @(
                's3', 'rm', "s3://$CliFilesBucketName", '--recursive', '--quiet'
            ) -AllowFailure -Quiet | Out-Null
        }

        Write-TitvoLog -Level Success -Message 'Bucket S3 limpiado'
    }
    else {
        Write-TitvoLog -Level Info -Message 'Bucket S3 ya esta vacio'
    }
}
else {
    Write-TitvoLog -Level Warning -Message "Bucket S3 '$CliFilesBucketName' no encontrado (puede ya estar eliminado)"
}
Write-Host ''

Disable-TitvoEcsClusters
Remove-TitvoCloudMapNamespaceServices

# Paso 3: Terragrunt
Write-TitvoLog -Level Info -Message 'Paso 3/5: Destruyendo infraestructura Terraform/Terragrunt'

$modules = @(
    'titvo-task-status-aws/aws',
    'titvo-task-trigger-aws/aws',
    'titvo-task-cli-files-aws/aws',
    'titvo-auth-setup-aws/aws',
    'titvo-mcp-gateway/aws',
    'titvo-agent-aws/aws',
    'titvo-rag-indexer/aws',
    'titvo-github-issue-aws/aws',
    'titvo-bitbucket-code-insights-aws/aws',
    'titvo-issue-report-aws/aws',
    'titvo-git-commit-files-aws/aws',
    'titvo-installer-ecr-publisher/aws',
    'titvo-security-scan-infra-aws/prod/us-east-1'
)

if (-not (Test-Path -LiteralPath $InfraDir)) {
    Write-TitvoLog -Level Warning -Message "Directorio de infraestructura '$InfraDir' no encontrado"
    Write-TitvoLog -Level Info -Message 'Saltando destruccion de Terraform'
}
else {
    Push-Location $InfraDir
    try {
        $lookupPath = Join-Path $InfraDir 'titvo-security-scan-infra-aws\prod\us-east-1\ssm\parameter\lookup'
        if (Test-Path -LiteralPath $lookupPath) {
            Push-Location $lookupPath
            $applyResult = Invoke-TitvoTerragrunt -ArgumentList @('apply', '--terragrunt-non-interactive')
            $applyResult.Output | Write-Host
            if ($applyResult.ExitCode -ne 0) {
                Write-TitvoLog -Level Warning -Message "terragrunt apply en lookup termino con rc=$($applyResult.ExitCode)"
            }
            Pop-Location
        }

        $upsertPath = Join-Path $InfraDir 'titvo-security-scan-infra-aws\prod\us-east-1\ssm\parameter\upsert'
        if (Test-Path -LiteralPath $upsertPath) {
            Remove-Item -LiteralPath $upsertPath -Recurse -Force
        }

        $totalModules = $modules.Count
        $current = 0

        foreach ($module in $modules) {
            $current++
            $moduleName = ($module -split '/')[0]
            $modulePath = Join-Path $InfraDir ($module -replace '/', '\')

            Write-Host ''
            Write-Host '----------------------------------------'
            Write-TitvoLog -Level Info -Message "[$current/$totalModules] Procesando modulo: $moduleName"
            Write-Host '----------------------------------------'

            if (-not (Test-Path -LiteralPath $modulePath)) {
                Write-TitvoLog -Level Warning -Message "Modulo $moduleName no encontrado (ya eliminado o no instalado)"
                continue
            }

            Push-Location $modulePath
            try {
                Write-TitvoLog -Level Info -Message 'Inicializando Terragrunt...'
                $initResult = Invoke-TitvoTerragrunt -ArgumentList @(
                    'run-all', 'init', '-reconfigure', '--terragrunt-non-interactive'
                )
                $initResult.Output |
                    Where-Object { $_ -notmatch 'terraform init' } |
                    Write-Host

                if ($initResult.ExitCode -eq 0) {
                    Write-TitvoLog -Level Success -Message 'Inicializacion completada'

                    Write-TitvoLog -Level Info -Message 'Destruyendo recursos...'
                    $destroyResult = Invoke-TitvoTerragrunt -ArgumentList @(
                        'run-all', 'destroy', '-auto-approve', '--terragrunt-non-interactive'
                    )
                    $destroyResult.Output | Write-Host
                    if ($destroyResult.ExitCode -eq 0) {
                        Write-TitvoLog -Level Success -Message "Modulo $moduleName destruido"
                    }
                    else {
                        Write-TitvoLog -Level Error -Message "Error al destruir modulo $moduleName"
                        Add-TitvoError
                    }
                }
                else {
                    Write-TitvoLog -Level Error -Message "Error al inicializar modulo $moduleName"
                    Add-TitvoError
                }
            }
            finally {
                Pop-Location
            }
        }
    }
    finally {
        Pop-Location
    }
}

# Paso 4: SSM
Write-TitvoLog -Level Info -Message 'Paso 4/5: Eliminando parametros SSM de infraestructura'
$ssmBasePath = '/tvo/security-scan/prod/infra'
$nextToken = $null
$deletedParams = 0

while ($true) {
    $args = @(
        'ssm', 'get-parameters-by-path',
        '--path', $ssmBasePath,
        '--recursive',
        '--with-decryption',
        '--region', $AwsRegion,
        '--max-results', '10'
    )
    if ($nextToken) { $args += @('--next-token', $nextToken) }

    try {
        $ssmResponse = Invoke-TitvoAwsJson -ArgumentList $args
    }
    catch {
        Write-TitvoLog -Level Error -Message "Error al listar parametros SSM en '$ssmBasePath'"
        Add-TitvoError
        break
    }

    $paramNames = @()
    $ssmParameters = Get-TitvoAwsJsonProperty -Object $ssmResponse -Name 'Parameters'
    if ($ssmParameters) {
        $paramNames = @($ssmParameters | ForEach-Object { $_.Name })
    }

    if ($paramNames.Count -gt 0) {
        try {
            $deleteArgs = @('ssm', 'delete-parameters', '--region', $AwsRegion, '--names') + $paramNames
            Invoke-TitvoAws -ArgumentList $deleteArgs -Quiet | Out-Null
            $deletedParams += $paramNames.Count
        }
        catch {
            Write-TitvoLog -Level Error -Message "Error al eliminar parametros SSM en '$ssmBasePath'"
            Add-TitvoError
        }
    }

    $nextToken = Get-TitvoAwsJsonProperty -Object $ssmResponse -Name 'NextToken'
    if ([string]::IsNullOrWhiteSpace([string]$nextToken)) { break }
}

if ($deletedParams -gt 0) {
    Write-TitvoLog -Level Success -Message "Parametros SSM eliminados: $deletedParams"
}
else {
    Write-TitvoLog -Level Info -Message "No se encontraron parametros SSM en '$ssmBasePath'"
}

# Paso 5: States y locks
Write-TitvoLog -Level Info -Message 'Paso 5/5: Eliminando states residuales de S3 y locks de DynamoDB'

$stateBuckets = @(
    "tvo-installer-ecr-publisher-$AwsRegion-$AwsAccountId",
    "tvo-agent-$AwsRegion-$AwsAccountId"
)
$stateKey = 'aws/ssm/upsert/terraform.tfstate'

foreach ($bucket in $stateBuckets) {
    Write-TitvoLog -Level Info -Message "Eliminando state: s3://$bucket/$stateKey"
    try {
        Invoke-TitvoAws -ArgumentList @(
            's3api', 'delete-object',
            '--bucket', $bucket,
            '--key', $stateKey,
            '--region', $AwsRegion
        ) -Quiet | Out-Null
    }
    catch {
        Write-TitvoLog -Level Warning -Message "No se pudo eliminar s3://$bucket/$stateKey (puede no existir)"
        Add-TitvoError
    }
}

$lockTable1 = "tvo-agent-$AwsRegion-$AwsAccountId-tfstate-lock"
$lockId1 = "tvo-agent-$AwsRegion-$AwsAccountId/aws/ssm/upsert/terraform.tfstate-md5"
Write-TitvoLog -Level Info -Message "Eliminando lock en tabla '$lockTable1'"
try {
    $keyJson = (@{ LockID = @{ S = $lockId1 } } | ConvertTo-Json -Compress)
    Invoke-TitvoAws -ArgumentList @(
        'dynamodb', 'delete-item',
        '--table-name', $lockTable1,
        '--region', $AwsRegion,
        '--key', $keyJson
    ) -Quiet | Out-Null
}
catch {
    Write-TitvoLog -Level Warning -Message "No se pudo eliminar item '$lockId1' en '$lockTable1' (puede no existir)"
    Add-TitvoError
}

$lockTable2 = "tvo-installer-ecr-publisher-$AwsRegion-$AwsAccountId-tfstate-lock"
$lockId2 = "tvo-installer-ecr-publisher-$AwsRegion-$AwsAccountId/aws/ssm/upsert/terraform.tfstate-md5"
Write-TitvoLog -Level Info -Message "Eliminando lock en tabla '$lockTable2'"
try {
    $keyJson = (@{ LockID = @{ S = $lockId2 } } | ConvertTo-Json -Compress)
    Invoke-TitvoAws -ArgumentList @(
        'dynamodb', 'delete-item',
        '--table-name', $lockTable2,
        '--region', $AwsRegion,
        '--key', $keyJson
    ) -Quiet | Out-Null
}
catch {
    Write-TitvoLog -Level Warning -Message "No se pudo eliminar item '$lockId2' en '$lockTable2' (puede no existir)"
    Add-TitvoError
}

Write-Host ''
Write-Host '=================================================='
if ($script:Errors -eq 0) {
    Write-TitvoLog -Level Success -Message 'Proceso completado exitosamente'
}
else {
    Write-TitvoLog -Level Warning -Message "Proceso completado con $($script:Errors) errores"
    Write-TitvoLog -Level Info -Message 'Revisa los logs arriba para mas detalles'
}
Write-Host '=================================================='

if ($script:Errors -gt 0) { exit 1 }
