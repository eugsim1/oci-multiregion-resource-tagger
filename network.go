package main

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"sync/atomic"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
)

func enqueueVcns(ctx context.Context, client core.VirtualNetworkClient, region string, cfg settings, jobs chan<- resourceJob, totals *counters) error {
	var page *string
	for {
		response, err := client.ListVcns(ctx, core.ListVcnsRequest{
			CompartmentId: common.String(cfg.compartmentID), Limit: common.Int(100), Page: page,
		})
		if err != nil {
			return fmt.Errorf("list VCNs: %w", err)
		}
		for _, vcn := range response.Items {
			if vcn.Id == nil {
				log.Printf("[%s] ERROR VCN returned without an OCID", region)
				atomic.AddInt64(&totals.failed, 1)
				continue
			}
			atomic.AddInt64(&totals.found, 1)
			atomic.AddInt64(&totals.foundVcns, 1)
			jobs <- resourceJob{kind: resourceVcn, id: *vcn.Id, name: stringValue(vcn.DisplayName), state: string(vcn.LifecycleState)}
		}
		if response.OpcNextPage == nil || *response.OpcNextPage == "" {
			return nil
		}
		page = response.OpcNextPage
	}
}

func enqueueSubnets(ctx context.Context, client core.VirtualNetworkClient, region string, cfg settings, jobs chan<- resourceJob, totals *counters) error {
	var page *string
	for {
		response, err := client.ListSubnets(ctx, core.ListSubnetsRequest{
			CompartmentId: common.String(cfg.compartmentID), Limit: common.Int(100), Page: page,
		})
		if err != nil {
			return fmt.Errorf("list subnets: %w", err)
		}
		for _, subnet := range response.Items {
			if subnet.Id == nil {
				log.Printf("[%s] ERROR subnet returned without an OCID", region)
				atomic.AddInt64(&totals.failed, 1)
				continue
			}
			atomic.AddInt64(&totals.found, 1)
			atomic.AddInt64(&totals.foundSubnets, 1)
			jobs <- resourceJob{kind: resourceSubnet, id: *subnet.Id, name: stringValue(subnet.DisplayName), state: string(subnet.LifecycleState)}
		}
		if response.OpcNextPage == nil || *response.OpcNextPage == "" {
			return nil
		}
		page = response.OpcNextPage
	}
}

func enqueueSecurityLists(ctx context.Context, client core.VirtualNetworkClient, region string, cfg settings, jobs chan<- resourceJob, totals *counters) error {
	var page *string
	for {
		response, err := client.ListSecurityLists(ctx, core.ListSecurityListsRequest{
			CompartmentId: common.String(cfg.compartmentID), Limit: common.Int(100), Page: page,
		})
		if err != nil {
			return fmt.Errorf("list security lists: %w", err)
		}
		for _, list := range response.Items {
			if list.Id == nil {
				log.Printf("[%s] ERROR security list returned without an OCID", region)
				atomic.AddInt64(&totals.failed, 1)
				continue
			}
			atomic.AddInt64(&totals.found, 1)
			atomic.AddInt64(&totals.foundSecurityLists, 1)
			jobs <- resourceJob{kind: resourceSecurityList, id: *list.Id, name: stringValue(list.DisplayName), state: string(list.LifecycleState)}
		}
		if response.OpcNextPage == nil || *response.OpcNextPage == "" {
			return nil
		}
		page = response.OpcNextPage
	}
}

func processVcn(ctx context.Context, client core.VirtualNetworkClient, region string, job resourceJob, cfg settings, totals *counters) {
	response, err := client.GetVcn(ctx, core.GetVcnRequest{VcnId: common.String(job.id)})
	if err != nil {
		logNetworkError(region, "getting", job, err, totals)
		return
	}
	vcn := response.Vcn
	freeformTags := mergeFreeformTags(vcn.FreeformTags, cfg.freeformTags)
	definedTags := mergeDefinedTags(vcn.DefinedTags, cfg.definedTags)
	if !networkTagsNeedUpdate(region, job, vcn.FreeformTags, vcn.DefinedTags, freeformTags, definedTags, cfg.apply, totals) {
		return
	}
	_, err = client.UpdateVcn(ctx, core.UpdateVcnRequest{
		VcnId: vcn.Id, IfMatch: response.Etag,
		UpdateVcnDetails: core.UpdateVcnDetails{FreeformTags: freeformTags, DefinedTags: definedTags},
	})
	if err != nil {
		logNetworkError(region, "updating", job, err, totals)
		return
	}
	logNetworkUpdated(region, job, totals)
}

func processSubnet(ctx context.Context, client core.VirtualNetworkClient, region string, job resourceJob, cfg settings, totals *counters) {
	response, err := client.GetSubnet(ctx, core.GetSubnetRequest{SubnetId: common.String(job.id)})
	if err != nil {
		logNetworkError(region, "getting", job, err, totals)
		return
	}
	subnet := response.Subnet
	freeformTags := mergeFreeformTags(subnet.FreeformTags, cfg.freeformTags)
	definedTags := mergeDefinedTags(subnet.DefinedTags, cfg.definedTags)
	if !networkTagsNeedUpdate(region, job, subnet.FreeformTags, subnet.DefinedTags, freeformTags, definedTags, cfg.apply, totals) {
		return
	}
	_, err = client.UpdateSubnet(ctx, core.UpdateSubnetRequest{
		SubnetId: subnet.Id, IfMatch: response.Etag,
		UpdateSubnetDetails: core.UpdateSubnetDetails{FreeformTags: freeformTags, DefinedTags: definedTags},
	})
	if err != nil {
		logNetworkError(region, "updating", job, err, totals)
		return
	}
	logNetworkUpdated(region, job, totals)
}

func processSecurityList(ctx context.Context, client core.VirtualNetworkClient, region string, job resourceJob, cfg settings, totals *counters) {
	response, err := client.GetSecurityList(ctx, core.GetSecurityListRequest{SecurityListId: common.String(job.id)})
	if err != nil {
		logNetworkError(region, "getting", job, err, totals)
		return
	}
	list := response.SecurityList
	freeformTags := mergeFreeformTags(list.FreeformTags, cfg.freeformTags)
	definedTags := mergeDefinedTags(list.DefinedTags, cfg.definedTags)
	if !networkTagsNeedUpdate(region, job, list.FreeformTags, list.DefinedTags, freeformTags, definedTags, cfg.apply, totals) {
		return
	}
	// Leave ingress and egress rules nil. The SDK omits these optional fields from the request body.
	_, err = client.UpdateSecurityList(ctx, core.UpdateSecurityListRequest{
		SecurityListId: list.Id, IfMatch: response.Etag,
		UpdateSecurityListDetails: core.UpdateSecurityListDetails{FreeformTags: freeformTags, DefinedTags: definedTags},
	})
	if err != nil {
		logNetworkError(region, "updating", job, err, totals)
		return
	}
	logNetworkUpdated(region, job, totals)
}

func networkTagsNeedUpdate(region string, job resourceJob, existingFreeform map[string]string, existingDefined map[string]map[string]interface{}, freeform map[string]string, defined map[string]map[string]interface{}, apply bool, totals *counters) bool {
	if reflect.DeepEqual(freeform, existingFreeform) && reflect.DeepEqual(defined, existingDefined) {
		log.Printf("[%s] UNCHANGED %s %s", region, job.kind, job.name)
		atomic.AddInt64(&totals.unchanged, 1)
		return false
	}
	if !apply {
		log.Printf("[%s] DRY-RUN would update %s %s (%s)", region, job.kind, job.name, job.id)
		atomic.AddInt64(&totals.wouldUpdate, 1)
		return false
	}
	return true
}

func logNetworkError(region, action string, job resourceJob, err error, totals *counters) {
	log.Printf("[%s] ERROR %s %s %s: %v", region, action, job.kind, job.name, err)
	atomic.AddInt64(&totals.failed, 1)
}

func logNetworkUpdated(region string, job resourceJob, totals *counters) {
	log.Printf("[%s] UPDATED %s %s", region, job.kind, job.name)
	atomic.AddInt64(&totals.updated, 1)
}
