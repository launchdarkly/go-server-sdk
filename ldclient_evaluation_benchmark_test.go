package ldclient

import (
	"encoding/json"
	"fmt"
	"testing"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datakinds"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
)

// These benchmarks cover the LDClient evaluation flow, including looking up the target flag, applying all
// relevant targets and rules, and producing analytics event data (but then discarding the event data,
// so the event processor logic is not included).
//
// This was adapted from a user-contributed PR: https://github.com/launchdarkly/go-server-sdk/pull/28

type evalBenchmarkEnv struct {
	client           *LDClient
	evalUser         lduser.User
	targetFeatureKey string
	targetUsers      []lduser.User
}

func newEvalBenchmarkEnv() *evalBenchmarkEnv {
	return &evalBenchmarkEnv{}
}

func (env *evalBenchmarkEnv) setUp(withEventGeneration bool, bc evalBenchmarkCase, variations []ldvalue.Value) {
	// Set up the client.
	env.client = makeTestClientWithConfig(func(c *Config) {
		if withEventGeneration {
			// In this mode, we use a stub EventProcessor implementation that immediately discards
			// every event, but the SDK will still generate the events before passing them to the stub,
			// so we are still measuring the overhead of that.
			c.Events = benchmarkStubEventProcessorFactory{}
		} else {
			// Completely disable all event functionality, so we are only testing the evaluation logic
			// (plus retrieval of the flag from the in-memory store). The SDK only behaves this way if
			// Events is set to the specific factory returned by NoEvents().
			c.Events = ldcomponents.NoEvents()
		}
	})

	// Set up the feature flag store. Note that we're using a regular in-memory data store, so the
	// benchmarks will include the overhead of calling Get on the store.
	testFlags := makeEvalBenchmarkFlags(bc, variations)
	for _, ff := range testFlags {
		env.client.store.Upsert(datakinds.Features, ff.Key, sharedtest.FlagDescriptor(*ff))
	}

	env.evalUser = makeEvalBenchmarkUser(bc)

	// Target a feature key in the middle of the list in case a linear search is being used.
	targetFeatureKeyIndex := 0
	if bc.numFlags > 0 {
		targetFeatureKeyIndex = bc.numFlags / 2
	}
	env.targetFeatureKey = fmt.Sprintf("flag-%d", targetFeatureKeyIndex)

	// Create users to match all of the user keys in the flag's target list. These will be used
	// only in BenchmarkUsersFoundInTargets; with all the other benchmarks, we are deliberately
	// using a user key that is *not* found in the targets.
	env.targetUsers = make([]lduser.User, bc.numTargets)
	for i := 0; i < bc.numTargets; i++ {
		env.targetUsers[i] = lduser.NewUser(makeEvalBenchmarkTargetUserKey(i))
	}
}

func (env *evalBenchmarkEnv) tearDown() {
	// Prepare for the next benchmark case.
	env.client.Close()
	env.client = nil
	env.targetFeatureKey = ""
}

type benchmarkStubEventProcessorFactory struct{}

func (f benchmarkStubEventProcessorFactory) CreateEventProcessor(context interfaces.ClientContext) (ldevents.EventProcessor, error) {
	return ldcomponents.NoEvents().CreateEventProcessor(context)
}

func makeEvalBenchmarkUser(bc evalBenchmarkCase) lduser.User {
	if bc.shouldMatch {
		builder := lduser.NewUserBuilder("user-match")
		switch bc.operator {
		case ldmodel.OperatorGreaterThan:
			builder.Custom("numAttr", ldvalue.Int(10000))
		case ldmodel.OperatorContains:
			builder.Name("name-0")
		case ldmodel.OperatorMatches:
			builder.Custom("stringAttr", ldvalue.String("stringAttr-0"))
		case ldmodel.OperatorAfter:
			builder.Custom("dateAttr", ldvalue.String("2999-12-31T00:00:00.000-00:00"))
		case ldmodel.OperatorIn:
			builder.Custom("stringAttr", ldvalue.String("stringAttr-0"))
		}
		return builder.Build()
	}
	// default is that the user will not be matched by any clause or target
	return lduser.NewUserBuilder("user-nomatch").
		Name("name-nomatch").
		Custom("stringAttr", ldvalue.String("stringAttr-nomatch")).
		Custom("numAttr", ldvalue.Int(0)).
		Custom("dateAttr", ldvalue.String("1980-01-01T00:00:00.000-00:00")).
		Build()
}

type evalBenchmarkCase struct {
	numUsers      int
	numFlags      int
	numVariations int
	numTargets    int
	numRules      int
	numClauses    int
	prereqsWidth  int
	prereqsDepth  int
	operator      ldmodel.Operator
	shouldMatch   bool
}

var ruleEvalBenchmarkCases = []evalBenchmarkCase{
	// simple
	{
		numUsers:      1000,
		numFlags:      1000,
		numVariations: 2,
		numTargets:    1,
	},

	// realistic
	{
		numUsers:      10000,
		numFlags:      10000,
		numVariations: 2,
		numTargets:    1,
	},
	{
		numUsers:      10000,
		numFlags:      10000,
		numVariations: 2,
		numTargets:    10,
	},
	{
		numUsers:      10000,
		numFlags:      1000,
		numVariations: 2,
		numRules:      1,
		numClauses:    1,
	},
	{
		numUsers:      10000,
		numFlags:      1000,
		numVariations: 2,
		numRules:      1,
		numClauses:    3,
	},
	{
		numUsers:      10000,
		numFlags:      1000,
		numVariations: 2,
		numRules:      5,
		numClauses:    3,
	},

	// prereqs
	{
		numUsers:      10000,
		numFlags:      1000,
		numVariations: 2,
		numRules:      1,
		numClauses:    1,
		prereqsWidth:  5,
		prereqsDepth:  1,
	},
	{
		numUsers:      10000,
		numFlags:      1000,
		numVariations: 2,
		numRules:      1,
		numClauses:    1,
		prereqsWidth:  1,
		prereqsDepth:  5,
	},
	{
		numUsers:      10000,
		numFlags:      1000,
		numVariations: 2,
		numTargets:    1,
		prereqsWidth:  2,
		prereqsDepth:  2,
	},
	{
		numUsers:      10000,
		numFlags:      1000,
		numVariations: 2,
		numRules:      1,
		numClauses:    1,
		prereqsWidth:  5,
		prereqsDepth:  5,
	},

	// operations - if not specified, the default is OperatorIn
	{
		numUsers:      10000,
		numFlags:      1000,
		numVariations: 2,
		numRules:      1,
		numClauses:    1,
		operator:      ldmodel.OperatorGreaterThan,
	},
	{
		numUsers:      10000,
		numFlags:      1000,
		numVariations: 2,
		numRules:      1,
		numClauses:    1,
		operator:      ldmodel.OperatorContains,
	},
	{
		numUsers:      10000,
		numFlags:      1000,
		numVariations: 2,
		numRules:      1,
		numClauses:    1,
		operator:      ldmodel.OperatorMatches,
	},
}

var targetMatchBenchmarkCases = []evalBenchmarkCase{
	{
		numUsers:      1000,
		numFlags:      1000,
		numVariations: 2,
		numTargets:    10,
	},
	{
		numUsers:      1000,
		numFlags:      1000,
		numVariations: 2,
		numTargets:    100,
	},
	{
		numUsers:      1000,
		numFlags:      1000,
		numVariations: 2,
		numTargets:    1000,
	},
}

var ruleMatchBenchmarkCases = []evalBenchmarkCase{
	// These cases are deliberately simple because the benchmark is meant to focus on the evaluation of
	// one specific type of matching operation. The user will match the first clause in the first rule.
	{
		numFlags:      1,
		numRules:      1,
		numClauses:    1,
		numVariations: 2,
		operator:      ldmodel.OperatorIn,
		shouldMatch:   true,
	},
	{
		numFlags:      1,
		numRules:      1,
		numClauses:    1,
		numVariations: 2,
		operator:      ldmodel.OperatorContains,
		shouldMatch:   true,
	},
	{
		numFlags:      1,
		numRules:      1,
		numClauses:    1,
		numVariations: 2,
		operator:      ldmodel.OperatorGreaterThan,
		shouldMatch:   true,
	},
	{
		numFlags:      1,
		numRules:      1,
		numClauses:    1,
		numVariations: 2,
		operator:      ldmodel.OperatorAfter,
		shouldMatch:   true,
	},
	{
		numFlags:      1,
		numRules:      1,
		numClauses:    1,
		numVariations: 2,
		operator:      ldmodel.OperatorMatches,
		shouldMatch:   true,
	},
}

var (
	// Always record the result of an operation to prevent the compiler eliminating the function call.
	//
	// Always store the result to a package level variable so the compiler cannot eliminate the benchmark itself.
	boolResult   bool
	intResult    int
	stringResult string
	jsonResult   ldvalue.Value
)

func benchmarkEval(
	b *testing.B,
	withEventGeneration bool,
	makeVariation func(int) ldvalue.Value,
	cases []evalBenchmarkCase,
	action func(*evalBenchmarkEnv),
) {
	env := newEvalBenchmarkEnv()
	for _, bc := range cases {
		variations := make([]ldvalue.Value, bc.numVariations)
		for i := 0; i < bc.numVariations; i++ {
			variations[i] = makeVariation(i)
		}
		env.setUp(withEventGeneration, bc, variations)

		b.Run(fmt.Sprintf("%+v", bc), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				action(env)
			}
		})
		env.tearDown()
	}
}

// This benchmark executes only a single basic evaluation case. It is mainly useful in very
// detailed profiling and allocation tracing where you don't want a huge log file.
func BenchmarkSingleVariation(b *testing.B) {
	singleCase := []evalBenchmarkCase{ruleEvalBenchmarkCases[0]}
	benchmarkEval(b, false, makeBoolVariation, singleCase, func(env *evalBenchmarkEnv) {
		boolResult, _ = env.client.BoolVariation(env.targetFeatureKey, env.evalUser, false)
	})
}

func BenchmarkSingleVariationWithEvents(b *testing.B) {
	singleCase := []evalBenchmarkCase{ruleEvalBenchmarkCases[0]}
	benchmarkEval(b, true, makeBoolVariation, singleCase, func(env *evalBenchmarkEnv) {
		boolResult, _ = env.client.BoolVariation(env.targetFeatureKey, env.evalUser, false)
	})
}

func BenchmarkBoolVariation(b *testing.B) {
	benchmarkEval(b, false, makeBoolVariation, ruleEvalBenchmarkCases, func(env *evalBenchmarkEnv) {
		boolResult, _ = env.client.BoolVariation(env.targetFeatureKey, env.evalUser, false)
	})
}

// The ___WithEvents version of the benchmark enables the LDClient code path that creates an evaluation
// event instance, even though the event will not be sent anywhere, so we can measure the overhead of
// that step. It is not repeated for BenchmarkIntVariation, etc., because the data type of the
// variation makes no difference in how events are generated.
func BenchmarkBoolVariationWithEvents(b *testing.B) {
	benchmarkEval(b, true, makeBoolVariation, ruleEvalBenchmarkCases, func(env *evalBenchmarkEnv) {
		boolResult, _ = env.client.BoolVariation(env.targetFeatureKey, env.evalUser, false)
	})
}

func BenchmarkIntVariation(b *testing.B) {
	benchmarkEval(b, false, makeIntVariation, ruleEvalBenchmarkCases, func(env *evalBenchmarkEnv) {
		intResult, _ = env.client.IntVariation(env.targetFeatureKey, env.evalUser, 0)
	})
}

func BenchmarkStringVariation(b *testing.B) {
	benchmarkEval(b, false, makeStringVariation, ruleEvalBenchmarkCases, func(env *evalBenchmarkEnv) {
		stringResult, _ = env.client.StringVariation(env.targetFeatureKey, env.evalUser, "variation-0")
	})
}

func BenchmarkJSONVariation(b *testing.B) {
	defaultValAsRawJSON := ldvalue.Raw(json.RawMessage(`{"result":{"value":[0]}}`))
	benchmarkEval(b, false, makeJSONVariation, ruleEvalBenchmarkCases, func(env *evalBenchmarkEnv) {
		jsonResult, _ = env.client.JSONVariation(env.targetFeatureKey, env.evalUser, defaultValAsRawJSON)
	})
}

func BenchmarkUsersFoundInTargets(b *testing.B) {
	benchmarkEval(b, false, makeBoolVariation,
		targetMatchBenchmarkCases,
		func(env *evalBenchmarkEnv) {
			for _, user := range env.targetUsers {
				r, _ := env.client.BoolVariation(env.targetFeatureKey, user, false)
				boolResult = r
			}
		})
}

func BenchmarkUserNotFoundInTargets(b *testing.B) {
	benchmarkEval(b, false, makeBoolVariation,
		targetMatchBenchmarkCases,
		func(env *evalBenchmarkEnv) {
			for _ = range env.targetUsers {
				r, _ := env.client.BoolVariation(env.targetFeatureKey, env.evalUser, false)
				boolResult = r
			}
		})
}

func BenchmarkUserMatchesRule(b *testing.B) {
	benchmarkEval(b, false, makeBoolVariation,
		ruleMatchBenchmarkCases,
		func(env *evalBenchmarkEnv) {
			boolResult, _ = env.client.BoolVariation(env.targetFeatureKey, env.evalUser, false)
		})
}

// Input data creation

// Except for when we're running BenchmarkUserMatchesRule, the flag rules and clauses we create here are
// intended *not* to match the user, so the more of them we create, the more we are testing the overhead
// of iterating through and evaluating all the clauses.

func makeBoolVariation(i int) ldvalue.Value {
	return ldvalue.Bool(i%2 == 0)
}

func makeIntVariation(i int) ldvalue.Value {
	return ldvalue.Int(i)
}

func makeStringVariation(i int) ldvalue.Value {
	return ldvalue.String(fmt.Sprintf("variation-%d", i))
}

func makeJSONVariation(i int) ldvalue.Value {
	return ldvalue.ObjectBuild().Set(
		"result",
		ldvalue.ObjectBuild().Set(
			"value", ldvalue.ArrayBuild().Add(ldvalue.Int(i)).Build(),
		).Build(),
	).Build()
}

func makeEvalBenchmarkClauses(numClauses int, op ldmodel.Operator) []ldmodel.Clause {
	clauses := make([]ldmodel.Clause, 0, numClauses)
	for i := 0; i < numClauses; i++ {
		clause := ldmodel.Clause{Op: op}
		switch op {
		case ldmodel.OperatorGreaterThan:
			clause.Attribute = "numAttr"
			clause.Values = []ldvalue.Value{ldvalue.Int(i)}
		case ldmodel.OperatorContains:
			clause.Attribute = "name"
			clause.Values = []ldvalue.Value{
				ldvalue.String(fmt.Sprintf("name-%d", i)),
				ldvalue.String(fmt.Sprintf("name-%d", i+1)),
				ldvalue.String(fmt.Sprintf("name-%d", i+2)),
			}
		case ldmodel.OperatorMatches:
			clause.Attribute = "stringAttr"
			clause.Values = []ldvalue.Value{
				ldvalue.String(fmt.Sprintf("stringAttr-%d", i)),
				ldvalue.String(fmt.Sprintf("stringAttr-%d", i+1)),
				ldvalue.String(fmt.Sprintf("stringAttr-%d", i+2)),
			}
		case ldmodel.OperatorAfter:
			clause.Attribute = "dateAttr"
			clause.Values = []ldvalue.Value{
				ldvalue.String(fmt.Sprintf("%d-01-01T00:00:00.000-00:00", 2000+i)),
				ldvalue.String(fmt.Sprintf("%d-01-01T00:00:00.000-00:00", 2001+i)),
				ldvalue.String(fmt.Sprintf("%d-01-01T00:00:00.000-00:00", 2002+i)),
			}
		default:
			clause.Op = ldmodel.OperatorIn
			clause.Attribute = "stringAttr"
			clause.Values = []ldvalue.Value{
				ldvalue.String(fmt.Sprintf("stringAttr-%d", i)),
				ldvalue.String(fmt.Sprintf("stringAttr-%d", i+1)),
				ldvalue.String(fmt.Sprintf("stringAttr-%d", i+2)),
			}
		}
		clauses = append(clauses, clause)
	}
	return clauses
}

func makeEvalBenchmarkTargetUserKey(i int) string {
	return fmt.Sprintf("user-%d", i)
}

func makeEvalBenchmarkFlags(bc evalBenchmarkCase, variations []ldvalue.Value) []*ldmodel.FeatureFlag {
	testFlags := make([]*ldmodel.FeatureFlag, 0, bc.numFlags)
	for i := 0; i < bc.numFlags; i++ {
		flag := ldbuilders.NewFlagBuilder(fmt.Sprintf("flag-%d", i)).
			Version(1).
			On(true).
			FallthroughVariation(1)
		for j := 0; j < bc.numVariations; j++ {
			values := make([]string, bc.numTargets)
			for k := 0; k < bc.numTargets; k++ {
				values[k] = makeEvalBenchmarkTargetUserKey(k)
			}
			flag.AddTarget(j, values...)
		}
		for j := 0; j < bc.numRules; j++ {
			flag.AddRule(ldbuilders.NewRuleBuilder().
				ID(fmt.Sprintf("%d-%d", i, j)).
				Clauses(makeEvalBenchmarkClauses(bc.numClauses, bc.operator)...).
				Variation(0))
		}
		f := flag.Build()
		testFlags = append(testFlags, &f)
	}

	if bc.prereqsWidth > 0 && bc.prereqsDepth > 0 {
		assignPrereqTree(testFlags, bc.prereqsWidth, bc.prereqsDepth)
	}

	return testFlags
}

// assignPrereqTree assigns prerequisites to each of the given feature flags such that each flag
// has at most `width` children and `depth` ancestors. If the depth of the prerequisite "tree"
// exceeds `depth`, a new tree is assigned starting with the next feature flag the root node.
func assignPrereqTree(flags []*ldmodel.FeatureFlag, width, depth int) {
	var parentLevel []*ldmodel.FeatureFlag
	levelIndex := 0

	i := 0
	for i < len(flags) {
		if levelIndex > depth {
			levelIndex = 0
			parentLevel = []*ldmodel.FeatureFlag{flags[i]}
		}
		if levelIndex == 0 {
			levelIndex++
			i++
			continue
		}

		var childLevel []*ldmodel.FeatureFlag
		for _, parent := range parentLevel {
			for w := 0; w < width && i+w < len(flags); w++ {
				child := flags[i+w]
				child.Prerequisites = []ldmodel.Prerequisite{{Key: parent.Key, Variation: 0}}
				childLevel = append(childLevel, child)
			}
			i += width
		}
		parentLevel = childLevel
		levelIndex++
	}
}
