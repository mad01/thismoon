---
id: ai-postmortem-section
label: likely_ai
bucket: ai
split: test
source: generated
license: generated for this eval
generator: claude-sonnet-5-5 via claude -p, 2026-10-09
words: 208
notes: Postmortem section with bold labels, a minute-by-minute bulleted timeline and a contributing-factors close. Prompt was Write the root cause and timeline section of an incident postmortem for an outage where an expired TLS certificate on an internal load balancer made API requests fail for 47 minutes. About 200 words. Output only the section text, nothing else.
---
**Root Cause and Timeline**

**Root cause:** The TLS certificate on the internal load balancer fronting the API tier expired at 09:12 UTC. The certificate had been issued manually two years earlier, outside our automated renewal pipeline. It was not tracked in the certificate inventory, and no expiry alert covered it. Once the certificate lapsed, upstream services rejected the handshake, so API requests failed with TLS verification errors.

**Timeline (UTC):**

- **09:12:** Certificate expires. Error rates on API requests climb to nearly 100%.
- **09:17:** Synthetic checks fail and page the on-call engineer.
- **09:24:** On-call acknowledges the page. Initial suspicion falls on a bad deploy, and a rollback of the morning release begins.
- **09:36:** The rollback does not help. Logs show "certificate has expired" errors from client services.
- **09:41:** The expired certificate on the load balancer is identified as the cause.
- **09:48:** A new certificate is requested manually and approved.
- **09:52:** The new certificate is installed on the load balancer.
- **09:59:** Error rates return to baseline. The incident is resolved after 47 minutes.

**Contributing factors:** There was no expiry monitoring for this certificate, and the runbook did not list certificate expiry as a common failure mode. That delayed diagnosis by roughly 24 minutes.
