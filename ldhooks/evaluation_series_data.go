package ldhooks

// EvaluationSeriesData is an immutable data type used for passing implementation-specific data between stages in the
// evaluation series.
type EvaluationSeriesData struct {
	data map[string]any
}

// EvaluationSeriesDataBuilder should be used by hook implementers to append data
type EvaluationSeriesDataBuilder struct {
	data map[string]any
}

// EmptyEvaluationSeriesData returns empty series data. This function is not intended for use by hook implementors.
// Hook implementations should always use NewEvaluationSeriesBuilder.
func EmptyEvaluationSeriesData() EvaluationSeriesData {
	return EvaluationSeriesData{
		data: make(map[string]any),
	}
}

// Get gets the value associated with the given key. If there is no value, then ok will be false.
func (b EvaluationSeriesData) Get(key string) (value any, ok bool) {
	val, ok := b.data[key]
	return val, ok
}

// AsAnyMap returns a copy of the contents of the series data as a map.
func (b EvaluationSeriesData) AsAnyMap() map[string]any {
	ret := make(map[string]any)
	for key, value := range b.data {
		ret[key] = value
	}
	return ret
}

// NewEvaluationSeriesBuilder creates an EvaluationSeriesDataBuilder based on the provided EvaluationSeriesData.
//
//	func(h MyHook) BeforeEvaluation(seriesContext EvaluationSeriesContext,
//		data EvaluationSeriesData) EvaluationSeriesData {
//		// Some hook functionality.
//		return NewEvaluationSeriesBuilder(data).Set("my-key", myValue).Build()
//	}
func NewEvaluationSeriesBuilder(data EvaluationSeriesData) *EvaluationSeriesDataBuilder {
	newData := make(map[string]any, len(data.data))
	for k, v := range data.data {
		newData[k] = v
	}
	return &EvaluationSeriesDataBuilder{
		data: newData,
	}
}

// Set sets the given key to the given value.
func (b *EvaluationSeriesDataBuilder) Set(key string, value any) *EvaluationSeriesDataBuilder {
	b.data[key] = value
	return b
}

// Merge copies the keys and values from the given map to the builder.
func (b *EvaluationSeriesDataBuilder) Merge(newValues map[string]any) *EvaluationSeriesDataBuilder {
	for k, v := range newValues {
		b.data[k] = v
	}
	return b
}

// Build builds an EvaluationSeriesData based on the contents of the builder.
func (b *EvaluationSeriesDataBuilder) Build() EvaluationSeriesData {
	newData := make(map[string]any, len(b.data))
	for k, v := range b.data {
		newData[k] = v
	}
	return EvaluationSeriesData{
		data: newData,
	}
}
