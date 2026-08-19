# Solution

## 1. Duplicate Webhook Deliveries / Idempotency

### What was broken

The webhook provider delivers events at least once. The same event can therefore
be delivered multiple times, including concurrent deliveries.

The original implementation first checked whether an event existed using
`EventExists()` and then inserted it:

1. Check whether `event_id` exists.
2. If it does not exist, insert the event.
3. Update the call.
4. Increment account statistics.

This created a race condition.

If two identical webhook requests arrived at approximately the same time, both
requests could execute the existence check before either request inserted the
event. Both would therefore believe that the event was new and continue
processing it.

This could result in duplicate event records and account statistics being
incremented more than once.

### Fix

PostgreSQL is now used as the source of truth for webhook deduplication.

The `events.event_id` column has a `UNIQUE` constraint:

```sql
event_id TEXT NOT NULL UNIQUE


## Bug 2: Recording Processing Was Not Reliable

### What was broken

Recording processing was started asynchronously using the HTTP request context. When the webhook request finished or its context was cancelled, the background recording task could also be cancelled before `recording_processed` was updated. Errors from the background task were also silently ignored.

### Fix

Recording processing was changed to run with a context independent of the webhook request lifecycle. This allows the background task to complete after the webhook has returned a successful response.

A regression test, `TestRecordingIsMarkedProcessed`, was added to verify that a webhook containing a recording eventually sets `recording_processed = TRUE`.

### Verification

The targeted test passes:

```bash
go test ./internal/ingest -run TestRecordingIsMarkedProcessed -count=1
```

## Bug 3: In-flight Recording Work Was Lost During Deployment

### What was broken

Recording processing was started in background goroutines. During deployment, the HTTP server performed a graceful shutdown, but those background goroutines were not tracked.

`http.Server.Shutdown()` waits for active HTTP handlers, but it does not wait for unrelated goroutines. As a result, the process could exit while recording processing was still running, causing in-flight work to disappear.

### Fix

The ingestion service now tracks active recording-processing jobs using a `sync.WaitGroup`.

Each recording job increments the wait group before starting and decrements it when finished. A new `WaitForInflight()` method waits for all active recording jobs to complete or until the shutdown context expires.

During shutdown, the server now:

1. Stops accepting new HTTP requests.
2. Waits for active HTTP handlers to finish.
3. Waits for in-flight recording processing to complete.
4. Exits the process.

A regression test, `TestWaitForInflightFinishesRecordingWork`, verifies that shutdown waits for recording processing and that the recording is marked processed before the service finishes shutting down.

### Verification

The targeted test passes:

```bash
go test ./internal/ingest -run TestWaitForInflightFinishesRecordingWork -count=1

