package mongo

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Topology describes the shape of the target cluster as detected at connect
// time and on every refresh. A detected topology may be single-node (no
// replica set) or a replica set with 1..N members. Sharded clusters are
// flagged but explicitly unsupported per spec § 18.
type Topology struct {
	Kind         TopologyKind `json:"kind"`
	ReplicaSet   string       `json:"replica_set,omitempty"`
	Primary      string       `json:"primary,omitempty"`
	Members      []Member     `json:"members"`
	DetectedAt   time.Time    `json:"detected_at"`
	Sharded      bool         `json:"sharded"`
	MaxReplicaLag time.Duration `json:"max_replica_lag_ns"`
}

// TopologyKind enumerates the detectable cluster shapes.
type TopologyKind string

// TopologyKind values.
const (
	TopologyStandalone TopologyKind = "standalone"
	TopologyReplicaSet TopologyKind = "replica_set"
	TopologyUnknown    TopologyKind = "unknown"
)

// Member is a single replica-set participant.
type Member struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	State      string    `json:"state"`
	Health     float64   `json:"health"`
	Self       bool      `json:"self"`
	Optime     time.Time `json:"optime"`
	SyncingTo  string    `json:"syncing_to,omitempty"`
	LagSeconds float64   `json:"lag_seconds"`
}

// DetectTopology classifies the target cluster. For replica sets it pulls
// `rs.status()` and reports per-member state and lag. For standalones it
// returns a one-member topology. Sharded clusters (where `hello` reports a
// `mongos` role) are returned with Sharded=true and the caller should refuse
// to proceed.
func (c *Client) DetectTopology(ctx context.Context) (Topology, error) {
	admin := c.raw.Database("admin")

	var hello bson.M
	if err := admin.RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		// Fall back for pre-5.0 wire protocol (we refuse those anyway).
		if err := admin.RunCommand(ctx, bson.D{{Key: "isMaster", Value: 1}}).Decode(&hello); err != nil {
			return Topology{}, fmt.Errorf("hello: %w", err)
		}
	}

	top := Topology{DetectedAt: time.Now().UTC()}

	if msg, _ := hello["msg"].(string); strings.EqualFold(msg, "isdbgrid") {
		top.Kind = TopologyUnknown
		top.Sharded = true
		return top, nil
	}

	rsName, hasRS := hello["setName"].(string)
	if !hasRS || rsName == "" {
		top.Kind = TopologyStandalone
		host, _ := hello["me"].(string)
		top.Members = []Member{{Name: host, State: "STANDALONE", Health: 1, Self: true}}
		return top, nil
	}

	top.Kind = TopologyReplicaSet
	top.ReplicaSet = rsName
	if primary, ok := hello["primary"].(string); ok {
		top.Primary = primary
	}

	var status bson.M
	if err := admin.RunCommand(ctx, bson.D{{Key: "replSetGetStatus", Value: 1}}).Decode(&status); err != nil {
		// Still return the minimal topology; the caller can choose to continue.
		return top, fmt.Errorf("replSetGetStatus: %w", err)
	}

	members, _ := status["members"].(bson.A)
	var primaryOptime time.Time
	for _, raw := range members {
		m, ok := raw.(bson.M)
		if !ok {
			continue
		}
		mem := memberFromStatus(m)
		if strings.EqualFold(mem.State, "PRIMARY") {
			primaryOptime = mem.Optime
		}
		top.Members = append(top.Members, mem)
	}
	// Second pass: compute lag against primary optime.
	var maxLag time.Duration
	for i := range top.Members {
		if strings.EqualFold(top.Members[i].State, "PRIMARY") || primaryOptime.IsZero() {
			continue
		}
		lag := primaryOptime.Sub(top.Members[i].Optime)
		if lag < 0 {
			lag = 0
		}
		top.Members[i].LagSeconds = lag.Seconds()
		if lag > maxLag {
			maxLag = lag
		}
	}
	top.MaxReplicaLag = maxLag
	return top, nil
}

func memberFromStatus(m bson.M) Member {
	mem := Member{
		Name:   asString(m["name"]),
		State:  asString(m["stateStr"]),
		Health: asFloat(m["health"]),
	}
	if v, ok := m["_id"].(int32); ok {
		mem.ID = int(v)
	} else if v, ok := m["_id"].(int64); ok {
		mem.ID = int(v)
	}
	if v, ok := m["self"].(bool); ok {
		mem.Self = v
	}
	if v, ok := m["syncSourceHost"].(string); ok {
		mem.SyncingTo = v
	} else if v, ok := m["syncingTo"].(string); ok {
		mem.SyncingTo = v
	}
	if ot, ok := m["optimeDate"].(bson.DateTime); ok {
		mem.Optime = ot.Time().UTC()
	} else if ot, ok := m["optimeDate"].(time.Time); ok {
		mem.Optime = ot.UTC()
	}
	return mem
}

func asFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	}
	return 0
}
