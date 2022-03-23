package callbackfixtures

type DataStoreSerializedCollection struct {
	Kind  string                         `json:"kind"`
	Items []DataStoreSerializedKeyedItem `json:"items"`
}

type DataStoreSerializedKeyedItem struct {
	Key  string                  `json:"key"`
	Item DataStoreSerializedItem `json:"item"`
}

type DataStoreSerializedItem struct {
	Version        int    `json:"version"`
	SerializedItem string `json:"serializedItem,omitempty"`
}

type DataStoreStatusResponse struct {
	Available   bool `json:"available"`
	Initialized bool `json:"initialized"`
}

type DataStoreInitParams struct {
	AllData []DataStoreSerializedCollection
}

type DataStoreGetParams struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
}

type DataStoreGetResponse struct {
	Item *DataStoreSerializedItem `json:"item,omitempty"`
}

type DataStoreGetAllParams struct {
	Kind string `json:"kind"`
}

type DataStoreGetAllResponse struct {
	Items []DataStoreSerializedKeyedItem `json:"items"`
}

type DataStoreUpsertParams struct {
	Kind string                  `json:"kind"`
	Key  string                  `json:"key"`
	Item DataStoreSerializedItem `json:"item"`
}

type DataStoreUpsertResponse struct {
	Updated bool `json:"updated"`
}
