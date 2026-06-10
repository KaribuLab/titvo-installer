#Requires -Version 5.1
<#
.SYNOPSIS
    Funciones de limpieza AWS (port de titvo_cleanup_orphans.sh).
    Dot-source despues de Titvo-Common.ps1.
#>

function Remove-TitvoSsmParametersByPathLocal {
    Remove-TitvoSsmParametersByPath -Path "/$script:TitvoPrefix" -Region $script:TitvoRegion -Apply:$script:TitvoApply
}

function Wait-TitvoCloudMapOperation {
    param([string]$OperationId)
    $ok = Start-TitvoWaitLoop -MaxAttempts 24 -SleepSeconds 5 `
        -WaitMessage "Cloud Map operation $OperationId" `
        -WarnMessage "Cloud Map operation sigue pendiente: $OperationId" `
        -Condition {
            $op = Invoke-TitvoAwsJsonLocal -AwsArgs @(
                'servicediscovery', 'get-operation', '--operation-id', $OperationId,
                '--region', $script:TitvoRegion, '--output', 'json'
            )
            if (-not $op -or -not $op.Operation) { return $false }
            $status = $op.Operation.Status
            if ($status -eq 'SUCCESS') { return $true }
            if ($status -eq 'FAIL') {
                $err = $op.Operation.ErrorMessage
                if (-not $err) { $err = 'Cloud Map operation failed' }
                Write-Host "[WARN] Cloud Map operation failed: $err"
                return $true
            }
            return $false
        }
    return $ok
}

function Remove-TitvoCloudMapNamespace {
    param([string]$NamespaceId)
    $services = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'servicediscovery', 'list-services',
        '--filters', "Name=NAMESPACE_ID,Values=$NamespaceId,Condition=EQ",
        '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($svc in @($services.Services)) {
        if (-not $svc) { continue }
        $instances = Invoke-TitvoAwsJsonLocal -AwsArgs @(
            'servicediscovery', 'list-instances', '--service-id', $svc.Id,
            '--region', $script:TitvoRegion, '--output', 'json'
        ) -IgnoreError
        foreach ($inst in @($instances.Instances)) {
            if (-not $inst) { continue }
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'servicediscovery', 'deregister-instance', '--service-id', $svc.Id,
                '--instance-id', $inst.Id, '--region', $script:TitvoRegion
            )
        }
        Invoke-TitvoRunAwsLocal -AwsArgs @(
            'servicediscovery', 'delete-service', '--id', $svc.Id, '--region', $script:TitvoRegion
        )
    }

    if ($script:TitvoApply) {
        $del = Invoke-TitvoAwsJsonLocal -AwsArgs @(
            'servicediscovery', 'delete-namespace', '--id', $NamespaceId,
            '--region', $script:TitvoRegion, '--output', 'json'
        )
        $opId = $null
        if ($del) { $opId = $del.OperationId }
        if ($opId) {
            Write-Host "[WAIT] Cloud Map namespace delete operation: $opId"
            Wait-TitvoCloudMapOperation -OperationId $opId | Out-Null
        }
    }
    else {
        Write-Host "[DRY-RUN] aws servicediscovery delete-namespace --id `"$NamespaceId`" --region `"$($script:TitvoRegion)`""
    }
}

function Get-TitvoCandidateSubnetIds {
    $subnets = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'ec2', 'describe-subnets', '--region', $script:TitvoRegion, '--output', 'json'
    )
    $ids = @()
    foreach ($s in @($subnets.Subnets)) {
        if (-not $s) { continue }
        if (Test-TitvoAwsTagPrefixMatch -Resource $s -Prefix $script:TitvoPrefix) {
            $ids += $s.SubnetId
        }
    }
    return $ids
}

function Wait-TitvoSubnetsDeletion {
    param([string[]]$SubnetIds)
    $remaining = @($SubnetIds)
    $ok = Start-TitvoWaitLoop -MaxAttempts 24 -SleepSeconds 5 `
        -WaitMessage "Subnets aun presentes: $($remaining.Count)" `
        -Condition {
            $still = @()
            foreach ($sid in $remaining) {
                if (-not $sid) { continue }
                try {
                    $null = Invoke-TitvoAws -AwsArgs @(
                        'ec2', 'describe-subnets', '--subnet-ids', $sid,
                        '--region', $script:TitvoRegion, '--output', 'json'
                    )
                    $still += $sid
                }
                catch { }
            }
            $script:subnetWaitRemaining = $still
            return ($still.Count -eq 0)
        }
    $remaining = $script:subnetWaitRemaining
    if (-not $ok) {
        foreach ($sid in $remaining) {
            if (-not $sid) { continue }
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'ec2', 'delete-subnet', '--subnet-id', $sid, '--region', $script:TitvoRegion
            )
        }
    }
}

function Remove-TitvoSubnets {
    $subnetIds = Get-TitvoCandidateSubnetIds
    foreach ($sid in $subnetIds) {
        if (-not $sid) { continue }
        Invoke-TitvoRunAwsLocal -AwsArgs @(
            'ec2', 'delete-subnet', '--subnet-id', $sid, '--region', $script:TitvoRegion
        )
    }
    if ($script:TitvoApply -and $subnetIds.Count -gt 0) {
        Wait-TitvoSubnetsDeletion -SubnetIds $subnetIds
    }
}

function Remove-TitvoRouteTables {
    $rtbs = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'ec2', 'describe-route-tables', '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($rtb in @($rtbs.RouteTables)) {
        if (-not $rtb) { continue }
        if (-not (Test-TitvoAwsTagPrefixMatch -Resource $rtb -Prefix $script:TitvoPrefix)) { continue }
        $hasNonMain = $false
        foreach ($a in @($rtb.Associations)) {
            if ($a -and -not $a.Main) { $hasNonMain = $true; break }
        }
        if (-not $hasNonMain) { continue }

        $detail = Invoke-TitvoAwsJsonLocal -AwsArgs @(
            'ec2', 'describe-route-tables', '--route-table-ids', $rtb.RouteTableId,
            '--region', $script:TitvoRegion, '--output', 'json'
        )
        foreach ($assoc in @($detail.RouteTables[0].Associations)) {
            if (-not $assoc -or $assoc.Main) { continue }
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'ec2', 'disassociate-route-table', '--association-id', $assoc.RouteTableAssociationId,
                '--region', $script:TitvoRegion
            )
        }
        Invoke-TitvoRunAwsLocal -AwsArgs @(
            'ec2', 'delete-route-table', '--route-table-id', $rtb.RouteTableId, '--region', $script:TitvoRegion
        )
    }
}

function Get-TitvoCandidateSecurityGroupIds {
    $sgs = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'ec2', 'describe-security-groups', '--region', $script:TitvoRegion, '--output', 'json'
    )
    $ids = @()
    $p = $script:TitvoPrefix.ToLowerInvariant()
    foreach ($sg in @($sgs.SecurityGroups)) {
        if (-not $sg) { continue }
        if ($sg.GroupName -eq 'default') { continue }
        if ($sg.Description -eq 'default VPC security group') { continue }
        $name = $sg.GroupName.ToLowerInvariant()
        $desc = if ($sg.Description) { $sg.Description.ToLowerInvariant() } else { '' }
        $tagMatch = Test-TitvoAwsTagPrefixMatch -Resource $sg -Prefix $script:TitvoPrefix
        if ($name.StartsWith($p) -or $name.Contains($p) -or $desc.Contains($p) -or $tagMatch) {
            $ids += $sg.GroupId
        }
    }
    return $ids
}

function Wait-TitvoVpcEndpointsDeletion {
    param([string[]]$VpcEndpointIds)
    if ($VpcEndpointIds.Count -eq 0) { return }
    Start-TitvoWaitLoop -MaxAttempts 60 -SleepSeconds 5 `
        -WaitMessage 'VPC endpoints aun presentes' `
        -WarnMessage 'Algunos VPC endpoints siguen presentes; intento continuar con security groups' `
        -Condition {
            try {
                $vpceArgs = @('ec2', 'describe-vpc-endpoints', '--vpc-endpoint-ids') + $VpcEndpointIds + @(
                    '--region', $script:TitvoRegion, '--output', 'json'
                )
                $r = Invoke-TitvoAwsJsonLocal -AwsArgs $vpceArgs
                if (-not $r -or -not $r.VpcEndpoints) { return $true }
                return (@($r.VpcEndpoints).Count -eq 0)
            }
            catch { return $true }
        } | Out-Null
}

function Remove-TitvoVpcEndpoints {
    $sgIds = Get-TitvoCandidateSecurityGroupIds
    $vpces = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'ec2', 'describe-vpc-endpoints', '--region', $script:TitvoRegion, '--output', 'json'
    )
    $vpceIds = @()
    foreach ($vpce in @($vpces.VpcEndpoints)) {
        if (-not $vpce) { continue }
        $match = $false
        foreach ($g in @($vpce.Groups)) {
            if ($g -and ($sgIds -contains $g.GroupId)) { $match = $true; break }
        }
        if (-not $match) {
            $match = Test-TitvoAwsTagPrefixMatch -Resource $vpce -Prefix $script:TitvoPrefix
        }
        if ($match) { $vpceIds += $vpce.VpcEndpointId }
    }
    foreach ($id in $vpceIds) {
        Invoke-TitvoRunAwsLocal -AwsArgs @(
            'ec2', 'delete-vpc-endpoints', '--vpc-endpoint-ids', $id, '--region', $script:TitvoRegion
        )
    }
    if ($script:TitvoApply -and $vpceIds.Count -gt 0) {
        Wait-TitvoVpcEndpointsDeletion -VpcEndpointIds $vpceIds
    }
}

function Wait-TitvoNetworkInterfacesDeleted {
    param([string[]]$EniIds)
    $remaining = @($EniIds)
    Start-TitvoWaitLoop -MaxAttempts 36 -SleepSeconds 5 `
        -WaitMessage "Network Interfaces aun presentes: $($remaining.Count)" `
        -WarnMessage 'Algunas Network Interfaces siguen presentes; intento continuar' `
        -Condition {
            $still = @()
            foreach ($eni in $remaining) {
                if (-not $eni) { continue }
                try {
                    $r = Invoke-TitvoAwsJsonLocal -AwsArgs @(
                        'ec2', 'describe-network-interfaces', '--network-interface-ids', $eni,
                        '--region', $script:TitvoRegion, '--output', 'json'
                    )
                    if ($r.NetworkInterfaces -and $r.NetworkInterfaces.Count -gt 0) {
                        $still += $eni
                    }
                }
                catch { }
            }
            $script:eniWaitRemaining = $still
            return ($still.Count -eq 0)
        } | Out-Null
}

function Remove-TitvoEnisForSecurityGroup {
    param([string]$GroupId)
    $enis = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'ec2', 'describe-network-interfaces',
        '--filters', "Name=group-id,Values=$GroupId",
        '--region', $script:TitvoRegion, '--output', 'json'
    ) -IgnoreError
    $eniIds = @()
    foreach ($eni in @($enis.NetworkInterfaces)) {
        if (-not $eni) { continue }
        $eniIds += $eni.NetworkInterfaceId
        $status = $eni.Status
        $attachId = $null
        if ($eni.Attachment) { $attachId = $eni.Attachment.AttachmentId }
        if ($attachId) {
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'ec2', 'detach-network-interface', '--attachment-id', $attachId,
                '--force', '--region', $script:TitvoRegion
            )
        }
        if ($status -eq 'available' -or $attachId) {
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'ec2', 'delete-network-interface', '--network-interface-id', $eni.NetworkInterfaceId,
                '--region', $script:TitvoRegion
            )
        }
        else {
            Write-Host "[INFO] ENI $($eni.NetworkInterfaceId) status=$status; se intentara borrar igual"
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'ec2', 'delete-network-interface', '--network-interface-id', $eni.NetworkInterfaceId,
                '--region', $script:TitvoRegion
            )
        }
    }
    if ($script:TitvoApply -and $eniIds.Count -gt 0) {
        Wait-TitvoNetworkInterfacesDeleted -EniIds $eniIds
    }
}

function Get-TitvoTargetedSgPermissions {
    param(
        [string]$TargetSg,
        [ValidateSet('ingress', 'egress')]
        [string]$Direction
    )
    $permsField = if ($Direction -eq 'egress') { 'IpPermissionsEgress' } else { 'IpPermissions' }
    $filterName = if ($Direction -eq 'egress') { 'egress.ip-permission.group-id' } else { 'ip-permission.group-id' }

    $data = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'ec2', 'describe-security-groups',
        '--filters', "Name=$filterName,Values=$TargetSg",
        '--region', $script:TitvoRegion, '--output', 'json'
    ) -IgnoreError
    if (-not $data) { return @() }

    $results = @()
    foreach ($sg in @($data.SecurityGroups)) {
        if (-not $sg) { continue }
        $perms = $sg.$permsField
        if (-not $perms) { continue }
        foreach ($perm in @($perms)) {
            if (-not $perm) { continue }
            $pairs = @($perm.UserIdGroupPairs | Where-Object { $_.GroupId -eq $TargetSg })
            if ($pairs.Count -eq 0) { continue }
            $minimal = @{
                IpProtocol = $perm.IpProtocol
                UserIdGroupPairs = @($pairs | ForEach-Object { @{ GroupId = $_.GroupId } })
                IpRanges = @()
                Ipv6Ranges = @()
                PrefixListIds = @()
            }
            if ($null -ne $perm.FromPort) { $minimal.FromPort = $perm.FromPort }
            if ($null -ne $perm.ToPort) { $minimal.ToPort = $perm.ToPort }
            $json = $minimal | ConvertTo-Json -Compress -Depth 10
            $results += [pscustomobject]@{ GroupId = $sg.GroupId; PermissionJson = $json }
        }
    }
    return $results
}

function Revoke-TitvoCrossSgReferences {
    param([string]$TargetSg)
    foreach ($row in (Get-TitvoTargetedSgPermissions -TargetSg $TargetSg -Direction 'ingress')) {
        Invoke-TitvoRunLocal -Description "revoke ingress $TargetSg on $($row.GroupId)" -ApplyAction {
            $null = Invoke-TitvoAws -AwsArgs @(
                'ec2', 'revoke-security-group-ingress', '--group-id', $row.GroupId,
                '--ip-permissions', $row.PermissionJson, '--region', $script:TitvoRegion
            ) -IgnoreError
        }
    }
    foreach ($row in (Get-TitvoTargetedSgPermissions -TargetSg $TargetSg -Direction 'egress')) {
        Invoke-TitvoRunLocal -Description "revoke egress $TargetSg on $($row.GroupId)" -ApplyAction {
            $null = Invoke-TitvoAws -AwsArgs @(
                'ec2', 'revoke-security-group-egress', '--group-id', $row.GroupId,
                '--ip-permissions', $row.PermissionJson, '--region', $script:TitvoRegion
            ) -IgnoreError
        }
    }
}

function Show-TitvoSgDependencies {
    param([string]$GroupId)
    Write-Host "[DIAG] Dependencias residuales de ${GroupId}:"
    $enis = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'ec2', 'describe-network-interfaces',
        '--filters', "Name=group-id,Values=$GroupId",
        '--region', $script:TitvoRegion, '--output', 'json'
    ) -IgnoreError
    $found = $false
    foreach ($eni in @($enis.NetworkInterfaces)) {
        if (-not $eni) { continue }
        $found = $true
        $attach = 'none'
        if ($eni.Attachment) {
            if ($eni.Attachment.InstanceId) { $attach = $eni.Attachment.InstanceId }
            elseif ($eni.Attachment.AttachmentId) { $attach = $eni.Attachment.AttachmentId }
        }
        $desc = if ($eni.Description) { $eni.Description } else { '' }
        Write-Host "  ENI $($eni.NetworkInterfaceId) status=$($eni.Status) desc=`"$desc`" attach=$attach"
    }
    if (-not $found) { Write-Host '  (sin ENIs)' }

    foreach ($filter in @('ip-permission.group-id', 'egress.ip-permission.group-id')) {
        $label = if ($filter -like 'egress*') { 'egress' } else { 'ingress' }
        $refs = Invoke-TitvoAwsJsonLocal -AwsArgs @(
            'ec2', 'describe-security-groups', '--filters', "Name=$filter,Values=$GroupId",
            '--region', $script:TitvoRegion, '--output', 'json'
        ) -IgnoreError
        foreach ($sg in @($refs.SecurityGroups)) {
            if ($sg) {
                Write-Host "  SG ref ${label}: $($sg.GroupId) ($($sg.GroupName))"
            }
        }
    }
}

function Remove-TitvoSecurityGroupWithRetry {
    param([string]$GroupId)
    for ($attempt = 1; $attempt -le 6; $attempt++) {
        if (-not $script:TitvoApply) {
            Write-Host "[DRY-RUN] aws ec2 delete-security-group --group-id `"$GroupId`" --region `"$($script:TitvoRegion)`""
            return
        }
        try {
            $null = Invoke-TitvoAws -AwsArgs @(
                'ec2', 'delete-security-group', '--group-id', $GroupId, '--region', $script:TitvoRegion
            )
            Write-Host "[OK] SG eliminado: $GroupId"
            return
        }
        catch {
            $err = $_.Exception.Message
            if ($err -match 'InvalidGroup\.NotFound') {
                Write-Host "[OK] SG $GroupId ya no existe"
                return
            }
            Write-Host "[WARN] No se pudo borrar $GroupId (intento $attempt/6): $err"
            if ($err -match 'DependencyViolation') {
                Remove-TitvoEnisForSecurityGroup -GroupId $GroupId
                Revoke-TitvoCrossSgReferences -TargetSg $GroupId
            }
            Start-Sleep -Seconds 10
        }
    }
    Write-Host "[ERROR] No se logro borrar SG $GroupId tras 6 intentos"
    Show-TitvoSgDependencies -GroupId $GroupId
}

function Remove-TitvoSecurityGroups {
    $groupIds = Get-TitvoCandidateSecurityGroupIds
    foreach ($gid in $groupIds) {
        Revoke-TitvoCrossSgReferences -TargetSg $gid
        Remove-TitvoEnisForSecurityGroup -GroupId $gid
    }
    foreach ($gid in $groupIds) {
        Remove-TitvoSecurityGroupWithRetry -GroupId $gid
    }
}

function Remove-TitvoEventBridgeRulesForBus {
    param([string]$BusName)
    if ($BusName -eq 'default') {
        $rulesJson = Invoke-TitvoAwsJsonLocal -AwsArgs @(
            'events', 'list-rules', '--name-prefix', $script:TitvoPrefix,
            '--region', $script:TitvoRegion, '--output', 'json'
        )
    }
    else {
        $rulesJson = Invoke-TitvoAwsJsonLocal -AwsArgs @(
            'events', 'list-rules', '--event-bus-name', $BusName,
            '--region', $script:TitvoRegion, '--output', 'json'
        )
    }
    foreach ($rule in @($rulesJson.Rules)) {
        if (-not $rule) { continue }
        $ruleName = $rule.Name
        if ($BusName -eq 'default') {
            $targets = Invoke-TitvoAwsJsonLocal -AwsArgs @(
                'events', 'list-targets-by-rule', '--rule', $ruleName,
                '--region', $script:TitvoRegion, '--output', 'json'
            )
        }
        else {
            $targets = Invoke-TitvoAwsJsonLocal -AwsArgs @(
                'events', 'list-targets-by-rule', '--rule', $ruleName,
                '--event-bus-name', $BusName, '--region', $script:TitvoRegion, '--output', 'json'
            )
        }
        $ids = @($targets.Targets | ForEach-Object { $_.Id })
        if ($ids.Count -gt 0) {
            $idsJson = ($ids | ConvertTo-Json -Compress)
            if ($BusName -eq 'default') {
                Invoke-TitvoRunAwsLocal -AwsArgs @(
                    'events', 'remove-targets', '--rule', $ruleName,
                    '--ids', $idsJson, '--region', $script:TitvoRegion
                )
            }
            else {
                Invoke-TitvoRunAwsLocal -AwsArgs @(
                    'events', 'remove-targets', '--rule', $ruleName,
                    '--event-bus-name', $BusName, '--ids', $idsJson, '--region', $script:TitvoRegion
                )
            }
        }
        if ($BusName -eq 'default') {
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'events', 'delete-rule', '--name', $ruleName, '--region', $script:TitvoRegion
            )
        }
        else {
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'events', 'delete-rule', '--name', $ruleName,
                '--event-bus-name', $BusName, '--region', $script:TitvoRegion
            )
        }
    }
}

function Wait-TitvoBatchJobQueuesDeletion {
    Start-TitvoWaitLoop -MaxAttempts 24 -SleepSeconds 5 `
        -WaitMessage 'Batch job queues aun presentes' `
        -WarnMessage 'Batch job queues siguen presentes; intento continuar con compute environments' `
        -Condition {
            $q = Invoke-TitvoAwsJsonLocal -AwsArgs @(
                'batch', 'describe-job-queues', '--region', $script:TitvoRegion, '--output', 'json'
            )
            $count = 0
            foreach ($jq in @($q.jobQueues)) {
                if ($jq -and (Test-StartsWithTitvoPrefix -Value $jq.jobQueueName -Prefix $script:TitvoPrefix)) {
                    $count++
                }
            }
            return ($count -eq 0)
        } | Out-Null
}

function Wait-TitvoBatchJobQueueDisabled {
    param([string]$JobQueue)
    Start-TitvoWaitLoop -MaxAttempts 36 -SleepSeconds 5 `
        -WaitMessage "Job queue $JobQueue" `
        -WarnMessage "Job queue $JobQueue sigue modificandose; intento borrarlo igual" `
        -Condition {
            $q = Invoke-TitvoAwsJsonLocal -AwsArgs @(
                'batch', 'describe-job-queues', '--job-queues', $JobQueue,
                '--region', $script:TitvoRegion, '--output', 'json'
            ) -IgnoreError
            if (-not $q -or -not $q.jobQueues -or $q.jobQueues.Count -eq 0) { return $true }
            $jq = $q.jobQueues[0]
            return ($jq.status -eq 'VALID' -and $jq.state -eq 'DISABLED')
        } | Out-Null
}

function Wait-TitvoBatchComputeEnvironmentDisabled {
    param([string]$ComputeEnvironment)
    Start-TitvoWaitLoop -MaxAttempts 36 -SleepSeconds 5 `
        -WaitMessage "Compute environment $ComputeEnvironment" `
        -WarnMessage "Compute environment $ComputeEnvironment sigue modificandose; intento borrarlo igual" `
        -Condition {
            $c = Invoke-TitvoAwsJsonLocal -AwsArgs @(
                'batch', 'describe-compute-environments', '--compute-environments', $ComputeEnvironment,
                '--region', $script:TitvoRegion, '--output', 'json'
            ) -IgnoreError
            if (-not $c -or -not $c.computeEnvironments -or $c.computeEnvironments.Count -eq 0) { return $true }
            $ce = $c.computeEnvironments[0]
            return ($ce.status -eq 'VALID' -and $ce.state -eq 'DISABLED')
        } | Out-Null
}

function Remove-TitvoIamPolicy {
    param([string]$PolicyArn)
    $entities = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'iam', 'list-entities-for-policy', '--policy-arn', $PolicyArn, '--output', 'json'
    )
    foreach ($r in @($entities.PolicyRoles)) {
        if (-not $r) { continue }
        Invoke-TitvoRunAwsLocal -AwsArgs @(
            'iam', 'detach-role-policy', '--role-name', $r.RoleName, '--policy-arn', $PolicyArn
        )
    }
    foreach ($u in @($entities.PolicyUsers)) {
        if (-not $u) { continue }
        Invoke-TitvoRunAwsLocal -AwsArgs @(
            'iam', 'detach-user-policy', '--user-name', $u.UserName, '--policy-arn', $PolicyArn
        )
    }
    foreach ($g in @($entities.PolicyGroups)) {
        if (-not $g) { continue }
        Invoke-TitvoRunAwsLocal -AwsArgs @(
            'iam', 'detach-group-policy', '--group-name', $g.GroupName, '--policy-arn', $PolicyArn
        )
    }
    $pol = Invoke-TitvoAwsJsonLocal -AwsArgs @('iam', 'get-policy', '--policy-arn', $PolicyArn, '--output', 'json')
    $dv = $pol.Policy.DefaultVersionId
    $vers = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'iam', 'list-policy-versions', '--policy-arn', $PolicyArn, '--output', 'json'
    )
    foreach ($v in @($vers.Versions)) {
        if (-not $v -or $v.VersionId -eq $dv) { continue }
        Invoke-TitvoRunAwsLocal -AwsArgs @(
            'iam', 'delete-policy-version', '--policy-arn', $PolicyArn, '--version-id', $v.VersionId
        )
    }
    Invoke-TitvoRunAwsLocal -AwsArgs @('iam', 'delete-policy', '--policy-arn', $PolicyArn)
}

function Remove-TitvoIamRole {
    param([string]$RoleName)
    $attached = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'iam', 'list-attached-role-policies', '--role-name', $RoleName, '--output', 'json'
    )
    foreach ($p in @($attached.AttachedPolicies)) {
        if (-not $p) { continue }
        Invoke-TitvoRunAwsLocal -AwsArgs @(
            'iam', 'detach-role-policy', '--role-name', $RoleName, '--policy-arn', $p.PolicyArn
        )
    }
    $inline = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'iam', 'list-role-policies', '--role-name', $RoleName, '--output', 'json'
    )
    foreach ($pn in @($inline.PolicyNames)) {
        if (-not $pn) { continue }
        Invoke-TitvoRunAwsLocal -AwsArgs @(
            'iam', 'delete-role-policy', '--role-name', $RoleName, '--policy-name', $pn
        )
    }
    $profiles = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'iam', 'list-instance-profiles-for-role', '--role-name', $RoleName, '--output', 'json'
    )
    foreach ($prof in @($profiles.InstanceProfiles)) {
        if (-not $prof) { continue }
        Invoke-TitvoRunAwsLocal -AwsArgs @(
            'iam', 'remove-role-from-instance-profile',
            '--instance-profile-name', $prof.InstanceProfileName, '--role-name', $RoleName
        )
        Invoke-TitvoRunAwsLocal -AwsArgs @(
            'iam', 'delete-instance-profile', '--instance-profile-name', $prof.InstanceProfileName
        )
    }
    Invoke-TitvoRunAwsLocal -AwsArgs @('iam', 'delete-role', '--role-name', $RoleName)
}

function Wait-TitvoLambdaEventSourceMappingsDeleted {
    Start-TitvoWaitLoop -MaxAttempts 24 -SleepSeconds 5 `
        -WaitMessage 'Event Source Mappings aun presentes' `
        -WarnMessage 'Algunos Event Source Mappings siguen presentes' `
        -Condition {
            $m = Invoke-TitvoAwsJsonLocal -AwsArgs @(
                'lambda', 'list-event-source-mappings', '--max-items', '100',
                '--region', $script:TitvoRegion, '--output', 'json'
            )
            $count = 0
            foreach ($esm in @($m.EventSourceMappings)) {
                if ($esm -and $esm.FunctionArn -and $esm.FunctionArn.Contains($script:TitvoPrefix)) {
                    $count++
                }
            }
            return ($count -eq 0)
        } | Out-Null
}

function Wait-TitvoEcsClusterEmpty {
    param([string]$Cluster)
    Start-TitvoWaitLoop -MaxAttempts 36 -SleepSeconds 5 `
        -WaitMessage "Cluster $Cluster aun tiene tareas" `
        -WarnMessage "Cluster $Cluster sigue con tareas activas" `
        -Condition {
            $t = Invoke-TitvoAwsJsonLocal -AwsArgs @(
                'ecs', 'list-tasks', '--cluster', $Cluster,
                '--region', $script:TitvoRegion, '--output', 'json'
            )
            $n = 0
            if ($t.taskArns) { $n = @($t.taskArns).Count }
            return ($n -eq 0)
        } | Out-Null
}

function Collect-TitvoUsedIamRoles {
    Write-Host 'Recolectando referencias en uso (IAM)...'
    $lambdas = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'lambda', 'list-functions', '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($fn in @($lambdas.Functions)) {
        if ($fn.Role) { Mark-TitvoUsedIamRole -Role (Get-TitvoArnTail -Arn $fn.Role) }
    }

    $tdList = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'ecs', 'list-task-definitions', '--status', 'ACTIVE',
        '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($tdArn in @($tdList.taskDefinitionArns)) {
        if (-not $tdArn) { continue }
        $data = Invoke-TitvoAwsJsonLocal -AwsArgs @(
            'ecs', 'describe-task-definition', '--task-definition', $tdArn,
            '--region', $script:TitvoRegion, '--output', 'json'
        )
        if ($data.taskDefinition.taskRoleArn) {
            Mark-TitvoUsedIamRole -Role (Get-TitvoArnTail -Arn $data.taskDefinition.taskRoleArn)
        }
        if ($data.taskDefinition.executionRoleArn) {
            Mark-TitvoUsedIamRole -Role (Get-TitvoArnTail -Arn $data.taskDefinition.executionRoleArn)
        }
    }

    $batch = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'batch', 'describe-job-definitions', '--status', 'ACTIVE',
        '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($jd in @($batch.jobDefinitions)) {
        if (-not $jd) { continue }
        $cp = $jd.containerProperties
        if (-not $cp) { continue }
        if ($cp.jobRoleArn) { Mark-TitvoUsedIamRole -Role (Get-TitvoArnTail -Arn $cp.jobRoleArn) }
        if ($cp.executionRoleArn) { Mark-TitvoUsedIamRole -Role (Get-TitvoArnTail -Arn $cp.executionRoleArn) }
    }
}

function Invoke-TitvoCleanupForPrefix {
    param([string]$Prefix)

    Set-TitvoCleanupContext -Region $script:TitvoRegion -Prefix $Prefix -Apply:$script:TitvoApply
    Write-Host "== Inicio limpieza candidatos prefix '$Prefix' (region $($script:TitvoRegion)) =="

    # 1) EventBridge
    Remove-TitvoEventBridgeRulesForBus -BusName 'default'
    $buses = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'events', 'list-event-buses', '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($bus in @($buses.EventBuses)) {
        if (-not $bus -or $bus.Name -eq 'default') { continue }
        if (-not (Test-StartsWithTitvoPrefix -Value $bus.Name -Prefix $Prefix)) { continue }
        Remove-TitvoEventBridgeRulesForBus -BusName $bus.Name
        Invoke-TitvoRunAwsLocal -AwsArgs @(
            'events', 'delete-event-bus', '--name', $bus.Name, '--region', $script:TitvoRegion
        )
    }

    # 2) API Gateway v2
    $apis = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'apigatewayv2', 'get-apis', '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($api in @($apis.Items)) {
        if ($api -and (Test-StartsWithTitvoPrefix -Value $api.Name -Prefix $Prefix)) {
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'apigatewayv2', 'delete-api', '--api-id', $api.ApiId, '--region', $script:TitvoRegion
            )
        }
    }

    # 3) Lambda ESM
    Write-Host 'Limpiando Lambda Event Source Mappings...'
    $esms = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'lambda', 'list-event-source-mappings', '--max-items', '100',
        '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($esm in @($esms.EventSourceMappings)) {
        if ($esm -and $esm.FunctionArn -and $esm.FunctionArn.Contains($Prefix)) {
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'lambda', 'delete-event-source-mapping', '--uuid', $esm.UUID, '--region', $script:TitvoRegion
            )
        }
    }
    if ($script:TitvoApply) { Wait-TitvoLambdaEventSourceMappingsDeleted }

    # 4) Lambda
    $lambdasDel = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'lambda', 'list-functions', '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($fn in @($lambdasDel.Functions)) {
        if ($fn -and (Test-StartsWithTitvoPrefix -Value $fn.FunctionName -Prefix $Prefix)) {
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'lambda', 'delete-function', '--function-name', $fn.FunctionName, '--region', $script:TitvoRegion
            )
        }
    }

    # 5) SQS
    $queues = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'sqs', 'list-queues', '--queue-name-prefix', $Prefix,
        '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($qurl in @($queues.QueueUrls)) {
        if ($qurl) {
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'sqs', 'delete-queue', '--queue-url', $qurl, '--region', $script:TitvoRegion
            )
        }
    }

    # 6) ECS
    $clusters = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'ecs', 'list-clusters', '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($cArn in @($clusters.clusterArns)) {
        $cname = Get-TitvoArnTail -Arn $cArn
        if (-not (Test-StartsWithTitvoPrefix -Value $cname -Prefix $Prefix)) { continue }
        $svcs = Invoke-TitvoAwsJsonLocal -AwsArgs @(
            'ecs', 'list-services', '--cluster', $cArn, '--region', $script:TitvoRegion, '--output', 'json'
        )
        foreach ($sArn in @($svcs.serviceArns)) {
            $sname = Get-TitvoArnTail -Arn $sArn
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'ecs', 'update-service', '--cluster', $cArn, '--service', $sname,
                '--desired-count', '0', '--region', $script:TitvoRegion
            )
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'ecs', 'delete-service', '--cluster', $cArn, '--service', $sname,
                '--force', '--region', $script:TitvoRegion
            )
        }
        if ($script:TitvoApply) { Wait-TitvoEcsClusterEmpty -Cluster $cArn }
        Invoke-TitvoRunAwsLocal -AwsArgs @(
            'ecs', 'delete-cluster', '--cluster', $cArn, '--region', $script:TitvoRegion
        )
    }
    $tds = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'ecs', 'list-task-definitions', '--status', 'ACTIVE',
        '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($td in @($tds.taskDefinitionArns)) {
        if (-not $td) { continue }
        $family = (($td -split '/')[1] -split ':')[0]
        if (Test-StartsWithTitvoPrefix -Value $family -Prefix $Prefix) {
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'ecs', 'deregister-task-definition', '--task-definition', $td, '--region', $script:TitvoRegion
            )
        }
    }

    # 7) Batch
    $jqAll = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'batch', 'describe-job-queues', '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($jqn in @($jqAll.jobQueues)) {
        if ($jqn -and (Test-StartsWithTitvoPrefix -Value $jqn.jobQueueName -Prefix $Prefix)) {
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'batch', 'update-job-queue', '--job-queue', $jqn.jobQueueName,
                '--state', 'DISABLED', '--region', $script:TitvoRegion
            )
            if ($script:TitvoApply) { Wait-TitvoBatchJobQueueDisabled -JobQueue $jqn.jobQueueName }
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'batch', 'delete-job-queue', '--job-queue', $jqn.jobQueueName, '--region', $script:TitvoRegion
            )
        }
    }
    if ($script:TitvoApply) { Wait-TitvoBatchJobQueuesDeletion }

    $ceAll = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'batch', 'describe-compute-environments', '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($cen in @($ceAll.computeEnvironments)) {
        if ($cen -and (Test-StartsWithTitvoPrefix -Value $cen.computeEnvironmentName -Prefix $Prefix)) {
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'batch', 'update-compute-environment', '--compute-environment', $cen.computeEnvironmentName,
                '--state', 'DISABLED', '--region', $script:TitvoRegion
            )
            if ($script:TitvoApply) { Wait-TitvoBatchComputeEnvironmentDisabled -ComputeEnvironment $cen.computeEnvironmentName }
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'batch', 'delete-compute-environment', '--compute-environment', $cen.computeEnvironmentName,
                '--region', $script:TitvoRegion
            )
        }
    }

    $jdAll = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'batch', 'describe-job-definitions', '--status', 'ACTIVE',
        '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($jd in @($jdAll.jobDefinitions)) {
        if ($jd -and (Test-StartsWithTitvoPrefix -Value $jd.jobDefinitionName -Prefix $Prefix)) {
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'batch', 'deregister-job-definition', '--job-definition', $jd.jobDefinitionArn,
                '--region', $script:TitvoRegion
            )
        }
    }

    # 8) ECR
    $repos = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'ecr', 'describe-repositories', '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($repo in @($repos.repositories)) {
        if ($repo -and (Test-StartsWithTitvoPrefix -Value $repo.repositoryName -Prefix $Prefix)) {
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'ecr', 'delete-repository', '--repository-name', $repo.repositoryName,
                '--force', '--region', $script:TitvoRegion
            )
        }
    }

    # 9) Secrets Manager
    $secrets = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'secretsmanager', 'list-secrets', '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($sec in @($secrets.SecretList)) {
        if ($sec -and (Test-StartsWithTitvoPrefix -Value $sec.Name -Prefix $Prefix)) {
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'secretsmanager', 'delete-secret', '--secret-id', $sec.ARN,
                '--force-delete-without-recovery', '--region', $script:TitvoRegion
            )
        }
    }

    # 10) SSM
    Remove-TitvoSsmParametersByPathLocal

    # 11) DynamoDB
    $tables = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'dynamodb', 'list-tables', '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($t in @($tables.TableNames)) {
        if ($t -and (Test-StartsWithTitvoPrefix -Value $t -Prefix $Prefix)) {
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'dynamodb', 'delete-table', '--table-name', $t, '--region', $script:TitvoRegion
            )
        }
    }

    # 12) S3
    $buckets = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        's3api', 'list-buckets', '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($b in @($buckets.Buckets)) {
        if (-not $b) { continue }
        if (Test-StartsWithTitvoPrefix -Value $b.Name -Prefix $Prefix) {
            Clear-TitvoS3Bucket -Bucket $b.Name -Region $script:TitvoRegion -Apply:$script:TitvoApply
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                's3api', 'delete-bucket', '--bucket', $b.Name, '--region', $script:TitvoRegion
            )
        }
    }

    # 13) Cloud Map
    $namespaces = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'servicediscovery', 'list-namespaces', '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($ns in @($namespaces.Namespaces)) {
        if ($ns -and $ns.Name -and $ns.Name.ToLowerInvariant().Contains($Prefix.ToLowerInvariant())) {
            Remove-TitvoCloudMapNamespace -NamespaceId $ns.Id
        }
    }

    # 14) Route tables
    Remove-TitvoRouteTables

    # 15) VPC endpoints
    Remove-TitvoVpcEndpoints

    # 16) ENIs
    Write-Host 'Limpiando Network Interfaces...'
    $enisAll = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'ec2', 'describe-network-interfaces', '--region', $script:TitvoRegion, '--output', 'json'
    )
    $eniIds = @()
    foreach ($eni in @($enisAll.NetworkInterfaces)) {
        if (-not $eni -or $eni.Status -ne 'available') { continue }
        if (Test-TitvoAwsTagPrefixMatch -Resource $eni -Prefix $Prefix -TagPropertyName 'TagSet') {
            $eniIds += $eni.NetworkInterfaceId
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'ec2', 'delete-network-interface', '--network-interface-id', $eni.NetworkInterfaceId,
                '--region', $script:TitvoRegion
            )
        }
    }
    if ($script:TitvoApply -and $eniIds.Count -gt 0) {
        Wait-TitvoNetworkInterfacesDeleted -EniIds $eniIds
    }

    # 17) Subnets
    Remove-TitvoSubnets

    # 18) Security groups
    Remove-TitvoSecurityGroups

    # 19) CloudWatch Logs
    $logs = Invoke-TitvoAwsJsonLocal -AwsArgs @(
        'logs', 'describe-log-groups', '--region', $script:TitvoRegion, '--output', 'json'
    )
    foreach ($lg in @($logs.logGroups)) {
        if ($lg -and $lg.logGroupName -and $lg.logGroupName.ToLowerInvariant().Contains($Prefix.ToLowerInvariant())) {
            Invoke-TitvoRunAwsLocal -AwsArgs @(
                'logs', 'delete-log-group', '--log-group-name', $lg.logGroupName, '--region', $script:TitvoRegion
            )
        }
    }

    # 20) IAM
    Write-Host 'IAM candidates (Titvo match):'
    $roles = Invoke-TitvoAwsJsonLocal -AwsArgs @('iam', 'list-roles', '--output', 'json')
    $p = $Prefix.ToLowerInvariant()
    foreach ($r in @($roles.Roles)) {
        if (-not $r) { continue }
        $rn = $r.RoleName.ToLowerInvariant()
        if (-not ($rn.StartsWith($p) -or $rn.Contains('tvo'))) { continue }
        if (Test-TitvoIamRoleInUse -Role $r.RoleName) {
            Write-Host "[SKIP] IAM role en uso: $($r.RoleName)"
            continue
        }
        if ($script:TitvoApply) {
            Remove-TitvoIamRole -RoleName $r.RoleName
        }
        else {
            Write-Host "[DRY-RUN] aws iam delete-role --role-name `"$($r.RoleName)`" (usar -Apply)"
        }
    }

    $policies = Invoke-TitvoAwsJsonLocal -AwsArgs @('iam', 'list-policies', '--scope', 'Local', '--output', 'json')
    foreach ($pol in @($policies.Policies)) {
        if (-not $pol) { continue }
        $pn = $pol.PolicyName.ToLowerInvariant()
        if (-not ($pn.StartsWith($p) -or $pn.Contains('tvo'))) { continue }
        if ($script:TitvoApply) {
            Remove-TitvoIamPolicy -PolicyArn $pol.Arn
        }
        else {
            Write-Host "[DRY-RUN] aws iam delete-policy --policy-arn `"$($pol.Arn)`" (usar -Apply)"
        }
    }

    Write-Host "== Fin limpieza para prefijo '$Prefix' =="
}
