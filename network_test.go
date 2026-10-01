package main

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
)

func TestNetworkTagUpdatesSendOnlyTagsAndETag(t *testing.T) {
	freeform := map[string]string{"Owner": "Platform", "Environment": "Production"}
	defined := map[string]map[string]interface{}{"Operations": {"CostCenter": "42"}}
	id, etag := common.String("ocid1.example"), common.String("current-etag")
	tests := []struct {
		name    string
		request interface {
			HTTPRequest(string, string, *common.OCIReadSeekCloser, map[string]string) (http.Request, error)
		}
	}{
		{"VCN", core.UpdateVcnRequest{VcnId: id, IfMatch: etag, UpdateVcnDetails: core.UpdateVcnDetails{FreeformTags: freeform, DefinedTags: defined}}},
		{"subnet", core.UpdateSubnetRequest{SubnetId: id, IfMatch: etag, UpdateSubnetDetails: core.UpdateSubnetDetails{FreeformTags: freeform, DefinedTags: defined}}},
		{"security list", core.UpdateSecurityListRequest{SecurityListId: id, IfMatch: etag, UpdateSecurityListDetails: core.UpdateSecurityListDetails{FreeformTags: freeform, DefinedTags: defined}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := test.request.HTTPRequest(http.MethodPut, "/networkResources/{id}", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := request.Header.Get("if-match"); got != *etag {
				t.Fatalf("if-match = %q, want %q", got, *etag)
			}
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]interface{}
			if err := json.Unmarshal(body, &fields); err != nil {
				t.Fatal(err)
			}
			if got, want := len(fields), 2; got != want {
				t.Fatalf("update body has %d fields, want tags only: %s", got, body)
			}
			if !reflect.DeepEqual(fields["freeformTags"], map[string]interface{}{"Owner": "Platform", "Environment": "Production"}) {
				t.Fatalf("freeform tags = %v", fields["freeformTags"])
			}
			if !reflect.DeepEqual(fields["definedTags"], map[string]interface{}{"Operations": map[string]interface{}{"CostCenter": "42"}}) {
				t.Fatalf("defined tags = %v", fields["definedTags"])
			}
		})
	}
}

func TestNetworkDryRunDoesNotUpdate(t *testing.T) {
	var totals counters
	job := resourceJob{kind: resourceSecurityList, id: "ocid1.securitylist", name: "default"}
	existing := map[string]string{"Owner": "Platform"}
	requested := mergeFreeformTags(existing, map[string]string{"Environment": "Production"})
	if networkTagsNeedUpdate("eu-paris-1", job, existing, nil, requested, nil, false, &totals) {
		t.Fatal("dry run must not request an update")
	}
	if totals.wouldUpdate != 1 || totals.updated != 0 {
		t.Fatalf("dry-run counts = %+v", totals)
	}
}
