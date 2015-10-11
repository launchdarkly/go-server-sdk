package ldclient

import (
	"testing"
)

func TestStartsWithOperator(t *testing.T) {
	var bar = "bar"
	if !operatorStartsWith("bar@foo", bar) {
		t.Errorf("Expected %s to start with bar", "foo@bar")
	}

	if !operatorStartsWith("bar", bar) {
		t.Errorf("Expected %s to start with bar", "bar")
	}

	if operatorStartsWith(4, bar) {
		t.Errorf("Did not expect %d to start with bar", 4)
	}

	if operatorStartsWith(true, bar) {
		t.Errorf("Did not expect %t to start with bar", true)
	}
}

func TestEndsWithOperator(t *testing.T) {
	var bar = "bar"
	if !operatorEndsWith("foo@bar", bar) {
		t.Errorf("Expected %s to end with bar", "foo@bar")
	}

	if !operatorEndsWith("bar", bar) {
		t.Errorf("Expected %s to end with bar", "bar")
	}

	if operatorEndsWith(4, bar) {
		t.Errorf("Did not expect %d to end with bar", 4)
	}

	if operatorEndsWith(true, bar) {
		t.Errorf("Did not expect %t to end with bar", true)
	}
}

func TestMatchesOperator(t *testing.T) {
	var pattern = "[A-Za-z]+"

	if !operatorMatches("Ozzz", pattern) {
		t.Errorf("Expected %S to match pattern %s", "Ozzz", pattern)
	}

	if operatorMatches("", pattern) {
		t.Errorf("Did not expect empty string to match pattern %s", pattern)
	}

}
