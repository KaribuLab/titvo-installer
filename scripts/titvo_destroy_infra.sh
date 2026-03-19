#!/bin/bash
# Script para destruir infraestructura de TITVO en AWS
# Uso: ./titvo_destroy_infra.sh

set -e  # Salir si hay errores
set -o pipefail  # Fallar si algún comando en un pipe falla

# Colores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Función para logging
log_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

log_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

log_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

log_error() {
    echo -e "${RED}❌ $1${NC}"
}

# Función para confirmar acciones peligrosas
confirm() {
    local message=$1
    local default=${2:-n}
    local response

    if [ "$default" = "y" ]; then
        read -p "$message (Y/n): " response
        response=${response:-y}
    else
        read -p "$message (y/N): " response
        response=${response:-n}
    fi

    [[ "$response" =~ ^[Yy]$ ]]
}

disable_tvo_ecs_clusters() {
    log_info "Paso 2.5/5: Deshabilitando clusters ECS con prefijo 'tvo'"

    local clusters_response
    clusters_response=$(aws ecs list-clusters --region "$AWS_REGION" --output json 2>/dev/null || echo '{"clusterArns":[]}')

    mapfile -t CLUSTERS < <(echo "$clusters_response" | jq -r '.clusterArns[]?')
    if [ "${#CLUSTERS[@]}" -eq 0 ]; then
        log_info "No se encontraron clusters ECS"
        echo ""
        return
    fi

    local found_tvo=false
    for cluster_arn in "${CLUSTERS[@]}"; do
        cluster_name="${cluster_arn##*/}"
        if [[ ! "$cluster_name" =~ ^tvo ]]; then
            continue
        fi

        found_tvo=true
        log_info "Procesando cluster ECS: $cluster_name"

        mapfile -t SERVICES < <(aws ecs list-services \
            --cluster "$cluster_arn" \
            --region "$AWS_REGION" \
            --query 'serviceArns[]' \
            --output text 2>/dev/null | tr '\t' '\n' | sed '/^None$/d;/^$/d')

        for service_arn in "${SERVICES[@]}"; do
            service_name="${service_arn##*/}"
            log_info "Escalando servicio a 0: $service_name"
            aws ecs update-service \
                --cluster "$cluster_arn" \
                --service "$service_arn" \
                --desired-count 0 \
                --region "$AWS_REGION" > /dev/null 2>&1 || {
                log_warning "No se pudo escalar $service_name a 0"
                ERRORS=$((ERRORS + 1))
            }

            aws ecs wait services-stable \
                --cluster "$cluster_arn" \
                --services "$service_arn" \
                --region "$AWS_REGION" > /dev/null 2>&1 || {
                log_warning "Timeout esperando estabilidad de $service_name"
            }

            log_info "Eliminando servicio ECS: $service_name"
            aws ecs delete-service \
                --cluster "$cluster_arn" \
                --service "$service_arn" \
                --force \
                --region "$AWS_REGION" > /dev/null 2>&1 || {
                log_warning "No se pudo eliminar servicio $service_name"
                ERRORS=$((ERRORS + 1))
            }
        done

        mapfile -t TASKS < <(aws ecs list-tasks \
            --cluster "$cluster_arn" \
            --region "$AWS_REGION" \
            --query 'taskArns[]' \
            --output text 2>/dev/null | tr '\t' '\n' | sed '/^None$/d;/^$/d')

        for task_arn in "${TASKS[@]}"; do
            task_id="${task_arn##*/}"
            log_info "Deteniendo task ECS: $task_id"
            aws ecs stop-task \
                --cluster "$cluster_arn" \
                --task "$task_arn" \
                --reason "titvo destroy pre-drain" \
                --region "$AWS_REGION" > /dev/null 2>&1 || {
                log_warning "No se pudo detener task $task_id"
                ERRORS=$((ERRORS + 1))
            }
        done

        log_info "Intentando eliminar cluster ECS: $cluster_name"
        aws ecs delete-cluster \
            --cluster "$cluster_arn" \
            --region "$AWS_REGION" > /dev/null 2>&1 || {
            log_warning "No se pudo eliminar cluster $cluster_name (Terraform debería intentar destruirlo)"
        }
    done

    if [ "$found_tvo" = false ]; then
        log_info "No se encontraron clusters ECS con prefijo 'tvo'"
    else
        log_success "Finalizó pre-proceso ECS"
    fi
    echo ""
}

delete_cloudmap_namespace_services() {
    local namespace_name="internal.titvo.com"
    log_info "Paso 2.6/5: Eliminando servicios Cloud Map del namespace '$namespace_name'"

    local namespace_id
    namespace_id=$(aws servicediscovery list-namespaces \
        --region "$AWS_REGION" \
        --query "Namespaces[?Name=='$namespace_name'].Id | [0]" \
        --output text 2>/dev/null || echo "None")

    if [ -z "$namespace_id" ] || [ "$namespace_id" = "None" ]; then
        log_info "Namespace '$namespace_name' no encontrado"
        echo ""
        return
    fi

    mapfile -t SD_SERVICE_IDS < <(aws servicediscovery list-services \
        --region "$AWS_REGION" \
        --filters "Name=NAMESPACE_ID,Values=$namespace_id,Condition=EQ" \
        --query 'Services[].Id' \
        --output text 2>/dev/null | tr '\t' '\n' | sed '/^None$/d;/^$/d')

    if [ "${#SD_SERVICE_IDS[@]}" -eq 0 ]; then
        log_info "Namespace '$namespace_name' no tiene servicios asociados"
        echo ""
        return
    fi

    for service_id in "${SD_SERVICE_IDS[@]}"; do
        log_info "Eliminando Cloud Map service: $service_id"
        aws servicediscovery delete-service \
            --id "$service_id" \
            --region "$AWS_REGION" > /dev/null 2>&1 || {
            log_warning "No se pudo eliminar Cloud Map service $service_id"
            ERRORS=$((ERRORS + 1))
        }
    done

    log_success "Finalizó limpieza de servicios Cloud Map para '$namespace_name'"
    echo ""
}

# Banner inicial
echo "=================================================="
echo "  🗑️  TITVO Infrastructure Destroyer"
echo "=================================================="
echo ""

# Cargar variables de entorno
if [ ! -f .env ]; then
    log_error "Archivo .env no encontrado"
    exit 1
fi

source .env

# Verificar variables requeridas
REQUIRED_VARS=("AWS_STAGE" "AWS_REGION" "AWS_ACCOUNT_ID")
for var in "${REQUIRED_VARS[@]}"; do
    if [ -z "${!var}" ]; then
        log_error "Variable $var no está definida en .env"
        exit 1
    fi
done

log_info "Cuenta AWS: $AWS_ACCOUNT_ID"
log_info "Stage: $AWS_STAGE"
log_info "Región: $AWS_REGION"
echo ""

# Confirmar destrucción
log_warning "Esta acción destruirá TODA la infraestructura de TITVO"
if ! confirm "¿Estás seguro de continuar?"; then
    log_info "Operación cancelada"
    exit 0
fi

echo ""
log_info "Iniciando proceso de destrucción..."
echo ""

# Variables del proyecto
PROJECT_NAME="titvo-security-scan"
REPO_NAME="${PROJECT_NAME}-ecr-${AWS_STAGE}"
CLI_FILES_BUCKET_NAME="${PROJECT_NAME}-reports-${AWS_STAGE}"
INFRA_DIR="$HOME/.titvo/infra"
if [ -n "$AWS_ACCOUNT_ID" ];
then
    CLI_FILES_BUCKET_NAME="${CLI_FILES_BUCKET_NAME}-${AWS_ACCOUNT_ID}"
fi
# Contador de errores
ERRORS=0

# ========================================
# 1. Limpiar ECR Repository
# ========================================
log_info "Paso 1/5: Limpiando repositorio ECR"
if aws ecr describe-repositories \
    --repository-names "$REPO_NAME" \
    --region "$AWS_REGION" \
    --output text > /dev/null 2>&1; then

    log_info "Repositorio ECR '$REPO_NAME' encontrado"

    # Listar imágenes
    IMAGES=$(aws ecr list-images \
        --repository-name "$REPO_NAME" \
        --region "$AWS_REGION" \
        --query 'imageIds[*]' \
        --output json 2>/dev/null || echo '[]')

    IMAGE_COUNT=$(echo "$IMAGES" | jq 'length')

    if [ "$IMAGE_COUNT" -gt 0 ]; then
        log_info "Eliminando $IMAGE_COUNT imágenes..."
        echo "$IMAGES" | aws ecr batch-delete-image \
            --repository-name "$REPO_NAME" \
            --region "$AWS_REGION" \
            --image-ids file:///dev/stdin > /dev/null
        log_success "Imágenes eliminadas de ECR"
    else
        log_info "Repositorio ECR ya está vacío"
    fi
else
    log_warning "Repositorio ECR '$REPO_NAME' no encontrado (puede ya estar eliminado)"
fi
echo ""

# ========================================
# 2. Limpiar S3 Bucket
# ========================================
log_info "Paso 2/5: Limpiando bucket S3"
if aws s3api head-bucket --bucket "$CLI_FILES_BUCKET_NAME" 2>/dev/null; then
    log_info "Bucket '$CLI_FILES_BUCKET_NAME' encontrado"

    # Contar objetos
    OBJECT_COUNT=$(aws s3 ls "s3://${CLI_FILES_BUCKET_NAME}" --recursive 2>/dev/null | wc -l || echo "0")

    if [ "$OBJECT_COUNT" -gt 0 ]; then
        log_info "Eliminando $OBJECT_COUNT objetos del bucket..."

        # Verificar si tiene versionado
        VERSIONING=$(aws s3api get-bucket-versioning \
            --bucket "$CLI_FILES_BUCKET_NAME" \
            --query 'Status' \
            --output text 2>/dev/null || echo "None")

        if [ "$VERSIONING" = "Enabled" ] || [ "$VERSIONING" = "Suspended" ]; then
            log_warning "Bucket tiene versionado habilitado, eliminando todas las versiones..."

            # Crear archivo temporal
            TMP_FILE="/tmp/s3_versions_$$.json"

            # Eliminar versiones
            aws s3api list-object-versions \
                --bucket "$CLI_FILES_BUCKET_NAME" \
                --output json \
                --query '{Objects: Versions[].{Key:Key,VersionId:VersionId}}' \
                > "$TMP_FILE" 2>/dev/null || echo '{"Objects":null}' > "$TMP_FILE"

            if [ "$(jq -r '.Objects // [] | length' "$TMP_FILE")" -gt 0 ]; then
                aws s3api delete-objects \
                    --bucket "$CLI_FILES_BUCKET_NAME" \
                    --delete "file://$TMP_FILE" \
                    --quiet 2>/dev/null || true
            fi

            # Eliminar marcadores
            aws s3api list-object-versions \
                --bucket "$CLI_FILES_BUCKET_NAME" \
                --output json \
                --query '{Objects: DeleteMarkers[].{Key:Key,VersionId:VersionId}}' \
                > "$TMP_FILE" 2>/dev/null || echo '{"Objects":null}' > "$TMP_FILE"

            if [ "$(jq -r '.Objects // [] | length' "$TMP_FILE")" -gt 0 ]; then
                aws s3api delete-objects \
                    --bucket "$CLI_FILES_BUCKET_NAME" \
                    --delete "file://$TMP_FILE" \
                    --quiet 2>/dev/null || true
            fi

            rm -f "$TMP_FILE"
        else
            # Sin versionado
            aws s3 rm "s3://${CLI_FILES_BUCKET_NAME}" --recursive --quiet
        fi

        log_success "Bucket S3 limpiado"
    else
        log_info "Bucket S3 ya está vacío"
    fi
else
    log_warning "Bucket S3 '$CLI_FILES_BUCKET_NAME' no encontrado (puede ya estar eliminado)"
fi
echo ""

disable_tvo_ecs_clusters
delete_cloudmap_namespace_services

# ========================================
# 3. Destruir infraestructura Terraform/Terragrunt
# ========================================
log_info "Paso 3/5: Destruyendo infraestructura Terraform/Terragrunt"

# Verificar que existe el directorio de infraestructura
if [ ! -d "$INFRA_DIR" ]; then
    log_warning "Directorio de infraestructura '$INFRA_DIR' no encontrado"
    log_info "Saltando destrucción de Terraform"
else
    cd "$INFRA_DIR" || exit 1

    # Lista de módulos a destruir en orden
    MODULES=(
        "titvo-auth-setup-aws/aws"
        "titvo-agent-aws/aws"
        "titvo-mcp-gateway-aws/aws"
        "titvo-task-cli-files-aws/aws"
        "titvo-task-status-aws/aws"
        "titvo-task-trigger-aws/aws"
        "titvo-security-scan-infra-aws/prod/us-east-1"
    )

    if [ -d titvo-security-scan-infra-aws/prod/us-east-1/ssm/parameter/lookup ]; then
        cd titvo-security-scan-infra-aws/prod/us-east-1/ssm/parameter/lookup
        terragrunt apply
        cd -
    fi

    if [ -d titvo-security-scan-infra-aws/prod/us-east-1/ssm/parameter/upsert ]; then
        rm -rf titvo-security-scan-infra-aws/prod/us-east-1/ssm/parameter/upsert
    fi

    TOTAL_MODULES=${#MODULES[@]}
    CURRENT=0

    for MODULE in "${MODULES[@]}"; do
        CURRENT=$((CURRENT + 1))
        MODULE_NAME=$(echo "$MODULE" | cut -d'/' -f1)
        MODULE_PATH="$MODULE"

        echo ""
        echo "----------------------------------------"
        log_info "[$CURRENT/$TOTAL_MODULES] Procesando módulo: $MODULE_NAME"
        echo "----------------------------------------"

        if [ -d "$MODULE_PATH" ]; then
            cd "$MODULE_PATH" || continue

            log_info "Inicializando Terragrunt..."
            if terragrunt run-all init -reconfigure --terragrunt-non-interactive 2>&1 | grep -v "terraform init"; then
                log_success "Inicialización completada"

                log_info "Destruyendo recursos..."
                if terragrunt run-all destroy -auto-approve --terragrunt-non-interactive; then
                    log_success "Módulo $MODULE_NAME destruido"
                else
                    log_error "Error al destruir módulo $MODULE_NAME"
                    ERRORS=$((ERRORS + 1))
                fi
            else
                log_error "Error al inicializar módulo $MODULE_NAME"
                ERRORS=$((ERRORS + 1))
            fi

            cd "$INFRA_DIR" || exit 1
        else
            log_warning "Módulo $MODULE_NAME no encontrado (ya eliminado o no instalado)"
        fi
    done
fi

# ========================================
# 4. Limpiar parámetros SSM
# ========================================
log_info "Paso 4/5: Eliminando parámetros SSM de infraestructura"
SSM_BASE_PATH="/tvo/security-scan/prod/infra"
NEXT_TOKEN=""
DELETED_PARAMS=0

while true; do
    if [ -n "$NEXT_TOKEN" ]; then
        SSM_RESPONSE=$(aws ssm get-parameters-by-path \
            --path "$SSM_BASE_PATH" \
            --recursive \
            --with-decryption \
            --region "$AWS_REGION" \
            --max-results 10 \
            --next-token "$NEXT_TOKEN" \
            --output json 2>/dev/null) || {
            log_error "Error al listar parámetros SSM en '$SSM_BASE_PATH'"
            ERRORS=$((ERRORS + 1))
            break
        }
    else
        SSM_RESPONSE=$(aws ssm get-parameters-by-path \
            --path "$SSM_BASE_PATH" \
            --recursive \
            --with-decryption \
            --region "$AWS_REGION" \
            --max-results 10 \
            --output json 2>/dev/null) || {
            log_error "Error al listar parámetros SSM en '$SSM_BASE_PATH'"
            ERRORS=$((ERRORS + 1))
            break
        }
    fi

    PARAM_NAMES=$(echo "$SSM_RESPONSE" | jq -r '.Parameters[].Name // empty')
    if [ -n "$PARAM_NAMES" ]; then
        mapfile -t PARAM_ARRAY <<< "$PARAM_NAMES"
        if aws ssm delete-parameters --region "$AWS_REGION" --names "${PARAM_ARRAY[@]}" > /dev/null 2>&1; then
            DELETED_PARAMS=$((DELETED_PARAMS + ${#PARAM_ARRAY[@]}))
        else
            log_error "Error al eliminar parámetros SSM en '$SSM_BASE_PATH'"
            ERRORS=$((ERRORS + 1))
        fi
    fi

    NEXT_TOKEN=$(echo "$SSM_RESPONSE" | jq -r '.NextToken // empty')
    if [ -z "$NEXT_TOKEN" ]; then
        break
    fi
done

if [ "$DELETED_PARAMS" -gt 0 ]; then
    log_success "Parámetros SSM eliminados: $DELETED_PARAMS"
else
    log_info "No se encontraron parámetros SSM en '$SSM_BASE_PATH'"
fi

# ========================================
# 5. Limpiar states residuales (S3 + DynamoDB locks)
# ========================================
log_info "Paso 5/5: Eliminando states residuales de S3 y locks de DynamoDB"

STATE_BUCKETS=(
    "tvo-installer-ecr-publisher-${AWS_REGION}-${AWS_ACCOUNT_ID}"
    "tvo-agent-${AWS_REGION}-${AWS_ACCOUNT_ID}"
)
STATE_KEY="aws/ssm/upsert/terraform.tfstate"

for bucket in "${STATE_BUCKETS[@]}"; do
    log_info "Eliminando state: s3://${bucket}/${STATE_KEY}"
    aws s3api delete-object \
        --bucket "$bucket" \
        --key "$STATE_KEY" \
        --region "$AWS_REGION" > /dev/null 2>&1 || {
        log_warning "No se pudo eliminar s3://${bucket}/${STATE_KEY} (puede no existir)"
        ERRORS=$((ERRORS + 1))
    }
done

LOCK_TABLE_1="tvo-agent-${AWS_REGION}-${AWS_ACCOUNT_ID}-tfstate-lock"
LOCK_ID_1="tvo-agent-${AWS_REGION}-${AWS_ACCOUNT_ID}/aws/ssm/upsert/terraform.tfstate-md5"
log_info "Eliminando lock en tabla '$LOCK_TABLE_1'"
aws dynamodb delete-item \
    --table-name "$LOCK_TABLE_1" \
    --region "$AWS_REGION" \
    --key "{\"LockID\":{\"S\":\"$LOCK_ID_1\"}}" > /dev/null 2>&1 || {
    log_warning "No se pudo eliminar item '$LOCK_ID_1' en '$LOCK_TABLE_1' (puede no existir)"
    ERRORS=$((ERRORS + 1))
}

LOCK_TABLE_2="tvo-installer-ecr-publisher-${AWS_REGION}-${AWS_ACCOUNT_ID}-tfstate-lock"
LOCK_ID_2="tvo-installer-ecr-publisher-${AWS_REGION}-${AWS_ACCOUNT_ID}/aws/ssm/upsert/terraform.tfstate-md5"
log_info "Eliminando lock en tabla '$LOCK_TABLE_2'"
aws dynamodb delete-item \
    --table-name "$LOCK_TABLE_2" \
    --region "$AWS_REGION" \
    --key "{\"LockID\":{\"S\":\"$LOCK_ID_2\"}}" > /dev/null 2>&1 || {
    log_warning "No se pudo eliminar item '$LOCK_ID_2' en '$LOCK_TABLE_2' (puede no existir)"
    ERRORS=$((ERRORS + 1))
}

echo ""

# ========================================
# Resumen final
# ========================================
echo ""
echo "=================================================="
if [ $ERRORS -eq 0 ]; then
    log_success "Proceso completado exitosamente"
else
    log_warning "Proceso completado con $ERRORS errores"
    log_info "Revisa los logs arriba para más detalles"
fi
echo "=================================================="
