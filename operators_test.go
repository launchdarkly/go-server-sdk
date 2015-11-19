package ldclient

import (
	"testing"
)

func TestStartsWithOperator(t *testing.T) {
	var bar = "bar"
	if !operatorStartsWithFn("bar@foo", bar) {
		t.Errorf("Expected %s to start with bar", "foo@bar")
	}

	if !operatorStartsWithFn("bar", bar) {
		t.Errorf("Expected %s to start with bar", "bar")
	}

	if operatorStartsWithFn(4, bar) {
		t.Errorf("Did not expect %d to start with bar", 4)
	}

	if operatorStartsWithFn(true, bar) {
		t.Errorf("Did not expect %t to start with bar", true)
	}
}

func TestEndsWithOperator(t *testing.T) {
	var bar = "bar"
	if !operatorEndsWithFn("foo@bar", bar) {
		t.Errorf("Expected %s to end with bar", "foo@bar")
	}

	if !operatorEndsWithFn("bar", bar) {
		t.Errorf("Expected %s to end with bar", "bar")
	}

	if operatorEndsWithFn(4, bar) {
		t.Errorf("Did not expect %d to end with bar", 4)
	}

	if operatorEndsWithFn(true, bar) {
		t.Errorf("Did not expect %t to end with bar", true)
	}
}

func TestContainsOperator(t *testing.T) {
	var bar = "bar"
	if !operatorContainsFn("foo@bar", bar) {
		t.Errorf("Expected %s to contain bar", "foo@bar")
	}

	if !operatorContainsFn("abarba", bar) {
		t.Errorf("Expected %s to contain a", "bar")
	}

	if operatorContainsFn(4, bar) {
		t.Errorf("Did not expect %d to contain bar", 4)
	}

	if operatorContainsFn(true, bar) {
		t.Errorf("Did not expect %t to contain bar", true)
	}
}

func TestMatchesOperator(t *testing.T) {
	var pattern = "[A-Za-z]+"

	if !operatorMatchesFn("Ozzz", pattern) {
		t.Errorf("Expected %S to match pattern %s", "Ozzz", pattern)
	}

	if operatorMatchesFn("", pattern) {
		t.Errorf("Did not expect empty string to match pattern %s", pattern)
	}

}
