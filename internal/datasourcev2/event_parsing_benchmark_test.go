package datasourcev2

import (
	"context"
	"testing"
)

// These benchmarks guard the single-pass polling-payload parser against regressions, and keep the
// previous reflection-based decode path measurable for comparison. The dominant cost of client
// initialization over FDv2 is parsing the initial polling payload, so this path is
// performance-sensitive: every SDK start pays it once per payload.

func BenchmarkParsePollingPayload(b *testing.B) {
	body := makePollingBody(b, 2000)
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		changeSet, err := parsePollingPayload(context.Background(), body)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := changeSet.Collections(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParsePollingPayloadReflectionReference(b *testing.B) {
	body := makePollingBody(b, 2000)
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		changeSet, err := parsePollingPayloadReflectionReference(body)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := changeSet.Collections(); err != nil {
			b.Fatal(err)
		}
	}
}
