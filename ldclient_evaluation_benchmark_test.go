package ldclient

import (
	"encoding/json"
	"fmt"
	"testing"

	"gopkg.in/launchdarkly/go-sdk-common.v1/ldvalue"
)

// These benchmarks cover the LDClient evaluation flow, including looking up the target flag, applying all
// relevant targets and rules, and producing analytics event data (but then discarding the event data,
// so the event processor logic is not included).
//
// This was adapted from a user-contributed PR: https://github.com/launchdarkly/go-server-sdk/pull/28

type evalBenchmarkEnv struct {
	client           *LDClient
	evalUser         User
	targetFeatureKey string
	targetUsers      []User
}

func newEvalBenchmarkEnv() *evalBenchmarkEnv {
	return &evalBenchmarkEnv{}
}

func (env *evalBenchmarkEnv) setUp(bc evalBenchmarkCase, variations []interface{}) {
	// Set up the client.
	env.client = makeTestClientWithConfig(func(c *Config) {
		c.SendEvents = false
		c.EventProcessor = nil
	})

	// Set up the feature flag store. Note that we're using a regular in-memory data store, so the
	// benchmarks will include the overhead of calling Get on the store.
	testFlags := makeEvalBenchmarkFlags(bc, variations)
	for _, ff := range testFlags {
		env.client.store.Upsert(Features, ff)
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
	env.targetUsers = make([]User, bc.numTargets)
	for i := 0; i < bc.numTargets; i++ {
		env.targetUsers[i] = NewUser(makeEvalBenchmarkTargetUserKey(i))
	}
}

func (env *evalBenchmarkEnv) tearDown() {
	// Prepare for the next benchmark case.
	env.client.Close()
	env.client = nil
	env.targetFeatureKey = ""
}

func makeEvalBenchmarkUser(bc evalBenchmarkCase) User {
	if bc.shouldMatch {
		builder := NewUserBuilder("user-match")
		switch bc.operator {
		case OperatorGreaterThan:
			builder.Custom("numAttr", ldvalue.Int(10000))
		case OperatorContains:
			builder.Name("name-0")
		case OperatorMatches:
			builder.Custom("stringAttr", ldvalue.String("stringAttr-0"))
		case OperatorAfter:
			builder.Custom("dateAttr", ldvalue.String("2999-12-31T00:00:00.000-00:00"))
		case OperatorIn:
			builder.Custom("stringAttr", ldvalue.String("stringAttr-0"))
		}
		return builder.Build()
	}
	// default is that the user will not be matched by any clause or target
	return NewUserBuilder("user-nomatch").
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
	operator      Operator
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
		operator:      OperatorGreaterThan,
	},
	{
		numUsers:      10000,
		numFlags:      1000,
		numVariations: 2,
		numRules:      1,
		numClauses:    1,
		operator:      OperatorContains,
	},
	{
		numUsers:      10000,
		numFlags:      1000,
		numVariations: 2,
		numRules:      1,
		numClauses:    1,
		operator:      OperatorMatches,
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
		operator:      OperatorIn,
		shouldMatch:   true,
	},
	{
		numFlags:      1,
		numRules:      1,
		numClauses:    1,
		numVariations: 2,
		operator:      OperatorContains,
		shouldMatch:   true,
	},
	{
		numFlags:      1,
		numRules:      1,
		numClauses:    1,
		numVariations: 2,
		operator:      OperatorGreaterThan,
		shouldMatch:   true,
	},
	{
		numFlags:      1,
		numRules:      1,
		numClauses:    1,
		numVariations: 2,
		operator:      OperatorAfter,
		shouldMatch:   true,
	},
	{
		numFlags:      1,
		numRules:      1,
		numClauses:    1,
		numVariations: 2,
		operator:      OperatorMatches,
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

func benchmarkEval(b *testing.B, makeVariation func(int) interface{}, cases []evalBenchmarkCase, action func(*evalBenchmarkEnv)) {
	env := newEvalBenchmarkEnv()
	for _, bc := range cases {
		variations := make([]interface{}, bc.numVariations)
		for i := 0; i < bc.numVariations; i++ {
			variations[i] = makeVariation(i)
		}
		env.setUp(bc, variations)

		b.Run(fmt.Sprintf("%+v", bc), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				action(env)
			}
		})
		env.tearDown()
	}
}

func BenchmarkBoolVariation(b *testing.B) {
	benchmarkEval(b, makeBoolVariation, ruleEvalBenchmarkCases, func(env *evalBenchmarkEnv) {
		r, _ := env.client.BoolVariation(env.targetFeatureKey, env.evalUser, false)
		boolResult = r
	})
}

func BenchmarkIntVariation(b *testing.B) {
	benchmarkEval(b, makeIntVariation, ruleEvalBenchmarkCases, func(env *evalBenchmarkEnv) {
		r, _ := env.client.IntVariation(env.targetFeatureKey, env.evalUser, 0)
		intResult = r
	})
}

func BenchmarkStringVariation(b *testing.B) {
	benchmarkEval(b, makeStringVariation, ruleEvalBenchmarkCases, func(env *evalBenchmarkEnv) {
		r, _ := env.client.StringVariation(env.targetFeatureKey, env.evalUser, "variation-0")
		stringResult = r
	})
}

func BenchmarkJSONVariation(b *testing.B) {
	defaultValAsRawJSON := ldvalue.Raw(json.RawMessage(`{"result":{"value":[0]}}`))
	benchmarkEval(b, makeJSONVariation, ruleEvalBenchmarkCases, func(env *evalBenchmarkEnv) {
		r, _ := env.client.JSONVariation(env.targetFeatureKey, env.evalUser, defaultValAsRawJSON)
		jsonResult = r
	})
}

func BenchmarkUsersFoundInTargets(b *testing.B) {
	benchmarkEval(b, makeBoolVariation,
		targetMatchBenchmarkCases,
		func(env *evalBenchmarkEnv) {
			for _, user := range env.targetUsers {
				r, _ := env.client.BoolVariation(env.targetFeatureKey, user, false)
				boolResult = r
			}
		})
}

func BenchmarkUserNotFoundInTargets(b *testing.B) {
	benchmarkEval(b, makeBoolVariation,
		targetMatchBenchmarkCases,
		func(env *evalBenchmarkEnv) {
			for _ = range env.targetUsers {
				r, _ := env.client.BoolVariation(env.targetFeatureKey, env.evalUser, false)
				boolResult = r
			}
		})
}

func BenchmarkUserMatchesRule(b *testing.B) {
	benchmarkEval(b, makeBoolVariation,
		ruleMatchBenchmarkCases,
		func(env *evalBenchmarkEnv) {
			boolResult, _ = env.client.BoolVariation(env.targetFeatureKey, env.evalUser, false)
		})
}

// Input data creation

// Except for when we're running BenchmarkUserMatchesRule, the flag rules and clauses we create here are
// intended *not* to match the user, so the more of them we create, the more we are testing the overhead
// of iterating through and evaluating all the clauses.

func newBenchmarkFlag(key string, fallThroughVariation int, targets []Target, rules []Rule, prereqs []Prerequisite, variations ...interface{}) *FeatureFlag {
	return &FeatureFlag{
		Key:           key,
		Version:       1,
		On:            true,
		Fallthrough:   VariationOrRollout{Variation: &fallThroughVariation},
		Variations:    variations,
		Rules:         rules,
		Targets:       targets,
		Prerequisites: prereqs,
	}
}

func makeBoolVariation(i int) interface{} {
	return bool(i%2 == 0)
}

func makeIntVariation(i int) interface{} {
	return i
}

func makeStringVariation(i int) interface{} {
	return fmt.Sprintf("variation-%d", i)
}

func makeJSONVariation(i int) interface{} {
	return map[string]interface{}{"result": map[string]interface{}{"value": []interface{}{i}}}
}

func makeEvalBenchmarkClauses(numClauses int, op Operator) []Clause {
	clauses := make([]Clause, 0, numClauses)
	for i := 0; i < numClauses; i++ {
		clause := Clause{Op: op}
		switch op {
		case OperatorGreaterThan:
			clause.Attribute = "numAttr"
			clause.Values = []interface{}{i}
		case OperatorContains:
			clause.Attribute = "name"
			clause.Values = []interface{}{
				fmt.Sprintf("name-%d", i),
				fmt.Sprintf("name-%d", i+1),
				fmt.Sprintf("name-%d", i+2),
			}
		case OperatorMatches:
			clause.Attribute = "stringAttr"
			clause.Values = []interface{}{
				fmt.Sprintf("stringAttr-%d", i),
				fmt.Sprintf("stringAttr-%d", i+1),
				fmt.Sprintf("stringAttr-%d", i+2),
			}
		case OperatorAfter:
			clause.Attribute = "dateAttr"
			clause.Values = []interface{}{
				fmt.Sprintf("%d-01-01T00:00:00.000-00:00", 2000+i),
				fmt.Sprintf("%d-01-01T00:00:00.000-00:00", 2001+i),
				fmt.Sprintf("%d-01-01T00:00:00.000-00:00", 2002+i),
			}
		default:
			clause.Op = OperatorIn
			clause.Attribute = "stringAttr"
			clause.Values = []interface{}{
				fmt.Sprintf("stringAttr-%d", i),
				fmt.Sprintf("stringAttr-%d", i+1),
				fmt.Sprintf("stringAttr-%d", i+2),
			}
		}
		clauses = append(clauses, clause)
	}
	return clauses
}

func makeEvalBenchmarkTargetUserKey(i int) string {
	return fmt.Sprintf("user-%d", i)
}

func makeEvalBenchmarkFlags(bc evalBenchmarkCase, variations []interface{}) []*FeatureFlag {
	testFlags := make([]*FeatureFlag, 0, bc.numFlags)
	for i := 0; i < bc.numFlags; i++ {
		targets := make([]Target, 0, bc.numVariations)
		for j := 0; j < bc.numVariations; j++ {
			values := make([]string, 0, bc.numTargets)
			for k := 0; k < bc.numTargets; k++ {
				values = append(values, makeEvalBenchmarkTargetUserKey(k))
			}
			targets = append(targets, Target{
				Values:    values,
				Variation: j,
			})
		}
		rules := make([]Rule, 0, bc.numRules)
		variation := 0
		for j := 0; j < bc.numRules; j++ {
			rules = append(rules, Rule{
				ID:      fmt.Sprintf("%d-%d", i, j),
				Clauses: makeEvalBenchmarkClauses(bc.numClauses, bc.operator),
				VariationOrRollout: VariationOrRollout{
					Variation: &variation,
				},
			})
		}

		testFlag := newBenchmarkFlag(fmt.Sprintf("flag-%d", i), 1, targets, rules, nil, variations...)
		testFlags = append(testFlags, testFlag)
	}

	if bc.prereqsWidth > 0 && bc.prereqsDepth > 0 {
		assignPrereqTree(testFlags, bc.prereqsWidth, bc.prereqsDepth)
	}

	return testFlags
}

// assignPrereqTree assigns prerequisites to each of the given feature flags such that each flag
// has at most `width` children and `depth` ancestors. If the depth of the prerequisite "tree"
// exceeds `depth`, a new tree is assigned starting with the next feature flag the root node.
func assignPrereqTree(flags []*FeatureFlag, width, depth int) {
	var parentLevel []*FeatureFlag
	levelIndex := 0

	i := 0
	for i < len(flags) {
		if levelIndex > depth {
			levelIndex = 0
			parentLevel = []*FeatureFlag{flags[i]}
		}
		if levelIndex == 0 {
			levelIndex++
			i++
			continue
		}

		var childLevel []*FeatureFlag
		for _, parent := range parentLevel {
			for w := 0; w < width && i+w < len(flags); w++ {
				child := flags[i+w]
				child.Prerequisites = []Prerequisite{{Key: parent.Key, Variation: 0}}
				childLevel = append(childLevel, child)
			}
			i += width
		}
		parentLevel = childLevel
		levelIndex++
	}
}
