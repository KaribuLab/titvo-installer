# titvo_cleanup_orphans.ps1
# Limpia recursos AWS huerfanos con prefijos tvo/titvo (port de titvo_cleanup_orphans.sh)
#
# Requisitos: PowerShell 5.1+, aws CLI en PATH
# Uso:
#   cd scripts
#   .\titvo_cleanup_orphans.ps1              # dry-run (solo muestra acciones)
#   .\titvo_cleanup_orphans.ps1 -Apply       # ejecuta borrados
#   $env:PREFIX = 'tvo'; .\titvo_cleanup_orphans.ps1 -Apply
#
# Si falla la ejecucion por politica:
#   powershell -NoProfile -ExecutionPolicy Bypass -File .\titvo_cleanup_orphans.ps1

[CmdletBinding()]
param(
    [switch]$Apply
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $ScriptDir 'lib\Titvo-Common.ps1')

Initialize-TitvoAwsEnvironment

$RepoRoot = Get-TitvoRepoRoot -ScriptRoot $ScriptDir
Import-TitvoDotEnv -EnvFile (Join-Path $RepoRoot '.env')

Test-TitvoCommand -Name 'aws'

$script:Region = if ($env:AWS_REGION) { $env:AWS_REGION } else { 'us-east-2' }
$script:DefaultPrefixes = @('tvo', 'titvo')
$script:Prefix = $null
$script:UsedIamRoles = New-Object 'System.Collections.Generic.HashSet[string]' ([StringComparer]::Ordinal)

function Mark-UsedIamRole {
    param([string]$Role)
    if ([string]::IsNullOrWhiteSpace($Role)) { return }
    [void]$script:UsedIamRoles.Add($Role)
}

function Test-IamRoleInUse {
    param([string]$Role)
    if ([string]::IsNullOrWhiteSpace($Role)) { return $false }
    return $script:UsedIamRoles.Contains($Role)
}

function Test-StartsWithCurrentPrefix {
    param([string]$Value)
    return Test-StartsWithPrefix -Value $Value -Prefix $script:Prefix
}

function Get-ListCandidateSecurityGroupIds {
    $sgs = Invoke-TitvoAwsJson -ArgumentList @(
        'ec2', 'describe-security-groups', '--region', $script:Region
    )

    $prefix = $script:Prefix.ToLowerInvariant()
    $results = @()

    foreach ($sg in @($sgs.SecurityGroups)) {
        if ($sg.GroupName -eq 'default') { continue }
        if ($sg.Description -eq 'default VPC security group') { continue }

        $nameMatch = $sg.GroupName.ToLowerInvariant().StartsWith($prefix) -or
            $sg.GroupName.ToLowerInvariant().Contains($prefix)
        $descMatch = $sg.Description.ToLowerInvariant().Contains($prefix)
        $tagMatch = Test-TitvoAwsTagPrefixMatch -Resource $sg -Prefix $script:Prefix

        if ($nameMatch -or $descMatch -or $tagMatch) {
            $results += $sg.GroupId
        }
    }

    return $results
}

function Get-TargetedPermissions {
    param(
        [string]$TargetSg,
        [ValidateSet('ingress', 'egress')]
        [string]$Direction
    )

    if ($Direction -eq 'egress') {
        $filterName = 'egress.ip-permission.group-id'
        $field = 'IpPermissionsEgress'
    }
    else {
        $filterName = 'ip-permission.group-id'
        $field = 'IpPermissions'
    }

    $response = Invoke-TitvoAwsJson -ArgumentList @(
        'ec2', 'describe-security-groups',
        "--filters", "Name=$filterName,Values=$TargetSg",
        '--region', $script:Region
    ) -AllowFailure

    $pairs = @()
    if (-not $response -or -not $response.SecurityGroups) { return $pairs }

    foreach ($sg in @($response.SecurityGroups)) {
        $perms = $sg.$field
        if (-not $perms) { continue }

        foreach ($perm in @($perms)) {
            $hasTarget = $false
            $userIdGroupPairs = Get-TitvoAwsJsonProperty -Object $perm -Name 'UserIdGroupPairs'
            if ($userIdGroupPairs) {
                foreach ($pair in @($userIdGroupPairs)) {
                    if ($pair.GroupId -eq $TargetSg) { $hasTarget = $true; break }
                }
            }
            if (-not $hasTarget) { continue }

            $minimal = [ordered]@{
                IpProtocol = $perm.IpProtocol
                UserIdGroupPairs = @(@{ GroupId = $TargetSg })
                IpRanges = @()
                Ipv6Ranges = @()
                PrefixListIds = @()
            }
            if ($null -ne $perm.FromPort) { $minimal.FromPort = $perm.FromPort }
            if ($null -ne $perm.ToPort) { $minimal.ToPort = $perm.ToPort }

            $pairs += [pscustomobject]@{
                GroupId = $sg.GroupId
                Permission = ($minimal | ConvertTo-Json -Compress -Depth 5)
            }
        }
    }

    return $pairs
}

function Revoke-CrossSgReferences {
    param(
        [string]$TargetSg,
        [switch]$Apply
    )

    foreach ($pair in (Get-TargetedPermissions -TargetSg $TargetSg -Direction 'ingress')) {
        $tmp = Write-TitvoTempJsonFile -Object ($pair.Permission | ConvertFrom-Json) -Prefix 'sg-perm'
        try {
            $fileUri = 'file://' + ($tmp -replace '\\', '/')
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'ec2', 'revoke-security-group-ingress',
                '--group-id', $pair.GroupId,
                '--ip-permissions', $fileUri,
                '--region', $script:Region
            ) | Out-Null
        }
        finally {
            Remove-TitvoTempFile -Path $tmp
        }
    }

    foreach ($pair in (Get-TargetedPermissions -TargetSg $TargetSg -Direction 'egress')) {
        $tmp = Write-TitvoTempJsonFile -Object ($pair.Permission | ConvertFrom-Json) -Prefix 'sg-perm'
        try {
            $fileUri = 'file://' + ($tmp -replace '\\', '/')
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'ec2', 'revoke-security-group-egress',
                '--group-id', $pair.GroupId,
                '--ip-permissions', $fileUri,
                '--region', $script:Region
            ) | Out-Null
        }
        finally {
            Remove-TitvoTempFile -Path $tmp
        }
    }
}

function Wait-ForNetworkInterfacesDeleted {
    param(
        [string[]]$EniIds,
        [switch]$Apply
    )

    if (-not $Apply -or $EniIds.Count -eq 0) { return }

    $remaining = @($EniIds)
    Start-TitvoWait -MaxAttempts 36 -SleepSeconds 5 `
        -WaitMessage "Network Interfaces aun presentes: $($remaining.Count)" `
        -WarnMessage 'Algunas Network Interfaces siguen presentes; intento continuar' `
        -Condition {
            $still = @()
            foreach ($eniId in $remaining) {
                try {
                    $r = Invoke-TitvoAwsJson -ArgumentList @(
                        'ec2', 'describe-network-interfaces',
                        '--network-interface-ids', $eniId,
                        '--region', $script:Region
                    ) -AllowFailure
                    if ($r -and $r.NetworkInterfaces -and $r.NetworkInterfaces.Count -gt 0) {
                        $still += $eniId
                    }
                }
                catch { }
            }
            $script:remaining = $still
            return ($still.Count -eq 0)
        } | Out-Null
}

function Detach-AndDeleteEnisForSg {
    param(
        [string]$GroupId,
        [switch]$Apply
    )

    $response = Invoke-TitvoAwsJson -ArgumentList @(
        'ec2', 'describe-network-interfaces',
        '--filters', "Name=group-id,Values=$GroupId",
        '--region', $script:Region
    ) -AllowFailure

    $eniIds = @()
    if ($response -and $response.NetworkInterfaces) {
        $eniIds = @($response.NetworkInterfaces | ForEach-Object { $_.NetworkInterfaceId })
    }

    foreach ($eniId in $eniIds) {
        Remove-TitvoNetworkInterface -EniId $eniId -Region $script:Region -Apply:$Apply
    }

    if ($Apply -and $eniIds.Count -gt 0) {
        Wait-ForNetworkInterfacesDeleted -EniIds $eniIds -Apply
    }
}

function Show-SgDependencies {
    param([string]$GroupId)

    Write-Host "[DIAG] Dependencias residuales de ${GroupId}:"

    $enis = Invoke-TitvoAwsJson -ArgumentList @(
        'ec2', 'describe-network-interfaces',
        '--filters', "Name=group-id,Values=$GroupId",
        '--region', $script:Region
    ) -AllowFailure

    if ($enis -and $enis.NetworkInterfaces -and $enis.NetworkInterfaces.Count -gt 0) {
        foreach ($eni in @($enis.NetworkInterfaces)) {
            $attach = 'none'
            $attachment = Get-TitvoAwsJsonProperty -Object $eni -Name 'Attachment'
            if ($attachment) {
                $instanceId = Get-TitvoAwsJsonProperty -Object $attachment -Name 'InstanceId'
                if ($instanceId) { $attach = $instanceId }
                else {
                    $attachId = Get-TitvoAwsJsonProperty -Object $attachment -Name 'AttachmentId'
                    if ($attachId) { $attach = $attachId }
                }
            }
            Write-Host "  ENI $($eni.NetworkInterfaceId) status=$($eni.Status) desc=`"$($eni.Description)`" attach=$attach"
        }
    }
    else {
        Write-Host '  (sin ENIs)'
    }

    foreach ($filter in @('ip-permission.group-id', 'egress.ip-permission.group-id')) {
        $label = if ($filter -like 'egress*') { 'egress' } else { 'ingress' }
        $refs = Invoke-TitvoAwsJson -ArgumentList @(
            'ec2', 'describe-security-groups',
            "--filters", "Name=$filter,Values=$GroupId",
            '--region', $script:Region
        ) -AllowFailure
        if ($refs -and $refs.SecurityGroups) {
            foreach ($sg in @($refs.SecurityGroups)) {
                Write-Host "  SG ref ${label}: $($sg.GroupId) ($($sg.GroupName))"
            }
        }
    }
}

function Try-DeleteSecurityGroup {
    param(
        [string]$GroupId,
        [switch]$Apply
    )

    if (-not $Apply) {
        Write-Host "[DRY-RUN] aws ec2 delete-security-group --group-id `"$GroupId`" --region `"$($script:Region)`""
        return $true
    }

    for ($attempt = 1; $attempt -le 6; $attempt++) {
        $result = Invoke-TitvoAwsProcess -ArgumentList @(
            'ec2', 'delete-security-group', '--group-id', $GroupId, '--region', $script:Region
        )
        $exitCode = $result.ExitCode

        if ($exitCode -eq 0) {
            Write-Host "[OK] SG eliminado: $GroupId"
            return $true
        }

        $err = ($result.Output -join "`n").Trim()
        if ($err -match 'InvalidGroup\.NotFound') {
            Write-Host "[OK] SG $GroupId ya no existe"
            return $true
        }

        Write-Host "[WARN] No se pudo borrar $GroupId (intento $attempt/6): $err"

        if ($err -match 'DependencyViolation') {
            Detach-AndDeleteEnisForSg -GroupId $GroupId -Apply
            Revoke-CrossSgReferences -TargetSg $GroupId -Apply
        }

        Start-Sleep -Seconds 10
    }

    Write-Host "[ERROR] No se logro borrar SG $GroupId tras 6 intentos"
    Show-SgDependencies -GroupId $GroupId
    return $false
}

function Remove-TitvoSecurityGroups {
    param([switch]$Apply)

    $groupIds = Get-ListCandidateSecurityGroupIds

    foreach ($groupId in $groupIds) {
        if ([string]::IsNullOrWhiteSpace($groupId)) { continue }
        Revoke-CrossSgReferences -TargetSg $groupId -Apply:$Apply
        Detach-AndDeleteEnisForSg -GroupId $groupId -Apply:$Apply
    }

    foreach ($groupId in $groupIds) {
        if ([string]::IsNullOrWhiteSpace($groupId)) { continue }
        Try-DeleteSecurityGroup -GroupId $groupId -Apply:$Apply | Out-Null
    }
}

function Remove-TitvoEventBridgeRulesForBus {
    param(
        [string]$BusName,
        [switch]$Apply
    )

    if ($BusName -eq 'default') {
        $rulesResponse = Invoke-TitvoAwsJson -ArgumentList @(
            'events', 'list-rules',
            '--name-prefix', $script:Prefix,
            '--region', $script:Region
        )
    }
    else {
        $rulesResponse = Invoke-TitvoAwsJson -ArgumentList @(
            'events', 'list-rules',
            '--event-bus-name', $BusName,
            '--region', $script:Region
        )
    }

    $rules = @()
    if ($rulesResponse -and $rulesResponse.Rules) {
        $rules = @($rulesResponse.Rules | ForEach-Object { $_.Name })
    }

    foreach ($rule in $rules) {
        if ([string]::IsNullOrWhiteSpace($rule)) { continue }

        if ($BusName -eq 'default') {
            $targetsResponse = Invoke-TitvoAwsJson -ArgumentList @(
                'events', 'list-targets-by-rule',
                '--rule', $rule,
                '--region', $script:Region
            )
        }
        else {
            $targetsResponse = Invoke-TitvoAwsJson -ArgumentList @(
                'events', 'list-targets-by-rule',
                '--rule', $rule,
                '--event-bus-name', $BusName,
                '--region', $script:Region
            )
        }

        $ids = @()
        if ($targetsResponse -and $targetsResponse.Targets) {
            $ids = @($targetsResponse.Targets | ForEach-Object { $_.Id })
        }

        if ($ids.Count -gt 0) {
            $idsJson = ($ids | ConvertTo-Json -Compress)
            if ($BusName -eq 'default') {
                Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                    'events', 'remove-targets',
                    '--rule', $rule,
                    '--ids', $idsJson,
                    '--region', $script:Region
                ) | Out-Null
            }
            else {
                Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                    'events', 'remove-targets',
                    '--rule', $rule,
                    '--event-bus-name', $BusName,
                    '--ids', $idsJson,
                    '--region', $script:Region
                ) | Out-Null
            }
        }

        if ($BusName -eq 'default') {
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'events', 'delete-rule',
                '--name', $rule,
                '--region', $script:Region
            ) | Out-Null
        }
        else {
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'events', 'delete-rule',
                '--name', $rule,
                '--event-bus-name', $BusName,
                '--region', $script:Region
            ) | Out-Null
        }
    }
}

function Wait-ForCloudMapOperation {
    param([string]$OperationId)

    Start-TitvoWait -MaxAttempts 24 -SleepSeconds 5 `
        -WaitMessage "Cloud Map operation $OperationId pendiente" `
        -WarnMessage "Cloud Map operation sigue pendiente: $OperationId" `
        -Condition {
            $op = Invoke-TitvoAwsJson -ArgumentList @(
                'servicediscovery', 'get-operation',
                '--operation-id', $OperationId,
                '--region', $script:Region
            )
            $status = $op.Operation.Status
            if ($status -eq 'SUCCESS') { return $true }
            if ($status -eq 'FAIL') {
                $msg = $op.Operation.ErrorMessage
                if (-not $msg) { $msg = 'Cloud Map operation failed' }
                Write-Host "[WARN] Cloud Map operation failed: $msg"
                return $true
            }
            Write-Host "[WAIT] Cloud Map operation $OperationId status=$status"
            return $false
        } | Out-Null
}

function Remove-TitvoCloudMapNamespace {
    param(
        [string]$NamespaceId,
        [switch]$Apply
    )

    $services = Invoke-TitvoAwsJson -ArgumentList @(
        'servicediscovery', 'list-services',
        '--filters', "Name=NAMESPACE_ID,Values=$NamespaceId,Condition=EQ",
        '--region', $script:Region
    )

    if ($services -and $services.Services) {
        foreach ($svc in @($services.Services)) {
            $instances = Invoke-TitvoAwsJson -ArgumentList @(
                'servicediscovery', 'list-instances',
                '--service-id', $svc.Id,
                '--region', $script:Region
            )
            if ($instances -and $instances.Instances) {
                foreach ($inst in @($instances.Instances)) {
                    Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                        'servicediscovery', 'deregister-instance',
                        '--service-id', $svc.Id,
                        '--instance-id', $inst.Id,
                        '--region', $script:Region
                    ) | Out-Null
                }
            }
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'servicediscovery', 'delete-service',
                '--id', $svc.Id,
                '--region', $script:Region
            ) | Out-Null
        }
    }

    if ($Apply) {
        $deleteResp = Invoke-TitvoAwsJson -ArgumentList @(
            'servicediscovery', 'delete-namespace',
            '--id', $NamespaceId,
            '--region', $script:Region
        )
        $opId = $deleteResp.OperationId
        if ($opId) {
            Write-Host "[WAIT] Cloud Map namespace delete operation: $opId"
            Wait-ForCloudMapOperation -OperationId $opId
        }
    }
    else {
        Write-Host "[DRY-RUN] aws servicediscovery delete-namespace --id `"$NamespaceId`" --region `"$($script:Region)`""
    }
}

function Remove-TitvoSubnets {
    param([switch]$Apply)

    $subnets = Invoke-TitvoAwsJson -ArgumentList @(
        'ec2', 'describe-subnets', '--region', $script:Region
    )

    $prefix = $script:Prefix.ToLowerInvariant()
    $subnetIds = @()

    foreach ($sn in @($subnets.Subnets)) {
        if (Test-TitvoAwsTagPrefixMatch -Resource $sn -Prefix $script:Prefix) {
            $subnetIds += $sn.SubnetId
        }
    }

    foreach ($subnetId in $subnetIds) {
        Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
            'ec2', 'delete-subnet', '--subnet-id', $subnetId, '--region', $script:Region
        ) | Out-Null
    }

    if ($Apply -and $subnetIds.Count -gt 0) {
        Start-TitvoWait -MaxAttempts 24 -SleepSeconds 5 `
            -WaitMessage "Subnets aun presentes: $($subnetIds.Count)" `
            -WarnMessage 'Algunas subnets siguen presentes' `
            -Condition {
                $still = @()
                foreach ($sid in $subnetIds) {
                    try {
                        Invoke-TitvoAwsJson -ArgumentList @(
                            'ec2', 'describe-subnets', '--subnet-ids', $sid, '--region', $script:Region
                        ) -AllowFailure | Out-Null
                        $still += $sid
                    }
                    catch { }
                }
                if ($still.Count -gt 0) {
                    foreach ($sid in $still) {
                        Invoke-TitvoRun -Apply -ArgumentList @(
                            'ec2', 'delete-subnet', '--subnet-id', $sid, '--region', $script:Region
                        ) | Out-Null
                    }
                }
                return ($still.Count -eq 0)
            } | Out-Null
    }
}

function Remove-TitvoVpcEndpoints {
    param([switch]$Apply)

    $sgIds = Get-ListCandidateSecurityGroupIds
    $endpoints = Invoke-TitvoAwsJson -ArgumentList @(
        'ec2', 'describe-vpc-endpoints', '--region', $script:Region
    )

    $prefix = $script:Prefix.ToLowerInvariant()
    $vpceIds = @()

    foreach ($vpce in @($endpoints.VpcEndpoints)) {
        $match = $false
        $vpceGroups = Get-TitvoAwsJsonProperty -Object $vpce -Name 'Groups'
        if ($vpceGroups) {
            foreach ($g in @($vpceGroups)) {
                if ($sgIds -contains $g.GroupId) { $match = $true; break }
            }
        }
        if (-not $match) {
            $match = Test-TitvoAwsTagPrefixMatch -Resource $vpce -Prefix $script:Prefix
        }
        if ($match) { $vpceIds += $vpce.VpcEndpointId }
    }

    foreach ($vpceId in $vpceIds) {
        Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
            'ec2', 'delete-vpc-endpoints',
            '--vpc-endpoint-ids', $vpceId,
            '--region', $script:Region
        ) | Out-Null
    }

    if ($Apply -and $vpceIds.Count -gt 0) {
        Start-TitvoWait -MaxAttempts 60 -SleepSeconds 5 `
            -WaitMessage "VPC endpoints aun presentes: $($vpceIds.Count)" `
            -WarnMessage 'Algunos VPC endpoints siguen presentes; intento continuar con security groups' `
            -Condition {
                try {
                    $r = Invoke-TitvoAwsJson -ArgumentList @(
                        'ec2', 'describe-vpc-endpoints',
                        '--vpc-endpoint-ids', ($vpceIds -join ','),
                        '--region', $script:Region
                    ) -AllowFailure
                    $count = 0
                    if ($r -and $r.VpcEndpoints) { $count = @($r.VpcEndpoints).Count }
                    return ($count -eq 0)
                }
                catch { return $true }
            } | Out-Null
    }
}

function Wait-ForBatchJobQueuesDeletion {
    param([switch]$Apply)
    if (-not $Apply) { return }

    Start-TitvoWait -MaxAttempts 24 -SleepSeconds 5 `
        -WaitMessage 'Batch job queues aun presentes' `
        -WarnMessage 'Batch job queues siguen presentes; intento continuar con compute environments' `
        -Condition {
            $r = Invoke-TitvoAwsJson -ArgumentList @('batch', 'describe-job-queues', '--region', $script:Region)
            $count = 0
            if ($r -and $r.jobQueues) {
                $count = @($r.jobQueues | Where-Object { $_.jobQueueName.StartsWith($script:Prefix) }).Count
            }
            return ($count -eq 0)
        } | Out-Null
}

function Wait-ForBatchJobQueueDisabled {
    param([string]$JobQueue)
    Start-TitvoWait -MaxAttempts 36 -SleepSeconds 5 `
        -WaitMessage "Job queue $JobQueue modificandose" `
        -WarnMessage "Job queue $JobQueue sigue modificandose; intento borrarlo igual" `
        -Condition {
            $r = Invoke-TitvoAwsJson -ArgumentList @(
                'batch', 'describe-job-queues',
                '--job-queues', $JobQueue,
                '--region', $script:Region
            ) -AllowFailure
            if (-not $r -or -not $r.jobQueues -or $r.jobQueues.Count -eq 0) { return $true }
            $jq = $r.jobQueues[0]
            return ($jq.status -eq 'VALID' -and $jq.state -eq 'DISABLED')
        } | Out-Null
}

function Wait-ForBatchComputeEnvironmentDisabled {
    param([string]$ComputeEnvironment)
    Start-TitvoWait -MaxAttempts 36 -SleepSeconds 5 `
        -WaitMessage "Compute environment $ComputeEnvironment modificandose" `
        -WarnMessage "Compute environment $ComputeEnvironment sigue modificandose; intento borrarlo igual" `
        -Condition {
            $r = Invoke-TitvoAwsJson -ArgumentList @(
                'batch', 'describe-compute-environments',
                '--compute-environments', $ComputeEnvironment,
                '--region', $script:Region
            ) -AllowFailure
            if (-not $r -or -not $r.computeEnvironments -or $r.computeEnvironments.Count -eq 0) { return $true }
            $ce = $r.computeEnvironments[0]
            return ($ce.status -eq 'VALID' -and $ce.state -eq 'DISABLED')
        } | Out-Null
}

function Wait-ForLambdaEventSourceMappingsDeleted {
    param([switch]$Apply)
    if (-not $Apply) { return }

    Start-TitvoWait -MaxAttempts 24 -SleepSeconds 5 `
        -WaitMessage 'Event Source Mappings aun presentes' `
        -WarnMessage 'Algunos Event Source Mappings siguen presentes' `
        -Condition {
            $r = Invoke-TitvoAwsJson -ArgumentList @(
                'lambda', 'list-event-source-mappings',
                '--max-items', '100',
                '--region', $script:Region
            )
            $count = 0
            if ($r -and $r.EventSourceMappings) {
                $count = @($r.EventSourceMappings | Where-Object { $_.FunctionArn -like "*$($script:Prefix)*" }).Count
            }
            return ($count -eq 0)
        } | Out-Null
}

function Wait-ForEcsClusterEmpty {
    param([string]$Cluster)
    Start-TitvoWait -MaxAttempts 36 -SleepSeconds 5 `
        -WaitMessage "Cluster $Cluster aun tiene tareas" `
        -WarnMessage "Cluster $Cluster sigue con tareas activas" `
        -Condition {
            $r = Invoke-TitvoAwsJson -ArgumentList @(
                'ecs', 'list-tasks', '--cluster', $Cluster, '--region', $script:Region
            )
            $count = 0
            if ($r -and $r.taskArns) { $count = @($r.taskArns).Count }
            return ($count -eq 0)
        } | Out-Null
}

function Remove-TitvoIamPolicy {
    param(
        [string]$PolicyArn,
        [switch]$Apply
    )

    $entities = Invoke-TitvoAwsJson -ArgumentList @(
        'iam', 'list-entities-for-policy', '--policy-arn', $PolicyArn
    )

    $policyRoles = Get-TitvoAwsJsonProperty -Object $entities -Name 'PolicyRoles'
    if ($policyRoles) {
        foreach ($pr in @($policyRoles)) {
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'iam', 'detach-role-policy', '--role-name', $pr.RoleName, '--policy-arn', $PolicyArn
            ) | Out-Null
        }
    }
    $policyUsers = Get-TitvoAwsJsonProperty -Object $entities -Name 'PolicyUsers'
    if ($policyUsers) {
        foreach ($pu in @($policyUsers)) {
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'iam', 'detach-user-policy', '--user-name', $pu.UserName, '--policy-arn', $PolicyArn
            ) | Out-Null
        }
    }
    $policyGroups = Get-TitvoAwsJsonProperty -Object $entities -Name 'PolicyGroups'
    if ($policyGroups) {
        foreach ($pg in @($policyGroups)) {
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'iam', 'detach-group-policy', '--group-name', $pg.GroupName, '--policy-arn', $PolicyArn
            ) | Out-Null
        }
    }

    $policy = Invoke-TitvoAwsJson -ArgumentList @('iam', 'get-policy', '--policy-arn', $PolicyArn)
    $defaultVersion = $policy.Policy.DefaultVersionId

    $versions = Invoke-TitvoAwsJson -ArgumentList @(
        'iam', 'list-policy-versions', '--policy-arn', $PolicyArn
    )
    $policyVersions = Get-TitvoAwsJsonProperty -Object $versions -Name 'Versions'
    if ($policyVersions) {
        foreach ($v in @($policyVersions)) {
            if ($v.VersionId -eq $defaultVersion) { continue }
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'iam', 'delete-policy-version', '--policy-arn', $PolicyArn, '--version-id', $v.VersionId
            ) | Out-Null
        }
    }

    Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
        'iam', 'delete-policy', '--policy-arn', $PolicyArn
    ) | Out-Null
}

function Remove-TitvoIamRole {
    param(
        [string]$RoleName,
        [switch]$Apply
    )

    $attached = Invoke-TitvoAwsJson -ArgumentList @(
        'iam', 'list-attached-role-policies', '--role-name', $RoleName
    )
    $attachedPolicies = Get-TitvoAwsJsonProperty -Object $attached -Name 'AttachedPolicies'
    if ($attachedPolicies) {
        foreach ($p in @($attachedPolicies)) {
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'iam', 'detach-role-policy', '--role-name', $RoleName, '--policy-arn', $p.PolicyArn
            ) | Out-Null
        }
    }

    $inline = Invoke-TitvoAwsJson -ArgumentList @(
        'iam', 'list-role-policies', '--role-name', $RoleName
    )
    $policyNames = Get-TitvoAwsJsonProperty -Object $inline -Name 'PolicyNames'
    if ($policyNames) {
        foreach ($pn in @($policyNames)) {
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'iam', 'delete-role-policy', '--role-name', $RoleName, '--policy-name', $pn
            ) | Out-Null
        }
    }

    $profiles = Invoke-TitvoAwsJson -ArgumentList @(
        'iam', 'list-instance-profiles-for-role', '--role-name', $RoleName
    )
    $instanceProfiles = Get-TitvoAwsJsonProperty -Object $profiles -Name 'InstanceProfiles'
    if ($instanceProfiles) {
        foreach ($ip in @($instanceProfiles)) {
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'iam', 'remove-role-from-instance-profile',
                '--instance-profile-name', $ip.InstanceProfileName,
                '--role-name', $RoleName
            ) | Out-Null
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'iam', 'delete-instance-profile', '--instance-profile-name', $ip.InstanceProfileName
            ) | Out-Null
        }
    }

    Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
        'iam', 'delete-role', '--role-name', $RoleName
    ) | Out-Null
}

function Collect-UsedIamReferences {
    # Recolectar despues de borrar Lambda/ECS/Batch para no marcar roles de recursos ya eliminados.
    $script:UsedIamRoles.Clear()

    Write-Host 'Recolectando referencias en uso (IAM)...'

    $lambdas = Invoke-TitvoAwsJson -ArgumentList @(
        'lambda', 'list-functions', '--region', $script:Region
    )
    if ($lambdas -and $lambdas.Functions) {
        foreach ($fn in @($lambdas.Functions)) {
            if ($fn.Role) {
                Mark-UsedIamRole -Role (($fn.Role -split '/')[-1])
            }
        }
    }

    $taskDefs = Invoke-TitvoAwsJson -ArgumentList @(
        'ecs', 'list-task-definitions', '--status', 'ACTIVE', '--region', $script:Region
    )
    if ($taskDefs -and $taskDefs.taskDefinitionArns) {
        foreach ($td in @($taskDefs.taskDefinitionArns)) {
            $data = Invoke-TitvoAwsJson -ArgumentList @(
                'ecs', 'describe-task-definition', '--task-definition', $td, '--region', $script:Region
            )
            if ($data.taskDefinition.taskRoleArn) {
                Mark-UsedIamRole -Role (($data.taskDefinition.taskRoleArn -split '/')[-1])
            }
            if ($data.taskDefinition.executionRoleArn) {
                Mark-UsedIamRole -Role (($data.taskDefinition.executionRoleArn -split '/')[-1])
            }
        }
    }

    $jobDefs = Invoke-TitvoAwsJson -ArgumentList @(
        'batch', 'describe-job-definitions', '--status', 'ACTIVE', '--region', $script:Region
    )
    if ($jobDefs -and $jobDefs.jobDefinitions) {
        foreach ($jd in @($jobDefs.jobDefinitions)) {
            $cp = $jd.containerProperties
            if ($cp.jobRoleArn) { Mark-UsedIamRole -Role (($cp.jobRoleArn -split '/')[-1]) }
            if ($cp.executionRoleArn) { Mark-UsedIamRole -Role (($cp.executionRoleArn -split '/')[-1]) }
        }
    }
}

function Invoke-TitvoCleanupForPrefix {
    param(
        [Parameter(Mandatory = $true)]
        [string]$CurrentPrefix,

        [switch]$Apply
    )

    $script:Prefix = $CurrentPrefix
    Write-Host "== Inicio limpieza candidatos prefix '$($script:Prefix)' (region $($script:Region)) =="

    # 1) EventBridge
    Remove-TitvoEventBridgeRulesForBus -BusName 'default' -Apply:$Apply

    $buses = Invoke-TitvoAwsJson -ArgumentList @(
        'events', 'list-event-buses', '--region', $script:Region
    )
    if ($buses -and $buses.EventBuses) {
        foreach ($bus in @($buses.EventBuses)) {
            if ($bus.Name -eq 'default') { continue }
            if (-not $bus.Name.StartsWith($script:Prefix)) { continue }
            Remove-TitvoEventBridgeRulesForBus -BusName $bus.Name -Apply:$Apply
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'events', 'delete-event-bus', '--name', $bus.Name, '--region', $script:Region
            ) | Out-Null
        }
    }

    # 2) API Gateway v2
    $apis = Invoke-TitvoAwsJson -ArgumentList @('apigatewayv2', 'get-apis', '--region', $script:Region)
    if ($apis -and $apis.Items) {
        foreach ($api in @($apis.Items)) {
            if ($api.Name.StartsWith($script:Prefix)) {
                Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                    'apigatewayv2', 'delete-api', '--api-id', $api.ApiId, '--region', $script:Region
                ) | Out-Null
            }
        }
    }

    # 3) Lambda Event Source Mappings
    Write-Host 'Limpiando Lambda Event Source Mappings...'
    $esm = Invoke-TitvoAwsJson -ArgumentList @(
        'lambda', 'list-event-source-mappings', '--max-items', '100', '--region', $script:Region
    )
    if ($esm -and $esm.EventSourceMappings) {
        foreach ($m in @($esm.EventSourceMappings)) {
            if ($m.FunctionArn -like "*$($script:Prefix)*") {
                Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                    'lambda', 'delete-event-source-mapping', '--uuid', $m.UUID, '--region', $script:Region
                ) | Out-Null
            }
        }
    }
    Wait-ForLambdaEventSourceMappingsDeleted -Apply:$Apply

    # 4) Lambda
    $lambdaList = Invoke-TitvoAwsJson -ArgumentList @(
        'lambda', 'list-functions', '--region', $script:Region
    )
    if ($lambdaList -and $lambdaList.Functions) {
        foreach ($fn in @($lambdaList.Functions)) {
            if ($fn.FunctionName.StartsWith($script:Prefix)) {
                Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                    'lambda', 'delete-function', '--function-name', $fn.FunctionName, '--region', $script:Region
                ) | Out-Null
            }
        }
    }

    # 5) SQS
    $queues = Invoke-TitvoAwsJson -ArgumentList @(
        'sqs', 'list-queues', '--queue-name-prefix', $script:Prefix, '--region', $script:Region
    ) -AllowFailure
    if ($queues -and $queues.QueueUrls) {
        foreach ($qurl in @($queues.QueueUrls)) {
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'sqs', 'delete-queue', '--queue-url', $qurl, '--region', $script:Region
            ) | Out-Null
        }
    }

    # 6) ECS
    $clusters = Invoke-TitvoAwsJson -ArgumentList @(
        'ecs', 'list-clusters', '--region', $script:Region
    )
    if ($clusters -and $clusters.clusterArns) {
        foreach ($c in @($clusters.clusterArns)) {
            $cname = ($c -split '/')[-1]
            if (-not (Test-StartsWithCurrentPrefix -Value $cname)) { continue }

            $services = Invoke-TitvoAwsJson -ArgumentList @(
                'ecs', 'list-services', '--cluster', $c, '--region', $script:Region
            )
            if ($services -and $services.serviceArns) {
                foreach ($s in @($services.serviceArns)) {
                    $sname = ($s -split '/')[-1]
                    Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                        'ecs', 'update-service', '--cluster', $c, '--service', $sname,
                        '--desired-count', '0', '--region', $script:Region
                    ) | Out-Null
                    Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                        'ecs', 'delete-service', '--cluster', $c, '--service', $sname,
                        '--force', '--region', $script:Region
                    ) | Out-Null
                }
            }

            if ($Apply) { Wait-ForEcsClusterEmpty -Cluster $c }
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'ecs', 'delete-cluster', '--cluster', $c, '--region', $script:Region
            ) | Out-Null
        }
    }

    $taskDefList = Invoke-TitvoAwsJson -ArgumentList @(
        'ecs', 'list-task-definitions', '--status', 'ACTIVE', '--region', $script:Region
    )
    if ($taskDefList -and $taskDefList.taskDefinitionArns) {
        foreach ($td in @($taskDefList.taskDefinitionArns)) {
            $tdName = ($td -split '/')[1] -split ':' | Select-Object -First 1
            if ($tdName.StartsWith($script:Prefix)) {
                Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                    'ecs', 'deregister-task-definition', '--task-definition', $td, '--region', $script:Region
                ) | Out-Null
            }
        }
    }

    # 7) Batch
    $jobQueues = Invoke-TitvoAwsJson -ArgumentList @(
        'batch', 'describe-job-queues', '--region', $script:Region
    )
    if ($jobQueues -and $jobQueues.jobQueues) {
        foreach ($jq in @($jobQueues.jobQueues)) {
            if (-not $jq.jobQueueName.StartsWith($script:Prefix)) { continue }
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'batch', 'update-job-queue', '--job-queue', $jq.jobQueueName,
                '--state', 'DISABLED', '--region', $script:Region
            ) | Out-Null
            if ($Apply) { Wait-ForBatchJobQueueDisabled -JobQueue $jq.jobQueueName }
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'batch', 'delete-job-queue', '--job-queue', $jq.jobQueueName, '--region', $script:Region
            ) | Out-Null
        }
    }
    Wait-ForBatchJobQueuesDeletion -Apply:$Apply

    $computeEnvs = Invoke-TitvoAwsJson -ArgumentList @(
        'batch', 'describe-compute-environments', '--region', $script:Region
    )
    if ($computeEnvs -and $computeEnvs.computeEnvironments) {
        foreach ($ce in @($computeEnvs.computeEnvironments)) {
            if (-not $ce.computeEnvironmentName.StartsWith($script:Prefix)) { continue }
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'batch', 'update-compute-environment',
                '--compute-environment', $ce.computeEnvironmentName,
                '--state', 'DISABLED', '--region', $script:Region
            ) | Out-Null
            if ($Apply) { Wait-ForBatchComputeEnvironmentDisabled -ComputeEnvironment $ce.computeEnvironmentName }
            Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                'batch', 'delete-compute-environment',
                '--compute-environment', $ce.computeEnvironmentName,
                '--region', $script:Region
            ) | Out-Null
        }
    }

    $batchJobDefs = Invoke-TitvoAwsJson -ArgumentList @(
        'batch', 'describe-job-definitions', '--status', 'ACTIVE', '--region', $script:Region
    )
    if ($batchJobDefs -and $batchJobDefs.jobDefinitions) {
        foreach ($jd in @($batchJobDefs.jobDefinitions)) {
            if ($jd.jobDefinitionName.StartsWith($script:Prefix)) {
                Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                    'batch', 'deregister-job-definition', '--job-definition', $jd.jobDefinitionArn, '--region', $script:Region
                ) | Out-Null
            }
        }
    }

    # 8) ECR
    $repos = Invoke-TitvoAwsJson -ArgumentList @(
        'ecr', 'describe-repositories', '--region', $script:Region
    )
    if ($repos -and $repos.repositories) {
        foreach ($repo in @($repos.repositories)) {
            if ($repo.repositoryName.StartsWith($script:Prefix)) {
                Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                    'ecr', 'delete-repository', '--repository-name', $repo.repositoryName,
                    '--force', '--region', $script:Region
                ) | Out-Null
            }
        }
    }

    # 9) Secrets Manager
    $secrets = Invoke-TitvoAwsJson -ArgumentList @(
        'secretsmanager', 'list-secrets', '--region', $script:Region
    )
    if ($secrets -and $secrets.SecretList) {
        foreach ($secret in @($secrets.SecretList)) {
            if ($secret.Name.StartsWith($script:Prefix)) {
                Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                    'secretsmanager', 'delete-secret', '--secret-id', $secret.ARN,
                    '--force-delete-without-recovery', '--region', $script:Region
                ) | Out-Null
            }
        }
    }

    # 10) SSM
    Remove-TitvoSsmParametersByPath -Path "/$($script:Prefix)" -Region $script:Region -Apply:$Apply

    # 11) DynamoDB
    $tables = Invoke-TitvoAwsJson -ArgumentList @(
        'dynamodb', 'list-tables', '--region', $script:Region
    )
    if ($tables -and $tables.TableNames) {
        foreach ($t in @($tables.TableNames)) {
            if (Test-StartsWithCurrentPrefix -Value $t) {
                Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                    'dynamodb', 'delete-table', '--table-name', $t, '--region', $script:Region
                ) | Out-Null
            }
        }
    }

    # 12) S3
    $buckets = Invoke-TitvoAwsJson -ArgumentList @(
        's3api', 'list-buckets', '--region', $script:Region
    )
    if ($buckets -and $buckets.Buckets) {
        foreach ($b in @($buckets.Buckets)) {
            if (Test-StartsWithCurrentPrefix -Value $b.Name) {
                Clear-TitvoS3Bucket -Bucket $b.Name -Region $script:Region -Apply:$Apply
                Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                    's3api', 'delete-bucket', '--bucket', $b.Name, '--region', $script:Region
                ) | Out-Null
            }
        }
    }

    # 13) Cloud Map
    $namespaces = Invoke-TitvoAwsJson -ArgumentList @(
        'servicediscovery', 'list-namespaces', '--region', $script:Region
    )
    if ($namespaces -and $namespaces.Namespaces) {
        foreach ($ns in @($namespaces.Namespaces)) {
            if ($ns.Name -like "*$($script:Prefix)*") {
                Remove-TitvoCloudMapNamespace -NamespaceId $ns.Id -Apply:$Apply
            }
        }
    }

    # 14) VPC Endpoints
    Remove-TitvoVpcEndpoints -Apply:$Apply

    # 15) Network Interfaces
    Write-Host 'Limpiando Network Interfaces...'
    $enis = Invoke-TitvoAwsJson -ArgumentList @(
        'ec2', 'describe-network-interfaces', '--region', $script:Region
    )
    $prefixLower = $script:Prefix.ToLowerInvariant()
    $eniIds = @()
    if ($enis -and $enis.NetworkInterfaces) {
        foreach ($eni in @($enis.NetworkInterfaces)) {
            if ($eni.Status -ne 'available') { continue }
            if (Test-TitvoAwsTagPrefixMatch -Resource $eni -Prefix $script:Prefix -TagPropertyName 'TagSet') {
                $eniIds += $eni.NetworkInterfaceId
            }
        }
    }
    foreach ($eniId in $eniIds) {
        Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
            'ec2', 'delete-network-interface',
            '--network-interface-id', $eniId,
            '--region', $script:Region
        ) | Out-Null
    }
    if ($Apply -and $eniIds.Count -gt 0) {
        Wait-ForNetworkInterfacesDeleted -EniIds $eniIds -Apply
    }

    # 16) Subnets
    Remove-TitvoSubnets -Apply:$Apply

    # 17) Security Groups
    Remove-TitvoSecurityGroups -Apply:$Apply

    # 18) CloudWatch Logs
    $logGroups = Invoke-TitvoAwsJson -ArgumentList @(
        'logs', 'describe-log-groups', '--region', $script:Region
    )
    if ($logGroups -and $logGroups.logGroups) {
        foreach ($lg in @($logGroups.logGroups)) {
            if ($lg.logGroupName -like "*$($script:Prefix)*") {
                Invoke-TitvoRun -Apply:$Apply -ArgumentList @(
                    'logs', 'delete-log-group', '--log-group-name', $lg.logGroupName, '--region', $script:Region
                ) | Out-Null
            }
        }
    }

    # 19) IAM
    Collect-UsedIamReferences
    Write-Host 'IAM candidates (Titvo match):'
    $prefixLower = $script:Prefix.ToLowerInvariant()

    $roles = Invoke-TitvoAwsJson -ArgumentList @('iam', 'list-roles')
    if ($roles -and $roles.Roles) {
        foreach ($r in @($roles.Roles)) {
            $rn = $r.RoleName.ToLowerInvariant()
            if (-not ($rn.StartsWith($prefixLower) -or $rn.Contains('tvo'))) { continue }
            if (Test-IamRoleInUse -Role $r.RoleName) {
                Write-Host "[SKIP] IAM role en uso: $($r.RoleName)"
                continue
            }
            if ($Apply) {
                Remove-TitvoIamRole -RoleName $r.RoleName -Apply
            }
            else {
                Write-Host "[DRY-RUN] aws iam delete-role --role-name `"$($r.RoleName)`" (usar -Apply)"
            }
        }
    }

    $policies = Invoke-TitvoAwsJson -ArgumentList @('iam', 'list-policies', '--scope', 'Local')
    if ($policies -and $policies.Policies) {
        foreach ($p in @($policies.Policies)) {
            $pn = $p.PolicyName.ToLowerInvariant()
            if (-not ($pn.StartsWith($prefixLower) -or $pn.Contains('tvo'))) { continue }
            if ($Apply) {
                Remove-TitvoIamPolicy -PolicyArn $p.Arn -Apply
            }
            else {
                Write-Host "[DRY-RUN] aws iam delete-policy --policy-arn `"$($p.Arn)`" (usar -Apply)"
            }
        }
    }

    Write-Host "== Fin limpieza para prefijo '$($script:Prefix)' =="
}

# --- Main ---
if ($env:PREFIX) {
    Write-Host "Iniciando limpieza con prefijo '$($env:PREFIX)' (especificado por variable de entorno)..."
    Write-Host ''
    Invoke-TitvoCleanupForPrefix -CurrentPrefix $env:PREFIX -Apply:$Apply
}
else {
    Write-Host 'Iniciando limpieza con prefijos por defecto...'
    Write-Host ''
    foreach ($prefix in $script:DefaultPrefixes) {
        Invoke-TitvoCleanupForPrefix -CurrentPrefix $prefix -Apply:$Apply
        Write-Host ''
    }
}

Write-Host '== Limpieza completada =='
Write-Host 'Tip: primero ejecuta sin -Apply; luego con -Apply'
