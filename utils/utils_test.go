package utils

import (
	"strings"
	"testing"
)

func TestSplit(t *testing.T) {
	testStrings := []string{"ExampleExampleID", "APIExampleID"}
	lowerJoinResults := []string{"example_example_id", "api_example_id"}

	for k, v := range testStrings {
		ret, err := SplitObjWords(v)
		if err != nil {
			t.Fatalf("return error: %v", err)
		}
		if lowerJoinResults[k] != strings.Join(ret, "_") {
			t.Fatalf("%v should convert to: %q but got: %q", v, lowerJoinResults[k], strings.Join(ret, "_"))
		}
	}
}
