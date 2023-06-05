package googlecloud

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/api/iterator"

	"flashcat.cloud/categraf/types"
)

// func (ins *Instance) DescribeMetric(w io.Writer, metricType string) error {
// 	req := &monitoringpb.GetMetricDescriptorRequest{
// 		Name: fmt.Sprintf("projects/%s/metricDescriptors/%s", ins.ProjectID, metricType),
// 	}
// 	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ins.Timeout))
// 	defer cancel()
//
// 	resp, err := ins.v3client.GetMetricDescriptor(ctx, req)
// 	if err != nil {
// 		return fmt.Errorf("could not get custom metric: %w", err)
// 	}
//
// 	fmt.Fprintf(w, "Name: %v\n", resp.GetName())
// 	fmt.Fprintf(w, "Description: %v\n", resp.GetDescription())
// 	fmt.Fprintf(w, "Type: %v\n", resp.GetType())
// 	fmt.Fprintf(w, "Metric Kind: %v\n", resp.GetMetricKind())
// 	fmt.Fprintf(w, "Value Type: %v\n", resp.GetValueType())
// 	fmt.Fprintf(w, "Unit: %v\n", resp.GetUnit())
// 	fmt.Fprintf(w, "Labels:\n")
// 	for _, l := range resp.GetLabels() {
// 		fmt.Fprintf(w, "\t%s (%s) - %s", l.GetKey(), l.GetValueType(), l.GetDescription())
// 	}
// 	return nil
// }
//
// func (ins *Instance) getMonitoredResource(w io.Writer, resource string) error {
// 	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ins.Timeout))
// 	defer cancel()
//
// 	req := &monitoringpb.GetMonitoredResourceDescriptorRequest{
// 		Name: fmt.Sprintf(resource),
// 	}
// 	resp, err := ins.v3client.GetMonitoredResourceDescriptor(ctx, req)
// 	if err != nil {
// 		return fmt.Errorf("could not get custom metric: %w", err)
// 	}
//
// 	fmt.Fprintf(w, "Name: %v\n", resp.GetName())
// 	fmt.Fprintf(w, "Description: %v\n", resp.GetDescription())
// 	fmt.Fprintf(w, "Type: %v\n", resp.GetType())
// 	fmt.Fprintf(w, "Labels:\n")
// 	for _, l := range resp.GetLabels() {
// 		fmt.Fprintf(w, "\t%s (%s) - %s", l.GetKey(), l.GetValueType(), l.GetDescription())
// 	}
// 	return nil
// }
//
// func (ins *Instance) readTimeSeriesFields(w io.Writer, filter string) error {
// 	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ins.Timeout))
// 	defer cancel()
//
// 	startTime := time.Now().UTC().Add(-time.Duration(ins.Period) - time.Duration(ins.Delay))
// 	endTime := time.Now().UTC().Add(-time.Duration(ins.Delay))
// 	req := &monitoringpb.ListTimeSeriesRequest{
// 		Name:   "projects/" + ins.ProjectID,
// 		Filter: filter,
// 		Interval: &monitoringpb.TimeInterval{
// 			StartTime: &timestamp.Timestamp{
// 				Seconds: startTime.Unix(),
// 			},
// 			EndTime: &timestamp.Timestamp{
// 				Seconds: endTime.Unix(),
// 			},
// 		},
// 		View: monitoringpb.ListTimeSeriesRequest_HEADERS,
// 	}
// 	fmt.Fprintln(w, "Found data points for the following instances:")
// 	it := ins.v3client.ListTimeSeries(ctx, req)
// 	for {
// 		resp, err := it.Next()
// 		if err == iterator.Done {
// 			break
// 		}
// 		if err != nil {
// 			return fmt.Errorf("could not read time series value: %w", err)
// 		}
// 		fmt.Fprintf(w, "\t%v\n", resp.GetMetric().GetLabels()["instance_name"])
// 	}
// 	fmt.Fprintln(w, "Done")
// 	return nil
// }
// func (ins *Instance) listMonitoredResources(w io.Writer) error {
// 	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ins.Timeout))
// 	defer cancel()
//
// 	req := &monitoringpb.ListMonitoredResourceDescriptorsRequest{
// 		Name: "projects/" + ins.ProjectID,
// 	}
// 	iter := ins.v3client.ListMonitoredResourceDescriptors(ctx, req)
//
// 	for {
// 		resp, err := iter.Next()
// 		if err == iterator.Done {
// 			break
// 		}
// 		if err != nil {
// 			return fmt.Errorf("Could not list time series: %w", err)
// 		}
// 		fmt.Fprintf(w, "%v\n", resp)
// 	}
// 	fmt.Fprintln(w, "Done")
// 	return nil
// }

func (ins *Instance) readTimeSeriesValue(slist *types.SampleList, filter string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ins.Timeout))
	defer cancel()

	startTime := time.Now().UTC().Add(-time.Duration(ins.Delay) - time.Duration(ins.Period)).Unix()
	endTime := time.Now().UTC().Add(time.Duration(ins.Delay)).Unix()

	req := &monitoringpb.ListTimeSeriesRequest{
		Name:   "projects/" + ins.ProjectID,
		Filter: filter,
		Interval: &monitoringpb.TimeInterval{
			StartTime: &timestamp.Timestamp{Seconds: startTime},
			EndTime:   &timestamp.Timestamp{Seconds: endTime},
		},
	}
	iter := ins.v3client.ListTimeSeries(ctx, req)

	for {
		resp, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("could not read time series value, %w ", err)
		}

		labels := make(map[string]string)
		for k, v := range resp.GetMetric().GetLabels() {
			labels[k] = v
		}
		for k, v := range resp.GetResource().GetLabels() {
			labels[k] = v
		}
		if !ins.UnmaskProjectID {
			delete(labels, "project_id")
		}
		labels["metric_type"] = resp.GetMetric().GetType()
		labels["resource_type"] = resp.GetResource().GetType()
		if labels["resource_type"] == "gce_instance" {
			labels[ins.GceHostTag] = labels["instance_name"]
		}
		metric := strings.ReplaceAll(labels["metric_type"], ".googleapis.com/", "")
		metric = strings.ReplaceAll(labels["metric_type"], "/", "_")
		samples := make([]*types.Sample, 0, len(resp.GetPoints()))
		for _, point := range resp.GetPoints() {
			samples = append(samples,
				types.NewSample("gcp", metric, point.GetValue().GetDoubleValue(), labels).
					SetTime(time.Unix(point.GetInterval().GetEndTime().GetSeconds(), 0)))
		}
		slist.PushFrontN(samples)
	}

	return nil
}

func (ins *Instance) ListMetrics() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ins.Timeout))
	defer cancel()

	req := &monitoringpb.ListMetricDescriptorsRequest{
		Name: "projects/" + ins.ProjectID,
	}
	iter := ins.v3client.ListMetricDescriptors(ctx, req)

	result := make([]string, 0, 100)
	for {
		resp, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("Could not list metrics: %w", err)
		}
		result = append(result, resp.GetType())
	}
	return result, nil
}
