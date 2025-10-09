#!/bin/bash
# JSON to PostgreSQL Migration Script
# Usage: ./scripts/migrate-to-db.sh [--dry-run] [--force]

set -e

# Colors
RED='\033[0:31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5433}"
DB_USER="${DB_USER:-holo_user}"
DB_PASS="${DB_PASS:-holo_password}"
DB_NAME="${DB_NAME:-holo_oshi_db}"

DRY_RUN=false
FORCE=false

# Parse arguments
for arg in "$@"; do
    case $arg in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --force)
            FORCE=true
            shift
            ;;
        *)
            ;;
    esac
done

echo "======================================="
echo " JSON to PostgreSQL Migration"
echo "======================================="
echo ""

# Step 1: Backup check
if [ ! -d "backups" ] || [ -z "$(ls -A backups 2>/dev/null)" ]; then
    echo -e "${RED}✗ No backups found!${NC}"
    echo "Please create a backup first."
    exit 1
fi

LATEST_BACKUP=$(ls -1dt backups/json_migration_* 2>/dev/null | head -1)
if [ -n "$LATEST_BACKUP" ]; then
    echo -e "${GREEN}✓ Backup found: $LATEST_BACKUP${NC}"
else
    echo -e "${RED}✗ No backup found${NC}"
    exit 1
fi

# Step 2: Check database connection
echo ""
echo "Checking database connection..."
if PGPASSWORD=$DB_PASS psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "SELECT 1" > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Database connection OK${NC}"
else
    echo -e "${RED}✗ Cannot connect to database${NC}"
    echo "Host: $DB_HOST:$DB_PORT"
    echo "User: $DB_USER"
    echo "Database: $DB_NAME"
    exit 1
fi

# Step 3: Check if tables already exist
TABLES_EXIST=$(PGPASSWORD=$DB_PASS psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -tAc "SELECT COUNT(*) FROM information_schema.tables WHERE table_name IN ('members', 'member_profiles')")

if [ "$TABLES_EXIST" -gt 0 ]; then
    if [ "$FORCE" = false ]; then
        echo -e "${YELLOW}⚠ Tables already exist!${NC}"
        echo "Use --force to drop and recreate tables"
        exit 1
    else
        echo -e "${YELLOW}⚠ Dropping existing tables (--force)${NC}"
        PGPASSWORD=$DB_PASS psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -f migrations/002_members_schema_down.sql
    fi
fi

# Step 4: Create schema
echo ""
echo "Creating database schema..."
if PGPASSWORD=$DB_PASS psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -f migrations/002_members_schema.sql > /dev/null; then
    echo -e "${GREEN}✓ Schema created${NC}"
else
    echo -e "${RED}✗ Failed to create schema${NC}"
    exit 1
fi

# Step 5: Build migration tool
echo ""
echo "Building migration tool..."
if go build -o bin/migrate_json_to_db cmd/migrate_json_to_db/main.go; then
    echo -e "${GREEN}✓ Migration tool built${NC}"
else
    echo -e "${RED}✗ Failed to build migration tool${NC}"
    exit 1
fi

# Step 6: Run migration
echo ""
if [ "$DRY_RUN" = true ]; then
    echo "Running migration (DRY RUN mode)..."
    ./bin/migrate_json_to_db \
        --dry-run \
        --db-host=$DB_HOST \
        --db-port=$DB_PORT \
        --db-user=$DB_USER \
        --db-pass=$DB_PASS \
        --db-name=$DB_NAME \
        --verbose
else
    echo "Running migration..."
    ./bin/migrate_json_to_db \
        --db-host=$DB_HOST \
        --db-port=$DB_PORT \
        --db-user=$DB_USER \
        --db-pass=$DB_PASS \
        --db-name=$DB_NAME \
        --verbose
fi

if [ $? -eq 0 ]; then
    echo ""
    echo -e "${GREEN}✓ Migration completed successfully${NC}"

    if [ "$DRY_RUN" = false ]; then
        # Step 7: Verification
        echo ""
        echo "Verification..."
        MEMBER_COUNT=$(PGPASSWORD=$DB_PASS psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -tAc "SELECT COUNT(*) FROM members")
        PROFILE_COUNT=$(PGPASSWORD=$DB_PASS psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -tAc "SELECT COUNT(*) FROM member_profiles")

        echo "Members inserted: $MEMBER_COUNT"
        echo "Profiles inserted: $PROFILE_COUNT"

        echo ""
        echo -e "${GREEN}✓ All verification checks passed${NC}"
    fi
else
    echo -e "${RED}✗ Migration failed${NC}"
    exit 1
fi

echo ""
echo "======================================="
echo "Migration process completed"
echo "======================================="

if [ "$DRY_RUN" = false ]; then
    echo ""
    echo "Next steps:"
    echo "1. Review the data in the database"
    echo "2. Update application code to use database"
    echo "3. Test thoroughly before removing JSON files"
    echo ""
    echo "Rollback command:"
    echo "  PGPASSWORD=$DB_PASS psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -f migrations/002_members_schema_down.sql"
fi
