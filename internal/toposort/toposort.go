// Package toposort provides a topological sort for segments/flags based on their dependencies.
package toposort

import (
	"sort"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	st "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// AdjacencyList is a map of vertices (kind/key) to neighbors (dependencies).
type AdjacencyList map[Vertex]Neighbors

// Neighbors is a set of vertices. It is used instead of a list for efficient lookup.
type Neighbors map[Vertex]struct{}

// Add adds a vertex to the set.
func (s Neighbors) Add(value Vertex) {
	s[value] = struct{}{}
}

// Contains returns true if the set contains the vertex.
func (s Neighbors) Contains(value Vertex) bool {
	_, ok := s[value]
	return ok
}

// Vertex represents a particular data item, identified by kind + key.
type Vertex struct {
	kind st.DataKind
	key  string
}

// NewVertex constructs a Vertex.
func NewVertex(kind st.DataKind, key string) Vertex {
	return Vertex{kind, key}
}

// Kind returns the data kind of the vertex.
func (v Vertex) Kind() st.DataKind {
	return v.kind
}

// Key returns the key of the vertex.
func (v Vertex) Key() string {
	return v.key
}

func doesDataKindSupportDependencies(kind st.DataKind) bool {
	return kind == datakinds.Features //nolint:megacheck
}

// Logic for ensuring that segments are processed before features; if we get any other data types that
// haven't been accounted for here, they'll come after those two in an arbitrary order.
func dataKindPriority(kind st.DataKind) int {
	switch kind.GetName() {
	case "segments":
		return 0
	case "features":
		return 1
	default:
		return len(kind.GetName()) + 2
	}
}

func addItemsInDependencyOrder(
	kind st.DataKind,
	itemsIn []st.KeyedItemDescriptor,
	out *[]st.KeyedItemDescriptor,
) {
	remainingItems := make(map[string]st.ItemDescriptor, len(itemsIn))
	for _, item := range itemsIn {
		remainingItems[item.Key] = item.Item
	}
	for len(remainingItems) > 0 {
		// pick a random item that hasn't been visited yet
		for firstKey := range remainingItems {
			addWithDependenciesFirst(kind, firstKey, remainingItems, out)
			break
		}
	}
}

func addWithDependenciesFirst(
	kind st.DataKind,
	startingKey string,
	remainingItems map[string]st.ItemDescriptor,
	out *[]st.KeyedItemDescriptor,
) {
	startItem := remainingItems[startingKey]
	delete(remainingItems, startingKey) // we won't need to visit this item again
	for dep := range GetNeighbors(kind, startItem) {
		if dep.kind == kind {
			if _, ok := remainingItems[dep.key]; ok {
				addWithDependenciesFirst(kind, dep.key, remainingItems, out)
			}
		}
	}
	*out = append(*out, st.KeyedItemDescriptor{Key: startingKey, Item: startItem})
}

// GetNeighbors returns all direct neighbors of the given item.
func GetNeighbors(kind st.DataKind, fromItem st.ItemDescriptor) Neighbors {
	// For any given flag or segment, find all the flags/segments that it directly references.
	// Transitive references are handled by recursive logic at a higher level.
	var ret Neighbors
	checkClauses := func(clauses []ldmodel.Clause) {
		for _, c := range clauses {
			if c.Op == ldmodel.OperatorSegmentMatch {
				for _, v := range c.Values {
					if v.Type() == ldvalue.StringType {
						if ret == nil {
							ret = make(Neighbors)
						}
						ret.Add(Vertex{datakinds.Segments, v.StringValue()})
					}
				}
			}
		}
	}
	switch kind {
	case datakinds.Features:
		if flag, ok := fromItem.Item.(*ldmodel.FeatureFlag); ok {
			if len(flag.Prerequisites) > 0 {
				ret = make(Neighbors, len(flag.Prerequisites))
				for _, p := range flag.Prerequisites {
					ret.Add(Vertex{datakinds.Features, p.Key})
				}
			}
			for _, r := range flag.Rules {
				checkClauses(r.Clauses)
			}
			return ret
		}

	case datakinds.Segments:
		if segment, ok := fromItem.Item.(*ldmodel.Segment); ok {
			for _, r := range segment.Rules {
				checkClauses(r.Clauses)
			}
		}
	}
	return ret
}

// Sort performs a topological sort on the given data collections, so that the items can be inserted into a
// persistent store to minimize the risk of evaluating a flag before its prerequisites/segments have been stored.
func Sort(allData []st.Collection) []st.Collection {
	collections := make([]st.Collection, 0, len(allData))
	for _, coll := range allData {
		if doesDataKindSupportDependencies(coll.Kind) {
			itemsOut := make([]st.KeyedItemDescriptor, 0, len(coll.Items))
			addItemsInDependencyOrder(coll.Kind, coll.Items, &itemsOut)
			collections = append(collections, st.Collection{Kind: coll.Kind, Items: itemsOut})
		} else {
			collections = append(collections, coll)
		}
	}
	sort.Slice(collections, func(i, j int) bool {
		return dataKindPriority(collections[i].Kind) < dataKindPriority(collections[j].Kind)
	})
	return collections
}
