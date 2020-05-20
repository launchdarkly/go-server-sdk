package ldclient

import (
	"fmt"
	"testing"
)

// These benchmarks cover data store operations with the in-memory store.
//
// There's no reason why the performance for flags should be different from segments, but to be truly
// implementation-neutral we'll benchmark each data kind separately anyway.

var ( // assign to package-level variables in benchmarks so function calls won't be optimized away
	inMemoryStoreBenchmarkResultErr   error
	inMemoryStoreBenchmarkResultItem  VersionedData
	inMemoryStoreBenchmarkResultItems map[string]VersionedData
)

type inMemoryStoreBenchmarkEnv struct {
	store             FeatureStore
	flags             []*FeatureFlag
	segments          []*Segment
	targetFlagKey     string
	targetSegmentKey  string
	targetFlagCopy    *FeatureFlag
	targetSegmentCopy *Segment
	unknownKey        string
	initData          map[VersionedDataKind]map[string]VersionedData
}

func newInMemoryStoreBenchmarkEnv() *inMemoryStoreBenchmarkEnv {
	return &inMemoryStoreBenchmarkEnv{
		store: NewInMemoryFeatureStore(nil),
	}
}

func (env *inMemoryStoreBenchmarkEnv) setUp(bc inMemoryStoreBenchmarkCase) {
	env.flags = make([]*FeatureFlag, bc.numFlags)
	for i := 0; i < bc.numFlags; i++ {
		flag := FeatureFlag{Key: fmt.Sprintf("flag-%d", i), Version: 10}
		env.flags[i] = &flag
	}
	for _, flag := range env.flags {
		env.store.Upsert(Features, flag)
	}
	f := env.flags[bc.numFlags/2] // arbitrarily pick a flag in the middle of the list
	env.targetFlagKey = f.Key
	f1 := *f
	env.targetFlagCopy = &f1

	env.segments = make([]*Segment, bc.numSegments)
	for i := 0; i < bc.numSegments; i++ {
		segment := Segment{Key: fmt.Sprintf("segment-%d", i), Version: 10}
		env.segments[i] = &segment
	}
	for _, segment := range env.segments {
		env.store.Upsert(Segments, segment)
	}
	s := env.segments[bc.numSegments/2]
	env.targetSegmentKey = s.Key
	s1 := *s
	env.targetSegmentCopy = &s1

	env.unknownKey = "no-match"
}

func setupInitData(env *inMemoryStoreBenchmarkEnv) {
	flags := make(map[string]VersionedData, len(env.flags))
	for _, f := range env.flags {
		flags[f.Key] = f
	}
	segments := make(map[string]VersionedData, len(env.segments))
	for _, s := range env.segments {
		segments[s.Key] = s
	}
	env.initData = map[VersionedDataKind]map[string]VersionedData{
		Features: flags,
		Segments: segments,
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
	dataKind := Features
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		inMemoryStoreBenchmarkResultItem, _ = env.store.Get(dataKind, env.targetFlagKey)
	})
}

func BenchmarkInMemoryStoreGetSegment(b *testing.B) {
	dataKind := Segments
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		inMemoryStoreBenchmarkResultItem, _ = env.store.Get(dataKind, env.targetSegmentKey)
	})
}

func BenchmarkInMemoryStoreGetUnknownFlag(b *testing.B) {
	dataKind := Features
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		inMemoryStoreBenchmarkResultItem, _ = env.store.Get(dataKind, env.unknownKey)
	})
}

func BenchmarkInMemoryStoreGetUnknownSegment(b *testing.B) {
	dataKind := Segments
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		inMemoryStoreBenchmarkResultItem, _ = env.store.Get(dataKind, env.unknownKey)
	})
}

func BenchmarkInMemoryStoreGetAllFlags(b *testing.B) {
	dataKind := Features
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		inMemoryStoreBenchmarkResultItems, _ = env.store.All(dataKind)
	})
}

func BenchmarkInMemoryStoreGetAllSegments(b *testing.B) {
	dataKind := Segments
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		inMemoryStoreBenchmarkResultItems, _ = env.store.All(dataKind)
	})
}

func BenchmarkInMemoryStoreUpsertExistingFlagSuccess(b *testing.B) {
	dataKind := Features
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetFlagCopy.Version++
		inMemoryStoreBenchmarkResultErr = env.store.Upsert(dataKind, env.targetFlagCopy)
	})
}

func BenchmarkInMemoryStoreUpsertExistingFlagFailure(b *testing.B) {
	dataKind := Features
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetFlagCopy.Version--
		inMemoryStoreBenchmarkResultErr = env.store.Upsert(dataKind, env.targetFlagCopy)
	})
}

func BenchmarkInMemoryStoreUpsertNewFlag(b *testing.B) {
	dataKind := Features
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetFlagCopy.Key = env.unknownKey
		inMemoryStoreBenchmarkResultErr = env.store.Upsert(dataKind, env.targetFlagCopy)
	})
}

func BenchmarkInMemoryStoreUpsertExistingSegmentSuccess(b *testing.B) {
	dataKind := Segments
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetSegmentCopy.Version++
		inMemoryStoreBenchmarkResultErr = env.store.Upsert(dataKind, env.targetSegmentCopy)
	})
}

func BenchmarkInMemoryStoreUpsertExistingSegmentFailure(b *testing.B) {
	dataKind := Segments
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetSegmentCopy.Version--
		inMemoryStoreBenchmarkResultErr = env.store.Upsert(dataKind, env.targetSegmentCopy)
	})
}

func BenchmarkInMemoryStoreUpsertNewSegment(b *testing.B) {
	dataKind := Segments
	benchmarkInMemoryStore(b, inMemoryStoreBenchmarkCases, nil, func(env *inMemoryStoreBenchmarkEnv, bc inMemoryStoreBenchmarkCase) {
		env.targetSegmentCopy.Key = env.unknownKey
		inMemoryStoreBenchmarkResultErr = env.store.Upsert(dataKind, env.targetSegmentCopy)
	})
}
