package ldevents

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldattr"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/launchdarkly/go-jsonstream/v3/jwriter"
	"github.com/launchdarkly/go-test-helpers/v3/jsonhelpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventContextFormatterConstructor(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		f := newEventContextFormatter(EventsConfiguration{})
		require.NotNil(t, f)

		assert.False(t, f.allAttributesPrivate)
		assert.Nil(t, f.privateAttributes)
	})

	t.Run("all private", func(t *testing.T) {
		f := newEventContextFormatter(EventsConfiguration{
			AllAttributesPrivate: true,
		})
		require.NotNil(t, f)

		assert.True(t, f.allAttributesPrivate)
		assert.Nil(t, f.privateAttributes)
	})

	t.Run("top-level private", func(t *testing.T) {
		private1, private2 := ldattr.NewRef("name"), ldattr.NewRef("email")
		f := newEventContextFormatter(EventsConfiguration{
			PrivateAttributes: []ldattr.Ref{private1, private2},
		})
		require.NotNil(t, f)

		assert.False(t, f.allAttributesPrivate)
		require.NotNil(t, f.privateAttributes)
		assert.Equal(t,
			map[string]*privateAttrLookupNode{
				"name":  {attribute: &private1},
				"email": {attribute: &private2},
			},
			f.privateAttributes)
	})

	t.Run("nested private", func(t *testing.T) {
		private1, private2, private3 := ldattr.NewRef("/name"),
			ldattr.NewRef("/address/street"), ldattr.NewRef("/address/city")
		f := newEventContextFormatter(EventsConfiguration{
			PrivateAttributes: []ldattr.Ref{private1, private2, private3},
		})
		require.NotNil(t, f)

		assert.False(t, f.allAttributesPrivate)
		require.NotNil(t, f.privateAttributes)
		assert.Equal(t,
			map[string]*privateAttrLookupNode{
				"name": {attribute: &private1},
				"address": {
					children: map[string]*privateAttrLookupNode{
						"street": {attribute: &private2},
						"city":   {attribute: &private3},
					},
				},
			},
			f.privateAttributes)
	})
}

func TestCheckGlobalPrivateAttrRefs(t *testing.T) {
	expectResult := func(t *testing.T, f eventContextFormatter, expectRedactedAttr *ldattr.Ref, expectHasNested bool, path ...string) {
		redactedAttr, hasNested := f.checkGlobalPrivateAttrRefs(path)
		assert.Equal(t, expectRedactedAttr, redactedAttr)
		assert.Equal(t, expectHasNested, hasNested)
	}

	t.Run("empty", func(t *testing.T) {
		f := newEventContextFormatter(EventsConfiguration{})
		require.NotNil(t, f)

		expectResult(t, f, nil, false, "name")
		expectResult(t, f, nil, false, "address", "street")
	})

	t.Run("top-level private", func(t *testing.T) {
		attrRef1, attrRef2 := ldattr.NewRef("name"), ldattr.NewRef("email")
		f := newEventContextFormatter(EventsConfiguration{
			PrivateAttributes: []ldattr.Ref{attrRef1, attrRef2},
		})
		require.NotNil(t, f)

		expectResult(t, f, &attrRef1, false, "name")
		expectResult(t, f, &attrRef2, false, "email")
		expectResult(t, f, nil, false, "address")
		expectResult(t, f, nil, false, "address", "street")
	})

	t.Run("nested private", func(t *testing.T) {
		attrRef1, attrRef2, attrRef3 := ldattr.NewRef("name"),
			ldattr.NewRef("/address/street"), ldattr.NewRef("/address/city")
		f := newEventContextFormatter(EventsConfiguration{
			PrivateAttributes: []ldattr.Ref{attrRef1, attrRef2, attrRef3},
		})
		require.NotNil(t, f)

		expectResult(t, f, &attrRef1, false, "name")
		expectResult(t, f, nil, true, "address") // note "true" indicating there are nested properties to filter
		expectResult(t, f, &attrRef2, false, "address", "street")
		expectResult(t, f, &attrRef3, false, "address", "city")
		expectResult(t, f, nil, false, "address", "zip")
	})
}

func TestEventContextFormatterOutput(t *testing.T) {
	objectValue := ldvalue.ObjectBuild().SetString("city", "SF").SetString("state", "CA").Build()

	type params struct {
		desc         string
		context      ldcontext.Context
		options      EventsConfiguration
		expectedJSON string
	}
	for _, p := range []params{
		{
			"no attributes private, single kind",
			ldcontext.NewBuilder("my-key").Kind("org").
				Name("my-name").
				SetString("attr1", "value1").
				SetValue("address", objectValue).
				Build(),
			EventsConfiguration{},
			`{"kind": "org", "key": "my-key",
				"name": "my-name", "attr1": "value1", "address": {"city": "SF", "state": "CA"}}`,
		},
		{
			"no attributes private, multi-kind",
			ldcontext.NewMulti(
				ldcontext.NewBuilder("org-key").Kind("org").
					Name("org-name").
					Build(),
				ldcontext.NewBuilder("user-key").
					Name("user-name").
					SetValue("address", objectValue).
					Build(),
			),
			EventsConfiguration{},
			`{"kind": "multi",
			    "org": {"key": "org-key", "name": "org-name"},
				"user": {"key": "user-key", "name": "user-name", "address": {"city": "SF", "state": "CA"}}}`,
		},
		{
			"anonymous",
			ldcontext.NewBuilder("my-key").Kind("org").
				Anonymous(true).
				Build(),
			EventsConfiguration{},
			`{"kind": "org", "key": "my-key", "anonymous": true}`,
		},
		{
			"all attributes private globally, single kind",
			ldcontext.NewBuilder("my-key").Kind("org").
				Name("my-name").
				SetString("attr1", "value1").
				SetValue("address", objectValue).
				Build(),
			EventsConfiguration{AllAttributesPrivate: true},
			`{"kind": "org", "key": "my-key",
				"_meta": {"redactedAttributes": ["address", "attr1", "name"]}}`,
		},
		{
			"all attributes private globally, multi-kind",
			ldcontext.NewMulti(
				ldcontext.NewBuilder("org-key").Kind("org").
					Name("org-name").
					Build(),
				ldcontext.NewBuilder("user-key").
					Name("user-name").
					SetValue("address", objectValue).
					Build(),
			),
			EventsConfiguration{AllAttributesPrivate: true},
			`{"kind": "multi",
			    "org": {"key": "org-key", "_meta": {"redactedAttributes": ["name"]}},
				"user": {"key": "user-key", "_meta": {"redactedAttributes": ["address", "name"]}}}`,
		},
		{
			"top-level attributes private globally, single kind",
			ldcontext.NewBuilder("my-key").Kind("org").
				Name("my-name").
				SetString("attr1", "value1").
				SetValue("address", objectValue).
				Build(),
			EventsConfiguration{PrivateAttributes: []ldattr.Ref{
				ldattr.NewRef("/name"), ldattr.NewRef("/address")}},
			`{"kind": "org", "key": "my-key", "attr1": "value1",
				"_meta": {"redactedAttributes": ["/address", "/name"]}}`,
		},
		{
			"top-level attributes private globally, multi-kind",
			ldcontext.NewMulti(
				ldcontext.NewBuilder("org-key").Kind("org").
					Name("org-name").
					SetString("attr1", "value1").
					SetString("attr2", "value2").
					Build(),
				ldcontext.NewBuilder("user-key").
					Name("user-name").
					SetString("attr1", "value1").
					SetString("attr3", "value3").
					Build(),
			),
			EventsConfiguration{PrivateAttributes: []ldattr.Ref{
				ldattr.NewRef("/name"), ldattr.NewRef("/attr1"), ldattr.NewRef("/attr3")}},
			`{"kind": "multi",
			    "org": {"key": "org-key", "attr2": "value2", "_meta": {"redactedAttributes": ["/attr1", "/name"]}},
				"user": {"key": "user-key", "_meta": {"redactedAttributes": ["/attr1", "/attr3", "/name"]}}}`,
		},
		{
			"top-level attributes private per context, single kind",
			ldcontext.NewBuilder("my-key").Kind("org").
				Name("my-name").
				SetString("attr1", "value1").
				SetValue("address", objectValue).
				Private("name", "address").
				Build(),
			EventsConfiguration{},
			`{"kind": "org", "key": "my-key", "attr1": "value1",
				"_meta": {"redactedAttributes": ["address", "name"]}}`,
		},
		{
			"top-level attributes private per context, multi-kind",
			ldcontext.NewMulti(
				ldcontext.NewBuilder("org-key").Kind("org").
					SetString("attr1", "value1").
					SetString("attr2", "value2").
					Private("attr1").
					Build(),
				ldcontext.NewBuilder("user-key").
					SetString("attr1", "value1").
					SetString("attr3", "value3").
					Private("attr3").
					Build(),
			),
			EventsConfiguration{},
			`{"kind": "multi",
			    "org": {"key": "org-key", "attr2": "value2", "_meta": {"redactedAttributes": ["attr1"]}},
				"user": {"key": "user-key", "attr1": "value1", "_meta": {"redactedAttributes": ["attr3"]}}}`,
		},
		{
			"nested attribute private globally",
			ldcontext.NewBuilder("my-key").Kind("org").
				Name("my-name").
				SetValue("address", objectValue).
				Build(),
			EventsConfiguration{PrivateAttributes: []ldattr.Ref{ldattr.NewRef("/address/city")}},
			`{"kind": "org", "key": "my-key",
				"name": "my-name", "address": {"state": "CA"},
				"_meta": {"redactedAttributes": ["/address/city"]}}`,
		},
		{
			"nested attribute private per context",
			ldcontext.NewBuilder("my-key").Kind("org").
				Name("my-name").
				SetValue("address", objectValue).
				PrivateRef(ldattr.NewRef("/address/city"), ldattr.NewRef("/name")).
				Build(),
			EventsConfiguration{},
			`{"kind": "org", "key": "my-key", "address": {"state": "CA"},
				"_meta": {"redactedAttributes": ["/address/city", "/name"]}}`,
		},
		{
			"nested attribute private per context, superseded by top-level reference",
			ldcontext.NewBuilder("my-key").Kind("org").
				Name("my-name").
				SetValue("address", objectValue).
				PrivateRef(ldattr.NewRef("/address/city"), ldattr.NewRef("/address")).
				Build(),
			EventsConfiguration{},
			`{"kind": "org", "key": "my-key",
				"name": "my-name", "_meta": {"redactedAttributes": ["/address"]}}`,
		},
		{
			"attribute name is escaped if necessary in redactedAttributes",
			ldcontext.NewBuilder("my-key").Kind("org").
				SetString("/a/b~c", "value").
				Build(),
			EventsConfiguration{AllAttributesPrivate: true},
			`{"kind": "org", "key": "my-key",
				"_meta": {"redactedAttributes": ["/~1a~1b~0c"]}}`,
		},
	} {
		t.Run(p.desc, func(t *testing.T) {
			f := newEventContextFormatter(p.options)
			w := jwriter.NewWriter()
			ec := Context(p.context)
			f.WriteContext(&w, &ec)
			require.NoError(t, w.Error())
			actualJSON := sortPrivateAttributesInOutputJSON(w.Bytes())
			jsonhelpers.AssertEqual(t, p.expectedJSON, actualJSON)
		})
	}
}

func TestPreserializedEventContextFormatterOutput(t *testing.T) {
	userContextPlaceholder := ldcontext.New("user-key")
	userContextJSON := `{"kind": "user", "key": "user-key", "name": "my-name",
		"_meta": {"redactedAttributes": ["/address/city", "name"]}}`
	orgContextPlaceholder := ldcontext.NewWithKind("org", "org-key")
	multiContext := ldcontext.NewMulti(userContextPlaceholder, orgContextPlaceholder)
	multiContextJSON := `{"kind": "multi",
	"org": {"key": "org-key", "name": "org-name",
		"_meta": {"redactedAttributes": ["email"]}},
	"user": {"key": "user-key", "name": "my-name",
		"_meta": {"redactedAttributes": ["/address/city", "name"]}}}`

	type params struct {
		desc         string
		eventContext EventInputContext
		options      EventsConfiguration
		expectedJSON string
	}
	for _, p := range []params{
		{
			"single kind",
			PreserializedContext(userContextPlaceholder, json.RawMessage(userContextJSON)),
			EventsConfiguration{},
			userContextJSON,
			// It's deliberate that there is an unredacted "name" property here even though we also put
			// "name" in the pre-redacted list; that's to prove that we are *not* applying any redaction
			// logic at all when there is a pre-redacted list.
		},
		{
			"multi-kind",
			PreserializedContext(multiContext, json.RawMessage(multiContextJSON)),
			EventsConfiguration{},
			multiContextJSON,
		},
		{
			"config options for private attributes are ignored",
			PreserializedContext(userContextPlaceholder, json.RawMessage(userContextJSON)),
			EventsConfiguration{AllAttributesPrivate: true},
			userContextJSON,
		},
	} {
		t.Run(p.desc, func(t *testing.T) {
			f := newEventContextFormatter(p.options)
			w := jwriter.NewWriter()
			f.WriteContext(&w, &p.eventContext)
			require.NoError(t, w.Error())
			actualJSON := w.Bytes() // don't need to sort the private attrs here because they are copied straight from the input
			jsonhelpers.AssertEqual(t, p.expectedJSON, actualJSON)
		})
	}
}

func TestWriteInvalidContext(t *testing.T) {
	badContext := ldcontext.New("")
	f := newEventContextFormatter(EventsConfiguration{})
	w := jwriter.NewWriter()
	ec := Context(badContext)
	f.WriteContext(&w, &ec)
	assert.Equal(t, badContext.Err(), w.Error())
}

func sortPrivateAttributesInOutputJSON(data []byte) []byte {
	parsed := ldvalue.Parse(data)
	if parsed.Type() != ldvalue.ObjectType {
		return data
	}
	var ret ldvalue.Value
	if parsed.GetByKey("kind").StringValue() != "multi" {
		ret = sortPrivateAttributesInSingleKind(parsed)
	} else {
		out := ldvalue.ObjectBuildWithCapacity(parsed.Count())
		for k, v := range parsed.AsValueMap().AsMap() {
			if k == "kind" {
				out.Set(k, v)
			} else {
				out.Set(k, sortPrivateAttributesInSingleKind(v))
			}
		}
		ret = out.Build()
	}
	return []byte(ret.JSONString())
}

func sortPrivateAttributesInSingleKind(parsed ldvalue.Value) ldvalue.Value {
	out := ldvalue.ObjectBuildWithCapacity(parsed.Count())
	for k, v := range parsed.AsValueMap().AsMap() {
		if k != "_meta" {
			out.Set(k, v)
			continue
		}
		outMeta := ldvalue.ObjectBuildWithCapacity(v.Count())
		for k1, v1 := range v.AsValueMap().AsMap() {
			if k1 != "redactedAttributes" {
				outMeta.Set(k1, v1)
				continue
			}
			values := v1.AsValueArray().AsSlice()
			sort.Slice(values, func(i, j int) bool {
				return values[i].StringValue() < values[j].StringValue()
			})
			outMeta.Set(k1, ldvalue.ArrayOf(values...))
		}
		out.Set(k, outMeta.Build())
	}
	return out.Build()
}
