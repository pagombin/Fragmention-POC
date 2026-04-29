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

	// Initialize Members with an empty (non-nil) slice so JSON marshalling
	// produces "[]" rather than "null". Browser code that does
	// `topology.members.map(...)` would otherwise crash on a null read.
	top := Topology{DetectedAt: time.Now().UTC(), Members: []Member{}}

	if msg, _ := hello["msg"].(string); strings.EqualFold(msg, "isdbgrid") {
		top.Kind = TopologyUnknown
		top.Sharded = true
		// Best-effort: enumerate shards via listShards. Managed sharded
		// clusters often restrict this; tolerate failure.
		if _, err := c.raw.Database("admin").ListCollections(ctx, bson.M{}); err == nil {
			var ls bson.M
			if err := admin.RunCommand(ctx, bson.D{{Key: "listShards", Value: 1}}).Decode(&ls); err == nil {
				if shards, ok := ls["shards"].(bson.A); ok {
					for _, s := range shards {
						sm, _ := s.(bson.M)
						top.Members = append(top.Members, Member{
							Name:   asString(sm["host"]),
							State:  "SHARD:" + asString(sm["_id"]),
							Health: 1,
						})
					}
				}
			}
		}
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

	// Decode replSetGetStatus into a typed struct. This avoids the fragile
	// interface{} assertions a `bson.M` decode would force - if the driver
	// returns a different concrete type for the inner array, our assertions
	// previously silently failed and we ended up with an empty members
	// slice even though the user's permissions were fine.
	type rsMember struct {
		ID             int32     `bson:"_id"`
		Name           string    `bson:"name"`
		StateStr       string    `bson:"stateStr"`
		Health         float64   `bson:"health"`
		Self           bool      `bson:"self"`
		OptimeDate     time.Time `bson:"optimeDate"`
		SyncSourceHost string    `bson:"syncSourceHost"`
	}
	type rsStatus struct {
		Members []rsMember `bson:"members"`
		Set     string     `bson:"set"`
		MyState int        `bson:"myState"`
	}
	var status rsStatus
	cmd := bson.D{{Key: "replSetGetStatus", Value: 1}}
	if err := admin.RunCommand(ctx, cmd).Decode(&status); err != nil {
		// Managed clusters (Atlas free tier, DigitalOcean managed)
		// restrict replSetGetStatus to roles the application user
		// often doesn't have. Fall back to the host list `hello`
		// returns - we lose lag/optime/health detail but keep names
		// and primary identification so the UI is useful.
		top.Members = membersFromHello(hello, top.Primary)
		return top, nil
	}

	if len(status.Members) == 0 {
		// Server allowed the command but returned no members. This is
		// abnormal; fall back so we still render something.
		top.Members = membersFromHello(hello, top.Primary)
		return top, nil
	}

	var primaryOptime time.Time
	for _, m := range status.Members {
		mem := Member{
			ID:        int(m.ID),
			Name:      m.Name,
			State:     m.StateStr,
			Health:    m.Health,
			Self:      m.Self,
			Optime:    m.OptimeDate.UTC(),
			SyncingTo: m.SyncSourceHost,
		}
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

// membersFromHello derives a minimal member list from `hello` output when
// `replSetGetStatus` is restricted (managed clusters). Health and optime
// are not available; we synthesise PRIMARY / SECONDARY based on
// hello.primary and hello.me.
func membersFromHello(hello bson.M, primary string) []Member {
	hosts, _ := hello["hosts"].(bson.A)
	if len(hosts) == 0 {
		return []Member{}
	}
	me, _ := hello["me"].(string)
	out := make([]Member, 0, len(hosts))
	for _, h := range hosts {
		name, _ := h.(string)
		state := "SECONDARY"
		if name == primary {
			state = "PRIMARY"
		}
		out = append(out, Member{
			Name:   name,
			State:  state,
			Health: 1,
			Self:   name == me,
		})
	}
	return out
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
