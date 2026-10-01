package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestResourceFlagsSelectOnlyRequestedTypes(t *testing.T) {
	tests := []struct {
		name          string
		compute       bool
		bootVolumes   bool
		blockVolumes  bool
		flagsProvided bool
		want          []string
	}{
		{name: "default selects all", want: []string{"compute instances", "boot volumes", "block volumes"}},
		{name: "compute only", compute: true, flagsProvided: true, want: []string{"compute instances"}},
		{name: "boot volumes only", bootVolumes: true, flagsProvided: true, want: []string{"boot volumes"}},
		{name: "block volumes only", blockVolumes: true, flagsProvided: true, want: []string{"block volumes"}},
		{name: "combined selection", compute: true, blockVolumes: true, flagsProvided: true, want: []string{"compute instances", "block volumes"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selection, err := selectResources(test.compute, test.bootVolumes, test.blockVolumes, test.flagsProvided)
			if err != nil {
				t.Fatalf("selectResources() error = %v", err)
			}
			if got := selection.names(); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("selected resource names = %v, want %v", got, test.want)
			}

			var listed []string
			listErrors := enqueueSelectedResources(
				selection,
				func() error { listed = append(listed, "compute instances"); return nil },
				func() error { listed = append(listed, "boot volumes"); return nil },
				func() error { listed = append(listed, "block volumes"); return nil },
			)
			if len(listErrors) != 0 {
				t.Fatalf("unexpected listing errors: %v", listErrors)
			}
			if !reflect.DeepEqual(listed, test.want) {
				t.Fatalf("listed resource types = %v, want %v", listed, test.want)
			}
		})
	}
}

func TestResourceFlagsRejectAllFalseWhenProvided(t *testing.T) {
	if _, err := selectResources(false, false, false, true); err == nil {
		t.Fatal("explicit resource flags set to false must not select all resource types")
	}
}

func TestSelectedResourceListingContinuesAfterError(t *testing.T) {
	var listed []string
	selection, err := selectResources(true, true, false, true)
	if err != nil {
		t.Fatal(err)
	}
	listErrors := enqueueSelectedResources(
		selection,
		func() error {
			listed = append(listed, "compute instances")
			return errors.New("compute listing failed")
		},
		func() error { listed = append(listed, "boot volumes"); return nil },
		func() error { t.Fatal("block volume listing should not run"); return nil },
	)
	if !reflect.DeepEqual(listed, []string{"compute instances", "boot volumes"}) {
		t.Fatalf("listed resource types = %v", listed)
	}
	if !reflect.DeepEqual(listErrors, []string{"compute listing failed"}) {
		t.Fatalf("listing errors = %v", listErrors)
	}
}

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

func TestParseFlattenedDefinedTags(t *testing.T) {
	got, err := parseFlattenedDefinedTags(
		`{"Operations.CostCenter":"42","Operations.Environment":"Production","Security.Classification":"Internal"}`,
	)
	if err != nil {
		t.Fatalf("parseFlattenedDefinedTags() error = %v", err)
	}

	want := map[string]map[string]interface{}{
		"Operations": {
			"CostCenter":  "42",
			"Environment": "Production",
		},
		"Security": {
			"Classification": "Internal",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseFlattenedDefinedTags() = %#v, want %#v", got, want)
	}
}

func TestParseFlattenedDefinedTagsRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "missing separator", input: `{"CostCenter":"42"}`},
		{name: "extra separator", input: `{"Operations.Billing.CostCenter":"42"}`},
		{name: "empty namespace", input: `{".CostCenter":"42"}`},
		{name: "empty key", input: `{"Operations.":"42"}`},
		{name: "non-string value", input: `{"Operations.CostCenter":42}`},
		{name: "invalid JSON", input: `{`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseFlattenedDefinedTags(test.input); err == nil {
				t.Fatalf("parseFlattenedDefinedTags(%q) returned no error", test.input)
			}
		})
	}
}

func TestParseFlattenedTagAssignments(t *testing.T) {
	got, err := parseFlattenedTagAssignments([]string{
		"Operations.CostCent=42",
		"Operations.Environment=Production",
		"Security.Classification=Internal=Reviewed",
	})
	if err != nil {
		t.Fatalf("parseFlattenedTagAssignments() error = %v", err)
	}

	want := map[string]map[string]interface{}{
		"Operations": {
			"CostCent":    "42",
			"Environment": "Production",
		},
		"Security": {
			"Classification": "Internal=Reviewed",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseFlattenedTagAssignments() = %#v, want %#v", got, want)
	}
}

func TestParseFlattenedTagAssignmentsRejectsMissingEquals(t *testing.T) {
	if _, err := parseFlattenedTagAssignments([]string{"Operations.CostCent"}); err == nil {
		t.Fatal("parseFlattenedTagAssignments() returned no error")
	}
}
