#!/bin/bash
set -e

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

DB_FILE=$1
UPLOADS_FILE=$2

if [[ -z "$DB_FILE" || -z "$UPLOADS_FILE" ]]; then
  echo "用法: $0 <db_backup.sql> <uploads_backup.tar.gz>"
  echo "示例: $0 backups/swpucat_db_20260821_120000.sql backups/swpucat_uploads_20260821_120000.tar.gz"
  exit 1
fi

DB_FILE="$(realpath "$DB_FILE")"
UPLOADS_FILE="$(realpath "$UPLOADS_FILE")"

echo "[1/4] 启动 PostgreSQL..."
cd "${PROJECT_DIR}/deployments"
docker compose up -d postgres

for i in {1..10}; do
  if docker exec swpucat-db pg_isready -U csa -d csa_db > /dev/null 2>&1; then
    break
  fi
  echo "等待 PostgreSQL 就绪... ($i/10)"
  sleep 2
done

echo "[2/4] 重建数据库..."
docker exec swpucat-db psql -U csa -d postgres -c "DROP DATABASE IF EXISTS csa_db;"
docker exec swpucat-db psql -U csa -d postgres -c "CREATE DATABASE csa_db OWNER csa;"

echo "[3/4] 导入数据库..."
docker exec -i swpucat-db psql -U csa -d csa_db < "$DB_FILE"

echo "[4/4] 恢复上传文件..."
docker run --rm \
  -v "${UPLOADS_FILE}:/backup.tar.gz:ro" \
  -v swpucat_uploads_data:/data \
  alpine sh -c "rm -rf /data/* && tar xzf /backup.tar.gz -C /data"

echo ""
echo "[完成] 启动所有服务..."
docker compose up -d

echo ""
echo "恢复完成"
