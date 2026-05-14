-- Validation queries for the Entitlement Exclusion Groups E2E test.
--
-- These run against the C1 *postgres* mirror (NOT against the baton-sql
-- fixture postgres). The C1 mirror is populated by the db-stream consumer
-- from the DynamoDB writes that uplift makes after a connector sync.
--
-- Three tables are involved:
--   pb_app_entitlement_exclusion_group_c1_m_655d50c4  -- AppEntitlementExclusionGroup
--   pb_app_entitlement_exclusion_group_entr_0e87042e  -- AppEntitlementExclusionGroupEntry
--   pb_app_entitlement_exclusion_group_entr_9fa75386  -- AppEntitlementExclusionGroupEntryLookup
--
-- pgdb generates `pb$`-prefixed columns. Identifiers are double-quoted because
-- the `$` is a PostgreSQL extension, not standard SQL.
--
-- Usage (from a c1 dev-shell pod or any psql with reach to the C1 postgres):
--   psql -h <c1-pg-host> -U <user> -d <db> \
--     -v tenant_id='<tenant-id>' \
--     -v app_id='<app-id>' \
--     -v connector_id='<connector-id>' \
--     -f validate.sql
--
-- Or interactively:
--   \set tenant_id   '<tenant-id>'
--   \set app_id      '<app-id>'
--   \set connector_id '<connector-id>'
--   \i validate.sql

\echo '==> bound vars: tenant_id =' :tenant_id ', app_id =' :app_id ', connector_id =' :connector_id

-- -----------------------------------------------------------------------------
-- 1. Exclusion groups for this tenant + app + connector.
--
-- Expected with the supplied fixture and the FF enabled: 6 rows.
--   database-main-role-tier
--   database-analytics-role-tier
--   database-billing-role-tier
--   database-billing-clearance-tier
--   database-reporting-role-tier
--   database-audit-role-tier
-- -----------------------------------------------------------------------------
\echo
\echo '==> [1] Exclusion groups (expect 6 rows, none deleted)'
SELECT
  "pb$id"                            AS id,
  "pb$connector_exclusion_group_id"  AS connector_exclusion_group_id,
  "pb$app_resource_type_id"          AS app_resource_type_id,
  "pb$app_resource_id"               AS app_resource_id,
  "pb$created_at"                    AS created_at,
  "pb$updated_at"                    AS updated_at,
  "pb$deleted_at"                    AS deleted_at
FROM pb_app_entitlement_exclusion_group_c1_m_655d50c4
WHERE "pb$tenant_id"   = '3CmpaAkVpaoiiwRv4RFu3wdjZLX'
  AND "pb$app_id"      = '3Dj1QeX2utQSCs5L5SvnhwqGHql'
  AND "pb$connector_id" = '3Dj1Qa2Qu4L5mikHYzYCKs0A8ty'
  AND "pb$deleted_at" IS NULL
ORDER BY "pb$connector_exclusion_group_id";

-- -----------------------------------------------------------------------------
-- 2. Entries per group.
--
-- Expected entry counts:
--   database-main-role-tier         -> 4   (reader/writer/admin/owner)
--   database-analytics-role-tier    -> 4   (viewer/analyst/engineer/admin)
--   database-billing-role-tier      -> 4   (viewer/processor/reviewer/admin)
--   database-billing-clearance-tier -> 3   (restricted/standard/elevated)
--   database-reporting-role-tier    -> 4   (consumer/author/maintainer/admin)
--   database-audit-role-tier        -> 3   (viewer/auditor/admin)
--   TOTAL                           -> 22
--
-- And exactly one row per group must have is_default = true.
-- -----------------------------------------------------------------------------
\echo
\echo '==> [2] Entries per group (expect totals above, exactly 1 default per group)'
SELECT
  g."pb$connector_exclusion_group_id" AS connector_group,
  g."pb$id"                           AS group_id,
  COUNT(*)                            AS entry_count,
  COUNT(*) FILTER (WHERE e."pb$is_default") AS default_count,
  MIN(e."pb$order")                   AS min_order,
  MAX(e."pb$order")                   AS max_order
FROM pb_app_entitlement_exclusion_group_c1_m_655d50c4 g
JOIN pb_app_entitlement_exclusion_group_entr_0e87042e e
  ON  e."pb$tenant_id"         = g."pb$tenant_id"
  AND e."pb$app_id"            = g."pb$app_id"
  AND e."pb$exclusion_group_id" = g."pb$id"
WHERE g."pb$tenant_id"   = '3CmpaAkVpaoiiwRv4RFu3wdjZLX'
  AND g."pb$app_id"      = '3Dj1QeX2utQSCs5L5SvnhwqGHql'
  AND g."pb$connector_id" = '3Dj1Qa2Qu4L5mikHYzYCKs0A8ty'
  AND g."pb$deleted_at" IS NULL
  AND e."pb$deleted_at" IS NULL
GROUP BY g."pb$connector_exclusion_group_id", g."pb$id"
ORDER BY g."pb$connector_exclusion_group_id";

-- -----------------------------------------------------------------------------
-- 3. Detailed entries per group.
-- -----------------------------------------------------------------------------
\echo
\echo '==> [3] Entry detail (ordered by group, then by order)'
SELECT
  g."pb$connector_exclusion_group_id" AS connector_group,
  e."pb$app_entitlement_id"            AS app_entitlement_id,
  e."pb$order"                         AS "order",
  e."pb$is_default"                    AS is_default
FROM pb_app_entitlement_exclusion_group_c1_m_655d50c4 g
JOIN pb_app_entitlement_exclusion_group_entr_0e87042e e
  ON  e."pb$tenant_id"         = g."pb$tenant_id"
  AND e."pb$app_id"            = g."pb$app_id"
  AND e."pb$exclusion_group_id" = g."pb$id"
WHERE g."pb$tenant_id"   = '3CmpaAkVpaoiiwRv4RFu3wdjZLX'
  AND g."pb$app_id"      = '3Dj1QeX2utQSCs5L5SvnhwqGHql'
  AND g."pb$connector_id" = '3Dj1Qa2Qu4L5mikHYzYCKs0A8ty'
  AND g."pb$deleted_at" IS NULL
  AND e."pb$deleted_at" IS NULL
ORDER BY g."pb$connector_exclusion_group_id", e."pb$order";

-- -----------------------------------------------------------------------------
-- 4. Entry lookups.
--
-- Invariant: every live entry has exactly one live lookup row pointing back
-- at the same exclusion group, and there should be no orphan lookups (lookups
-- without a corresponding entry).
-- -----------------------------------------------------------------------------
\echo
\echo '==> [4] Entry-to-lookup parity per group (expect entries == lookups, orphans == 0)'
WITH live_entries AS (
  SELECT
    e."pb$app_id",
    e."pb$exclusion_group_id",
    e."pb$app_entitlement_id"
  FROM pb_app_entitlement_exclusion_group_entr_0e87042e e
  JOIN pb_app_entitlement_exclusion_group_c1_m_655d50c4 g
    ON  g."pb$tenant_id" = e."pb$tenant_id"
    AND g."pb$app_id"    = e."pb$app_id"
    AND g."pb$id"        = e."pb$exclusion_group_id"
  WHERE e."pb$tenant_id"   = '3CmpaAkVpaoiiwRv4RFu3wdjZLX'
    AND e."pb$app_id"      = '3Dj1QeX2utQSCs5L5SvnhwqGHql'
    AND g."pb$connector_id" = '3Dj1Qa2Qu4L5mikHYzYCKs0A8ty'
    AND e."pb$deleted_at" IS NULL
    AND g."pb$deleted_at" IS NULL
),
live_lookups AS (
  SELECT
    l."pb$app_id",
    l."pb$exclusion_group_id",
    l."pb$app_entitlement_id"
  FROM pb_app_entitlement_exclusion_group_entr_9fa75386 l
  JOIN pb_app_entitlement_exclusion_group_c1_m_655d50c4 g
    ON  g."pb$tenant_id" = l."pb$tenant_id"
    AND g."pb$app_id"    = l."pb$app_id"
    AND g."pb$id"        = l."pb$exclusion_group_id"
  WHERE l."pb$tenant_id"   = '3CmpaAkVpaoiiwRv4RFu3wdjZLX'
    AND l."pb$app_id"      = '3Dj1QeX2utQSCs5L5SvnhwqGHql'
    AND g."pb$connector_id" = '3Dj1Qa2Qu4L5mikHYzYCKs0A8ty'
    AND l."pb$deleted_at" IS NULL
    AND g."pb$deleted_at" IS NULL
)
SELECT
  COALESCE(e."pb$exclusion_group_id", l."pb$exclusion_group_id") AS group_id,
  COUNT(e.*)                                                     AS entry_count,
  COUNT(l.*)                                                     AS lookup_count,
  COUNT(*) FILTER (WHERE e."pb$app_entitlement_id" IS NULL)      AS orphan_lookups,
  COUNT(*) FILTER (WHERE l."pb$app_entitlement_id" IS NULL)      AS missing_lookups
FROM live_entries e
FULL OUTER JOIN live_lookups l
  ON  e."pb$app_id"             = l."pb$app_id"
  AND e."pb$exclusion_group_id" = l."pb$exclusion_group_id"
  AND e."pb$app_entitlement_id" = l."pb$app_entitlement_id"
GROUP BY COALESCE(e."pb$exclusion_group_id", l."pb$exclusion_group_id")
ORDER BY 1;

-- -----------------------------------------------------------------------------
-- 5. Multi-default guard.
--
-- C1 rejects more than one default entitlement per exclusion group. This must
-- always return zero rows. If it returns anything, uplift accepted a bad
-- group and that is a bug.
-- -----------------------------------------------------------------------------
\echo
\echo '==> [5] Multi-default guard (expect 0 rows)'
SELECT
  g."pb$connector_exclusion_group_id" AS connector_group,
  g."pb$id"                            AS group_id,
  COUNT(*) FILTER (WHERE e."pb$is_default") AS default_count
FROM pb_app_entitlement_exclusion_group_c1_m_655d50c4 g
JOIN pb_app_entitlement_exclusion_group_entr_0e87042e e
  ON  e."pb$tenant_id"         = g."pb$tenant_id"
  AND e."pb$app_id"            = g."pb$app_id"
  AND e."pb$exclusion_group_id" = g."pb$id"
WHERE g."pb$tenant_id"   = '3CmpaAkVpaoiiwRv4RFu3wdjZLX'
  AND g."pb$app_id"      = '3Dj1QeX2utQSCs5L5SvnhwqGHql'
  AND g."pb$connector_id" = '3Dj1Qa2Qu4L5mikHYzYCKs0A8ty'
  AND g."pb$deleted_at" IS NULL
  AND e."pb$deleted_at" IS NULL
GROUP BY g."pb$connector_exclusion_group_id", g."pb$id"
HAVING COUNT(*) FILTER (WHERE e."pb$is_default") <> 1;

-- -----------------------------------------------------------------------------
-- 6. Per-resource group placement.
--
-- The model says: one exclusion group is scoped to one app resource. Two
-- groups sharing the same app_resource_id is allowed (billing has both a
-- role-tier and a clearance-tier in the fixture); the same group spanning
-- multiple app_resource_ids is NOT. This must return zero rows.
-- -----------------------------------------------------------------------------
\echo
\echo '==> [6] Group-per-resource invariant (expect 0 rows)'
SELECT
  "pb$connector_exclusion_group_id"           AS connector_group,
  COUNT(DISTINCT "pb$app_resource_id")        AS distinct_app_resources
FROM pb_app_entitlement_exclusion_group_c1_m_655d50c4
WHERE "pb$tenant_id"   = '3CmpaAkVpaoiiwRv4RFu3wdjZLX'
  AND "pb$app_id"      = '3Dj1QeX2utQSCs5L5SvnhwqGHql'
  AND "pb$connector_id" = '3Dj1Qa2Qu4L5mikHYzYCKs0A8ty'
  AND "pb$deleted_at" IS NULL
GROUP BY "pb$connector_exclusion_group_id"
HAVING COUNT(DISTINCT "pb$app_resource_id") <> 1;

-- -----------------------------------------------------------------------------
-- 7. Negative test (feature flag disabled).
--
-- Run this against a tenant where the entitlement_exclusion_groups FF was
-- never enabled. All three counts must be 0.
-- -----------------------------------------------------------------------------
\echo
\echo '==> [7] FF-disabled negative check (run against a tenant with FF off; expect 0 / 0 / 0)'
SELECT
  (SELECT COUNT(*) FROM pb_app_entitlement_exclusion_group_c1_m_655d50c4
    WHERE "pb$tenant_id" = '3CmpaAkVpaoiiwRv4RFu3wdjZLX' AND "pb$app_id" = '3Dj1QeX2utQSCs5L5SvnhwqGHql'
      AND "pb$connector_id" = '3Dj1Qa2Qu4L5mikHYzYCKs0A8ty' AND "pb$deleted_at" IS NULL) AS groups,
  (SELECT COUNT(*) FROM pb_app_entitlement_exclusion_group_entr_0e87042e
    WHERE "pb$tenant_id" = '3CmpaAkVpaoiiwRv4RFu3wdjZLX' AND "pb$app_id" = '3Dj1QeX2utQSCs5L5SvnhwqGHql'
      AND "pb$deleted_at" IS NULL) AS entries,
  (SELECT COUNT(*) FROM pb_app_entitlement_exclusion_group_entr_9fa75386
    WHERE "pb$tenant_id" = '3CmpaAkVpaoiiwRv4RFu3wdjZLX' AND "pb$app_id" = '3Dj1QeX2utQSCs5L5SvnhwqGHql'
      AND "pb$deleted_at" IS NULL) AS lookups;

-- -----------------------------------------------------------------------------
-- 8. Cleanup-after-sync verification.
--
-- After deleting `writer` (or any one level) from the fixture and re-syncing:
--   * the parent group still exists
--   * the writer entry is gone (soft-deleted)
--   * the writer lookup is gone (soft-deleted)
--   * the remaining siblings are untouched
--
-- Set the level you removed and the connector group it belonged to.
--
--   \set removed_app_entitlement_slug 'writer'
--   \set removed_connector_group     'database-main-role-tier'
-- -----------------------------------------------------------------------------
\echo
\echo '==> [8] Cleanup verification — counts for the removed level (expect 0 live, >= 1 soft-deleted)'
WITH g AS (
  SELECT "pb$id" AS group_id
  FROM pb_app_entitlement_exclusion_group_c1_m_655d50c4
  WHERE "pb$tenant_id"                  = '3CmpaAkVpaoiiwRv4RFu3wdjZLX'
    AND "pb$app_id"                     = '3Dj1QeX2utQSCs5L5SvnhwqGHql'
    AND "pb$connector_id"               = '3Dj1Qa2Qu4L5mikHYzYCKs0A8ty'
    AND "pb$connector_exclusion_group_id" = :'removed_connector_group'
    AND "pb$deleted_at" IS NULL
)
SELECT
  'entries' AS kind,
  COUNT(*) FILTER (WHERE e."pb$deleted_at" IS NULL)     AS live,
  COUNT(*) FILTER (WHERE e."pb$deleted_at" IS NOT NULL) AS soft_deleted
FROM pb_app_entitlement_exclusion_group_entr_0e87042e e
JOIN g ON g.group_id = e."pb$exclusion_group_id"
WHERE e."pb$tenant_id" = '3CmpaAkVpaoiiwRv4RFu3wdjZLX'
  AND e."pb$app_id"    = '3Dj1QeX2utQSCs5L5SvnhwqGHql'
  AND e."pb$app_entitlement_id" LIKE '%' || :'removed_app_entitlement_slug' || '%'
UNION ALL
SELECT
  'lookups',
  COUNT(*) FILTER (WHERE l."pb$deleted_at" IS NULL),
  COUNT(*) FILTER (WHERE l."pb$deleted_at" IS NOT NULL)
FROM pb_app_entitlement_exclusion_group_entr_9fa75386 l
JOIN g ON g.group_id = l."pb$exclusion_group_id"
WHERE l."pb$tenant_id" = '3CmpaAkVpaoiiwRv4RFu3wdjZLX'
  AND l."pb$app_id"    = '3Dj1QeX2utQSCs5L5SvnhwqGHql'
  AND l."pb$app_entitlement_id" LIKE '%' || :'removed_app_entitlement_slug' || '%';

-- -----------------------------------------------------------------------------
-- 9. Full-group cleanup verification.
--
-- After deleting every row from database_access_levels for a database and
-- re-syncing, the whole exclusion group should be soft-deleted (along with
-- its entries and lookups).
--
--   \set removed_connector_group 'database-main-role-tier'
-- -----------------------------------------------------------------------------
\echo
\echo '==> [9] Full-group cleanup (expect 0 live, all soft-deleted for the named connector group)'
SELECT
  'group' AS kind,
  COUNT(*) FILTER (WHERE "pb$deleted_at" IS NULL)     AS live,
  COUNT(*) FILTER (WHERE "pb$deleted_at" IS NOT NULL) AS soft_deleted
FROM pb_app_entitlement_exclusion_group_c1_m_655d50c4
WHERE "pb$tenant_id"                    = '3CmpaAkVpaoiiwRv4RFu3wdjZLX'
  AND "pb$app_id"                       = '3Dj1QeX2utQSCs5L5SvnhwqGHql'
  AND "pb$connector_id"                 = '3Dj1Qa2Qu4L5mikHYzYCKs0A8ty'
  AND "pb$connector_exclusion_group_id" = :'removed_connector_group'
UNION ALL
SELECT
  'entries',
  COUNT(*) FILTER (WHERE e."pb$deleted_at" IS NULL),
  COUNT(*) FILTER (WHERE e."pb$deleted_at" IS NOT NULL)
FROM pb_app_entitlement_exclusion_group_entr_0e87042e e
JOIN pb_app_entitlement_exclusion_group_c1_m_655d50c4 g
  ON  g."pb$tenant_id"        = e."pb$tenant_id"
  AND g."pb$app_id"           = e."pb$app_id"
  AND g."pb$id"               = e."pb$exclusion_group_id"
WHERE g."pb$tenant_id"                    = '3CmpaAkVpaoiiwRv4RFu3wdjZLX'
  AND g."pb$app_id"                       = '3Dj1QeX2utQSCs5L5SvnhwqGHql'
  AND g."pb$connector_id"                 = '3Dj1Qa2Qu4L5mikHYzYCKs0A8ty'
  AND g."pb$connector_exclusion_group_id" = :'removed_connector_group'
UNION ALL
SELECT
  'lookups',
  COUNT(*) FILTER (WHERE l."pb$deleted_at" IS NULL),
  COUNT(*) FILTER (WHERE l."pb$deleted_at" IS NOT NULL)
FROM pb_app_entitlement_exclusion_group_entr_9fa75386 l
JOIN pb_app_entitlement_exclusion_group_c1_m_655d50c4 g
  ON  g."pb$tenant_id"        = l."pb$tenant_id"
  AND g."pb$app_id"           = l."pb$app_id"
  AND g."pb$id"               = l."pb$exclusion_group_id"
WHERE g."pb$tenant_id"                    = '3CmpaAkVpaoiiwRv4RFu3wdjZLX'
  AND g."pb$app_id"                       = '3Dj1QeX2utQSCs5L5SvnhwqGHql'
  AND g."pb$connector_id"                 = '3Dj1Qa2Qu4L5mikHYzYCKs0A8ty'
  AND g."pb$connector_exclusion_group_id" = :'removed_connector_group';
