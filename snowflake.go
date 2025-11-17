package ocsnowflake

import (
    "errors"
    "strconv"
    "strings"
    "sync"
    "time"
)

const (
    nodeBits     = 10
    sequenceBits = 12

    maxNode     = -1 ^ (-1 << nodeBits)
    maxSequence = -1 ^ (-1 << sequenceBits)

    nodeShift = sequenceBits
    timeShift = nodeBits + sequenceBits
)

var (
    // ErrInvalidNode is returned when a node ID falls outside the allowed range [0, 1023].
    ErrInvalidNode = errors.New("ocsnowflake: invalid node id")
    // ErrInvalidBatchCount is returned when GenerateBatch is called with a non-positive count.
    ErrInvalidBatchCount = errors.New("ocsnowflake: batch count must be positive")
)

// Epoch defines the reference epoch for generated Snowflake IDs (milliseconds since Unix epoch).
// The default matches 2025-10-10 10:10:10 UTC.
var Epoch int64 = 1760091010000

// ID represents a Snowflake identifier encoded as an unsigned 64-bit integer.
type ID uint64

// Node generates Snowflake IDs that embed node and sequence information.
type Node struct {
    mu            sync.Mutex
    node          int64
    lastTimestamp int64
    sequence      int64
}

// NewNode constructs a Node bound to the provided node ID. Valid IDs range from 0 to 1023.
func NewNode(nodeID int64) (*Node, error) {
    if nodeID < 0 || nodeID > maxNode {
        return nil, ErrInvalidNode
    }

    return &Node{
        node:          nodeID,
        lastTimestamp: -1,
    }, nil
}

// Generate returns a single Snowflake ID.
func (n *Node) Generate() ID {
    ids := n.GenerateBatch(1)
    return ids[0]
}

// GenerateBatch creates count Snowflake IDs under a single critical section for optimal throughput.
// A non-positive count results in a panic because it indicates a programming error.
func (n *Node) GenerateBatch(count int) []ID {
    if count <= 0 {
        panic(ErrInvalidBatchCount)
    }

    n.mu.Lock()
    defer n.mu.Unlock()

    ids := make([]ID, count)
    for i := 0; i < count; i++ {
        timestamp := currentTimestamp()

        if timestamp < n.lastTimestamp {
            timestamp = n.waitNextMillis(n.lastTimestamp)
        }

        if timestamp == n.lastTimestamp {
            n.sequence++
            if n.sequence > maxSequence {
                timestamp = n.waitNextMillis(timestamp)
                n.sequence = 0
            }
        } else {
            n.sequence = 0
        }

        n.lastTimestamp = timestamp
        ids[i] = composeID(timestamp, n.node, n.sequence)
    }

    return ids
}

// Uint64 returns the raw uint64 value.
func (id ID) Uint64() uint64 {
    return uint64(id)
}

// String returns a decimal string representation of the ID.
func (id ID) String() string {
    return strconv.FormatUint(uint64(id), 10)
}

// Bytes returns the ID encoded as a decimal string byte slice.
func (id ID) Bytes() []byte {
    return []byte(id.String())
}

// EpochTimeInt64 returns the embedded timestamp as milliseconds since the Unix epoch.
func (id ID) EpochTimeInt64() int64 {
    timestamp := int64(id >> timeShift)
    return timestamp + Epoch
}

// EpochTimeString returns the timestamp as a decimal string.
func (id ID) EpochTimeString() string {
    return strconv.FormatInt(id.EpochTimeInt64(), 10)
}

// Node extracts the node component from the ID.
func (id ID) Node() int64 {
    return int64(id>>nodeShift) & maxNode
}

// Sequence extracts the sequence bits from the ID.
func (id ID) Sequence() int64 {
    return int64(id) & maxSequence
}

// MarshalJSON encodes the ID as a quoted decimal string to remain safe for JavaScript environments.
func (id ID) MarshalJSON() ([]byte, error) {
    buf := make([]byte, 0, 22)
    buf = append(buf, '"')
    buf = strconv.AppendUint(buf, uint64(id), 10)
    buf = append(buf, '"')
    return buf, nil
}

// UnmarshalJSON decodes a quoted decimal string (or null) into an ID.
func (id *ID) UnmarshalJSON(b []byte) error {
    if string(b) == "null" {
        *id = 0
        return nil
    }

    if len(b) < 2 || b[0] != '"' || b[len(b)-1] != '"' {
        return errors.New("ocsnowflake: json value must be a quoted string")
    }

    value, err := strconv.ParseUint(string(b[1:len(b)-1]), 10, 64)
    if err != nil {
        return err
    }

    *id = ID(value)
    return nil
}

// ParseUint64 converts a uint64 into an ID.
func ParseUint64(u uint64) ID {
    return ID(u)
}

// ParseString parses a decimal string into an ID.
func ParseString(s string) (ID, error) {
    v, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
    if err != nil {
        return 0, err
    }
    return ID(v), nil
}

// ParseBytes parses a byte slice containing a decimal string into an ID.
func ParseBytes(b []byte) (ID, error) {
    v, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
    if err != nil {
        return 0, err
    }
    return ID(v), nil
}

func (n *Node) waitNextMillis(last int64) int64 {
    timestamp := currentTimestamp()
    for timestamp <= last {
        timestamp = currentTimestamp()
    }
    return timestamp
}

func currentTimestamp() int64 {
    ts := time.Now().UnixMilli() - Epoch
    if ts < 0 {
        return 0
    }
    return ts
}

func composeID(timestamp, node, sequence int64) ID {
    return ID((uint64(timestamp) << timeShift) | (uint64(node) << nodeShift) | uint64(sequence))
}
