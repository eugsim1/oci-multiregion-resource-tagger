package main

import (
	"reflect"
	"testing"
)

func TestMergeFreeformTagsPreservesAndOverrides(t *testing.T) {
	existing := map[string]string{
		"Owner":       "Platform",
		"Environment": "Development",
	}
	requested := map[string]string{
		"Environment": "Production",
		"ManagedBy":   "oci-multiregion-resource-tagger",
	}

	got := mergeFreeformTags(existing, requested)
	want := map[string]string{
		"Owner":       "Platform",
		"Environment": "Production",
		"ManagedBy":   "oci-multiregion-resource-tagger",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeFreeformTags() = %#v, want %#v", got, want)
	}
	if existing["Environment"] != "Development" {
		t.Fatal("mergeFreeformTags mutated the existing map")
	}
}

func TestMergeDefinedTagsPreservesNamespacesAndOverrides(t *testing.T) {
	existing := map[string]map[string]interface{}{
		"Operations": {
			"CostCenter":  "100",
			"Environment": "Development",
		},
		"Security": {
			"Classification": "Internal",
		},
	}
	requested := map[string]map[string]interface{}{
		"Operations": {
			"Environment": "Production",
		},
		"Automation": {
			"ManagedBy": "GoTagger",
		},
	}

	got := mergeDefinedTags(existing, requested)
	want := map[string]map[string]interface{}{
		"Operations": {
			"CostCenter":  "100",
			"Environment": "Production",
		},
		"Security": {
			"Classification": "Internal",
		},
		"Automation": {
			"ManagedBy": "GoTagger",
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeDefinedTags() = %#v, want %#v", got, want)
	}
	if existing["Operations"]["Environment"] != "Development" {
		t.Fatal("mergeDefinedTags mutated the existing map")
	}
}

func TestStringValue(t *testing.T) {
	value := "example"
	if got := stringValue(&value); got != value {
		t.Fatalf("stringValue() = %q, want %q", got, value)
	}
	if got := stringValue(nil); got != "" {
		t.Fatalf("stringValue(nil) = %q, want empty string", got)
	}
}
