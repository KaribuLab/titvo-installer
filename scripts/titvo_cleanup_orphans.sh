#!/usr/bin/env bash
set -euo pipefail

# Evita que AWS CLI abra un pager interactivo durante la limpieza.
export AWS_PAGER=""

dotenv() {
  local env_file="$1"
  [[ -f "$env_file" ]] || return 0
  set -a
  # shellcheck disable=SC1090
  source "$env_file"
  set +a
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
dotenv "$REPO_ROOT/.env"

DEFAULT_PREFIXES=("tvo" "titvo")
REGION="${AWS_REGION:-us-east-2}"
APPLY=0

if [[ "${1:-}" == "--apply" ]]; then
  APPLY=1
fi

need() { command -v "$1" >/dev/null 2>&1 || { echo "Falta comando: $1"; exit 1; }; }
need aws
need jq

run() {
  if [[ "$APPLY" -eq 1 ]]; then
    echo "[APPLY] $*"
    local _rc=0
    eval "$@" || _rc=$?
    if [[ "$_rc" -ne 0 ]]; then
      echo "[WARN] comando fallo (rc=$_rc): $*"
    fi
    return 0
  else
    echo "[DRY-RUN] $*"
  fi
}

# ---------- helpers ----------
# Rellena un array desde stdin (compatible con Bash 3.2 en macOS; mapfile requiere Bash 4+).
read_lines_into() {
  local arr_name=$1
  local line
  local -a _lines=()
  while IFS= read -r line; do
    [[ -n "$line" ]] && _lines+=("$line")
  done
  array_copy "$arr_name" _lines
}

# Copia un array indexado (evita "${src[@]}" vacío con set -u en Bash 3.2).
array_copy() {
  local dest_name=$1 src_name=$2
  local i len
  eval "${dest_name}=()"
  eval "len=\${#${src_name}[@]}"
  for ((i = 0; i < len; i++)); do
    eval "${dest_name}+=(\"\${${src_name}[i]}\")"
  done
}

# Copia argumentos posicionales a un array indexado.
array_from_args() {
  local dest_name=$1
  shift
  eval "${dest_name}=()"
  local arg
  for arg in "$@"; do
    eval "${dest_name}+=(\"\$arg\")"
  done
}

USED_IAM_ROLES=""

mark_used_iam_role() {
  local role="$1"
  [[ -z "$role" ]] && return
  case $'\n'"$USED_IAM_ROLES"$'\n' in
    *$'\n'"$role"$'\n'*) return ;;
  esac
  if [[ -z "$USED_IAM_ROLES" ]]; then
    USED_IAM_ROLES="$role"
  else
    USED_IAM_ROLES="$USED_IAM_ROLES"$'\n'"$role"
  fi
}

iam_role_in_use() {
  local role="$1"
  [[ -z "$role" ]] && return 1
  case $'\n'"$USED_IAM_ROLES"$'\n' in
    *$'\n'"$role"$'\n'*) return 0 ;;
  esac
  return 1
}

SKIPPED_ITEMS=""

record_skip() {
  local reason="$1"
  local resource="$2"
  local entry="[$PREFIX] $reason: $resource"
  [[ -z "$resource" ]] && return
  case $'\n'"$SKIPPED_ITEMS"$'\n' in
    *$'\n'"$entry"$'\n'*) return ;;
  esac
  if [[ -z "$SKIPPED_ITEMS" ]]; then
    SKIPPED_ITEMS="$entry"
  else
    SKIPPED_ITEMS="$SKIPPED_ITEMS"$'\n'"$entry"
  fi
}

print_skipped_summary() {
  [[ "$APPLY" -ne 1 ]] && return
  echo ""
  if [[ -z "$SKIPPED_ITEMS" ]]; then
    echo "== Recursos omitidos (skipped): ninguno =="
    return
  fi
  local count=0
  local line
  echo "== Recursos omitidos (skipped) =="
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    echo "  - $line"
    count=$((count + 1))
  done <<< "$SKIPPED_ITEMS"
  echo "Total omitidos: $count"
}

starts_with_prefix() {
  [[ "$1" == "$PREFIX"* ]]
}

empty_s3_bucket() {
  local bucket="$1"
  local key_marker=""
  local version_marker=""
  local uploads_key_marker=""
  local uploads_id_marker=""
  local versions
  local uploads

  run "aws s3 rm s3://$bucket --recursive --region $REGION || true"

  while :; do
    if [[ -n "$key_marker" ]]; then
      versions="$(aws s3api list-object-versions --bucket "$bucket" --region "$REGION" --key-marker "$key_marker" --version-id-marker "$version_marker" --output json 2>/dev/null || echo '{}')"
    else
      versions="$(aws s3api list-object-versions --bucket "$bucket" --region "$REGION" --output json 2>/dev/null || echo '{}')"
    fi

    echo "$versions" | jq -r '.Versions[]? | @base64' | while read -r v; do
      key="$(echo "$v" | base64 -d | jq -r '.Key')"
      vid="$(echo "$v" | base64 -d | jq -r '.VersionId')"
      run "aws s3api delete-object --bucket \"$bucket\" --key \"$key\" --version-id \"$vid\" --region \"$REGION\""
    done

    echo "$versions" | jq -r '.DeleteMarkers[]? | @base64' | while read -r v; do
      key="$(echo "$v" | base64 -d | jq -r '.Key')"
      vid="$(echo "$v" | base64 -d | jq -r '.VersionId')"
      run "aws s3api delete-object --bucket \"$bucket\" --key \"$key\" --version-id \"$vid\" --region \"$REGION\""
    done

    if [[ "$(echo "$versions" | jq -r '.IsTruncated // false')" != "true" ]]; then
      break
    fi

    key_marker="$(echo "$versions" | jq -r '.NextKeyMarker // empty')"
    version_marker="$(echo "$versions" | jq -r '.NextVersionIdMarker // empty')"
  done

  while :; do
    if [[ -n "$uploads_key_marker" ]]; then
      uploads="$(aws s3api list-multipart-uploads --bucket "$bucket" --region "$REGION" --key-marker "$uploads_key_marker" --upload-id-marker "$uploads_id_marker" --output json 2>/dev/null || echo '{}')"
    else
      uploads="$(aws s3api list-multipart-uploads --bucket "$bucket" --region "$REGION" --output json 2>/dev/null || echo '{}')"
    fi

    echo "$uploads" | jq -r '.Uploads[]? | @base64' | while read -r u; do
      key="$(echo "$u" | base64 -d | jq -r '.Key')"
      uid="$(echo "$u" | base64 -d | jq -r '.UploadId')"
      run "aws s3api abort-multipart-upload --bucket \"$bucket\" --key \"$key\" --upload-id \"$uid\" --region \"$REGION\""
    done

    if [[ "$(echo "$uploads" | jq -r '.IsTruncated // false')" != "true" ]]; then
      break
    fi

    uploads_key_marker="$(echo "$uploads" | jq -r '.NextKeyMarker // empty')"
    uploads_id_marker="$(echo "$uploads" | jq -r '.NextUploadIdMarker // empty')"
  done
}

delete_ssm_parameters_by_path() {
  local path="/$PREFIX"
  local next_token=""
  local params
  local -a names=()
  local i

  while :; do
    if [[ -n "$next_token" ]]; then
      params="$(aws ssm get-parameters-by-path --path "$path" --recursive --with-decryption --region "$REGION" --output json --next-token "$next_token")"
    else
      params="$(aws ssm get-parameters-by-path --path "$path" --recursive --with-decryption --region "$REGION" --output json)"
    fi

    read_lines_into names < <(echo "$params" | jq -r '.Parameters[]?.Name')
    local _quoted _j _end
    for ((i = 0; i < ${#names[@]}; i += 10)); do
      _quoted=""
      _end=$((i + 10))
      ((_end > ${#names[@]})) && _end=${#names[@]}
      for ((_j = i; _j < _end; _j++)); do
        _quoted+=" $(printf '%q' "${names[_j]}")"
      done
      [[ -z "$_quoted" ]] && continue
      run "aws ssm delete-parameters --region \"$REGION\" --names$_quoted"
    done

    next_token="$(echo "$params" | jq -r '.NextToken // empty')"
    [[ -z "$next_token" ]] && break
  done
}

delete_cloud_map_namespace() {
  local namespace_id="$1"
  local service_ids
  local op_id

  service_ids="$(aws servicediscovery list-services --filters Name=NAMESPACE_ID,Values="$namespace_id",Condition=EQ --region "$REGION" --output json | jq -r '.Services[]?.Id')"
  while read -r service_id; do
    [[ -z "$service_id" ]] && continue
    aws servicediscovery list-instances --service-id "$service_id" --region "$REGION" --output json | jq -r '.Instances[]?.Id' | while read -r instance_id; do
      [[ -z "$instance_id" ]] && continue
      run "aws servicediscovery deregister-instance --service-id \"$service_id\" --instance-id \"$instance_id\" --region \"$REGION\""
    done
    run "aws servicediscovery delete-service --id \"$service_id\" --region \"$REGION\""
  done <<< "$service_ids"

  if [[ "$APPLY" -eq 1 ]]; then
    op_id="$(aws servicediscovery delete-namespace --id "$namespace_id" --region "$REGION" --output json | jq -r '.OperationId // empty')"
    if [[ -n "$op_id" ]]; then
      echo "[WAIT] Cloud Map namespace delete operation: $op_id"
      wait_for_cloud_map_operation "$op_id"
    fi
  else
    echo "[DRY-RUN] aws servicediscovery delete-namespace --id \"$namespace_id\" --region \"$REGION\""
  fi
}

wait_for_cloud_map_operation() {
  local operation_id="$1"
  local max_attempts=24
  local sleep_seconds=5
  local attempt=1
  local status
  local error_message

  while [[ "$attempt" -le "$max_attempts" ]]; do
    status="$(aws servicediscovery get-operation --operation-id "$operation_id" --region "$REGION" --output json | jq -r '.Operation.Status // empty')"
    case "$status" in
      SUCCESS)
        return 0
        ;;
      FAIL)
        error_message="$(aws servicediscovery get-operation --operation-id "$operation_id" --region "$REGION" --output json | jq -r '.Operation.ErrorMessage // "Cloud Map operation failed"')"
        echo "[WARN] Cloud Map operation failed: $error_message"
        return 1
        ;;
    esac
    echo "[WAIT] Cloud Map operation $operation_id status=$status (intento $attempt/$max_attempts)"
    sleep "$sleep_seconds"
    attempt=$((attempt + 1))
  done

  echo "[WARN] Cloud Map operation sigue pendiente: $operation_id"
  return 1
}

detach_and_delete_eni_by_id() {
  local eni_id="$1"
  local attachment_id
  local status

  [[ -z "$eni_id" ]] && return 0

  status="$(aws ec2 describe-network-interfaces --network-interface-ids "$eni_id" --region "$REGION" --output json 2>/dev/null \
    | jq -r '.NetworkInterfaces[0].Status // empty')"
  attachment_id="$(aws ec2 describe-network-interfaces --network-interface-ids "$eni_id" --region "$REGION" --output json 2>/dev/null \
    | jq -r '.NetworkInterfaces[0].Attachment.AttachmentId // empty')"

  if [[ -n "$attachment_id" ]]; then
    run "aws ec2 detach-network-interface --attachment-id \"$attachment_id\" --force --region \"$REGION\""
  fi

  if [[ "$status" == "available" || -n "$attachment_id" ]]; then
    run "aws ec2 delete-network-interface --network-interface-id \"$eni_id\" --region \"$REGION\""
  else
    echo "[INFO] ENI $eni_id status=$status (no available, posiblemente gestionado por AWS); se intentara borrar igual"
    run "aws ec2 delete-network-interface --network-interface-id \"$eni_id\" --region \"$REGION\""
  fi
}

delete_subnets() {
  local -a subnet_ids=()
  local subnet_id

  read_lines_into subnet_ids < <(aws ec2 describe-subnets --region "$REGION" --output json \
  | jq -r --arg p "$PREFIX" '.Subnets[]?
      | select(
          (.Tags // []) | any(
            (.Key == "Name" and (.Value | ascii_downcase | startswith($p)))
            or (.Key == "Project" and (.Value | ascii_downcase | contains($p)))
          )
        )
      | .SubnetId')

  local _i
  for ((_i = 0; _i < ${#subnet_ids[@]}; _i++)); do
    subnet_id="${subnet_ids[_i]}"
    [[ -z "$subnet_id" ]] && continue
    run "aws ec2 delete-subnet --subnet-id \"$subnet_id\" --region \"$REGION\""
  done

  if [[ "$APPLY" -eq 1 && "${#subnet_ids[@]}" -gt 0 ]]; then
    wait_for_subnets_deletion "${subnet_ids[@]}"
  fi
}

wait_for_subnets_deletion() {
  local max_attempts=24
  local sleep_seconds=5
  local attempt=1
  local remaining_ids=()
  local subnet_id
  local _i

  array_from_args remaining_ids "$@"

  while [[ "$attempt" -le "$max_attempts" && "${#remaining_ids[@]}" -gt 0 ]]; do
    local -a still_remaining=()
    local _i
    for ((_i = 0; _i < ${#remaining_ids[@]}; _i++)); do
      subnet_id="${remaining_ids[_i]}"
      [[ -z "$subnet_id" ]] && continue
      if aws ec2 describe-subnets --subnet-ids "$subnet_id" --region "$REGION" --output json >/dev/null 2>&1; then
        still_remaining+=("$subnet_id")
      fi
    done

    if [[ "${#still_remaining[@]}" -eq 0 ]]; then
      return 0
    fi

    echo "[WAIT] Subnets aun presentes: ${#still_remaining[@]} (intento $attempt/$max_attempts)"
    sleep "$sleep_seconds"
    array_copy remaining_ids still_remaining
    attempt=$((attempt + 1))
  done

  for ((_i = 0; _i < ${#remaining_ids[@]}; _i++)); do
    subnet_id="${remaining_ids[_i]}"
    [[ -z "$subnet_id" ]] && continue
    run "aws ec2 delete-subnet --subnet-id \"$subnet_id\" --region \"$REGION\""
  done

  return 0
}

list_candidate_security_group_ids() {
  aws ec2 describe-security-groups --region "$REGION" --output json \
  | jq -r --arg p "$PREFIX" '.SecurityGroups[]?
    | select(.GroupName != "default")
    | select(.Description != "default VPC security group")
    | select(
        (.GroupName | ascii_downcase | startswith($p))
        or (.GroupName | ascii_downcase | contains($p))
        or (.Description | ascii_downcase | contains($p))
        or ((.Tags // []) | any(
          (.Key == "Name" and (.Value | ascii_downcase | startswith($p)))
          or (.Key == "Project" and (.Value | ascii_downcase | contains($p)))
        ))
      )
    | .GroupId'
}

delete_vpc_endpoints() {
  local sg_ids_json
  local -a vpce_ids=()
  local vpce_id

  sg_ids_json="$(list_candidate_security_group_ids | jq -R . | jq -s .)"
  read_lines_into vpce_ids < <(aws ec2 describe-vpc-endpoints --region "$REGION" --output json \
  | jq -r --argjson sg_ids "$sg_ids_json" --arg p "$PREFIX" '.VpcEndpoints[]?
      | select(
          any(.Groups[]?.GroupId; $sg_ids | index(.))
          or ((.Tags // []) | any(
            (.Key == "Name" and (.Value | ascii_downcase | startswith($p)))
            or (.Key == "Project" and (.Value | ascii_downcase | contains($p)))
          ))
        )
      | .VpcEndpointId')

  local _i
  for ((_i = 0; _i < ${#vpce_ids[@]}; _i++)); do
    vpce_id="${vpce_ids[_i]}"
    [[ -z "$vpce_id" ]] && continue
    run "aws ec2 delete-vpc-endpoints --vpc-endpoint-ids \"$vpce_id\" --region \"$REGION\""
  done

  if [[ "$APPLY" -eq 1 && "${#vpce_ids[@]}" -gt 0 ]]; then
    wait_for_vpc_endpoints_deletion "${vpce_ids[@]}"
  fi
}

wait_for_vpc_endpoints_deletion() {
  local max_attempts=60
  local sleep_seconds=5
  local attempt=1
  local remaining

  while [[ "$attempt" -le "$max_attempts" ]]; do
    remaining="$(aws ec2 describe-vpc-endpoints --vpc-endpoint-ids "$@" --region "$REGION" --output json 2>/dev/null | jq -r '.VpcEndpoints | length' 2>/dev/null || true)"
    if [[ -z "$remaining" || "$remaining" == "0" ]]; then
      return 0
    fi
    echo "[WAIT] VPC endpoints aun presentes: $remaining (intento $attempt/$max_attempts)"
    sleep "$sleep_seconds"
    attempt=$((attempt + 1))
  done

  echo "[WARN] Algunos VPC endpoints siguen presentes; intento continuar con security groups"
  return 1
}

wait_for_network_interfaces_deleted() {
  local -a eni_ids=()
  local max_attempts=36
  local sleep_seconds=5
  local attempt=1
  local remaining
  local -a still_remaining=()
  local _i

  array_from_args eni_ids "$@"

  while [[ "$attempt" -le "$max_attempts" && "${#eni_ids[@]}" -gt 0 ]]; do
    still_remaining=()
    for ((_i = 0; _i < ${#eni_ids[@]}; _i++)); do
      eni_id="${eni_ids[_i]}"
      [[ -z "$eni_id" ]] && continue
      if aws ec2 describe-network-interfaces --network-interface-ids "$eni_id" --region "$REGION" --output json 2>/dev/null | jq -e '.NetworkInterfaces[0]' >/dev/null 2>&1; then
        still_remaining+=("$eni_id")
      fi
    done

    if [[ "${#still_remaining[@]}" -eq 0 ]]; then
      return 0
    fi

    echo "[WAIT] Network Interfaces aun presentes: ${#still_remaining[@]} (intento $attempt/$max_attempts)"
    sleep "$sleep_seconds"
    array_copy eni_ids still_remaining
    attempt=$((attempt + 1))
  done

  echo "[WARN] Algunas Network Interfaces siguen presentes; intento continuar"
  return 1
}

detach_and_delete_enis_for_sg() {
  local group_id="$1"
  local -a eni_ids=()
  local eni_id

  read_lines_into eni_ids < <(aws ec2 describe-network-interfaces \
    --filters "Name=group-id,Values=$group_id" \
    --region "$REGION" --output json 2>/dev/null \
    | jq -r '.NetworkInterfaces[]?.NetworkInterfaceId')

  local _i
  for ((_i = 0; _i < ${#eni_ids[@]}; _i++)); do
    eni_id="${eni_ids[_i]}"
    detach_and_delete_eni_by_id "$eni_id"
  done

  if [[ "$APPLY" -eq 1 && "${#eni_ids[@]}" -gt 0 ]]; then
    wait_for_network_interfaces_deleted "${eni_ids[@]}" || true
  fi
}

_build_targeted_permissions() {
  # Devuelve, por cada SG que referencia $target en el sentido indicado
  # (ingress o egress), pares "<group_id>\t<permission_json>" donde el
  # permission_json es una copia minima del IpPermission que conserva
  # protocolo/puertos pero deja UserIdGroupPairs SOLO con el target_sg y
  # vacia IpRanges/Ipv6Ranges/PrefixListIds para no borrar reglas ajenas.
  local target_sg="$1"
  local direction="$2" # "ingress" | "egress"
  local perms_field filter_name

  if [[ "$direction" == "egress" ]]; then
    perms_field="IpPermissionsEgress"
    filter_name="egress.ip-permission.group-id"
  else
    perms_field="IpPermissions"
    filter_name="ip-permission.group-id"
  fi

  aws ec2 describe-security-groups \
    --filters "Name=$filter_name,Values=$target_sg" \
    --region "$REGION" --output json 2>/dev/null \
  | jq -r --arg target "$target_sg" --arg field "$perms_field" '
      .SecurityGroups[]?
      | . as $sg
      | $sg[$field][]?
      | select((.UserIdGroupPairs // []) | any(.GroupId == $target))
      | {
          IpProtocol: .IpProtocol,
          FromPort: (if has("FromPort") then .FromPort else null end),
          ToPort: (if has("ToPort") then .ToPort else null end),
          UserIdGroupPairs: [ .UserIdGroupPairs[]? | select(.GroupId == $target) | {GroupId: .GroupId} ],
          IpRanges: [],
          Ipv6Ranges: [],
          PrefixListIds: []
        }
      | with_entries(select(.value != null))
      | [$sg.GroupId, (. | tostring)]
      | @tsv
    '
}

revoke_cross_sg_references() {
  local target_sg="$1"
  local other_sg
  local perm_block

  while IFS=$'\t' read -r other_sg perm_block; do
    [[ -z "$other_sg" || "$other_sg" == "null" ]] && continue
    run "aws ec2 revoke-security-group-ingress --group-id \"$other_sg\" --ip-permissions '$perm_block' --region \"$REGION\""
  done < <(_build_targeted_permissions "$target_sg" "ingress")

  while IFS=$'\t' read -r other_sg perm_block; do
    [[ -z "$other_sg" || "$other_sg" == "null" ]] && continue
    run "aws ec2 revoke-security-group-egress --group-id \"$other_sg\" --ip-permissions '$perm_block' --region \"$REGION\""
  done < <(_build_targeted_permissions "$target_sg" "egress")
}

diagnose_sg_dependencies() {
  local group_id="$1"
  local enis
  local sg_refs

  echo "[DIAG] Dependencias residuales de $group_id:"

  enis="$(aws ec2 describe-network-interfaces \
    --filters "Name=group-id,Values=$group_id" \
    --region "$REGION" --output json 2>/dev/null \
    | jq -r '.NetworkInterfaces[]? | "  ENI \(.NetworkInterfaceId) status=\(.Status) desc=\"\(.Description // "")\" attach=\(.Attachment.InstanceId // .Attachment.AttachmentId // "none")"')"
  if [[ -n "$enis" ]]; then
    echo "$enis"
  else
    echo "  (sin ENIs)"
  fi

  sg_refs="$(aws ec2 describe-security-groups \
    --filters "Name=ip-permission.group-id,Values=$group_id" \
    --region "$REGION" --output json 2>/dev/null \
    | jq -r '.SecurityGroups[]? | "  SG ref ingress: \(.GroupId) (\(.GroupName))"')"
  if [[ -n "$sg_refs" ]]; then
    echo "$sg_refs"
  fi

  sg_refs="$(aws ec2 describe-security-groups \
    --filters "Name=egress.ip-permission.group-id,Values=$group_id" \
    --region "$REGION" --output json 2>/dev/null \
    | jq -r '.SecurityGroups[]? | "  SG ref egress:  \(.GroupId) (\(.GroupName))"')"
  if [[ -n "$sg_refs" ]]; then
    echo "$sg_refs"
  fi
}

try_delete_security_group() {
  local group_id="$1"
  local max_attempts=6
  local sleep_seconds=10
  local attempt=1
  local rc=0
  local err

  while [[ "$attempt" -le "$max_attempts" ]]; do
    if [[ "$APPLY" -eq 1 ]]; then
      err="$(aws ec2 delete-security-group --group-id "$group_id" --region "$REGION" 2>&1)"
      rc=$?
    else
      echo "[DRY-RUN] aws ec2 delete-security-group --group-id \"$group_id\" --region \"$REGION\""
      return 0
    fi

    if [[ "$rc" -eq 0 ]]; then
      echo "[OK] SG eliminado: $group_id"
      return 0
    fi

    if echo "$err" | grep -q "InvalidGroup.NotFound"; then
      echo "[OK] SG $group_id ya no existe"
      return 0
    fi

    echo "[WARN] No se pudo borrar $group_id (intento $attempt/$max_attempts): $err"

    if echo "$err" | grep -q "DependencyViolation"; then
      detach_and_delete_enis_for_sg "$group_id"
      revoke_cross_sg_references "$group_id"
    fi

    sleep "$sleep_seconds"
    attempt=$((attempt + 1))
  done

  echo "[ERROR] No se logro borrar SG $group_id tras $max_attempts intentos"
  diagnose_sg_dependencies "$group_id"
  return 1
}

delete_security_groups() {
  local -a group_ids=()
  local group_id

  read_lines_into group_ids < <(list_candidate_security_group_ids)

  local _i
  for ((_i = 0; _i < ${#group_ids[@]}; _i++)); do
    group_id="${group_ids[_i]}"
    [[ -z "$group_id" ]] && continue
    revoke_cross_sg_references "$group_id"
    detach_and_delete_enis_for_sg "$group_id"
  done

  for ((_i = 0; _i < ${#group_ids[@]}; _i++)); do
    group_id="${group_ids[_i]}"
    [[ -z "$group_id" ]] && continue
    try_delete_security_group "$group_id" || true
  done
}

delete_eventbridge_rules_for_bus() {
  local bus_name="$1"
  local rules
  local ids
  local ids_json

  if [[ "$bus_name" == "default" ]]; then
    rules="$(aws events list-rules --name-prefix "$PREFIX" --region "$REGION" --output json | jq -r '.Rules[]?.Name')"
  else
    rules="$(aws events list-rules --event-bus-name "$bus_name" --region "$REGION" --output json | jq -r '.Rules[]?.Name')"
  fi

  while read -r rule; do
    [[ -z "$rule" ]] && continue
    if [[ "$bus_name" == "default" ]]; then
      ids="$(aws events list-targets-by-rule --rule "$rule" --region "$REGION" --output json | jq -r '.Targets[]?.Id')"
    else
      ids="$(aws events list-targets-by-rule --rule "$rule" --event-bus-name "$bus_name" --region "$REGION" --output json | jq -r '.Targets[]?.Id')"
    fi

    if [[ -n "$ids" ]]; then
      ids_json="$(printf '%s\n' "$ids" | jq -R . | jq -s .)"
      if [[ "$bus_name" == "default" ]]; then
        run "aws events remove-targets --rule \"$rule\" --ids '$ids_json' --region \"$REGION\""
      else
        run "aws events remove-targets --rule \"$rule\" --event-bus-name \"$bus_name\" --ids '$ids_json' --region \"$REGION\""
      fi
    fi

    if [[ "$bus_name" == "default" ]]; then
      run "aws events delete-rule --name \"$rule\" --region \"$REGION\""
    else
      run "aws events delete-rule --name \"$rule\" --event-bus-name \"$bus_name\" --region \"$REGION\""
    fi
  done <<< "$rules"
}

wait_for_batch_job_queues_deletion() {
  local max_attempts=24
  local sleep_seconds=5
  local attempt=1
  local remaining

  while [[ "$attempt" -le "$max_attempts" ]]; do
    remaining="$(aws batch describe-job-queues --region "$REGION" --output json \
      | jq -r --arg p "$PREFIX" '[.jobQueues[]? | select(.jobQueueName | startswith($p))] | length')"

    if [[ "$remaining" == "0" ]]; then
      return 0
    fi

    echo "[WAIT] Batch job queues aun presentes: $remaining (intento $attempt/$max_attempts)"
    sleep "$sleep_seconds"
    attempt=$((attempt + 1))
  done

  echo "[WARN] Batch job queues siguen presentes; intento continuar con compute environments"
  return 1
}

wait_for_batch_job_queue_disabled() {
  local job_queue="$1"
  local max_attempts=36
  local sleep_seconds=5
  local attempt=1
  local status
  local state

  while [[ "$attempt" -le "$max_attempts" ]]; do
    status="$(aws batch describe-job-queues --job-queues "$job_queue" --region "$REGION" --output json 2>/dev/null | jq -r '.jobQueues[0].status // empty')"
    state="$(aws batch describe-job-queues --job-queues "$job_queue" --region "$REGION" --output json 2>/dev/null | jq -r '.jobQueues[0].state // empty')"

    if [[ -z "$status" ]]; then
      return 0
    fi

    if [[ "$status" == "VALID" && "$state" == "DISABLED" ]]; then
      return 0
    fi

    echo "[WAIT] Job queue $job_queue status=$status state=$state (intento $attempt/$max_attempts)"
    sleep "$sleep_seconds"
    attempt=$((attempt + 1))
  done

  echo "[WARN] Job queue $job_queue sigue modificandose; intento borrarlo igual"
  return 1
}

wait_for_batch_compute_environment_disabled() {
  local compute_environment="$1"
  local max_attempts=36
  local sleep_seconds=5
  local attempt=1
  local status
  local state

  while [[ "$attempt" -le "$max_attempts" ]]; do
    status="$(aws batch describe-compute-environments --compute-environments "$compute_environment" --region "$REGION" --output json 2>/dev/null | jq -r '.computeEnvironments[0].status // empty')"
    state="$(aws batch describe-compute-environments --compute-environments "$compute_environment" --region "$REGION" --output json 2>/dev/null | jq -r '.computeEnvironments[0].state // empty')"

    if [[ -z "$status" ]]; then
      return 0
    fi

    if [[ "$status" == "VALID" && "$state" == "DISABLED" ]]; then
      return 0
    fi

    echo "[WAIT] Compute environment $compute_environment status=$status state=$state (intento $attempt/$max_attempts)"
    sleep "$sleep_seconds"
    attempt=$((attempt + 1))
  done

  echo "[WARN] Compute environment $compute_environment sigue modificandose; intento borrarlo igual"
  return 1
}

delete_iam_policy() {
  local policy_arn="$1"
  local default_version

  aws iam list-entities-for-policy --policy-arn "$policy_arn" --output json | jq -r '.PolicyRoles[]?.RoleName' | while read -r role_name; do
    [[ -z "$role_name" ]] && continue
    run "aws iam detach-role-policy --role-name \"$role_name\" --policy-arn \"$policy_arn\""
  done

  aws iam list-entities-for-policy --policy-arn "$policy_arn" --output json | jq -r '.PolicyUsers[]?.UserName' | while read -r user_name; do
    [[ -z "$user_name" ]] && continue
    run "aws iam detach-user-policy --user-name \"$user_name\" --policy-arn \"$policy_arn\""
  done

  aws iam list-entities-for-policy --policy-arn "$policy_arn" --output json | jq -r '.PolicyGroups[]?.GroupName' | while read -r group_name; do
    [[ -z "$group_name" ]] && continue
    run "aws iam detach-group-policy --group-name \"$group_name\" --policy-arn \"$policy_arn\""
  done

  default_version="$(aws iam get-policy --policy-arn "$policy_arn" --output json | jq -r '.Policy.DefaultVersionId')"
  aws iam list-policy-versions --policy-arn "$policy_arn" --output json | jq -r --arg dv "$default_version" '.Versions[]? | select(.VersionId != $dv) | .VersionId' | while read -r version_id; do
    [[ -z "$version_id" ]] && continue
    run "aws iam delete-policy-version --policy-arn \"$policy_arn\" --version-id \"$version_id\""
  done

  run "aws iam delete-policy --policy-arn \"$policy_arn\""
}

delete_iam_role() {
  local role_name="$1"

  aws iam list-attached-role-policies --role-name "$role_name" --output json | jq -r '.AttachedPolicies[]?.PolicyArn' | while read -r p; do
    [[ -z "$p" ]] && continue
    run "aws iam detach-role-policy --role-name \"$role_name\" --policy-arn \"$p\""
  done

  aws iam list-role-policies --role-name "$role_name" --output json | jq -r '.PolicyNames[]?' | while read -r pn; do
    [[ -z "$pn" ]] && continue
    run "aws iam delete-role-policy --role-name \"$role_name\" --policy-name \"$pn\""
  done

  aws iam list-instance-profiles-for-role --role-name "$role_name" --output json | jq -r '.InstanceProfiles[]?.InstanceProfileName' | while read -r profile_name; do
    [[ -z "$profile_name" ]] && continue
    run "aws iam remove-role-from-instance-profile --instance-profile-name \"$profile_name\" --role-name \"$role_name\""
    run "aws iam delete-instance-profile --instance-profile-name \"$profile_name\""
  done

  run "aws iam delete-role --role-name \"$role_name\""
}

collect_used_iam_references() {
  # Recolectar despues de borrar Lambda/ECS/Batch para no marcar roles de recursos ya eliminados.
  USED_IAM_ROLES=""

  echo "Recolectando referencias en uso (IAM)..."

  # Lambda refs
  while IFS=$'\t' read -r fn ptype imageuri rolearn; do
    if [[ "$ptype" == "Image" && -n "$imageuri" && "$imageuri" != "null" ]]; then
      :
    fi
    if [[ -n "$rolearn" && "$rolearn" != "null" ]]; then
      role="${rolearn##*/}"
      mark_used_iam_role "$role"
    fi
  done < <(
    aws lambda list-functions --region "$REGION" --output json \
      | jq -r '.Functions[] | [.FunctionName, .PackageType, .Code.ImageUri, .Role] | @tsv'
  )

  # ECS refs
  taskdefs="$(aws ecs list-task-definitions --status ACTIVE --region "$REGION" --output json | jq -r '.taskDefinitionArns[]?')"
  while read -r td; do
    [[ -z "$td" ]] && continue
    data="$(aws ecs describe-task-definition --task-definition "$td" --region "$REGION" --output json)"
    while read -r img; do
      [[ -z "$img" || "$img" == "null" ]] && continue
      if [[ "$img" == *.dkr.ecr.*.amazonaws.com/* ]]; then
        :
      fi
    done < <(echo "$data" | jq -r '.taskDefinition.containerDefinitions[]?.image')

    while read -r r; do
      [[ -z "$r" || "$r" == "null" ]] && continue
      mark_used_iam_role "${r##*/}"
    done < <(echo "$data" | jq -r '.taskDefinition.taskRoleArn, .taskDefinition.executionRoleArn')
  done <<< "$taskdefs"

  # Batch refs
  jobdefs="$(aws batch describe-job-definitions --status ACTIVE --region "$REGION" --output json | jq -r '.jobDefinitions[]?.jobDefinitionArn')"
  while read -r jd; do
    [[ -z "$jd" ]] && continue
    data="$(aws batch describe-job-definitions --job-definitions "$jd" --region "$REGION" --output json)"
    while read -r img; do
      [[ -z "$img" || "$img" == "null" ]] && continue
      if [[ "$img" == *.dkr.ecr.*.amazonaws.com/* ]]; then
        :
      fi
    done < <(echo "$data" | jq -r '.jobDefinitions[]?.containerProperties.image')

    while read -r r; do
      [[ -z "$r" || "$r" == "null" ]] && continue
      mark_used_iam_role "${r##*/}"
    done < <(echo "$data" | jq -r '.jobDefinitions[]?.containerProperties.jobRoleArn, .jobDefinitions[]?.containerProperties.executionRoleArn')
  done <<< "$jobdefs"
}

run_cleanup_for_prefix() {
  local current_prefix="$1"
  PREFIX="$current_prefix"
  echo "== Inicio limpieza candidatos prefix '$PREFIX' (region $REGION) =="

# 1) EventBridge rules + targets
# default bus rules with prefix
delete_eventbridge_rules_for_bus "default"

# custom event buses with prefix + their rules
aws events list-event-buses --region "$REGION" --output json \
| jq -r --arg p "$PREFIX" '.EventBuses[]? | select(.Name != "default") | select(.Name | startswith($p)) | .Name' \
| while read -r bus_name; do
  [[ -z "$bus_name" ]] && continue
  delete_eventbridge_rules_for_bus "$bus_name"
  run "aws events delete-event-bus --name \"$bus_name\" --region \"$REGION\""
done

# 2) API Gateway v2
aws apigatewayv2 get-apis --region "$REGION" --output json \
| jq -r --arg p "$PREFIX" '.Items[]? | select(.Name | startswith($p)) | .ApiId' \
| while read -r api; do
  [[ -z "$api" ]] && continue
  run "aws apigatewayv2 delete-api --api-id \"$api\" --region \"$REGION\""
done

wait_for_lambda_event_source_mappings_deleted() {
  local max_attempts=24
  local sleep_seconds=5
  local attempt=1
  local remaining

  while [[ "$attempt" -le "$max_attempts" ]]; do
    remaining="$(aws lambda list-event-source-mappings --max-items 100 --region "$REGION" --output json \
      | jq -r --arg p "$PREFIX" '[.EventSourceMappings[]? | select(.FunctionArn | contains($p))] | length')"

    if [[ "$remaining" == "0" ]]; then
      return 0
    fi

    echo "[WAIT] Event Source Mappings aun presentes: $remaining (intento $attempt/$max_attempts)"
    sleep "$sleep_seconds"
    attempt=$((attempt + 1))
  done

  echo "[WARN] Algunos Event Source Mappings siguen presentes"
  return 1
}

# 3) Lambda Event Source Mappings (antes de Lambda y SQS)
echo "Limpiando Lambda Event Source Mappings..."
aws lambda list-event-source-mappings --max-items 100 --region "$REGION" --output json \
| jq -r --arg p "$PREFIX" '.EventSourceMappings[]? | select(.FunctionArn | contains($p)) | .UUID' \
| while read -r uuid; do
  [[ -z "$uuid" ]] && continue
  run "aws lambda delete-event-source-mapping --uuid \"$uuid\" --region \"$REGION\""
done

if [[ "$APPLY" -eq 1 ]]; then
  wait_for_lambda_event_source_mappings_deleted || true
fi

# 4) Lambda
aws lambda list-functions --region "$REGION" --output json \
| jq -r --arg p "$PREFIX" '.Functions[] | select(.FunctionName | startswith($p)) | .FunctionName' \
| while read -r fn; do
  run "aws lambda delete-function --function-name \"$fn\" --region \"$REGION\""
done

# 5) SQS
aws sqs list-queues --queue-name-prefix "$PREFIX" --region "$REGION" --output json \
| jq -r '.QueueUrls[]?' | while read -r qurl; do
  run "aws sqs delete-queue --queue-url \"$qurl\" --region \"$REGION\""
done

wait_for_ecs_cluster_empty() {
  local cluster="$1"
  local max_attempts=36
  local sleep_seconds=5
  local attempt=1
  local running_tasks

  while [[ "$attempt" -le "$max_attempts" ]]; do
    running_tasks="$(aws ecs list-tasks --cluster "$cluster" --region "$REGION" --output json | jq -r '.taskArns | length')"

    if [[ "$running_tasks" == "0" ]]; then
      return 0
    fi

    echo "[WAIT] Cluster $cluster aun tiene $running_tasks tareas (intento $attempt/$max_attempts)"
    sleep "$sleep_seconds"
    attempt=$((attempt + 1))
  done

  echo "[WARN] Cluster $cluster sigue con tareas activas"
  return 1
}

# 6) ECS services + clusters
aws ecs list-clusters --region "$REGION" --output json | jq -r '.clusterArns[]?' | while read -r c; do
  cname="${c##*/}"
  starts_with_prefix "$cname" || continue

  aws ecs list-services --cluster "$c" --region "$REGION" --output json | jq -r '.serviceArns[]?' | while read -r s; do
    sname="${s##*/}"
    run "aws ecs update-service --cluster \"$c\" --service \"$sname\" --desired-count 0 --region \"$REGION\""
    run "aws ecs delete-service --cluster \"$c\" --service \"$sname\" --force --region \"$REGION\""
  done

  if [[ "$APPLY" -eq 1 ]]; then
    wait_for_ecs_cluster_empty "$c" || true
  fi

  run "aws ecs delete-cluster --cluster \"$c\" --region \"$REGION\""
done

aws ecs list-task-definitions --status ACTIVE --region "$REGION" --output json \
| jq -r --arg p "$PREFIX" '.taskDefinitionArns[]? | select((split("/")[1] | split(":")[0]) | startswith($p))' \
| while read -r td; do
  [[ -z "$td" ]] && continue
  run "aws ecs deregister-task-definition --task-definition \"$td\" --region \"$REGION\""
done

# 7) Batch (job queues, compute env, job defs)
aws batch describe-job-queues --region "$REGION" --output json \
| jq -r --arg p "$PREFIX" '.jobQueues[]? | select(.jobQueueName | startswith($p)) | .jobQueueName' \
| while read -r jqn; do
  run "aws batch update-job-queue --job-queue \"$jqn\" --state DISABLED --region \"$REGION\""
  if [[ "$APPLY" -eq 1 ]]; then
    wait_for_batch_job_queue_disabled "$jqn" || true
  fi
  run "aws batch delete-job-queue --job-queue \"$jqn\" --region \"$REGION\""
done

if [[ "$APPLY" -eq 1 ]]; then
  wait_for_batch_job_queues_deletion || true
fi

aws batch describe-compute-environments --region "$REGION" --output json \
| jq -r --arg p "$PREFIX" '.computeEnvironments[]? | select(.computeEnvironmentName | startswith($p)) | .computeEnvironmentName' \
| while read -r cen; do
  run "aws batch update-compute-environment --compute-environment \"$cen\" --state DISABLED --region \"$REGION\""
  if [[ "$APPLY" -eq 1 ]]; then
    wait_for_batch_compute_environment_disabled "$cen" || true
  fi
  run "aws batch delete-compute-environment --compute-environment \"$cen\" --region \"$REGION\""
done

aws batch describe-job-definitions --status ACTIVE --region "$REGION" --output json \
| jq -r --arg p "$PREFIX" '.jobDefinitions[]? | select(.jobDefinitionName | startswith($p)) | .jobDefinitionArn' \
| while read -r jd; do
  run "aws batch deregister-job-definition --job-definition \"$jd\" --region \"$REGION\""
done

# 8) ECR
aws ecr describe-repositories --region "$REGION" --output json \
| jq -r --arg p "$PREFIX" '.repositories[]? | select(.repositoryName | startswith($p)) | .repositoryName' \
| while read -r repo; do
  [[ -z "$repo" ]] && continue
  run "aws ecr delete-repository --repository-name \"$repo\" --force --region \"$REGION\""
done

# 9) Secrets Manager
aws secretsmanager list-secrets --region "$REGION" --output json \
| jq -r --arg p "$PREFIX" '.SecretList[]? | select(.Name | startswith($p)) | .ARN' \
| while read -r secret_arn; do
  [[ -z "$secret_arn" ]] && continue
  run "aws secretsmanager delete-secret --secret-id \"$secret_arn\" --force-delete-without-recovery --region \"$REGION\""
done

# 10) SSM Parameter Store
delete_ssm_parameters_by_path

# 11) DynamoDB
aws dynamodb list-tables --region "$REGION" --output json \
| jq -r '.TableNames[]?' | while read -r t; do
  starts_with_prefix "$t" || continue
  run "aws dynamodb delete-table --table-name \"$t\" --region \"$REGION\""
done

# 12) S3
aws s3api list-buckets --region "$REGION" --output json \
| jq -r '.Buckets[]?.Name' | while read -r b; do
  starts_with_prefix "$b" || continue
  empty_s3_bucket "$b"
  run "aws s3api delete-bucket --bucket \"$b\" --region \"$REGION\""
done

# 13) Cloud Map (namespace contiene prefijo)
aws servicediscovery list-namespaces --region "$REGION" --output json \
| jq -r --arg p "$PREFIX" '.Namespaces[]? | select(.Name | contains($p)) | .Id' \
| while read -r namespace_id; do
  [[ -z "$namespace_id" ]] && continue
  delete_cloud_map_namespace "$namespace_id"
done

# 14) VPC Endpoints
delete_vpc_endpoints

# 15) Network Interfaces (ENIs - ECS/Batch pueden crear estas)
echo "Limpiando Network Interfaces..."
local -a ENI_IDS=()
local _i eni_id
read_lines_into ENI_IDS < <(aws ec2 describe-network-interfaces --region "$REGION" --output json \
| jq -r --arg p "$PREFIX" '.NetworkInterfaces[]?
    | select(.Status == "available")
    | select(
        any(.TagSet[]?; (.Key == "Name" and (.Value | ascii_downcase | startswith($p)))
          or (.Key == "Project" and (.Value | ascii_downcase | contains($p))))
    )
    | .NetworkInterfaceId')

for ((_i = 0; _i < ${#ENI_IDS[@]}; _i++)); do
  eni_id="${ENI_IDS[_i]}"
  [[ -z "$eni_id" ]] && continue
  run "aws ec2 delete-network-interface --network-interface-id \"$eni_id\" --region \"$REGION\""
done

if [[ "$APPLY" -eq 1 && "${#ENI_IDS[@]}" -gt 0 ]]; then
  wait_for_network_interfaces_deleted "${ENI_IDS[@]}" || true
fi

# 16) Subnets
delete_subnets

# 17) Security Groups
delete_security_groups

# 18) CloudWatch Logs
aws logs describe-log-groups --region "$REGION" --output json \
| jq -r --arg p "$PREFIX" '.logGroups[]? | select(.logGroupName | contains($p)) | .logGroupName' \
| while read -r log_group; do
  [[ -z "$log_group" ]] && continue
  run "aws logs delete-log-group --log-group-name \"$log_group\" --region \"$REGION\""
done

# 19) IAM
collect_used_iam_references
echo "IAM candidates (Titvo match):"
aws iam list-roles --output json | jq -r --arg p "$PREFIX" '.Roles[]?
  | select((.RoleName | ascii_downcase | startswith($p)) or (.RoleName | ascii_downcase | contains("tvo")))
  | .RoleName' \
| while read -r r; do
  if iam_role_in_use "$r"; then
    record_skip "IAM role (en uso)" "$r"
    echo "[SKIP] IAM role en uso: $r"
    continue
  fi
  if [[ "$APPLY" -eq 1 ]]; then
    delete_iam_role "$r"
  else
    echo "[DRY-RUN] aws iam delete-role --role-name \"$r\" (usar --apply)"
  fi
done

aws iam list-policies --scope Local --output json | jq -r --arg p "$PREFIX" '.Policies[]?
  | select((.PolicyName | ascii_downcase | startswith($p)) or (.PolicyName | ascii_downcase | contains("tvo")))
  | .Arn' \
| while read -r policy_arn; do
  [[ -z "$policy_arn" ]] && continue
  if [[ "$APPLY" -eq 1 ]]; then
    delete_iam_policy "$policy_arn"
  else
    echo "[DRY-RUN] aws iam delete-policy --policy-arn \"$policy_arn\" (usar --apply)"
  fi
done

  echo "== Fin limpieza para prefijo '$PREFIX' =="
}

# Ejecutar limpieza con prefijos
# Si el usuario especifico PREFIX via variable de entorno, usar solo ese
# Si no, usar ambos prefijos por defecto: tvo y titvo

if [[ -n "${PREFIX:-}" ]]; then
  echo "Iniciando limpieza con prefijo '$PREFIX' (especificado por variable de entorno)..."
  echo ""
  run_cleanup_for_prefix "$PREFIX"
else
  echo "Iniciando limpieza con prefijos por defecto..."
  echo ""
  for prefix in "${DEFAULT_PREFIXES[@]}"; do
    run_cleanup_for_prefix "$prefix"
    echo ""
  done
fi

echo "== Limpieza completada =="
print_skipped_summary
echo "Tip: primero ejecuta sin --apply; luego con --apply"
