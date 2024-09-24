package memorystorev2

import (
	"fmt"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// These benchmarks cover data store operations with the in-memory store.
//
// There's no reason why the performance for flags should be different from segments, but to be truly
// implementation-neutral we'll benchmark each data kind separately anyway.

var ( // assign to package-level variables in benchmarks so function calls won't be optimized away
	inMemoryStoreBenchmarkResultItem  ldstoretypes.ItemDescriptor
	inMemoryStoreBenchmarkResultItems []ldstoretypes.KeyedItemDescriptor
	updates                           map[ldstoretypes.DataKind]map[string]bool
)

type inMemoryStoreBenchmarkEnv struct {
	store             *Store
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
		store: New(ldlog.NewDisabledLoggers()),
	}
}

func (env *inMemoryStoreBenchmarkEnv) setUp(bc inMemoryStoreBenchmarkCase) {
	env.flags = make([]*ldmodel.FeatureFlag, bc.numFlags)
	for i := 0; i < bc.numFlags; i++ {
		flag := ldbuilders.NewFlagBuilder(fmt.Sprintf("flag-%d", i)).Version(10).Build()
		env.flags[i] = &flag
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

	s := env.segments[bc.numSegments/2]
	env.targetSegmentKey = s.Key
	s1 := ldbuilders.NewSegmentBuilder(s.Key).Version(s.Version).Build()
	env.targetSegmentCopy = &s1

	env.unknownKey = "no-match"

	basis := []ldstoretypes.Collection{
		{
			Kind:  datakinds.Features,
			Items: make([]ldstoretypes.KeyedItemDescriptor, len(env.flags)),
		},
		{
			Kind:  datakinds.Segments,
			Items: make([]ldstoretypes.KeyedItemDescriptor, len(env.segments)),
		},
	}

	for i, f := range env.flags {
		basis[0].Items[i] = ldstoretypes.KeyedItemDescriptor{Key: f.Key, Item: sharedtest.FlagDescriptor(*f)}
	}

	for i, s := range env.segments {
		basis[1].Items[i] = ldstoretypes.KeyedItemDescriptor{Key: s.Key, Item: sharedtest.SegmentDescriptor(*s)}
	}

	env.store.SetBasis(basis)
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
		{Kind: datakinds.Features, Items: flags},
		{Kind: datakinds.Segments, Items: segments},
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
	{
		numFlags:    1,
		numSegments: 1,
	},
	{
		numFlags:    100,
		numSegments: 100,
	},
	{
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
		env.store.SetBasis(env.initData)
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
		delta := makeCollections(dataKind, env.targetFlagKey, sharedtest.FlagDescriptor(*env.targetFlagCopy))
		updates = env.store.ApplyDelta(delta)
	})
}

func BenchmarkInMemoryStoreUpsertExistingFlagFailure(b *testing.B) {
	dataKind := datakinds.Features
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetFlagCopy.Version--
		delta := makeCollections(dataKind, env.targetFlagKey, sharedtest.FlagDescriptor(*env.targetFlagCopy))
		updates = env.store.ApplyDelta(delta)
	})
}

func BenchmarkInMemoryStoreUpsertNewFlag(b *testing.B) {
	dataKind := datakinds.Features
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetFlagCopy.Key = env.unknownKey
		delta := makeCollections(dataKind, env.unknownKey, sharedtest.FlagDescriptor(*env.targetFlagCopy))
		updates = env.store.ApplyDelta(delta)
	})
}

func BenchmarkInMemoryStoreUpsertExistingSegmentSuccess(b *testing.B) {
	dataKind := datakinds.Segments
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetSegmentCopy.Version++
		delta := makeCollections(dataKind, env.targetSegmentKey, sharedtest.SegmentDescriptor(*env.targetSegmentCopy))
		updates = env.store.ApplyDelta(delta)
	})
}

func BenchmarkInMemoryStoreUpsertExistingSegmentFailure(b *testing.B) {
	dataKind := datakinds.Segments
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetSegmentCopy.Version--
		delta := makeCollections(dataKind, env.targetSegmentKey, sharedtest.SegmentDescriptor(*env.targetSegmentCopy))
		updates = env.store.ApplyDelta(delta)
	})
}

func BenchmarkInMemoryStoreUpsertNewSegment(b *testing.B) {
	dataKind := datakinds.Segments
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetSegmentCopy.Key = env.unknownKey
		delta := makeCollections(dataKind, env.unknownKey, sharedtest.SegmentDescriptor(*env.targetSegmentCopy))
		updates = env.store.ApplyDelta(delta)
	})
}
