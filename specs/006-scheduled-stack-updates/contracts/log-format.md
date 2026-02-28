# Log Format Contract: Scheduled Stack Updates

**Feature**: 006-scheduled-stack-updates  
**Version**: 1.0.0  
**Date**: February 27, 2026

## Overview

This document defines the structured log format for scheduled update operations. External systems (log aggregators, monitoring systems, alerting tools) can parse these logs for observability and alerting.

## Log Format

**Format**: Structured logging via Go `slog` package  
**Encoding**: Text (default) or JSON (when `LOG_FORMAT=json`)  
**Output**: stdout  
**Timezone**: UTC

## Log Levels

- **INFO**: Normal operations and successful completions
- **WARN**: Non-fatal errors, skipped operations, retryable failures
- **ERROR**: Fatal errors, scheduler failures, service issues
- **DEBUG**: Detailed traces (only when `LOG_LEVEL=debug`)

## Update Cycle Events

### Cycle Start

**When**: Update cycle begins (cron trigger fires)

**Text Format**:
```
INFO update cycle started cycle_id=550e8400-e29b-41d4-a716-446655440000 scheduled_time="2026-02-27T03:00:00Z" stacks=5
```

**JSON Format**:
```json
{
  "time": "2026-02-27T03:00:00Z",
  "level": "INFO",
  "msg": "update cycle started",
  "cycle_id": "550e8400-e29b-41d4-a716-446655440000",
  "scheduled_time": "2026-02-27T03:00:00Z",
  "stacks": 5
}
```

**Fields**:
- `cycle_id` (string): Unique UUID for this cycle
- `scheduled_time` (ISO 8601): When cycle was triggered
- `stacks` (int): Number of stacks to process

---

### Image Pull - Start

**When**: Beginning to pull images for a stack

**Text Format**:
```
INFO pulling images stack=myapp project=myapp-prod images=3
```

**JSON Format**:
```json
{
  "time": "2026-02-27T03:00:05Z",
  "level": "INFO",
  "msg": "pulling images",
  "stack": "myapp",
  "project": "myapp-prod",
  "images": 3
}
```

**Fields**:
- `stack` (string): Stack name  (directory name)
- `project` (string): Docker Compose project name
- `images` (int): Number of images to pull

---

### Image Pull - Complete

**When**: Images pulled successfully for a stack

**Text Format**:
```
INFO images pulled stack=myapp duration=12.5s updated=2 unchanged=1
```

**JSON Format**:
```json
{
  "time": "2026-02-27T03:00:17Z",
  "level": "INFO",
  "msg": "images pulled",
  "stack": "myapp",
  "duration": "12.5s",
  "updated": 2,
  "unchanged": 1
}
```

**Fields**:
- `stack` (string): Stack name
- `duration` (duration string): How long pull took
- `updated` (int): Images that changed (digest differ)
- `unchanged` (int): Images already current

---

### Image Pull - Failed

**When**: Image pull failed for a stack

**Text Format**:
```
WARN image pull failed stack=myapp error="network timeout" continued=true
```

**JSON Format**:
```json
{
  "time": "2026-02-27T03:00:17Z",
  "level": "WARN",
  "msg": "image pull failed",
  "stack": "myapp",
  "error": "network timeout",
  "continued": true
}
```

**Fields**:
- `stack` (string): Stack that failed
- `error` (string): Error message
- `continued` (bool): Whether cycle continues with other stacks

---

### Stack Update - Start

**When**: Reconciling a stack (recreating containers)

**Text Format**:
```
INFO reconciling stack stack=myapp project=myapp-prod reason="images changed"
```

**JSON Format**:
```json
{
  "time": "2026-02-27T03:00:18Z",
  "level": "INFO",
  "msg": "reconciling stack",
  "stack": "myapp",
  "project": "myapp-prod",
  "reason": "images changed"
}
```

**Fields**:
- `stack` (string): Stack name
- `project` (string): Docker Compose project
- `reason` (string): Why reconciliation triggered

---

### Stack Update - Complete

**When**: Stack successfully updated

**Text Format**:
```
INFO stack updated stack=myapp containers_updated=3 duration=8.2s
```

**JSON Format**:
```json
{
  "time": "2026-02-27T03:00:26Z",
  "level": "INFO",
  "msg": "stack updated",
  "stack": "myapp",
  "containers_updated": 3,
  "duration": "8.2s"
}
```

**Fields**:
- `stack` (string): Stack name
- `containers_updated` (int): Containers recreated
- `duration` (duration string): Update time

---

### Stack Skipped

**When**: Stack skipped (stopped, error state, or manual modification detected)

**Text Format**:
```
WARN stack skipped stack=myapp reason="stopped"
```

**JSON Format**:
```json
{
  "time": "2026-02-27T03:00:30Z",
  "level": "WARN",
  "msg": "stack skipped",
  "stack": "myapp",
  "reason": "stopped"
}
```

**Fields**:
- `stack` (string): Stack name
- `reason` (string): Why skipped (stopped, error, conflict, etc.)

---

### Image Prune

**When**: System-wide image prune completed

**Text Format**:
```
INFO images pruned count=15 space_reclaimed="1.2GB" duration=3.5s
```

**JSON Format**:
```json
{
  "time": "2026-02-27T03:05:45Z",
  "level": "INFO",
  "msg": "images pruned",
  "count": 15,
  "space_reclaimed": "1.2GB",
  "duration": "3.5s"
}
```

**Fields**:
- `count` (int): Number of images removed
- `space_reclaimed` (string): Disk space freed
- `duration` (duration string): Prune operation time

---

### Cycle Complete

**When**: Update cycle finished successfully

**Text Format**:
```
INFO update cycle completed cycle_id=550e8400-e29b-41d4-a716-446655440000 duration=6m15s stacks_processed=5 containers_updated=12 images_pruned=15 space_reclaimed="1.2GB" errors=1
```

**JSON Format**:
```json
{
  "time": "2026-02-27T03:06:15Z",
  "level": "INFO",
  "msg": "update cycle completed",
  "cycle_id": "550e8400-e29b-41d4-a716-446655440000",
  "duration": "6m15s",
  "stacks_processed": 5,
  "containers_updated": 12,
  "images_pruned": 15,
  "space_reclaimed": "1.2GB",
  "errors": 1
}
```

**Fields**:
- `cycle_id` (string): UUID from cycle start
- `duration` (duration string): Total cycle time
- `stacks_processed` (int): Stacks attempted
- `containers_updated` (int): Total containers recreated
- `images_pruned` (int): Total images removed
- `space_reclaimed` (string): Total disk space freed
- `errors` (int): Non-fatal error count

---

### Cycle Terminated

**When**: Cycle terminated early (new schedule triggered, shutdown, or critical error)

**Text Format**:
```
WARN update cycle terminated cycle_id=550e8400-e29b-41d4-a716-446655440000 reason="new schedule triggered" stacks_processed=2 stacks_remaining=3
```

**JSON Format**:
```json
{
  "time": "2026-02-27T03:03:00Z",
  "level": "WARN",
  "msg": "update cycle terminated",
  "cycle_id": "550e8400-e29b-41d4-a716-446655440000",
  "reason": "new schedule triggered",
  "stacks_processed": 2,
  "stacks_remaining": 3
}
```

**Fields**:
- `cycle_id` (string): UUID of terminated cycle
- `reason` (string): Why terminated (new_schedule, shutdown, error)
- `stacks_processed` (int): Stacks completed before termination
- `stacks_remaining` (int): Stacks not processed

---

## Error Events

### Scheduler Error

**When**: Scheduler itself encounters an error

**Text Format**:
```
ERROR scheduler error error="cron parse failed" action="using default schedule"
```

**JSON Format**:
```json
{
  "time": "2026-02-27T00:00:05Z",
  "level": "ERROR",
  "msg": "scheduler error",
  "error": "cron parse failed",
  "action": "using default schedule"
}
```

---

### Docker Error

**When**: Docker daemon issues detected

**Text Format**:
```
ERROR docker daemon unreachable error="connection refused" action="retrying"
```

**JSON Format**:
```json
{
  "time": "2026-02-27T03:00:10Z",
  "level": "ERROR",
  "msg": "docker daemon unreachable",
  "error": "connection refused",
  "action": "retrying"
}
```

---

## Monitoring Queries

### Check Cycle Success Rate

**Goal**: Calculate percentage of successful cycles

**Query Pattern** (JSON logs):
```
msg="update cycle completed" | where errors == 0 | count
---
msg="update cycle completed" | count
```

**Threshold**: Alert if success rate < 95% (per SC-003)

---

### Check Cycle Duration

**Goal**: Ensure cycles complete within 30 minutes

**Query Pattern**:
```
msg="update cycle completed" | parse duration | where duration_minutes > 30
```

**Threshold**: Alert if duration > 30 minutes (per SC-001)

---

### Check Image Pull Failures

**Goal**: Detect network or registry issues

**Query Pattern**:
```
msg="image pull failed" | count by error | top 10
```

**Threshold**: Alert if >5 failures in single cycle

---

### Check Disk Space Reclaimed

**Goal**: Verify pruning is effective

**Query Pattern**:
```
msg="images pruned" | parse space_reclaimed | avg
```

**Threshold**: Alert if <20% reduction when images exist (per SC-005)

---

### Check Stack Skips

**Goal**: Identify problematic stacks

**Query Pattern**:
```
msg="stack skipped" | count by stack, reason
```

**Threshold**: Alert if same stack skipped >3 consecutive cycles

---

## Log Retention

**Recommendations**:
- Retain cycle completion logs for 90 days (compliance/audit)
- Retain error logs for 30 days (troubleshooting)
- Retain debug logs for 7 days (detailed investigation)

## Parsing Examples

### Extract Cycle Summary (shell/grep)

```bash
# Find all completed cycles
grep "update cycle completed" logs.txt

# Extract space reclaimed
grep "images pruned" logs.txt | grep-o 'space_reclaimed="[^"]*"'

# Count errors per cycle
grep "update cycle completed" logs.txt | grep -o 'errors=[0-9]*'
```

### Parse JSON Logs (jq)

```bash
# Get average cycle duration
cat logs.json | jq 'select(.msg=="update cycle completed") | .duration' | avg

# List failed stacks
cat logs.json | jq 'select(.msg=="image pull failed") | .stack'

# Count by error type
cat logs.json | jq 'select(.level=="ERROR") | .error' | sort | uniq -c
```

### Splunk Query

```spl
index=docker-cd msg="update cycle completed"
| stats avg(duration) as avg_duration, 
        sum(containers_updated) as total_updated,
        sum(errors) as total_errors
        by date_hour
```

### Elasticsearch Query

```json
{
  "query": {
    "bool": {
      "must": [
        { "match": { "msg": "update cycle completed" }},
        { "range": { "time": { "gte": "now-24h" }}}
      ]
    }
  },
  "aggs": {
    "avg_duration": { "avg": { "field": "duration" }},
    "total_errors": { "sum": { "field": "errors" }}
  }
}
```

## Contract Guarantees

**What This Contract Promises**:
1. Log messages are stable (won't change format between versions)
2. Field names are consistent across all events
3. JSON format is valid and parseable
4. Error messages include actionable information
5. Timestamps are always UTC in ISO 8601 format

**What This Contract Does NOT Promise**:
1. Log order (concurrent operations may interleave)
2. Log deduplication (same event may log multiple times)
3. Log delivery guarantees (depends on log aggregator)
4. Backwards compatibility for DEBUG level logs (may change)

## Version History

- **1.0.0** (2026-02-27): Initial log format definition
  - Cycle start/complete events
  - Image pull events
  - Stack update events
  - Prune events
  - Error events
