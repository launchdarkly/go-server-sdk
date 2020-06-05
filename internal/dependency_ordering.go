package internal

import (
	"sort"

	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

func sortCollectionsForDataStoreInit(allData []interfaces.StoreCollection) []interfaces.StoreCollection {
	colls := make([]interfaces.StoreCollection, 0, len(allData))
	for _, coll := range allData {
		if doesDataKindSupportDependencies(coll.Kind) {
			itemsOut := make([]interfaces.StoreKeyedItemDescriptor, 0, len(coll.Items))
			addItemsInDependencyOrder(coll.Items, &itemsOut)
			colls = append(colls, interfaces.StoreCollection{Kind: coll.Kind, Items: itemsOut})
		} else {
			colls = append(colls, coll)
		}
	}
	sort.Slice(colls, func(i, j int) bool {
		return dataKindPriority(colls[i].Kind) < dataKindPriority(colls[j].Kind)
	})
	return colls
}

func doesDataKindSupportDependencies(kind interfaces.StoreDataKind) bool {
	return kind == interfaces.DataKindFeatures() //nolint:megacheck
}

func addItemsInDependencyOrder(itemsIn []interfaces.StoreKeyedItemDescriptor, out *[]interfaces.StoreKeyedItemDescriptor) {
	remainingItems := make(map[string]interfaces.StoreItemDescriptor, len(itemsIn))
	for _, item := range itemsIn {
		remainingItems[item.Key] = item.Item
	}
	for len(remainingItems) > 0 {
		// pick a random item that hasn't been visited yet
		for firstKey := range remainingItems {
			addWithDependenciesFirst(firstKey, remainingItems, out)
			break
		}
	}
}

func addWithDependenciesFirst(
	startingKey string,
	remainingItems map[string]interfaces.StoreItemDescriptor,
	out *[]interfaces.StoreKeyedItemDescriptor,
) {
	startItem := remainingItems[startingKey]
	delete(remainingItems, startingKey) // we won't need to visit this item again
	for _, prereqKey := range getDependencyKeys(startItem) {
		if _, ok := remainingItems[prereqKey]; ok {
			addWithDependenciesFirst(prereqKey, remainingItems, out)
		}
	}
	*out = append(*out, interfaces.StoreKeyedItemDescriptor{Key: startingKey, Item: startItem})
}

func getDependencyKeys(item interfaces.StoreItemDescriptor) []string {
	var ret []string
	switch i := item.Item.(type) {
	case *ldmodel.FeatureFlag:
		for _, p := range i.Prerequisites {
			ret = append(ret, p.Key)
		}
	}
	return ret
}

// Logic for ensuring that segments are processed before features; if we get any other data types that
// haven't been accounted for here, they'll come after those two in an arbitrary order.
func dataKindPriority(kind interfaces.StoreDataKind) int {
	switch kind.GetName() {
	case "segments":
		return 0
	case "features":
		return 1
	default:
		return len(kind.GetName()) + 2
	}
}
