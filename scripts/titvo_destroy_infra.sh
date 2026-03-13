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
REQUIRED_VARS=("AWS_STAGE" "AWS_REGION")
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
log_info "Paso 1/3: Limpiando repositorio ECR"
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
log_info "Paso 2/3: Limpiando bucket S3"
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

# ========================================
# 3. Destruir infraestructura Terraform/Terragrunt
# ========================================
log_info "Paso 3/3: Destruyendo infraestructura Terraform/Terragrunt"

# Verificar que existe el directorio de infraestructura
if [ ! -d "$INFRA_DIR" ]; then
    log_warning "Directorio de infraestructura '$INFRA_DIR' no encontrado"
    log_info "Saltando destrucción de Terraform"
else
    cd "$INFRA_DIR" || exit 1

    # Lista de módulos a destruir en orden
    MODULES=(
        "titvo-auth-setup-aws/aws"
        "titvo-security-scan/aws"
        "titvo-task-cli-files-aws/aws"
        "titvo-task-status-aws/aws"
        "titvo-task-trigger-aws/aws"
        "titvo-security-scan-infra-aws/prod/us-east-1"
    )

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
