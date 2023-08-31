package datastore

import (
	"fmt"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v6/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v6/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems/ldstoretypes"
)

// These benchmarks cover data store operations with the in-memory store.
//
// There's no reason why the performance for flags should be different from segments, but to be truly
// implementation-neutral we'll benchmark each data kind separately anyway.

var ( // assign to package-level variables in benchmarks so function calls won't be optimized away
	inMemoryStoreBenchmarkResultErr   error
	inMemoryStoreBenchmarkResultItem  ldstoretypes.ItemDescriptor
	inMemoryStoreBenchmarkResultItems []ldstoretypes.KeyedItemDescriptor
)

type inMemoryStoreBenchmarkEnv struct {
	store                    subsystems.DataStore
	flags                    []*ldmodel.FeatureFlag
	segments                 []*ldmodel.Segment
	configOverrides          []*ldmodel.ConfigOverride
	metrics                  []*ldmodel.Metric
	targetFlagKey            string
	targetSegmentKey         string
	targetConfigOverrideKey  string
	targetMetricKey          string
	targetFlagCopy           *ldmodel.FeatureFlag
	targetSegmentCopy        *ldmodel.Segment
	targetConfigOverrideCopy *ldmodel.ConfigOverride
	targetMetricCopy         *ldmodel.Metric
	unknownKey               string
	initData                 []ldstoretypes.Collection
}

func newInMemoryStoreBenchmarkEnv() *inMemoryStoreBenchmarkEnv {
	return &inMemoryStoreBenchmarkEnv{
		store: NewInMemoryDataStore(ldlog.NewDisabledLoggers()),
	}
}

func (env *inMemoryStoreBenchmarkEnv) setUp(bc inMemoryStoreBenchmarkCase) {
	env.flags = make([]*ldmodel.FeatureFlag, bc.numFlags)
	for i := 0; i < bc.numFlags; i++ {
		flag := ldbuilders.NewFlagBuilder(fmt.Sprintf("flag-%d", i)).Version(10).Build()
		env.flags[i] = &flag
	}
	for _, flag := range env.flags {
		env.store.Upsert(datakinds.Features, flag.Key, sharedtest.FlagDescriptor(*flag))
	}
	f := env.flags[bc.numFlags/2] // arbitrarily pick a flag in the middle of the list
	env.targetFlagKey = f.Key
	f1 := ldbuilders.NewFlagBuilder(f.Key).Version(f.Version).Build()
	env.targetFlagCopy = &f1

	env.segments = make([]*ldmodel.Segment, bc.numFlags)
	for i := 0; i < bc.numSegments; i++ {
		segment := ldbuilders.NewSegmentBuilder(fmt.Sprintf("segment-%d", i)).Version(10).Build()
		env.segments[i] = &segment
	}
	for _, segment := range env.segments {
		env.store.Upsert(datakinds.Segments, segment.Key, sharedtest.SegmentDescriptor(*segment))
	}
	s := env.segments[bc.numSegments/2]
	env.targetSegmentKey = s.Key
	s1 := ldbuilders.NewSegmentBuilder(s.Key).Version(s.Version).Build()
	env.targetSegmentCopy = &s1

	env.configOverrides = make([]*ldmodel.ConfigOverride, bc.numConfigOverrides)
	for i := 0; i < bc.numConfigOverrides; i++ {
		override := ldbuilders.NewConfigOverrideBuilder(fmt.Sprintf("overrides-%d", i)).Version(10).Build()
		env.configOverrides[i] = &override
	}
	for _, override := range env.configOverrides {
		env.store.Upsert(datakinds.ConfigOverrides, override.Key, sharedtest.ConfigOverrideDescriptor(*override))
	}
	o := env.configOverrides[bc.numConfigOverrides/2]
	env.targetConfigOverrideKey = o.Key
	o1 := ldbuilders.NewConfigOverrideBuilder(o.Key).Version(o.Version).Build()
	env.targetConfigOverrideCopy = &o1

	env.metrics = make([]*ldmodel.Metric, bc.numMetrics)
	for i := 0; i < bc.numMetrics; i++ {
		metric := ldbuilders.NewMetricBuilder(fmt.Sprintf("overrides-%d", i)).Version(10).Build()
		env.metrics[i] = &metric
	}
	for _, metric := range env.metrics {
		env.store.Upsert(datakinds.Metrics, metric.Key, sharedtest.MetricDescriptor(*metric))
	}
	m := env.metrics[bc.numMetrics/2]
	env.targetMetricKey = o.Key
	m1 := ldbuilders.NewMetricBuilder(m.Key).Version(m.Version).Build()
	env.targetMetricCopy = &m1

	env.unknownKey = "no-match"
}

func setupInitData(env *inMemoryStoreBenchmarkEnv) {
	flags := make([]ldstoretypes.KeyedItemDescriptor, len(env.flags))
	for i, f := range env.flags {
		flags[i] = ldstoretypes.KeyedItemDescriptor{Key: f.Key, Item: sharedtest.FlagDescriptor(*f)}
	}
	segments := make([]ldstoretypes.KeyedItemDescriptor, len(env.segments))
	for i, s := range env.segments {
		segments[i] = ldstoretypes.KeyedItemDescriptor{Key: s.Key, Item: sharedtest.SegmentDescriptor(*s)}
	}
	overrides := make([]ldstoretypes.KeyedItemDescriptor, len(env.configOverrides))
	for i, o := range env.configOverrides {
		overrides[i] = ldstoretypes.KeyedItemDescriptor{Key: o.Key, Item: sharedtest.ConfigOverrideDescriptor(*o)}
	}
	metrics := make([]ldstoretypes.KeyedItemDescriptor, len(env.metrics))
	for i, m := range env.metrics {
		metrics[i] = ldstoretypes.KeyedItemDescriptor{Key: m.Key, Item: sharedtest.MetricDescriptor(*m)}
	}
	env.initData = []ldstoretypes.Collection{
		{Kind: datakinds.Features, Items: flags},
		{Kind: datakinds.Segments, Items: segments},
		{Kind: datakinds.ConfigOverrides, Items: overrides},
		{Kind: datakinds.Metrics, Items: metrics},
	}
}

func (env *inMemoryStoreBenchmarkEnv) tearDown() {
}

type inMemoryStoreBenchmarkCase struct {
	numFlags           int
	numSegments        int
	numConfigOverrides int
	numMetrics         int
	withInitData       bool
}

var inMemoryStoreBenchmarkCases = []inMemoryStoreBenchmarkCase{
	{
		numFlags:           1,
		numSegments:        1,
		numConfigOverrides: 1,
		numMetrics:         1,
	},
	{
		numFlags:           100,
		numSegments:        100,
		numConfigOverrides: 100,
		numMetrics:         100,
	},
	{
		numFlags:           1000,
		numSegments:        1000,
		numConfigOverrides: 1000,
		numMetrics:         1000,
	},
}

func benchmarkInMemoryStore(
	b *testing.B,
	cases []inMemoryStoreBenchmarkCase,
	setupAction func(*inMemoryStoreBenchmarkEnv),
	benchmarkAction func(*inMemoryStoreBenchmarkEnv, inMemoryStoreBenchmarkCase),
) {
	env := newInMemoryStoreBenchmarkEnv()
	for _, bc := range cases {
		env.setUp(bc)

		if setupAction != nil {
			setupAction(env)
		}

		b.Run(fmt.Sprintf("%+v", bc), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				benchmarkAction(env, bc)
			}
		})
		env.tearDown()
	}
}

func BenchmarkInMemoryStoreInit(b *testing.B) {
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, setupInitData, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		inMemoryStoreBenchmarkResultErr = env.store.Init(env.initData)
	})
}

func BenchmarkInMemoryStoreGetFlag(b *testing.B) {
	dataKind := datakinds.Features
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		inMemoryStoreBenchmarkResultItem, _ = env.store.Get(dataKind, env.targetFlagKey)
	})
}

func BenchmarkInMemoryStoreGetSegment(b *testing.B) {
	dataKind := datakinds.Segments
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		inMemoryStoreBenchmarkResultItem, _ = env.store.Get(dataKind, env.targetSegmentKey)
	})
}

func BenchmarkInMemoryStoreGetConfigOverride(b *testing.B) {
	dataKind := datakinds.ConfigOverrides
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		inMemoryStoreBenchmarkResultItem, _ = env.store.Get(dataKind, env.targetConfigOverrideKey)
	})
}

func BenchmarkInMemoryStoreGetMetric(b *testing.B) {
	dataKind := datakinds.Metrics
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		inMemoryStoreBenchmarkResultItem, _ = env.store.Get(dataKind, env.targetMetricKey)
	})
}

func BenchmarkInMemoryStoreGetUnknownFlag(b *testing.B) {
	dataKind := datakinds.Features
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		inMemoryStoreBenchmarkResultItem, _ = env.store.Get(dataKind, env.unknownKey)
	})
}

func BenchmarkInMemoryStoreGetUnknownSegment(b *testing.B) {
	dataKind := datakinds.Segments
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		inMemoryStoreBenchmarkResultItem, _ = env.store.Get(dataKind, env.unknownKey)
	})
}

func BenchmarkInMemoryStoreGetUnknownConfigOverride(b *testing.B) {
	dataKind := datakinds.ConfigOverrides
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		inMemoryStoreBenchmarkResultItem, _ = env.store.Get(dataKind, env.unknownKey)
	})
}

func BenchmarkInMemoryStoreGetUnknownMetric(b *testing.B) {
	dataKind := datakinds.Metrics
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		inMemoryStoreBenchmarkResultItem, _ = env.store.Get(dataKind, env.unknownKey)
	})
}

func BenchmarkInMemoryStoreGetAllFlags(b *testing.B) {
	dataKind := datakinds.Features
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		inMemoryStoreBenchmarkResultItems, _ = env.store.GetAll(dataKind)
	})
}

func BenchmarkInMemoryStoreGetAllSegments(b *testing.B) {
	dataKind := datakinds.Segments
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		inMemoryStoreBenchmarkResultItems, _ = env.store.GetAll(dataKind)
	})
}

func BenchmarkInMemoryStoreGetAllConfigOverrides(b *testing.B) {
	dataKind := datakinds.ConfigOverrides
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		inMemoryStoreBenchmarkResultItems, _ = env.store.GetAll(dataKind)
	})
}

func BenchmarkInMemoryStoreGetAllMetrics(b *testing.B) {
	dataKind := datakinds.Metrics
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		inMemoryStoreBenchmarkResultItems, _ = env.store.GetAll(dataKind)
	})
}

func BenchmarkInMemoryStoreUpsertExistingFlagSuccess(b *testing.B) {
	dataKind := datakinds.Features
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetFlagCopy.Version++
		_, inMemoryStoreBenchmarkResultErr = env.store.Upsert(dataKind, env.targetFlagKey,
			sharedtest.FlagDescriptor(*env.targetFlagCopy))
	})
}

func BenchmarkInMemoryStoreUpsertExistingFlagFailure(b *testing.B) {
	dataKind := datakinds.Features
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetFlagCopy.Version--
		_, inMemoryStoreBenchmarkResultErr = env.store.Upsert(dataKind, env.targetFlagKey,
			sharedtest.FlagDescriptor(*env.targetFlagCopy))
	})
}

func BenchmarkInMemoryStoreUpsertNewFlag(b *testing.B) {
	dataKind := datakinds.Features
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetFlagCopy.Key = env.unknownKey
		_, inMemoryStoreBenchmarkResultErr = env.store.Upsert(dataKind, env.unknownKey,
			sharedtest.FlagDescriptor(*env.targetFlagCopy))
	})
}

func BenchmarkInMemoryStoreUpsertExistingSegmentSuccess(b *testing.B) {
	dataKind := datakinds.Segments
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetSegmentCopy.Version++
		_, inMemoryStoreBenchmarkResultErr = env.store.Upsert(dataKind, env.targetSegmentKey,
			sharedtest.SegmentDescriptor(*env.targetSegmentCopy))
	})
}

func BenchmarkInMemoryStoreUpsertExistingSegmentFailure(b *testing.B) {
	dataKind := datakinds.Segments
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetSegmentCopy.Version--
		_, inMemoryStoreBenchmarkResultErr = env.store.Upsert(dataKind, env.targetSegmentKey,
			sharedtest.SegmentDescriptor(*env.targetSegmentCopy))
	})
}

func BenchmarkInMemoryStoreUpsertNewSegment(b *testing.B) {
	dataKind := datakinds.Segments
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetSegmentCopy.Key = env.unknownKey
		_, inMemoryStoreBenchmarkResultErr = env.store.Upsert(dataKind, env.unknownKey,
			sharedtest.SegmentDescriptor(*env.targetSegmentCopy))
	})
}

func BenchmarkInMemoryStoreUpsertExistingConfigOverrideSuccess(b *testing.B) {
	dataKind := datakinds.ConfigOverrides
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetConfigOverrideCopy.Version++
		_, inMemoryStoreBenchmarkResultErr = env.store.Upsert(dataKind, env.targetConfigOverrideKey,
			sharedtest.ConfigOverrideDescriptor(*env.targetConfigOverrideCopy))
	})
}

func BenchmarkInMemoryStoreUpsertExistingConfigOverrideFailure(b *testing.B) {
	dataKind := datakinds.ConfigOverrides
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetConfigOverrideCopy.Version--
		_, inMemoryStoreBenchmarkResultErr = env.store.Upsert(dataKind, env.targetConfigOverrideKey,
			sharedtest.ConfigOverrideDescriptor(*env.targetConfigOverrideCopy))
	})
}

func BenchmarkInMemoryStoreUpsertNewConfigOverride(b *testing.B) {
	dataKind := datakinds.ConfigOverrides
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetConfigOverrideCopy.Key = env.unknownKey
		_, inMemoryStoreBenchmarkResultErr = env.store.Upsert(dataKind, env.unknownKey,
			sharedtest.ConfigOverrideDescriptor(*env.targetConfigOverrideCopy))
	})
}

func BenchmarkInMemoryStoreUpsertExistingMetricSuccess(b *testing.B) {
	dataKind := datakinds.Metrics
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetMetricCopy.Version++
		_, inMemoryStoreBenchmarkResultErr = env.store.Upsert(dataKind, env.targetMetricKey,
			sharedtest.MetricDescriptor(*env.targetMetricCopy))
	})
}

func BenchmarkInMemoryStoreUpsertExistingMetricFailure(b *testing.B) {
	dataKind := datakinds.Metrics
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetMetricCopy.Version--
		_, inMemoryStoreBenchmarkResultErr = env.store.Upsert(dataKind, env.targetMetricKey,
			sharedtest.MetricDescriptor(*env.targetMetricCopy))
	})
}

func BenchmarkInMemoryStoreUpsertNewMetric(b *testing.B) {
	dataKind := datakinds.Metrics
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetMetricCopy.Key = env.unknownKey
		_, inMemoryStoreBenchmarkResultErr = env.store.Upsert(dataKind, env.unknownKey,
			sharedtest.MetricDescriptor(*env.targetMetricCopy))
	})
}
