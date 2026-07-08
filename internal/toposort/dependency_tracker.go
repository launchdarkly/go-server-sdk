package toposort

import (
	st "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// DependencyTracker maintains a bidirectional dependency graph that can be updated whenever an
// item has changed.
//
// A nearly identical unexported implementation exists in internal/datasource for the FDv1 code
// path; that copy will go away when FDv1 is removed.
type DependencyTracker struct {
	dependenciesFrom AdjacencyList
	dependenciesTo   AdjacencyList
}

// NewDependencyTracker creates a DependencyTracker with an empty dependency graph.
func NewDependencyTracker() *DependencyTracker {
	return &DependencyTracker{
		make(AdjacencyList),
		make(AdjacencyList),
	}
}

// UpdateDependenciesFrom updates the dependency graph when an item has changed.
func (d *DependencyTracker) UpdateDependenciesFrom(
	kind st.DataKind,
	fromKey string,
	fromItem st.ItemDescriptor,
) {
	fromWhat := NewVertex(kind, fromKey)
	updatedDependencies := GetNeighbors(kind, fromItem)

	oldDependencySet := d.dependenciesFrom[fromWhat]
	for oldDep := range oldDependencySet {
		depsToThisOldDep := d.dependenciesTo[oldDep]
		if depsToThisOldDep != nil {
			delete(depsToThisOldDep, fromWhat)
		}
	}

	d.dependenciesFrom[fromWhat] = updatedDependencies
	for newDep := range updatedDependencies {
		depsToThisNewDep := d.dependenciesTo[newDep]
		if depsToThisNewDep == nil {
			depsToThisNewDep = make(Neighbors)
			d.dependenciesTo[newDep] = depsToThisNewDep
		}
		depsToThisNewDep.Add(fromWhat)
	}
}

// Reset clears the dependency graph.
func (d *DependencyTracker) Reset() {
	d.dependenciesFrom = make(AdjacencyList)
	d.dependenciesTo = make(AdjacencyList)
}

// AddAffectedItems populates the given set with the union of the initial item and all items that
// directly or indirectly depend on it (based on the current state of the dependency graph).
func (d *DependencyTracker) AddAffectedItems(itemsOut Neighbors, initialModifiedItem Vertex) {
	if !itemsOut.Contains(initialModifiedItem) {
		itemsOut.Add(initialModifiedItem)
		affectedItems := d.dependenciesTo[initialModifiedItem]
		for affectedItem := range affectedItems {
			d.AddAffectedItems(itemsOut, affectedItem)
		}
	}
}
