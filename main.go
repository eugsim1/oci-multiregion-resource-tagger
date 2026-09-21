package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
)

type settings struct {
	compartmentID   string
	apply           bool
	regionWorkers   int
	resourceWorkers int
	freeformTags    map[string]string
	definedTags     map[string]map[string]interface{}
}

type counters struct {
	found             int64
	foundInstances    int64
	foundBootVolumes  int64
	foundBlockVolumes int64
	wouldUpdate       int64
	updated           int64
	unchanged         int64
	skipped           int64
	failed            int64
	regionFailures    int64
}

type resourceKind string

const (
	resourceInstance    resourceKind = "instance"
	resourceBootVolume  resourceKind = "boot-volume"
	resourceBlockVolume resourceKind = "block-volume"
)

type resourceJob struct {
	kind  resourceKind
	id    string
	name  string
	state string
}

type regionResult struct {
	region string
	err    error
}

func main() {
	compartmentID := flag.String(
		"compartment-id",
		"",
		"OCID of the compartment containing the resources",
	)
	freeformJSON := flag.String(
		"freeform-tags",
		"{}",
		`Free-form tags as JSON, for example {"Environment":"Production"}`,
	)
	definedJSON := flag.String(
		"defined-tags",
		"{}",
		`Defined tags as JSON, for example {"Operations":{"CostCenter":"42"}}`,
	)
	apply := flag.Bool(
		"apply",
		false,
		"Actually update resources; without this option, only show planned changes",
	)
	regionWorkers := flag.Int(
		"region-workers",
		3,
		"Maximum number of OCI regions processed concurrently",
	)
	resourceWorkers := flag.Int(
		"resource-workers",
		5,
		"Maximum number of resources processed concurrently in each region",
	)
	instanceWorkers := flag.Int(
		"instance-workers",
		0,
		"Deprecated alias for -resource-workers",
	)

	flag.Parse()

	if *compartmentID == "" {
		log.Fatal("-compartment-id is required")
	}
	if *regionWorkers < 1 {
		log.Fatal("-region-workers must be at least 1")
	}
	if *instanceWorkers > 0 {
		*resourceWorkers = *instanceWorkers
	}
	if *resourceWorkers < 1 {
		log.Fatal("-resource-workers must be at least 1")
	}

	var freeformTags map[string]string
	if err := json.Unmarshal([]byte(*freeformJSON), &freeformTags); err != nil {
		log.Fatalf("invalid -freeform-tags JSON: %v", err)
	}

	var definedTags map[string]map[string]interface{}
	if err := json.Unmarshal([]byte(*definedJSON), &definedTags); err != nil {
		log.Fatalf("invalid -defined-tags JSON: %v", err)
	}

	if len(freeformTags) == 0 && len(definedTags) == 0 {
		log.Fatal("at least one free-form or defined tag must be specified")
	}

	cfg := settings{
		compartmentID:   *compartmentID,
		apply:           *apply,
		regionWorkers:   *regionWorkers,
		resourceWorkers: *resourceWorkers,
		freeformTags:    freeformTags,
		definedTags:     definedTags,
	}

	ctx := context.Background()
	provider := common.DefaultConfigProvider()

	// Enable exponential backoff and retries for transient errors, including 429s.
	retryPolicy := common.DefaultRetryPolicy()
	common.GlobalRetry = &retryPolicy

	regions, err := readySubscribedRegions(ctx, provider)
	if err != nil {
		log.Fatalf("discover subscribed regions: %v", err)
	}
	if len(regions) == 0 {
		log.Fatal("the tenancy has no READY region subscriptions")
	}

	log.Printf("READY regions (%d): %s", len(regions), strings.Join(regions, ", "))
	if !cfg.apply {
		log.Print("DRY-RUN mode: no resource will be modified")
	}

	var totals counters
	results := runRegions(ctx, provider, regions, cfg, &totals)

	for result := range results {
		if result.err != nil {
			atomic.AddInt64(&totals.regionFailures, 1)
			log.Printf("[%s] REGION ERROR: %v", result.region, result.err)
			continue
		}
		log.Printf("[%s] region completed", result.region)
	}

	log.Printf(
		"Finished: found=%d instances=%d boot-volumes=%d block-volumes=%d would-update=%d updated=%d unchanged=%d skipped=%d failed=%d region-failures=%d",
		atomic.LoadInt64(&totals.found),
		atomic.LoadInt64(&totals.foundInstances),
		atomic.LoadInt64(&totals.foundBootVolumes),
		atomic.LoadInt64(&totals.foundBlockVolumes),
		atomic.LoadInt64(&totals.wouldUpdate),
		atomic.LoadInt64(&totals.updated),
		atomic.LoadInt64(&totals.unchanged),
		atomic.LoadInt64(&totals.skipped),
		atomic.LoadInt64(&totals.failed),
		atomic.LoadInt64(&totals.regionFailures),
	)

	if atomic.LoadInt64(&totals.failed) > 0 ||
		atomic.LoadInt64(&totals.regionFailures) > 0 {
		os.Exit(1)
	}
}

func readySubscribedRegions(
	ctx context.Context,
	provider common.ConfigurationProvider,
) ([]string, error) {
	tenancyID, err := provider.TenancyOCID()
	if err != nil {
		return nil, fmt.Errorf("read tenancy OCID: %w", err)
	}

	client, err := identity.NewIdentityClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("create Identity client: %w", err)
	}

	response, err := client.ListRegionSubscriptions(
		ctx,
		identity.ListRegionSubscriptionsRequest{
			TenancyId: common.String(tenancyID),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("list region subscriptions: %w", err)
	}

	regions := make([]string, 0, len(response.Items))
	for _, subscription := range response.Items {
		if subscription.Status != identity.RegionSubscriptionStatusReady ||
			subscription.RegionName == nil || *subscription.RegionName == "" {
			continue
		}
		regions = append(regions, *subscription.RegionName)
	}

	sort.Strings(regions)
	return regions, nil
}

func runRegions(
	ctx context.Context,
	provider common.ConfigurationProvider,
	regions []string,
	cfg settings,
	totals *counters,
) <-chan regionResult {
	jobs := make(chan string)
	results := make(chan regionResult, len(regions))

	workerCount := cfg.regionWorkers
	if workerCount > len(regions) {
		workerCount = len(regions)
	}

	var workers sync.WaitGroup
	workers.Add(workerCount)

	for i := 0; i < workerCount; i++ {
		go func() {
			defer workers.Done()
			for region := range jobs {
				err := processRegion(ctx, provider, region, cfg, totals)
				results <- regionResult{region: region, err: err}
			}
		}()
	}

	go func() {
		for _, region := range regions {
			jobs <- region
		}
		close(jobs)
	}()

	go func() {
		workers.Wait()
		close(results)
	}()

	return results
}

func processRegion(
	ctx context.Context,
	provider common.ConfigurationProvider,
	region string,
	cfg settings,
	totals *counters,
) error {
	computeClient, err := core.NewComputeClientWithConfigurationProvider(provider)
	if err != nil {
		return fmt.Errorf("create Compute client: %w", err)
	}
	computeClient.SetRegion(region)

	blockClient, err := core.NewBlockstorageClientWithConfigurationProvider(provider)
	if err != nil {
		return fmt.Errorf("create Block Storage client: %w", err)
	}
	blockClient.SetRegion(region)

	jobs := make(chan resourceJob)
	var workers sync.WaitGroup
	workers.Add(cfg.resourceWorkers)

	for i := 0; i < cfg.resourceWorkers; i++ {
		go func() {
			defer workers.Done()
			for job := range jobs {
				processResource(
					ctx,
					computeClient,
					blockClient,
					region,
					job,
					cfg,
					totals,
				)
			}
		}()
	}

	var listErrors []string
	if err := enqueueInstances(ctx, computeClient, region, cfg, jobs, totals); err != nil {
		listErrors = append(listErrors, err.Error())
	}
	if err := enqueueBootVolumes(ctx, blockClient, region, cfg, jobs, totals); err != nil {
		listErrors = append(listErrors, err.Error())
	}
	if err := enqueueBlockVolumes(ctx, blockClient, region, cfg, jobs, totals); err != nil {
		listErrors = append(listErrors, err.Error())
	}

	close(jobs)
	workers.Wait()

	if len(listErrors) > 0 {
		return fmt.Errorf("%s", strings.Join(listErrors, "; "))
	}
	return nil
}

func enqueueInstances(
	ctx context.Context,
	client core.ComputeClient,
	region string,
	cfg settings,
	jobs chan<- resourceJob,
	totals *counters,
) error {
	var page *string
	for {
		response, listErr := client.ListInstances(ctx, core.ListInstancesRequest{
			CompartmentId: common.String(cfg.compartmentID),
			Limit:         common.Int(100),
			Page:          page,
		})
		if listErr != nil {
			return fmt.Errorf("list instances: %w", listErr)
		}

		for _, instance := range response.Items {
			if instance.Id == nil {
				log.Printf("[%s] ERROR instance returned without an OCID", region)
				atomic.AddInt64(&totals.failed, 1)
				continue
			}
			atomic.AddInt64(&totals.found, 1)
			atomic.AddInt64(&totals.foundInstances, 1)
			jobs <- resourceJob{
				kind:  resourceInstance,
				id:    *instance.Id,
				name:  stringValue(instance.DisplayName),
				state: string(instance.LifecycleState),
			}
		}

		if response.OpcNextPage == nil || *response.OpcNextPage == "" {
			break
		}
		page = response.OpcNextPage
	}
	return nil
}

func enqueueBootVolumes(
	ctx context.Context,
	client core.BlockstorageClient,
	region string,
	cfg settings,
	jobs chan<- resourceJob,
	totals *counters,
) error {
	var page *string
	for {
		response, err := client.ListBootVolumes(ctx, core.ListBootVolumesRequest{
			CompartmentId: common.String(cfg.compartmentID),
			Limit:         common.Int(100),
			Page:          page,
		})
		if err != nil {
			return fmt.Errorf("list boot volumes: %w", err)
		}

		for _, volume := range response.Items {
			if volume.Id == nil {
				log.Printf("[%s] ERROR boot volume returned without an OCID", region)
				atomic.AddInt64(&totals.failed, 1)
				continue
			}
			atomic.AddInt64(&totals.found, 1)
			atomic.AddInt64(&totals.foundBootVolumes, 1)
			jobs <- resourceJob{
				kind:  resourceBootVolume,
				id:    *volume.Id,
				name:  stringValue(volume.DisplayName),
				state: string(volume.LifecycleState),
			}
		}

		if response.OpcNextPage == nil || *response.OpcNextPage == "" {
			break
		}
		page = response.OpcNextPage
	}
	return nil
}

func enqueueBlockVolumes(
	ctx context.Context,
	client core.BlockstorageClient,
	region string,
	cfg settings,
	jobs chan<- resourceJob,
	totals *counters,
) error {
	var page *string
	for {
		response, err := client.ListVolumes(ctx, core.ListVolumesRequest{
			CompartmentId: common.String(cfg.compartmentID),
			Limit:         common.Int(100),
			Page:          page,
		})
		if err != nil {
			return fmt.Errorf("list block volumes: %w", err)
		}

		for _, volume := range response.Items {
			if volume.Id == nil {
				log.Printf("[%s] ERROR block volume returned without an OCID", region)
				atomic.AddInt64(&totals.failed, 1)
				continue
			}
			atomic.AddInt64(&totals.found, 1)
			atomic.AddInt64(&totals.foundBlockVolumes, 1)
			jobs <- resourceJob{
				kind:  resourceBlockVolume,
				id:    *volume.Id,
				name:  stringValue(volume.DisplayName),
				state: string(volume.LifecycleState),
			}
		}

		if response.OpcNextPage == nil || *response.OpcNextPage == "" {
			break
		}
		page = response.OpcNextPage
	}
	return nil
}

func processResource(
	ctx context.Context,
	computeClient core.ComputeClient,
	blockClient core.BlockstorageClient,
	region string,
	job resourceJob,
	cfg settings,
	totals *counters,
) {
	if job.state == "TERMINATED" || job.state == "TERMINATING" {
		log.Printf(
			"[%s] SKIP %s %s: lifecycle state is %s",
			region,
			job.kind,
			job.name,
			job.state,
		)
		atomic.AddInt64(&totals.skipped, 1)
		return
	}

	switch job.kind {
	case resourceInstance:
		processInstance(ctx, computeClient, region, job, cfg, totals)
	case resourceBootVolume:
		processBootVolume(ctx, blockClient, region, job, cfg, totals)
	case resourceBlockVolume:
		processBlockVolume(ctx, blockClient, region, job, cfg, totals)
	default:
		log.Printf("[%s] ERROR unsupported resource type %q", region, job.kind)
		atomic.AddInt64(&totals.failed, 1)
	}
}

func processInstance(
	ctx context.Context,
	client core.ComputeClient,
	region string,
	job resourceJob,
	cfg settings,
	totals *counters,
) {

	response, err := client.GetInstance(ctx, core.GetInstanceRequest{
		InstanceId: common.String(job.id),
	})
	if err != nil {
		log.Printf(
			"[%s] ERROR getting %s %s: %v",
			region,
			job.kind,
			job.name,
			err,
		)
		atomic.AddInt64(&totals.failed, 1)
		return
	}

	instance := response.Instance
	freeformTags := mergeFreeformTags(instance.FreeformTags, cfg.freeformTags)
	definedTags := mergeDefinedTags(instance.DefinedTags, cfg.definedTags)

	if reflect.DeepEqual(freeformTags, instance.FreeformTags) &&
		reflect.DeepEqual(definedTags, instance.DefinedTags) {
		log.Printf("[%s] UNCHANGED %s %s", region, job.kind, stringValue(instance.DisplayName))
		atomic.AddInt64(&totals.unchanged, 1)
		return
	}

	if !cfg.apply {
		log.Printf(
			"[%s] DRY-RUN would update %s %s (%s)",
			region,
			job.kind,
			stringValue(instance.DisplayName),
			stringValue(instance.Id),
		)
		atomic.AddInt64(&totals.wouldUpdate, 1)
		return
	}

	_, err = client.UpdateInstance(ctx, core.UpdateInstanceRequest{
		InstanceId: instance.Id,
		IfMatch:    response.Etag,
		UpdateInstanceDetails: core.UpdateInstanceDetails{
			FreeformTags: freeformTags,
			DefinedTags:  definedTags,
		},
	})
	if err != nil {
		log.Printf(
			"[%s] ERROR updating %s %s: %v",
			region,
			job.kind,
			stringValue(instance.DisplayName),
			err,
		)
		atomic.AddInt64(&totals.failed, 1)
		return
	}

	log.Printf("[%s] UPDATED %s %s", region, job.kind, stringValue(instance.DisplayName))
	atomic.AddInt64(&totals.updated, 1)
}

func processBootVolume(
	ctx context.Context,
	client core.BlockstorageClient,
	region string,
	job resourceJob,
	cfg settings,
	totals *counters,
) {
	response, err := client.GetBootVolume(ctx, core.GetBootVolumeRequest{
		BootVolumeId: common.String(job.id),
	})
	if err != nil {
		log.Printf("[%s] ERROR getting %s %s: %v", region, job.kind, job.name, err)
		atomic.AddInt64(&totals.failed, 1)
		return
	}

	volume := response.BootVolume
	freeformTags := mergeFreeformTags(volume.FreeformTags, cfg.freeformTags)
	definedTags := mergeDefinedTags(volume.DefinedTags, cfg.definedTags)

	if reflect.DeepEqual(freeformTags, volume.FreeformTags) &&
		reflect.DeepEqual(definedTags, volume.DefinedTags) {
		log.Printf("[%s] UNCHANGED %s %s", region, job.kind, stringValue(volume.DisplayName))
		atomic.AddInt64(&totals.unchanged, 1)
		return
	}

	if !cfg.apply {
		log.Printf(
			"[%s] DRY-RUN would update %s %s (%s)",
			region,
			job.kind,
			stringValue(volume.DisplayName),
			stringValue(volume.Id),
		)
		atomic.AddInt64(&totals.wouldUpdate, 1)
		return
	}

	_, err = client.UpdateBootVolume(ctx, core.UpdateBootVolumeRequest{
		BootVolumeId: volume.Id,
		IfMatch:      response.Etag,
		UpdateBootVolumeDetails: core.UpdateBootVolumeDetails{
			FreeformTags: freeformTags,
			DefinedTags:  definedTags,
		},
	})
	if err != nil {
		log.Printf(
			"[%s] ERROR updating %s %s: %v",
			region,
			job.kind,
			stringValue(volume.DisplayName),
			err,
		)
		atomic.AddInt64(&totals.failed, 1)
		return
	}

	log.Printf("[%s] UPDATED %s %s", region, job.kind, stringValue(volume.DisplayName))
	atomic.AddInt64(&totals.updated, 1)
}

func processBlockVolume(
	ctx context.Context,
	client core.BlockstorageClient,
	region string,
	job resourceJob,
	cfg settings,
	totals *counters,
) {
	response, err := client.GetVolume(ctx, core.GetVolumeRequest{
		VolumeId: common.String(job.id),
	})
	if err != nil {
		log.Printf("[%s] ERROR getting %s %s: %v", region, job.kind, job.name, err)
		atomic.AddInt64(&totals.failed, 1)
		return
	}

	volume := response.Volume
	freeformTags := mergeFreeformTags(volume.FreeformTags, cfg.freeformTags)
	definedTags := mergeDefinedTags(volume.DefinedTags, cfg.definedTags)

	if reflect.DeepEqual(freeformTags, volume.FreeformTags) &&
		reflect.DeepEqual(definedTags, volume.DefinedTags) {
		log.Printf("[%s] UNCHANGED %s %s", region, job.kind, stringValue(volume.DisplayName))
		atomic.AddInt64(&totals.unchanged, 1)
		return
	}

	if !cfg.apply {
		log.Printf(
			"[%s] DRY-RUN would update %s %s (%s)",
			region,
			job.kind,
			stringValue(volume.DisplayName),
			stringValue(volume.Id),
		)
		atomic.AddInt64(&totals.wouldUpdate, 1)
		return
	}

	_, err = client.UpdateVolume(ctx, core.UpdateVolumeRequest{
		VolumeId: volume.Id,
		IfMatch:  response.Etag,
		UpdateVolumeDetails: core.UpdateVolumeDetails{
			FreeformTags: freeformTags,
			DefinedTags:  definedTags,
		},
	})
	if err != nil {
		log.Printf(
			"[%s] ERROR updating %s %s: %v",
			region,
			job.kind,
			stringValue(volume.DisplayName),
			err,
		)
		atomic.AddInt64(&totals.failed, 1)
		return
	}

	log.Printf("[%s] UPDATED %s %s", region, job.kind, stringValue(volume.DisplayName))
	atomic.AddInt64(&totals.updated, 1)
}

func mergeFreeformTags(
	existing map[string]string,
	requested map[string]string,
) map[string]string {
	result := make(map[string]string, len(existing)+len(requested))

	for key, value := range existing {
		result[key] = value
	}
	for key, value := range requested {
		result[key] = value
	}

	return result
}

func mergeDefinedTags(
	existing map[string]map[string]interface{},
	requested map[string]map[string]interface{},
) map[string]map[string]interface{} {
	result := make(map[string]map[string]interface{})

	for namespace, tags := range existing {
		result[namespace] = make(map[string]interface{}, len(tags))
		for key, value := range tags {
			result[namespace][key] = value
		}
	}

	for namespace, tags := range requested {
		if _, exists := result[namespace]; !exists {
			result[namespace] = make(map[string]interface{})
		}
		for key, value := range tags {
			result[namespace][key] = value
		}
	}

	return result
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
