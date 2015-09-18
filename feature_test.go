package ldclient

import (
	"testing"
)

func TestTargetRuleWithBooleanValueMatches(t *testing.T) {
	custom := make(map[string]interface{})
	key := "test@test.com"

	custom["member"] = true
	user := User{
		Key:    &key,
		Custom: &custom,
	}

	rule := TargetRule{
		Attribute: "member",
		Op:        "in",
		Values:    []interface{}{true},
	}

	if !rule.matchCustom(user) {
		t.Errorf("Custom rule should match, but doesn't")
	}
}

func TestTargetRuleWithIntValueMatches(t *testing.T) {
	custom := make(map[string]interface{})
	key := "test@test.com"

	custom["customer_rank"] = 10000
	user := User{
		Key:    &key,
		Custom: &custom,
	}

	rule := TargetRule{
		Attribute: "customer_rank",
		Op:        "in",
		Values:    []interface{}{10000, 20000},
	}

	if !rule.matchCustom(user) {
		t.Errorf("Custom rule should match, but doesn't")
	}
}

func TestTargetRuleWithIntValueDoesNotMatch(t *testing.T) {
	custom := make(map[string]interface{})
	key := "test@test.com"

	custom["customer_rank"] = 10000
	user := User{
		Key:    &key,
		Custom: &custom,
	}

	rule := TargetRule{
		Attribute: "customer_rank",
		Op:        "in",
		Values:    []interface{}{30000, 20000},
	}

	if rule.matchCustom(user) {
		t.Errorf("Custom rule should not match, but does")
	}
}

func TestTargetRuleWithStringValueMatches(t *testing.T) {
	custom := make(map[string]interface{})
	key := "test@test.com"

	custom["group"] = "microsoft"
	user := User{
		Key:    &key,
		Custom: &custom,
	}

	rule := TargetRule{
		Attribute: "group",
		Op:        "in",
		Values:    []interface{}{"microsoft"},
	}

	if !rule.matchCustom(user) {
		t.Errorf("Custom rule should match, but doesn't")
	}
}

func TestTargetRuleWithStringValuesMatches(t *testing.T) {
	custom := make(map[string]interface{})
	key := "test@test.com"

	custom["groups"] = []string{"microsoft", "google"}
	user := User{
		Key:    &key,
		Custom: &custom,
	}

	rule := TargetRule{
		Attribute: "groups",
		Op:        "in",
		Values:    []interface{}{"microsoft"},
	}

	if !rule.matchCustom(user) {
		t.Errorf("Custom rule should match, but doesn't")
	}
}

func TestTargetRuleWithStringArrayMatches(t *testing.T) {
	custom := make(map[string]interface{})
	key := "test@test.com"

	custom["groups"] = [2]string{"microsoft", "google"}
	user := User{
		Key:    &key,
		Custom: &custom,
	}

	rule := TargetRule{
		Attribute: "groups",
		Op:        "in",
		Values:    []interface{}{"microsoft"},
	}

	if !rule.matchCustom(user) {
		t.Errorf("Custom rule should match, but doesn't")
	}
}

func TestTargetRuleWithHeterogenousArrayMatches(t *testing.T) {
	custom := make(map[string]interface{})
	key := "test@test.com"

	custom["groups"] = [2]interface{}{3, "microsoft"}
	user := User{
		Key:    &key,
		Custom: &custom,
	}

	rule := TargetRule{
		Attribute: "groups",
		Op:        "in",
		Values:    []interface{}{"microsoft"},
	}

	if !rule.matchCustom(user) {
		t.Errorf("Custom rule should match, but doesn't")
	}
}
