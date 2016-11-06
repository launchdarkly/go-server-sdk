package ldclient

import (
	"testing"
)
// We should only be testing go-specific things here.
// Business logic test cases should be in the json test data when possible.

func TestLessThanOperator(t *testing.T) {
	if !operatorLessThanFn(int(1), float64(1.99999)) {
		t.Errorf("LessThan operator got unexpected result from input: 1 < 1.99")
	}
	if !operatorLessThanFn(int(1), uint(2)) {
		t.Errorf("LessThan operator got unexpected result from input: 1 < 2")
	}
}

func TestGreaterThanOperator(t *testing.T) {
	if !operatorGreaterThanFn(int(2), float64(1.99999)) {
		t.Errorf("GreaterThan operator got unexpected result from input: 2 > 1.99")
	}
	if !operatorGreaterThanFn(int(2), uint(1)) {
		t.Errorf("GreaterThan operator got unexpected result from input: 2 > 1")
	}
}

func TestParseNilTime(t *testing.T) {
	if ParseTime(nil) != nil {
		t.Errorf("Didn't get expected error when parsing nil date")
	}
}

func TestParseInvalidTimestamp(t *testing.T) {
	if ParseTime("May 10, 1987") != nil {
		t.Errorf("Didn't get expected error when parsing invalid timestamp")
	}

}