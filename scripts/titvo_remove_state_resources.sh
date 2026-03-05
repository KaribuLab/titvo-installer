#!/bin/bash
# Script para borrar buckets de S3 y tablas de DynamoDB con prefijos específicos
# Uso: ./titvo_remove_state_resources.sh

source .env
set -e  # Salir si hay errores

# Definir múltiples prefijos
PREFIXES=(
    "tvo",
    "titvo"
)

# PRIORIZAR AWS_REGION sobre otras variables
if [ -n "$AWS_REGION" ]; then
    REGION="$AWS_REGION"
elif [ -n "$AWS_DEFAULT_REGION" ]; then
    REGION="$AWS_DEFAULT_REGION"
elif [ -n "$REGION" ]; then
    REGION="$REGION"
else
    REGION="us-east-1"
fi

# DEBUG: Mostrar qué región se está usando
echo "DEBUG: Variables de región disponibles:"
echo "  AWS_REGION=$AWS_REGION"
echo "  AWS_DEFAULT_REGION=$AWS_DEFAULT_REGION"
echo "  REGION (del .env)=$REGION"
echo "  REGION (final a usar)=$REGION"
echo ""

# Colores
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

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

echo "=================================================="
echo "  🗑️  AWS Resource Cleanup Tool"
echo "=================================================="
echo ""
log_info "Cuenta AWS: $AWS_ACCOUNT_ID"
log_info "Región a usar: $REGION"
echo ""
log_info "Prefijos configurados:"
for PREFIX in "${PREFIXES[@]}"; do
    echo "  - $PREFIX"
done
echo ""

# ========================================
# Buscar recursos
# ========================================
log_info "🔍 Buscando recursos..."

# Buscar buckets S3
ALL_BUCKETS=""
for PREFIX in "${PREFIXES[@]}"; do
    FOUND=$(aws s3api list-buckets --query "Buckets[?starts_with(Name, '$PREFIX')].Name" --output text 2>/dev/null || echo "")
    if [ -n "$FOUND" ]; then
        ALL_BUCKETS="$ALL_BUCKETS $FOUND"
    fi
done
ALL_BUCKETS=$(echo "$ALL_BUCKETS" | xargs)

# Buscar tablas DynamoDB
echo "DEBUG: Buscando tablas en región: $REGION"
ALL_TABLES=""
for PREFIX in "${PREFIXES[@]}"; do
    echo "DEBUG: Ejecutando: aws dynamodb list-tables --region $REGION"
    FOUND=$(aws dynamodb list-tables --region "$REGION" --output json 2>/dev/null | \
        jq -r ".TableNames[] | select(startswith(\"$PREFIX\"))" | tr '\n' ' ')
    if [ -n "$FOUND" ]; then
        echo "DEBUG: Tablas encontradas con prefijo $PREFIX: $FOUND"
        ALL_TABLES="$ALL_TABLES $FOUND"
    fi
done
ALL_TABLES=$(echo "$ALL_TABLES" | xargs)

# Mostrar resumen
echo ""
echo "📊 Recursos encontrados:"
echo ""

if [ -n "$ALL_BUCKETS" ]; then
    BUCKET_COUNT=$(echo "$ALL_BUCKETS" | wc -w)
    log_info "S3 Buckets ($BUCKET_COUNT):"
    echo "$ALL_BUCKETS" | tr ' ' '\n' | sed 's/^/  - /'
else
    log_info "S3 Buckets: Ninguno encontrado"
fi

echo ""

if [ -n "$ALL_TABLES" ]; then
    TABLE_COUNT=$(echo "$ALL_TABLES" | wc -w)
    log_info "DynamoDB Tables ($TABLE_COUNT):"
    echo "$ALL_TABLES" | tr ' ' '\n' | sed 's/^/  - /'
else
    log_info "DynamoDB Tables: Ninguna encontrada"
fi

# Verificar si hay recursos para eliminar
if [ -z "$ALL_BUCKETS" ] && [ -z "$ALL_TABLES" ]; then
    echo ""
    log_success "No se encontraron recursos con los prefijos especificados"
    exit 0
fi

# Confirmar antes de proceder
echo ""
read -p "⚠️  ¿Estás seguro de que quieres borrar TODOS estos recursos? (y/N): " -r
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    log_warning "Operación cancelada"
    exit 0
fi

echo ""
echo "🗑️  Iniciando borrado de recursos..."

# ========================================
# Función para limpiar bucket S3
# ========================================
clean_bucket_completely() {
    local bucket=$1

    log_info "Verificando estado del bucket..."

    # Verificar versionado
    local versioning=$(aws s3api get-bucket-versioning --bucket "$bucket" --query 'Status' --output text 2>/dev/null || echo "None")
    log_info "Estado de versionado: $versioning"

    # Listar TODOS los objetos (versiones, marcadores, etc)
    local all_versions=$(aws s3api list-object-versions --bucket "$bucket" --output json 2>/dev/null || echo '{}')

    # Contar versiones
    local version_count=$(echo "$all_versions" | jq -r '.Versions // [] | length')
    local marker_count=$(echo "$all_versions" | jq -r '.DeleteMarkers // [] | length')

    log_info "Versiones: $version_count | Marcadores: $marker_count"

    # Si hay versiones o marcadores, eliminarlos
    if [ "$version_count" -gt 0 ] || [ "$marker_count" -gt 0 ]; then
        log_info "Limpiando bucket con versionado..."

        # Crear archivo temporal
        local tmp_dir="/tmp/s3_cleanup_$$"
        mkdir -p "$tmp_dir"

        # Guardar todo el contenido
        echo "$all_versions" > "$tmp_dir/all_versions.json"

        # Eliminar versiones en lotes de 1000 (límite de AWS)
        if [ "$version_count" -gt 0 ]; then
            log_info "Eliminando $version_count versiones..."

            local batch_size=1000
            local processed=0

            while [ $processed -lt $version_count ]; do
                # Crear JSON para este lote
                jq -c "{Objects: [.Versions[$processed:$((processed + batch_size))] | .[] | {Key: .Key, VersionId: .VersionId}]}" \
                    "$tmp_dir/all_versions.json" > "$tmp_dir/batch_versions.json"

                local batch_count=$(jq '.Objects | length' "$tmp_dir/batch_versions.json")

                if [ "$batch_count" -gt 0 ]; then
                    # Eliminar este lote
                    aws s3api delete-objects \
                        --bucket "$bucket" \
                        --delete "file://$tmp_dir/batch_versions.json" \
                        --output json 2>&1 | jq -r '.Deleted[]? | "    ✓ \(.Key)"' | head -3

                    log_info "  Procesado lote: $batch_count objetos"
                fi

                processed=$((processed + batch_size))
            done
        fi

        # Eliminar marcadores de eliminación
        if [ "$marker_count" -gt 0 ]; then
            log_info "Eliminando $marker_count marcadores..."

            jq -c '{Objects: [.DeleteMarkers[] | {Key: .Key, VersionId: .VersionId}]}' \
                "$tmp_dir/all_versions.json" > "$tmp_dir/markers.json"

            aws s3api delete-objects \
                --bucket "$bucket" \
                --delete "file://$tmp_dir/markers.json" \
                --quiet 2>&1
        fi

        # Limpiar temporales
        rm -rf "$tmp_dir"
    fi

    # Verificar objetos sin versión (método tradicional)
    local normal_objects=$(aws s3 ls s3://$bucket --recursive 2>/dev/null || echo "")

    if [ -n "$normal_objects" ]; then
        local obj_count=$(echo "$normal_objects" | wc -l)
        log_info "Eliminando $obj_count objetos normales..."
        aws s3 rm s3://$bucket --recursive --quiet
    fi

    # Verificación final
    local final_check=$(aws s3api list-object-versions --bucket "$bucket" --output json 2>/dev/null || echo '{}')
    local final_versions=$(echo "$final_check" | jq -r '.Versions // [] | length')
    local final_markers=$(echo "$final_check" | jq -r '.DeleteMarkers // [] | length')

    if [ "$final_versions" -eq 0 ] && [ "$final_markers" -eq 0 ]; then
        log_success "Bucket completamente vacío"
        return 0
    else
        log_warning "Aún quedan objetos: $final_versions versiones, $final_markers marcadores"
        return 1
    fi
}

# ========================================
# Función para eliminar tabla DynamoDB
# ========================================
delete_dynamodb_table() {
    local table=$1
    local region=$2

    echo "DEBUG: delete_dynamodb_table llamada con:"
    echo "  table=$table"
    echo "  region=$region"

    log_info "Verificando tabla en región: $region"

    # Verificar si la tabla existe
    echo "DEBUG: Ejecutando: aws dynamodb describe-table --table-name $table --region $region"
    if ! aws dynamodb describe-table \
        --table-name "$table" \
        --region "$region" \
        --output json > /dev/null 2>&1; then
        log_warning "Tabla '$table' no existe o no es accesible en región $region"

        # DEBUG: Intentar ver en qué región está
        echo "DEBUG: Buscando tabla en otras regiones..."
        for test_region in us-east-1 us-west-2 eu-west-1; do
            if aws dynamodb describe-table --table-name "$table" --region "$test_region" > /dev/null 2>&1; then
                echo "DEBUG: ¡Tabla encontrada en región: $test_region!"
            fi
        done

        return 1
    fi

    # Obtener información de la tabla
    local table_info=$(aws dynamodb describe-table \
        --table-name "$table" \
        --region "$region" \
        --output json 2>/dev/null)

    local item_count=$(echo "$table_info" | jq -r '.Table.ItemCount // 0')
    local table_size=$(echo "$table_info" | jq -r '.Table.TableSizeBytes // 0')
    local table_status=$(echo "$table_info" | jq -r '.Table.TableStatus')

    # Convertir tamaño a MB
    local size_mb=$(echo "scale=2; $table_size / 1024 / 1024" | bc 2>/dev/null || echo "0")

    log_info "Estado: $table_status | Items: $item_count | Tamaño: ${size_mb} MB"

    # Verificar si tiene Point-in-Time Recovery habilitado
    local pitr=$(aws dynamodb describe-continuous-backups \
        --table-name "$table" \
        --region "$region" \
        --query 'ContinuousBackupsDescription.PointInTimeRecoveryDescription.PointInTimeRecoveryStatus' \
        --output text 2>/dev/null || echo "DISABLED")

    if [ "$pitr" = "ENABLED" ]; then
        log_info "Point-in-Time Recovery está habilitado"
    fi

    # Verificar si tiene streams habilitado
    local stream_arn=$(echo "$table_info" | jq -r '.Table.LatestStreamArn // empty')
    if [ -n "$stream_arn" ]; then
        log_info "Tabla tiene DynamoDB Streams habilitado"
    fi

    # Eliminar la tabla
    log_info "Eliminando tabla DynamoDB..."
    echo "DEBUG: Ejecutando: aws dynamodb delete-table --table-name $table --region $region"

    if aws dynamodb delete-table \
        --table-name "$table" \
        --region "$region" \
        --output json > /dev/null 2>&1; then

        log_info "Esperando a que la tabla sea eliminada..."

        # Esperar hasta que la tabla sea eliminada (máximo 5 minutos)
        local wait_count=0
        local max_wait=60  # 60 intentos de 5 segundos = 5 minutos

        while [ $wait_count -lt $max_wait ]; do
            if ! aws dynamodb describe-table \
                --table-name "$table" \
                --region "$region" > /dev/null 2>&1; then
                log_success "Tabla eliminada exitosamente"
                return 0
            fi

            sleep 5
            wait_count=$((wait_count + 1))

            if [ $((wait_count % 6)) -eq 0 ]; then
                log_info "  Aún esperando... (${wait_count}0 segundos)"
            fi
        done

        log_warning "Timeout esperando eliminación (puede completarse en background)"
        return 0
    else
        log_error "Error al eliminar tabla"
        echo "DEBUG: Mostrando último error de AWS CLI:"
        aws dynamodb delete-table --table-name "$table" --region "$region" 2>&1
        return 1
    fi
}

# ========================================
# Procesar S3 Buckets
# ========================================
if [ -n "$ALL_BUCKETS" ]; then
    echo ""
    echo "=================================================="
    echo "  📦 Procesando S3 Buckets"
    echo "=================================================="

    TOTAL=$(echo "$ALL_BUCKETS" | wc -w)
    CURRENT=0

    for BUCKET in $ALL_BUCKETS; do
        CURRENT=$((CURRENT + 1))
        echo ""
        echo "[$CURRENT/$TOTAL] 🔄 Bucket: $BUCKET"
        echo "--------------------------------------------------"

        # Verificar si el bucket existe
        if ! aws s3api head-bucket --bucket "$BUCKET" 2>/dev/null; then
            log_warning "Bucket no accesible o no existe"
            continue
        fi

        # Limpiar el bucket
        if clean_bucket_completely "$BUCKET"; then
            log_info "Eliminando bucket..."

            if aws s3api delete-bucket --bucket "$BUCKET" 2>&1; then
                log_success "Bucket eliminado"
            else
                log_error "Error al eliminar bucket"
            fi
        else
            log_error "No se pudo vaciar completamente el bucket"
        fi
    done
fi

# ========================================
# Procesar DynamoDB Tables
# ========================================
if [ -n "$ALL_TABLES" ]; then
    echo ""
    echo "=================================================="
    echo "  🗄️  Procesando DynamoDB Tables"
    echo "=================================================="

    TOTAL=$(echo "$ALL_TABLES" | wc -w)
    CURRENT=0

    for TABLE in $ALL_TABLES; do
        CURRENT=$((CURRENT + 1))
        echo ""
        echo "[$CURRENT/$TOTAL] 🔄 Tabla: $TABLE"
        echo "--------------------------------------------------"

        # PASAR EXPLÍCITAMENTE LA REGIÓN
        echo "DEBUG: Llamando a delete_dynamodb_table con región: $REGION"
        delete_dynamodb_table "$TABLE" "$REGION"
    done
fi

# ========================================
# Resumen final
# ========================================
echo ""
echo "=================================================="
log_success "Proceso completado!"
echo "=================================================="
echo ""
log_info "Resumen:"
if [ -n "$ALL_BUCKETS" ]; then
    echo "  📦 S3 Buckets procesados: $(echo "$ALL_BUCKETS" | wc -w)"
fi
if [ -n "$ALL_TABLES" ]; then
    echo "  🗄️  DynamoDB Tables procesadas: $(echo "$ALL_TABLES" | wc -w)"
fi
echo ""
