# JSON Migration Backup Information

## Backup Details
- Date: $(date '+%Y-%m-%d %H:%M:%S')
- Location: backups/json_migration_20251009_101939/
- Git Commit: $(git rev-parse HEAD)
- Git Branch: $(git branch --show-current)

## Backed Up Files
- members.json (30KB)
- official_talents.json (14KB)  
- official_profiles_ko.json (3.6KB)
- official_profiles_raw.json (161KB)
- official_profiles_ko/*.json (76 files)
- official_profiles_raw/*.json (76 files)

## Restoration Command
```bash
# Full restoration
cp -r backups/json_migration_20251009_101939/data/* internal/domain/data/

# Verify restoration
git diff internal/domain/data/
```

## Rollback Strategy
1. Data rollback: Restore this backup into `internal/domain/data/`
2. Application rollback: Revert commits if needed
3. Database rollback: Apply manual SQL changes (자동화된 스크립트 제거됨)

## Critical Notes
- This backup was created BEFORE JSON → PostgreSQL migration
- Contains all original data including inconsistencies
- Keep this backup until migration is verified in production
