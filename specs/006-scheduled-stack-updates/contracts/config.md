# Configuration Contract: Scheduled Stack Updates

**Feature**: 006-scheduled-stack-updates  
**Version**: 1.0.0  
**Date**: February 27, 2026

## Overview

This document defines the configuration interface for the scheduled stack updates feature. External systems (deployment scripts, orchestration tools, documentation) should reference this contract.

## Environment Variables

### UPDATER_ENABLED

**Type**: Boolean  
**Required**: No  
**Default**: `false`  
**Valid Values**: `true`, `false`, `1`, `0`, `yes`, `no`

**Purpose**: Enable or disable automatic scheduled stack updates

**Behavior**:
- `true`: Scheduler starts on service startup, updates run per schedule
- `false`: Scheduler does not start, no automatic updates occur

**Example**:
```bash
UPDATER_ENABLED=true
```

**Validation**:
- Parsed as boolean using `strconv.ParseBool`
- Invalid values fall back to `false`
- No error logged for missing variable (defaults applied)

---

### UPDATER_CRON

**Type**: String (cron expression)  
**Required**: No  
**Default**: `0 3 * * *` (3 AM UTC daily)  
**Valid Format**: 5-field cron expression (minute hour day month weekday)

**Purpose**: Define when automatic stack updates should run

**Format**: `minute hour day-of-month month day-of-week`
- minute: 0-59
- hour: 0-23
- day-of-month: 1-31
- month: 1-12
- day-of-week: 0-6 (0=Sunday)

**Special Characters**:
- `*`: Any value
- `,`: Value list (e.g., `1,3,5`)
- `-`: Range (e.g., `1-5`)
- `/`: Step values (e.g., `*/15` = every 15 minutes)

**Examples**:
```bash
# Every night at 3 AM UTC (default)
UPDATER_CRON="0 3 * * *"

# Every 6 hours
UPDATER_CRON="0 */6 * * *"

# Every weekday at 2:30 AM UTC
UPDATER_CRON="30 2 * * 1-5"

# Twice daily: 3 AM and 3 PM UTC
UPDATER_CRON="0 3,15 * * *"
```

**Validation**:
- Parsed using `github.com/robfig/cron/v3` parser
- Invalid expression → Error logged, falls back to default
- Service continues with default schedule if expression is invalid
- Logged at startup: "Using schedule: X"

**Timezone**:
- All times are in UTC (non-configurable in v1.0.0)
- System timezone is ignored
- Cron scheduler runs in UTC regardless of host timezone

---

## Configuration Loading

**Load Order**:
1. Defaults applied
2. Environment variables read
3. Values validated
4. Invalid values fall back to defaults
5. Final config logged at startup

**Example Startup Log**:
```
INFO docker-cd starting version=dev
INFO scheduler configuration enabled=true schedule="0 3 * * *"
```

## Configuration Changes

**Runtime Changes**: Not supported
- Configuration is loaded only at service startup
- Changing environment variables requires service restart
- No hot reload mechanism

**Validation Timing**: Startup only
- Invalid cron expression prevents scheduler from starting
- Service continues running with scheduler disabled
- Manual updates via API still work (if available)

**Deployment Pattern**:
```bash
# docker-compose.yml or environment file
environment:
  UPDATER_ENABLED: "true"
  UPDATER_CRON: "0 3 * * *"
```

## Backward Compatibility

**Version**: 1.0.0 (Initial)

**Defaults Ensure**:
- Feature is opt-in (disabled by default)
- Existing deployments unaffected until explicitly enabled
- Default schedule is sensible for most use cases

**Future Versions**:
- New fields will be added as optional with defaults
- Existing fields will not be renamed or removed
- Breaking changes will bump major version

## Integration Examples

### Docker Compose

```yaml
services:
  docker-cd:
    image: docker-cd:latest
    environment:
      # Enable scheduled updates
      UPDATER_ENABLED: "true"
      # Run at 2 AM UTC daily
      UPDATER_CRON: "0 2 * * *"
      # Other existing config...
      GIT_REPO_URL: "https://github.com/org/repo"
      GIT_ACCESS_TOKEN: "${GITHUB_TOKEN}"
      GIT_REVISION: "main"
```

### Kubernetes ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: docker-cd-config
data:
  UPDATER_ENABLED: "true"
  UPDATER_CRON: "0 3 * * *"
---
apiVersion: v1
kind: Pod
metadata:
  name: docker-cd
spec:
  containers:
  - name: docker-cd
    image: docker-cd:latest
    envFrom:
    - configMapRef:
        name: docker-cd-config
```

### Shell Script

```bash
#!/bin/bash
export UPDATER_ENABLED=true
export UPDATER_CRON="0 3 * * *"
./docker-cd
```

## Testing Configuration

**Validation**:
```bash
# Test with valid configuration
UPDATER_ENABLED=true UPDATER_CRON="0 3 * * *" ./docker-cd

# Expected log: "scheduler configuration enabled=true schedule=\"0 3 * * *\""
```

**Error Cases**:
```bash
# Invalid cron expression
UPDATER_CRON="invalid" ./docker-cd
# Expected log: "invalid cron expression, using default: 0 3 * * *"

# Updater disabled
UPDATER_ENABLED=false ./docker-cd
# Expected log: "scheduler disabled"
```

## Contract Guarantees

**What This Contract Promises**:
1. Environment variables follow standard naming convention
2. Boolean parsing is permissive (true/false/1/0/yes/no)
3. Invalid values fall back to safe defaults
4. Service continues running even with invalid scheduler config
5. Configuration is logged at startup for verification

**What This Contract Does NOT Promise**:
1. Hot reload of configuration changes
2. Validation of scheduler effectiveness (e.g., whether schedule makes sense)
3. Persistence of configuration across deployments
4. Configuration via files or APIs (environment only)

## Monitoring

**Configuration Verification**:
- Check startup logs for "scheduler configuration" entry
- Verify enabled=true if expecting updates
- Verify schedule matches intended cron expression

**Configuration Issues**:
- Look for "invalid cron expression" warnings
- Check "scheduler disabled" if updates aren't running
- Verify environment variables are  actually set in container

## Related Contracts

- [Configuration Schema](./config-schema.md) - Complete configuration reference
- [Log Format](./log-format.md) - Structured log output format
- [Update Status](./update-status.md) - Status reporting (future)

## Version History

- **1.0.0** (2026-02-27): Initial contract definition
  - UPDATER_ENABLED
  - UPDATER_CRON
