package ocsnowflake

import (
    "encoding/json"
    "fmt"
    "sync"
    "testing"
    "time"
)

func TestNewNodeValidation(t *testing.T) {
    if _, err := NewNode(-1); err == nil {
        t.Fatal("expected error for negative node id")
    }
    if _, err := NewNode(2048); err == nil {
        t.Fatal("expected error for oversized node id")
    }

    if node, err := NewNode(42); err != nil {
        t.Fatalf("unexpected error for valid node: %v", err)
    } else if node.node != 42 {
        t.Fatalf("node id mismatch want 42 got %d", node.node)
    }
}

func TestGenerateMonotonic(t *testing.T) {
    node, err := NewNode(1)
    if err != nil {
        t.Fatalf("failed to create node: %v", err)
    }

    var last ID
    for i := 0; i < 100; i++ {
        id := node.Generate()
        if i > 0 && id <= last {
            t.Fatalf("ids not strictly increasing at index %d", i)
        }
        last = id
    }
}

func TestGenerateBatch(t *testing.T) {
    node, err := NewNode(2)
    if err != nil {
        t.Fatalf("failed to create node: %v", err)
    }

    ids := node.GenerateBatch(256)
    if len(ids) != 256 {
        t.Fatalf("expected 256 ids got %d", len(ids))
    }

    for i := 1; i < len(ids); i++ {
        if ids[i] <= ids[i-1] {
            t.Fatalf("batch ids not monotonic at %d", i)
        }
    }
}

func TestGenerateBatchPanicsOnInvalidCount(t *testing.T) {
    node, err := NewNode(3)
    if err != nil {
        t.Fatalf("failed to create node: %v", err)
    }

    defer func() {
        if r := recover(); r == nil {
            t.Fatal("expected panic for invalid batch count")
        }
    }()

    node.GenerateBatch(0)
}

func TestHelpersAndJSON(t *testing.T) {
    node, err := NewNode(4)
    if err != nil {
        t.Fatalf("failed to create node: %v", err)
    }

    original := node.Generate()

    if got := ParseUint64(original.Uint64()); got != original {
        t.Fatal("ParseUint64 mismatch")
    }

    if got, err := ParseString(original.String()); err != nil || got != original {
        t.Fatalf("ParseString mismatch err=%v", err)
    }

    if got, err := ParseBytes(original.Bytes()); err != nil || got != original {
        t.Fatalf("ParseBytes mismatch err=%v", err)
    }

    ts := original.EpochTimeInt64()
    if ts < Epoch {
        t.Fatalf("timestamp before epoch: %d", ts)
    }

    if original.Node() != 4 {
        t.Fatalf("expected node=4 got %d", original.Node())
    }

    data, err := json.Marshal(original)
    if err != nil {
        t.Fatalf("marshal failed: %v", err)
    }

    var decoded ID
    if err := json.Unmarshal(data, &decoded); err != nil {
        t.Fatalf("unmarshal failed: %v", err)
    }

    if decoded != original {
        t.Fatal("json roundtrip mismatch")
    }
}

func TestEpochOverride(t *testing.T) {
    node, err := NewNode(5)
    if err != nil {
        t.Fatalf("failed to create node: %v", err)
    }

    prevEpoch := Epoch
    Epoch = time.Now().Add(-time.Hour).UnixMilli()
    defer func() { Epoch = prevEpoch }()

    id := node.Generate()
    ts := id.EpochTimeInt64()
    if ts < Epoch {
        t.Fatalf("expected timestamp >= epoch, got %d", ts)
    }
}

func BenchmarkGenerate(b *testing.B) {
    node, err := NewNode(6)
    if err != nil {
        b.Fatalf("failed to create node: %v", err)
    }

    b.ReportAllocs()
    b.ResetTimer()

    for i := 0; i < b.N; i++ {
        _ = node.Generate()
    }

    reportIDsPerMs(b, 1)
}

func BenchmarkGenerateBatch(b *testing.B) {
    sizes := []int{16, 64, 256, 1024}

    for _, size := range sizes {
        size := size
        b.Run(fmt.Sprintf("n=%d", size), func(b *testing.B) {
            node, err := NewNode(7)
            if err != nil {
                b.Fatalf("failed to create node: %v", err)
            }

            b.ReportAllocs()
            b.ResetTimer()

            for i := 0; i < b.N; i++ {
                ids := node.GenerateBatch(size)
                if len(ids) != size {
                    b.Fatalf("expected %d ids got %d", size, len(ids))
                }
            }

            reportIDsPerMs(b, size)
        })
    }
}

func BenchmarkGenerateUncapped(b *testing.B) {
    fn := &uncappedRealisticNode{node: 8}

    b.ReportAllocs()
    b.ResetTimer()

    for i := 0; i < b.N; i++ {
        _ = fn.Generate()
    }

    reportIDsPerMs(b, 1)
}

func BenchmarkGenerateBatchUncapped(b *testing.B) {
    sizes := []int{16, 64, 256, 1024}

    for _, size := range sizes {
        size := size
        b.Run(fmt.Sprintf("n=%d", size), func(b *testing.B) {
            node := &uncappedRealisticNode{node: 9}

            b.ReportAllocs()
            b.ResetTimer()

            for i := 0; i < b.N; i++ {
                ids := node.generateBatch(size)
                if len(ids) != size {
                    b.Fatalf("expected %d ids got %d", size, len(ids))
                }
            }

            reportIDsPerMs(b, size)
        })
    }
}

type uncappedRealisticNode struct {
    mu        sync.Mutex
    node      int64
    timestamp int64
    sequence  int64
}

func (n *uncappedRealisticNode) Generate() ID {
    return n.generateBatch(1)[0]
}

func (n *uncappedRealisticNode) generateBatch(count int) []ID {
    n.mu.Lock()
    defer n.mu.Unlock()

    ids := make([]ID, count)
    for i := range ids {
        if n.sequence > maxSequence {
            n.sequence = 0
            n.timestamp++
        }
        ids[i] = composeID(n.timestamp, n.node, n.sequence)
        n.sequence++
    }
    return ids
}

func reportIDsPerMs(b *testing.B, idsPerOp int) {
    elapsed := b.Elapsed()
    if elapsed <= 0 {
        return
    }
    totalIDs := float64(idsPerOp * b.N)
    idsPerMs := totalIDs / (float64(elapsed) / float64(time.Millisecond))
    b.ReportMetric(idsPerMs, "ids/ms")
}
