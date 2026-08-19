package store_test

import (
	"context"
	"testing"

	"github.com/convin/webhook-ingest/internal/store"
	"github.com/convin/webhook-ingest/internal/testutil"
)

func TestInsertEventThenExists(t *testing.T) {
	s := testutil.NewStore(t)
	eventID, callID, accountID := testutil.IDs(t, s)
	ctx := context.Background()

	evt := store.Event{
		EventID: eventID, CallID: callID, AccountID: accountID,
		Status: "completed", DurationSec: 10, Payload: []byte(`{}`),
	}

	exists, err := s.EventExists(ctx, eventID)
	if err != nil {
		t.Fatalf("EventExists: %v", err)
	}
	if exists {
		t.Fatal("expected event to be absent before insert")
	}

	inserted, err := s.InsertEvent(ctx, evt)
	if err != nil {
		t.Fatalf("InsertEvent: %v", err)
	}
	if !inserted {
		t.Fatal("expected first insert to report inserted=true")
	}

	exists, err = s.EventExists(ctx, eventID)
	if err != nil {
		t.Fatalf("EventExists: %v", err)
	}
	if !exists {
		t.Fatal("expected event to exist after insert")
	}
}

func TestDuplicateInsertEventIsIgnored(t *testing.T) {
	s := testutil.NewStore(t)
	eventID, callID, accountID := testutil.IDs(t, s)
	ctx := context.Background()

	evt := store.Event{
		EventID: eventID, CallID: callID, AccountID: accountID,
		Status: "completed", DurationSec: 10, Payload: []byte(`{}`),
	}

	inserted, err := s.InsertEvent(ctx, evt)
	if err != nil {
		t.Fatalf("first InsertEvent: %v", err)
	}
	if !inserted {
		t.Fatal("expected first insert to report inserted=true")
	}

	inserted, err = s.InsertEvent(ctx, evt)
	if err != nil {
		t.Fatalf("duplicate InsertEvent: %v", err)
	}
	if inserted {
		t.Fatal("expected duplicate insert to report inserted=false")
	}

	var count int
	err = s.Pool().QueryRow(
		ctx,
		`SELECT COUNT(*) FROM events WHERE event_id = $1`,
		eventID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count events: %v", err)
	}

	if count != 1 {
		t.Fatalf("stored %d events, want 1", count)
	}
}

func TestIncrementAccountStatsAccumulates(t *testing.T) {
	s := testutil.NewStore(t)
	_, _, accountID := testutil.IDs(t, s)
	ctx := context.Background()

	if err := s.IncrementAccountStats(ctx, accountID, 30); err != nil {
		t.Fatalf("IncrementAccountStats: %v", err)
	}
	if err := s.IncrementAccountStats(ctx, accountID, 12); err != nil {
		t.Fatalf("IncrementAccountStats: %v", err)
	}

	got, err := s.AccountStats(ctx, accountID)
	if err != nil {
		t.Fatalf("AccountStats: %v", err)
	}
	if got.CallCount != 2 || got.TotalDurationSec != 42 {
		t.Fatalf("got %+v, want CallCount=2 TotalDurationSec=42", got)
	}
}

func TestUpsertCallThenMarkRecordingProcessed(t *testing.T) {
	s := testutil.NewStore(t)
	eventID, callID, accountID := testutil.IDs(t, s)
	ctx := context.Background()

	evt := store.Event{
		EventID: eventID, CallID: callID, AccountID: accountID,
		Status: "completed", DurationSec: 10,
		RecordingURL: "https://example.com/a.wav", Payload: []byte(`{}`),
	}
	if err := s.UpsertCall(ctx, evt); err != nil {
		t.Fatalf("UpsertCall: %v", err)
	}
	if err := s.MarkRecordingProcessed(ctx, callID); err != nil {
		t.Fatalf("MarkRecordingProcessed: %v", err)
	}

	var processed bool
	row := s.Pool().QueryRow(
		ctx,
		`SELECT recording_processed FROM calls WHERE call_id = $1`,
		callID,
	)
	if err := row.Scan(&processed); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !processed {
		t.Fatal("expected recording_processed to be true")
	}
}
func TestPendingRecordings(t *testing.T) {
	s := testutil.NewStore(t)
	eventID, callID, accountID := testutil.IDs(t, s)
	ctx := context.Background()

	evt := store.Event{
		EventID:      eventID,
		CallID:       callID,
		AccountID:    accountID,
		Status:       "completed",
		DurationSec:  10,
		RecordingURL: "https://example.com/test.wav",
		Payload:      []byte(`{}`),
	}

	if err := s.UpsertCall(ctx, evt); err != nil {
		t.Fatalf("UpsertCall: %v", err)
	}

	pending, err := s.PendingRecordings(ctx)
	if err != nil {
		t.Fatalf("PendingRecordings: %v", err)
	}

	if len(pending) != 1 {
		t.Fatalf("got %d pending recordings, want 1", len(pending))
	}

	if pending[0].CallID != callID {
		t.Fatalf("got call ID %q, want %q", pending[0].CallID, callID)
	}

	if err := s.MarkRecordingProcessed(ctx, callID); err != nil {
		t.Fatalf("MarkRecordingProcessed: %v", err)
	}

	pending, err = s.PendingRecordings(ctx)
	if err != nil {
		t.Fatalf("PendingRecordings after processing: %v", err)
	}

	if len(pending) != 0 {
		t.Fatalf("got %d pending recordings after processing, want 0", len(pending))
	}
}
