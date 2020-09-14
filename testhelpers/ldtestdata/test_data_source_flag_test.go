package ldtestdata

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents/ldstoreimpl"
)

func verifyFlag(t *testing.T, configureFlag func(*FlagBuilder), expectedProps string) {
	expectedJSON := `{"key":"flagkey","version":1,"salt":"",` + expectedProps + `}`
	testDataSourceTest(func(p testDataSourceTestParams) {
		p.withDataSource(t, func(interfaces.DataSource) {
			f := p.td.Flag("flagkey")
			configureFlag(f)
			p.td.Update(f)
			up := p.updates.DataStore.WaitForUpsert(t, ldstoreimpl.Features(), "flagkey", 1, time.Millisecond)
			upJSON := ldstoreimpl.Features().Serialize(up.Item)
			assert.JSONEq(t, expectedJSON, string(upJSON))
		})
	})
}

func TestFlagConfig(t *testing.T) {
	t.Run("simple boolean flag", func(t *testing.T) {
		basicProps := `"variations":[true,false],"offVariation":1`
		onProps := basicProps + `,"on":true`
		offProps := basicProps // currently we don't bother serializing false properties
		fallthroughTrueProps := `,"fallthrough":{"variation":0}`
		fallthroughFalseProps := `,"fallthrough":{"variation":1}`

		verifyFlag(t, func(f *FlagBuilder) {}, onProps+fallthroughTrueProps)
		verifyFlag(t, func(f *FlagBuilder) { f.BooleanFlag() }, onProps+fallthroughTrueProps)
		verifyFlag(t, func(f *FlagBuilder) { f.On(true) }, onProps+fallthroughTrueProps)
		verifyFlag(t, func(f *FlagBuilder) { f.On(false) }, offProps+fallthroughTrueProps)
		verifyFlag(t, func(f *FlagBuilder) { f.VariationForAllUsers(false) }, onProps+fallthroughFalseProps)
		verifyFlag(t, func(f *FlagBuilder) { f.VariationForAllUsers(true) }, onProps+fallthroughTrueProps)

		verifyFlag(t, func(f *FlagBuilder) {
			f.FallthroughVariation(true).OffVariation(false)
		}, onProps+fallthroughTrueProps)

		verifyFlag(t, func(f *FlagBuilder) {
			f.FallthroughVariation(false).OffVariation(true)
		}, onProps+`,"offVariation":0,"fallthrough":{"variation":1}`)
	})

	t.Run("using boolean config methods forces flag to be boolean", func(t *testing.T) {
		booleanProps := `"variations":[true,false],"on":true,"offVariation":1,"fallthrough":{"variation":0}`

		verifyFlag(t, func(f *FlagBuilder) {
			f.Variations(ldvalue.Int(1), ldvalue.Int(2))
			f.BooleanFlag()
		}, booleanProps)

		verifyFlag(t, func(f *FlagBuilder) {
			f.Variations(ldvalue.Bool(true), ldvalue.Int(2))
			f.BooleanFlag()
		}, booleanProps)

		verifyFlag(t, func(f *FlagBuilder) {
			f.ValueForAllUsers(ldvalue.String("x"))
			f.BooleanFlag()
		}, booleanProps)
	})

	t.Run("flag with string variations", func(t *testing.T) {
		basicProps := `"on":true,"variations":["red","green","blue"],"offVariation":0,"fallthrough":{"variation":2}`

		verifyFlag(t, func(f *FlagBuilder) {
			f.Variations(threeStringValues...).OffVariationIndex(0).FallthroughVariationIndex(2)
		}, basicProps)
	})

	t.Run("user targets", func(t *testing.T) {
		booleanFlagBasicProps := `"on":true,"variations":[true,false],"offVariation":1,"fallthrough":{"variation":0}`

		verifyFlag(t, func(f *FlagBuilder) {
			f.VariationForUser("a", true).VariationForUser("b", true)
		}, booleanFlagBasicProps+`,"targets":[{"variation":0,"values":["a","b"]}]`)

		verifyFlag(t, func(f *FlagBuilder) {
			f.VariationForUser("a", true).VariationForUser("a", true)
		}, booleanFlagBasicProps+`,"targets":[{"variation":0,"values":["a"]}]`)

		verifyFlag(t, func(f *FlagBuilder) {
			f.VariationForUser("a", false).VariationForUser("b", true).VariationForUser("c", false)
		}, booleanFlagBasicProps+`,"targets":[{"variation":0,"values":["b"]},{"variation":1,"values":["a","c"]}]`)

		verifyFlag(t, func(f *FlagBuilder) {
			f.VariationForUser("a", true).VariationForUser("b", true).VariationForUser("a", false)
		}, booleanFlagBasicProps+`,"targets":[{"variation":0,"values":["b"]},{"variation":1,"values":["a"]}]`)

		stringFlagBasicProps := `"on":true,"variations":["red","green","blue"],"offVariation":0,"fallthrough":{"variation":2}`

		verifyFlag(t, func(f *FlagBuilder) {
			f.Variations(threeStringValues...).OffVariationIndex(0).FallthroughVariationIndex(2).
				VariationIndexForUser("a", 2).VariationIndexForUser("b", 2)
		}, stringFlagBasicProps+`,"targets":[{"variation":2,"values":["a","b"]}]`)

		verifyFlag(t, func(f *FlagBuilder) {
			f.Variations(threeStringValues...).OffVariationIndex(0).FallthroughVariationIndex(2).
				VariationIndexForUser("a", 2).VariationIndexForUser("b", 1).VariationIndexForUser("c", 2)
		}, stringFlagBasicProps+`,"targets":[{"variation":1,"values":["b"]},{"variation":2,"values":["a","c"]}]`)
	})
}

func TestRuleConfig(t *testing.T) {
	booleanFlagBasicProps := `"on":true,"variations":[true,false],"offVariation":1,"fallthrough":{"variation":0}`

	t.Run("simple match returning variation 0/true", func(t *testing.T) {
		matchReturnsVariation0 := booleanFlagBasicProps +
			`,"rules":[{"id":"rule0","variation":0,"clauses":[` +
			`{"attribute":"name","op":"in","values":["Lucy"]}` +
			`]}]` // note that currently we don't bother serializing "negate" or "trackEvents" if they're false

		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatch(lduser.NameAttribute, ldvalue.String("Lucy")).ThenReturn(true)
		}, matchReturnsVariation0)

		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatch(lduser.NameAttribute, ldvalue.String("Lucy")).ThenReturnIndex(0)
		}, matchReturnsVariation0)
	})

	t.Run("simple match returning variation 1/false", func(t *testing.T) {
		matchReturnsVariation1 := booleanFlagBasicProps +
			`,"rules":[{"id":"rule0","variation":1,"clauses":[` +
			`{"attribute":"name","op":"in","values":["Lucy"]}` +
			`]}]` // note that currently we don't bother serializing "negate" or "trackEvents" if they're false

		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatch(lduser.NameAttribute, ldvalue.String("Lucy")).ThenReturn(false)
		}, matchReturnsVariation1)

		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatch(lduser.NameAttribute, ldvalue.String("Lucy")).ThenReturnIndex(1)
		}, matchReturnsVariation1)
	})

	t.Run("negated match", func(t *testing.T) {
		verifyFlag(t, func(f *FlagBuilder) {
			f.IfNotMatch(lduser.NameAttribute, ldvalue.String("Lucy")).ThenReturn(true)
		}, booleanFlagBasicProps+`,"rules":[{"id":"rule0","variation":0,"clauses":[`+
			`{"attribute":"name","op":"in","values":["Lucy"],"negate":true}`+
			`]}]`)
	})

	t.Run("multiple clauses", func(t *testing.T) {
		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatch(lduser.NameAttribute, ldvalue.String("Lucy")).
				AndMatch(lduser.CountryAttribute, ldvalue.String("gb")).
				ThenReturn(true)
		}, booleanFlagBasicProps+`,"rules":[{"id":"rule0","variation":0,"clauses":[`+
			`{"attribute":"name","op":"in","values":["Lucy"]}`+
			`,{"attribute":"country","op":"in","values":["gb"]}`+
			`]}]`)
	})

	t.Run("multiple rules", func(t *testing.T) {
		verifyFlag(t, func(f *FlagBuilder) {
			f.IfMatch(lduser.NameAttribute, ldvalue.String("Lucy")).ThenReturn(true).
				IfMatch(lduser.NameAttribute, ldvalue.String("Mina")).ThenReturn(true)
		}, booleanFlagBasicProps+`,"rules":[`+
			`{"id":"rule0","variation":0,"clauses":[`+
			`{"attribute":"name","op":"in","values":["Lucy"]}`+
			`]}`+
			`,{"id":"rule1","variation":0,"clauses":[`+
			`{"attribute":"name","op":"in","values":["Mina"]}`+
			`]}]`)
	})
}
