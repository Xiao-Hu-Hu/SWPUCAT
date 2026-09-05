#!/bin/bash
set -e

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BACKUP_DIR="${PROJECT_DIR}/backups"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p "${BACKUP_DIR}"

echo "[1/3] 备份 PostgreSQL 数据库..."
docker exec swpucat-db pg_dump -U csa -d csa_db > "${BACKUP_DIR}/swpucat_db_${TIMESTAMP}.sql"

echo "[2/3] 备份上传文件..."
docker run --rm \
  -v swpucat_uploads_data:/data \
  -v "${BACKUP_DIR}:/backup" \
  alpine tar czf "/backup/swpucat_uploads_${TIMESTAMP}.tar.gz" -C /data .

echo "[3/3] 备份 .env 配置文件..."
if [[ -f "${PROJECT_DIR}/.env" ]]; then
  cp "${PROJECT_DIR}/.env" "${BACKUP_DIR}/swpucat_env_${TIMESTAMP}"
fi

echo ""
echo "备份完成，位于: ${BACKUP_DIR}"
ls -lh "${BACKUP_DIR}"/*"${TIMESTAMP}"*
