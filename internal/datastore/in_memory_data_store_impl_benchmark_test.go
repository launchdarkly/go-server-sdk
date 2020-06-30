package datastore

import (
	"fmt"
	"testing"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/ldstoretypes"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datakinds"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
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
	store             interfaces.DataStore
	flags             []*ldmodel.FeatureFlag
	segments          []*ldmodel.Segment
	targetFlagKey     string
	targetSegmentKey  string
	targetFlagCopy    *ldmodel.FeatureFlag
	targetSegmentCopy *ldmodel.Segment
	unknownKey        string
	initData          []ldstoretypes.Collection
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
		segment := ldbuilders.NewSegmentBuilder(fmt.Sprintf("flag-%d", i)).Version(10).Build()
		env.segments[i] = &segment
	}
	for _, segment := range env.segments {
		env.store.Upsert(datakinds.Segments, segment.Key, sharedtest.SegmentDescriptor(*segment))
	}
	s := env.segments[bc.numSegments/2]
	env.targetSegmentKey = s.Key
	s1 := ldbuilders.NewSegmentBuilder(s.Key).Version(s.Version).Build()
	env.targetSegmentCopy = &s1

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
	env.initData = []ldstoretypes.Collection{
		ldstoretypes.Collection{Kind: datakinds.Features, Items: flags},
		ldstoretypes.Collection{Kind: datakinds.Segments, Items: segments},
	}
}

func (env *inMemoryStoreBenchmarkEnv) tearDown() {
}

type inMemoryStoreBenchmarkCase struct {
	numFlags     int
	numSegments  int
	withInitData bool
}

var inMemoryStoreBenchmarkCases = []inMemoryStoreBenchmarkCase{
	inMemoryStoreBenchmarkCase{
		numFlags:    1,
		numSegments: 1,
	},
	inMemoryStoreBenchmarkCase{
		numFlags:    100,
		numSegments: 100,
	},
	inMemoryStoreBenchmarkCase{
		numFlags:    1000,
		numSegments: 1000,
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
