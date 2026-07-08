# Execution ID Mismatch Troubleshooting Guide

## Problem Description

The confirmation service is receiving error messages like:
```
"fill execution ID 22275 does not match current execution ID 23746"
```

This indicates that fill messages in Kafka contain execution IDs that don't match the current execution IDs returned by the Execution Service.

## Root Causes

### Scenario A: Execution Service Returns Wrong ID (Most Likely)
1. **Execution Service Bug**: The service returns a different execution ID than requested
2. **Database Issues**: Foreign key relationships or data corruption
3. **Load Balancer/Proxy**: Incorrect routing or caching
4. **API Redirects**: The service redirects to a different execution

### Scenario B: Message Processing Issues
1. **Stale Messages**: Old fill messages in Kafka that reference superseded executions
2. **Message Ordering**: Messages being processed out of order
3. **Race Conditions**: Multiple services updating executions simultaneously
4. **System Recovery**: Messages accumulated during system downtime

## Immediate Solutions

### Option 1: Enable Warning Mode (Recommended for immediate fix)

Update your `config.yaml` with:
```yaml
validation:
  skip_execution_id_validation: false
  max_message_age_minutes: 60
  warn_on_validation_failures: true  # This will log warnings instead of errors
```

This will:
- Continue processing messages despite execution ID mismatches
- Log warnings instead of errors
- Skip messages older than 60 minutes

### Option 2: Skip Execution ID Validation (Use with caution)

```yaml
validation:
  skip_execution_id_validation: true  # Skip validation entirely
  max_message_age_minutes: 60
  warn_on_validation_failures: true
```

**Warning**: This bypasses an important safety check. Use only temporarily.

### Option 3: Filter Old Messages

```yaml
validation:
  skip_execution_id_validation: false
  max_message_age_minutes: 30  # Reduce to 30 minutes
  warn_on_validation_failures: false
```

This will reject messages older than 30 minutes.

## Long-term Solutions

### 1. Kafka Topic Cleanup

Clean up old messages from the Kafka topic:
```bash
# Check current topic retention
kafka-configs.sh --bootstrap-server localhost:9092 --describe --entity-type topics --entity-name fills

# Set shorter retention (e.g., 1 hour)
kafka-configs.sh --bootstrap-server localhost:9092 --alter --entity-type topics --entity-name fills --add-config retention.ms=3600000
```

### 2. Message Deduplication

Implement proper message deduplication based on:
- Fill ID
- Execution ID
- Timestamp

### 3. Execution Service Synchronization

Ensure the Execution Service properly handles:
- Concurrent updates
- Version conflicts
- State transitions

### 4. Message Ordering

Consider using:
- Kafka partitioning by execution ID
- Message keys for ordering
- Idempotent processing

## Monitoring

Add alerts for:
- High execution ID mismatch rates
- Old message processing
- Validation failure patterns

## Debugging Steps

### 1. Enable Debug Logging
```yaml
logging:
  level: "debug"
```

### 2. Test Execution Service Directly
```bash
# Test the specific execution ID that's failing
curl -H "Accept: application/json" \
     -H "X-Correlation-ID: debug-test" \
     "http://globeco-execution-service:8084/api/v1/execution/22275"
```

### 3. Check Logs for ID Mismatch Warnings
Look for logs like:
```
"Execution Service returned different ID than requested"
```

## Testing the Fix

1. Deploy with debug logging and warning mode enabled
2. Monitor logs for execution ID mismatch warnings
3. Check if the Execution Service is returning wrong IDs
4. Verify message processing continues
5. Investigate the root cause in the Execution Service if IDs don't match

## Configuration Examples

### Development/Testing
```yaml
validation:
  skip_execution_id_validation: true
  max_message_age_minutes: 0  # No age limit
  warn_on_validation_failures: true
```

### Production (Strict)
```yaml
validation:
  skip_execution_id_validation: false
  max_message_age_minutes: 30
  warn_on_validation_failures: false
```

### Production (Recovery Mode)
```yaml
validation:
  skip_execution_id_validation: false
  max_message_age_minutes: 60
  warn_on_validation_failures: true
```

## Rollback Plan

If issues persist, revert to original configuration:
```yaml
# Remove or comment out the validation section
# validation:
#   skip_execution_id_validation: false
#   max_message_age_minutes: 60
#   warn_on_validation_failures: true
```

The service will use default strict validation.