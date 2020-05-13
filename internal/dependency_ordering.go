package internal

import (
	"sort"

	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

func TransformUnorderedDataToOrderedData(allData map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData) []interfaces.StoreCollection {
	colls := make([]interfaces.StoreCollection, 0, len(allData))
	for kind, itemsMap := range allData {
		items := make([]interfaces.VersionedData, 0, len(itemsMap))
		if doesDataKindSupportDependencies(kind) {
			addItemsInDependencyOrder(itemsMap, &items)
		} else {
			for _, item := range itemsMap {
				items = append(items, item)
			}
		}
		colls = append(colls, interfaces.StoreCollection{Kind: kind, Items: items})
	}
	sort.Slice(colls, func(i, j int) bool {
		return dataKindPriority(colls[i].Kind) < dataKindPriority(colls[j].Kind)
	})
	return colls
}

func doesDataKindSupportDependencies(kind interfaces.VersionedDataKind) bool {
	return kind == interfaces.DataKindFeatures() //nolint:megacheck
}

func addItemsInDependencyOrder(itemsMap map[string]interfaces.VersionedData, out *[]interfaces.VersionedData) {
	remainingItems := make(map[string]interfaces.VersionedData, len(itemsMap))
	for key, item := range itemsMap { // copy the map because we'll be consuming it
		remainingItems[key] = item
	}
	for len(remainingItems) > 0 {
		// pick a random item that hasn't been visited yet
		for _, item := range remainingItems {
			addWithDependenciesFirst(item, remainingItems, out)
			break
		}
	}
}

func addWithDependenciesFirst(startItem interfaces.VersionedData, remainingItems map[string]interfaces.VersionedData, out *[]interfaces.VersionedData) {
	delete(remainingItems, startItem.GetKey()) // we won't need to visit this item again
	for _, prereqKey := range getDependencyKeys(startItem) {
		prereqItem := remainingItems[prereqKey]
		if prereqItem != nil {
			addWithDependenciesFirst(prereqItem, remainingItems, out)
		}
	}
	*out = append(*out, startItem)
}

func getDependencyKeys(item interfaces.VersionedData) []string {
	var ret []string
	switch i := item.(type) {
	case *ldmodel.FeatureFlag:
		for _, p := range i.Prerequisites {
			ret = append(ret, p.Key)
		}
	}
	return ret
}

// Logic for ensuring that segments are processed before features; if we get any other data types that
// haven't been accounted for here, they'll come after those two in an arbitrary order.
func dataKindPriority(kind interfaces.VersionedDataKind) int {
	switch kind.GetNamespace() {
	case "segments":
		return 0
	case "features":
		return 1
	default:
		return len(kind.GetNamespace()) + 2
	}
}
