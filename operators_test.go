package ldclient

import (
	"testing"
)

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
