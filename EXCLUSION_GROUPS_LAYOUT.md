# Entitlement Exclusion Groups — Fixture Layout

Visual reference for the E2E fixture in
[entitlement-exclusion-fixture.sql](entitlement-exclusion-fixture.sql), grouped
the way C1 sees it after the connector syncs.

```
┌──────────────────────────────────────────────────────────────────────┐
│ database/main                                                        │
│                                                                      │
│  exclusion_group: database-main-role-tier         (1 group, 4 entries)
│   ┌───────────────────────────────────────────┐                      │
│   │ [10]  reader   ★ default                  │  pick at most one    │
│   │ [20]  writer                              │  (and at least one,  │
│   │ [30]  admin                               │   because there's a  │
│   │ [40]  owner                               │   default ⇒ group is │
│   └───────────────────────────────────────────┘  mandatory)          │
└──────────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────────┐
│ database/analytics                                                   │
│                                                                      │
│  exclusion_group: database-analytics-role-tier    (1 group, 4 entries)
│   ┌───────────────────────────────────────────┐                      │
│   │ [10]  viewer   ★ default                  │                      │
│   │ [20]  analyst                             │                      │
│   │ [30]  engineer                            │                      │
│   │ [40]  admin                               │                      │
│   └───────────────────────────────────────────┘                      │
└──────────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────────┐
│ database/billing                              ← the interesting one  │
│                                                                      │
│  exclusion_group: database-billing-role-tier      (group 1 of 2)     │
│   ┌───────────────────────────────────────────┐                      │
│   │ [10]  viewer        ★ default             │                      │
│   │ [20]  processor                           │   one pick from here │
│   │ [30]  reviewer                            │                      │
│   │ [40]  admin                               │                      │
│   └───────────────────────────────────────────┘                      │
│                                                                      │
│  exclusion_group: database-billing-clearance-tier (group 2 of 2)     │
│   ┌───────────────────────────────────────────┐                      │
│   │ [10]  clearance-restricted ★ default      │   AND one pick from  │
│   │ [20]  clearance-standard                  │   here — both groups │
│   │ [30]  clearance-elevated                  │   apply to the same  │
│   └───────────────────────────────────────────┘   resource           │
└──────────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────────┐
│ database/reporting                                                   │
│                                                                      │
│  exclusion_group: database-reporting-role-tier    (1 group, 4 entries)
│   ┌───────────────────────────────────────────┐                      │
│   │ [10]  consumer  ★ default                 │                      │
│   │ [20]  author                              │                      │
│   │ [30]  maintainer                          │                      │
│   │ [40]  admin                               │                      │
│   └───────────────────────────────────────────┘                      │
└──────────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────────┐
│ database/audit                                                       │
│                                                                      │
│  exclusion_group: database-audit-role-tier        (1 group, 3 entries)
│   ┌───────────────────────────────────────────┐                      │
│   │ [10]  viewer   ★ default                  │                      │
│   │ [20]  auditor                             │                      │
│   │ [30]  admin                               │                      │
│   └───────────────────────────────────────────┘                      │
└──────────────────────────────────────────────────────────────────────┘
```

Reading the brackets: `[N]` is the `sort_order` (proto field 2 — "higher =
more privilege"), `★` marks `is_default = true` (proto field 3 — the fallback
grant when nothing else is specified, and the signal that the group is
mandatory rather than optional).

## Totals

These line up with the expected counts in
[validate.sql](validate.sql) checks #1 and #2.

| | count |
|---|---|
| Databases (app resources) | 5 |
| Exclusion groups | 6 |
| Entries across all groups | 4+4+4+3+4+3 = **22** |
| Defaults (exactly one per group) | 6 |

## Invariants pinned down by validate.sql

- **Each group lives on exactly one database.** Group → 1 `app_resource_id`.
  Violated would mean billing's role-tier somehow spanned billing+analytics.
  (validate.sql check #6)
- **One default per group, max.** Two `is_default = true` rows in the same
  group → C1 rejects the sync. (check #5)
- **A resource can host multiple groups.** Billing demonstrates this — a user
  can hold `processor` AND `clearance-elevated` simultaneously because they
  live in different groups. Inside one group only one pick is allowed.
  (implicit in checks #1 and #2)
- **Soft-delete on removal.** Drop a level from the fixture → the
  corresponding entry & lookup get tombstoned, siblings untouched.
  (check #8)
- **Drop all levels for a database → whole group tombstones.** (check #9)

## Multi-group-per-resource example in the grants

The grant data in
[entitlement-exclusion-fixture.sql](entitlement-exclusion-fixture.sql) is
intentionally constructed so that several users hold one entitlement from
*each* of billing's two exclusion groups simultaneously — e.g. `admin` holds
both `admin` (role-tier) and `clearance-elevated` (clearance-tier) on
`database/billing`. That combination is only legal because the two
entitlements belong to different exclusion groups; if they shared a group,
C1 would reject the second grant.
