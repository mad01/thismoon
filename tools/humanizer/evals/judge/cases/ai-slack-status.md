---
id: ai-slack-status
label: likely_ai
bucket: ai
source: generated
license: generated for this eval
generator: claude-sonnet-5-5 via claude -p, 2026-10-08
words: 218
notes: Slack status update with emoji, bold section labels and an asks list. Prompt was Write a Slack message to your engineering team giving a status update on a Postgres major-version upgrade that is running two days behind schedule. Output only the message, nothing else.
---
:warning: **Postgres 14 → 16 upgrade: running 2 days behind**

Quick update on the upgrade. We're now targeting **Thursday** for the production cutover, not Tuesday.

**Where we are**
- Staging upgrade is complete and the app test suite passes against it.
- Logical replication to the new cluster is in sync, with lag under 1s.
- Extension compatibility is sorted. `pg_partman` needed a version bump.

**Why we slipped**
- The first staging dry run took about 9 hours, not the 3 we planned. The reindex step on the two largest tables was much slower than we estimated.
- We found 4 queries in the billing service that picked worse plans on 16. Fixes are in review, and 2 are already merged.

**What's left**
- Rerun the full dry run with the new reindex approach (tomorrow morning).
- Merge and deploy the remaining 2 query fixes.
- Final go/no-go on Wednesday at 3pm.

**What I need from you**
- Reviews on the open query-fix PRs, linked in the thread below. Please look today if you can.
- If your service has queries you think might be sensitive to planner changes, tell me by EOD tomorrow.

The maintenance window is still 02:00–04:00 UTC and rollback is ready. Nothing changes for you until then. I'll post again after tomorrow's dry run.
