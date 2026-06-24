# Postmortem: [Incident Title]

**Date:** YYYY-MM-DD
**Severity:** P0/P1/P2
**Authors:** [names]
**Status:** Draft/Final

## 1. Summary
Brief description of the incident (1-2 sentences).

## 2. Impact
- **Duration:** [start time] to [end time] ([total time])
- **Users affected:** [count or percentage]
- **Revenue impact:** [if applicable]
- **SLO impact:** [which SLOs were violated, error budget consumed]

## 3. Root Cause
The underlying cause of the incident (technical).

## 4. Timeline
All times in UTC.

| Time | Event |
|------|-------|
| 00:00 | Alert fired |
| 00:05 | On-call acknowledged |
| 00:10 | Mitigation applied |
| 00:30 | Service restored |

## 5. Trigger
What caused the incident to happen at this specific time (deployment, config change, traffic spike, etc.).

## 6. Mitigation
What was done to restore service.

## 7. Root Cause Analysis (5 Whys)
1. Why did the service go down? → [answer]
2. Why did [answer 1] happen? → [answer]
3. Why did [answer 2] happen? → [answer]
4. Why did [answer 3] happen? → [answer]
5. Why did [answer 4] happen? → [root cause]

## 8. Action Items
| Action | Owner | Priority | Due Date | Status |
|--------|-------|----------|----------|--------|
| [action] | [name] | P0 | [date] | Open |
| [action] | [name] | P1 | [date] | Open |

## Lessons Learned
- What went well
- What went poorly
- Where we got lucky
