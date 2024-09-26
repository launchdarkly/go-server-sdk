package datasource

import (
	"github.com/launchdarkly/go-server-sdk/v7/internal/toposort"
	st "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// Maintains a bidirectional dependency graph that can be updated whenever an item has changed.
type dependencyTracker struct {
	dependenciesFrom toposort.AdjacencyList
	dependenciesTo   toposort.AdjacencyList
}

func newDependencyTracker() *dependencyTracker {
	return &dependencyTracker{
		make(toposort.AdjacencyList),
		make(toposort.AdjacencyList),
	}
}

// Updates the dependency graph when an item has changed.
func (d *dependencyTracker) updateDependenciesFrom(
	kind st.DataKind,
	fromKey string,
	fromItem st.ItemDescriptor,
) {
	fromWhat := toposort.NewVertex(kind, fromKey)
	updatedDependencies := toposort.GetNeighbors(kind, fromItem)

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
			depsToThisNewDep = make(toposort.Neighbors)
			d.dependenciesTo[newDep] = depsToThisNewDep
		}
		depsToThisNewDep.Add(fromWhat)
	}
}

func (d *dependencyTracker) reset() {
	d.dependenciesFrom = make(toposort.AdjacencyList)
	d.dependenciesTo = make(toposort.AdjacencyList)
}

// Populates the given set with the union of the initial item and all items that directly or indirectly
// depend on it (based on the current state of the dependency graph).
func (d *dependencyTracker) addAffectedItems(itemsOut toposort.Neighbors, initialModifiedItem toposort.Vertex) {
	if !itemsOut.Contains(initialModifiedItem) {
		itemsOut.Add(initialModifiedItem)
		affectedItems := d.dependenciesTo[initialModifiedItem]
		for affectedItem := range affectedItems {
			d.addAffectedItems(itemsOut, affectedItem)
		}
	}
}
