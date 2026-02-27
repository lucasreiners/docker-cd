# Quickstart: Scheduled Stack Updates

**Feature**: 006-scheduled-stack-updates  
**Date**: February 27, 2026

## What This Feature Does

Automatically updates all your managed compose stacks on a schedule:
1. **Pulls** latest images for all stacks
2. **Updates** containers when images change
3. **Prunes** unused images to save disk space
4. **Logs** everything for visibility

Default: runs every night at 3 AM UTC.

## Quick Setup (5 minutes)

### Step 1: Enable the Scheduler

Add to your environment or docker-compose.yml:

```bash
UPDATER_ENABLED=true
```

That's it! Uses default schedule (3 AM UTC daily).

### Step 2: Restart the Service

```bash
docker-compose restart docker-cd
```

### Step 3: Verify It's Running

Check logs for:
```
INFO scheduler configuration enabled=true schedule="0 3 * * *"
```

## Custom Schedule

Want a different time? Set the cron expression:

```bash
UPDATER_ENABLED=true
UPDATER_CRON="0 2 * * *"  # 2 AM UTC instead
```

### Common Schedules

```bash
# Every 6 hours
UPDATER_CRON="0 */6 * * *"

# Twice daily: 3 AM and 3 PM
UPDATER_CRON="0 3,15 * * *"

# Weekdays only at 4 AM
UPDATER_CRON="0 4 * * 1-5"

# Every 15 minutes (testing)
UPDATER_CRON="*/15 * * * *"
```

## What Happens During an Update?

### 1. Scheduled Time Arrives (e.g., 3 AM UTC)

```
INFO update cycle started cycle_id=abc-123 stacks=5
```

### 2. For Each Stack

```
INFO pulling images stack=myapp images=3
INFO images pulled stack=myapp updated=2 unchanged=1
INFO reconciling stack stack=myapp reason="images changed"
INFO stack updated stack=myapp containers_updated=3
```

### 3. Cleanup

```
INFO images pruned count=15 space_reclaimed="1.2GB"
```

### 4. Summary

```
INFO update cycle completed duration=6m15s containers_updated=12 space_reclaimed="1.2GB"
```

Your stacks are now running the latest images!

## Checking Update Status

### View Last Update

```bash
docker logs docker-cd | grep "update cycle completed" | tail -1
```

Example output:
```
INFO update cycle completed duration=6m15s stacks_processed=5 containers_updated=12
```

### See What Updated

```bash
docker logs docker-cd | grep "stack updated"
```

### Check for Errors

```bash
docker logs docker-cd | grep "WARN\|ERROR" | tail -20
```

## Common Scenarios

### Scenario: You want updates only on weekends

```bash
UPDATER_ENABLED=true
UPDATER_CRON="0 3 * * 0,6"  # Sunday and Saturday at 3 AM
```

### Scenario: You want to test immediately

Change to run every 5 minutes:
```bash
UPDATER_CRON="*/5 * * * *"
```

Watch logs:
```bash
docker logs -f docker-cd | grep "update cycle"
```

Change back to normal schedule after testing!

### Scenario: You want to disable updates temporarily

```bash
UPDATER_ENABLED=false
```

Restart service. Updates stop. Re-enable when ready.

### Scenario: Some stacks shouldn't update automatically

The scheduler updates ALL managed stacks. To exclude a stack:
- Remove it from the Git deploy directory, OR
- Stop managing it with docker-cd

(Per-stack scheduling is not available in v1.0)

## Understanding the Logs

### Success Indicators

- ✅ `update cycle completed` - Cycle finished
- ✅ `errors=0` - No problems
- ✅ `containers_updated=N` - N containers recreated
- ✅ `space_reclaimed="X"` - Disk space freed

### Warning Signs

- ⚠️ `image pull failed` - Network or registry issue (retried next cycle)
- ⚠️ `stack skipped` - Stack was stopped or in error state
- ⚠️ `errors=N` where N>0 - Some non-fatal issues occurred

### Error Indicators

- ❌ `scheduler error` - Scheduler itself had a problem
- ❌ `docker daemon unreachable` - Can't talk to Docker

## Integration Examples

### Docker Compose

```yaml
version: '3.8'
services:
  docker-cd:
    image: docker-cd:latest
    environment:
      # Existing config
      GIT_REPO_URL: "https://github.com/org/stacks"
      GIT_ACCESS_TOKEN: "${GITHUB_TOKEN}"
      GIT_REVISION: "main"
      
      # Enable scheduled updates
      UPDATER_ENABLED: "true"
      UPDATER_CRON: "0 3 * * *"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
```

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: docker-cd
spec:
  template:
    spec:
      containers:
      - name: docker-cd
        image: docker-cd:latest
        env:
        - name: UPDATER_ENABLED
          value: "true"
        - name: UPDATER_CRON
          value: "0 3 * * *"
        # ... other env vars
```

### Systemd Service

```ini
[Service]
Environment="UPDATER_ENABLED=true"
Environment="UPDATER_CRON=0 3 * * *"
ExecStart=/usr/local/bin/docker-cd
```

## Behavior Details

### What Gets Updated?

- **All images** in all stacks (regardless of tag)
- **Only changed containers** are recreated
- **Unchanged containers** keep running
- **All unused images** are pruned (system-wide)

### What If Something Goes Wrong?

- **One stack fails**: Other stacks continue updating
- **Network failure**: Logged as warning, retried next cycle
- **Critical error**: Cycle stops, logged as error
- **Service restarts**: In-progress cycle is abandoned

### Overlapping Updates

If the next scheduled time arrives while an update is running:
- Current update is **terminated**
- New update starts fresh
- All remaining stacks will be processed in new cycle

### Restart Behavior

If the service restarts during an update:
- In-progress update is **abandoned**
- No resume attempt
- Next scheduled time will start fresh cycle

## Performance Notes

### How Long Do Updates Take?

- Depends on: number of stacks, image sizes, network speed
- Target: Complete within **30 minutes** for typical setups
- Stacks are processed **sequentially** (one at a time)

### Resource Usage

- **Network**: Pulls all images (can be large)
- **Disk**: Temporarily higher (new images downloaded before old removed)
- **CPU/Memory**: Minimal (just orchestration)
- **Service Availability**: Continues serving HTTP during updates

### Disk Space

- Images are pulled first (disk usage increases)
- Containers updated (disk usage stays high)
- Prune removes unused images (disk usage decreases)
- Net result: Usually **saves disk space**

## Troubleshooting

### Updates Aren't Running

1. Check if enabled:
   ```bash
   docker logs docker-cd | grep "scheduler configuration"
   ```
   Should show: `enabled=true`

2. Check schedule:
   ```bash
   docker logs docker-cd | grep "scheduler configuration"
   ```
   Should show: `schedule="0 3 * * *"`

3. Wait for scheduled time (check timezone - all times are UTC!)

### Updates Failing

1. Check Docker daemon:
   ```bash
   docker ps
   ```
   Should work without errors.

2. Check network access:
   ```bash
   docker pull nginx:latest
   ```
   Should succeed.

3. Check logs for specific errors:
   ```bash
   docker logs docker-cd | grep ERROR
   ```

### Updates Too Slow

- Reduce number of stacks
- Improve network speed
- Use local/cached images when possible
- Consider less frequent schedule

### Too Much Disk Usage

- Check if prune is running:
  ```bash
  docker logs docker-cd | grep "images pruned"
  ```
- Run manual prune:
  ```bash
  docker image prune -af
  ```

## FAQ

**Q: Can I run updates manually instead of scheduled?**  
A: Yes, disable the updater (`UPDATER_ENABLED=false`) and trigger updates via API or manually.

**Q: Can I exclude specific stacks?**  
A: Not in v1.0. All managed stacks are updated. Remove stacks from Git repo to exclude.

**Q: What if I want different schedules for different stacks?**  
A: Not supported in v1.0. All stacks use the same schedule.

**Q: Are updates safe?**  
A: Yes. Containers are recreated using the same reconciliation logic as Git-triggered updates. Services may experience brief downtime during container recreation.

**Q: What timezone is the schedule in?**  
A: Always UTC. Use a time zone converter to plan your schedule.

**Q: Can I see update history?**  
A: Check logs. All cycles are logged, including results and timing. Use a log aggregator for long-term history.

**Q: What happens if I change the schedule while service is running?**  
A: Schedule changes require a service restart. Change won't take effect until restart.

**Q: Does this work with private registries?**  
A: Yes, as long as Docker is authenticated to your registry (same as normal docker pull).

## Next Steps

- **Monitor**: Set up log aggregation or alerts on update failures
- **Tune**: Adjust schedule based on your traffic patterns
- **Automate**: Configure via Infrastructure-as-Code
- **Document**: Update your ops runbooks with update schedule

## Related Documentation

- [Configuration Contract](./contracts/config.md) - Detailed config reference
- [Log Format Contract](./contracts/log-format.md) - Structured log parsing
- [Data Model](./data-model.md) - Internal data structures
- [Research](./research.md) - Technical decisions

## Support

**Issues**: Report bugs or issues to project issue tracker  
**Questions**: Check existing documentation or ask in project discussions  
**Feature Requests**: Submit enhancement requests with use case details
