package desiredstate

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestWorkByteFieldAccountingMatchesExactRetainedJSON(t *testing.T) {
	b := NewBroadcaster()
	ctx := b.BeginOperation(context.Background(), "deployment", "test", "web", "")
	id := b.BeginWork(ctx, WorkOptions{Source: "reconcile", Stack: "web", Phase: "pulling"})
	p := WorkPullProgress{Image: "image", LayerID: "layer", Status: "Downloading", BytesCurrent: 1}
	b.WorkPullProgress(id, p)
	for _, value := range []int64{0, 9, 10, 999, 9007199254740991} {
		p.BytesCurrent, p.BytesTotal = value, value
		b.WorkPullProgress(id, p)
		b.mu.Lock()
		exact := 0
		for _, run := range b.work.runs {
			exact += detailBytes(run)
		}
		if exact != b.work.detailBytes || exact > WorkDetailBudget {
			t.Fatalf("incorrect incremental JSON accounting: tracked=%d exact=%d", b.work.detailBytes, exact)
		}
		b.mu.Unlock()
	}
	b.EndWork(id, "completed", "done", "")
	if b.work.detailBytes != 0 {
		t.Fatal("terminal retained accounting", b.work.detailBytes)
	}
}

func TestWorkByteGrowthEvictsBeforeDeliveryAtExactBudget(t *testing.T) {
	b := NewBroadcaster()
	ctx := b.BeginOperation(context.Background(), "deployment", "test", "web", "")
	id := b.BeginWork(ctx, WorkOptions{Source: "reconcile", Stack: "web", Phase: "pulling"})
	p := WorkPullProgress{Image: "image", LayerID: "layer", Status: "Downloading", BytesCurrent: 1}
	b.WorkPullProgress(id, p)
	b.mu.Lock()
	run := b.work.runs[id]
	run.Images[0].Layers[0].ID += strings.Repeat("x", WorkDetailBudget-detailBytes(run))
	p.LayerID = run.Images[0].Layers[0].ID
	// Zero nanoseconds gives subsequent timestamps non-negative size growth.
	run.Images[0].Layers[0].UpdatedAt = time.Now().UTC().Truncate(time.Second)
	run.Images[0].Layers[0].ID += strings.Repeat("x", WorkDetailBudget-detailBytes(run))
	p.LayerID = run.Images[0].Layers[0].ID
	b.boundWorkDetailsLocked()
	if b.work.detailBytes != WorkDetailBudget {
		t.Fatal("fixture not at exact cap", b.work.detailBytes)
	}
	run.lastSent = time.Now().Add(time.Hour)
	b.mu.Unlock()
	p.BytesCurrent, p.BytesTotal = 9007199254740991, 9007199254740991
	b.WorkPullProgress(id, p)
	b.mu.Lock()
	if b.work.detailBytes > WorkDetailBudget || detailBytes(run) != b.work.detailBytes || len(run.Images[0].Layers) != 0 || !run.DetailsTruncated || run.Images[0].LayerCount != 1 {
		t.Fatal("byte growth exceeded retention cap or lost observed count")
	}
	b.mu.Unlock()
	snapshot := workSnapshot(t, b)
	if len(snapshot.Runs) != 1 || !snapshot.DetailsTruncated {
		t.Fatal("snapshot lost truncation")
	}
	b.EndWork(id, "completed", "done", "")
}

func benchmarkWorkByteTicks(b *testing.B, large bool) {
	broadcaster := NewBroadcaster()
	ctx := broadcaster.BeginOperation(context.Background(), "deployment", "test", "web", "")
	id := broadcaster.BeginWork(ctx, WorkOptions{Source: "reconcile", Stack: "web", Phase: "pulling"})
	layerID := "layer"
	if large {
		layerID = strings.Repeat("x", 1800*1024)
	}
	p := WorkPullProgress{Image: "image", LayerID: layerID, Status: "Downloading", BytesCurrent: 1, BytesTotal: 9007199254740991}
	broadcaster.WorkPullProgress(id, p)
	broadcaster.mu.Lock()
	broadcaster.work.runs[id].lastSent = time.Now().Add(time.Hour)
	broadcaster.mu.Unlock()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.BytesCurrent = int64(i % 1000000)
		broadcaster.WorkPullProgress(id, p)
	}
	b.StopTimer()
	broadcaster.EndWork(id, "completed", "done", "")
}

func BenchmarkWorkByteTickSmall(b *testing.B)   { benchmarkWorkByteTicks(b, false) }
func BenchmarkWorkByteTick1800KiB(b *testing.B) { benchmarkWorkByteTicks(b, true) }
