// Command cloudwatch-metrics pushes ec2 instance memory information as custom
// CloudWatch metrics.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/artyom/meminfo"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/pkg/errors"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	hostname, err := os.Hostname()
	if err != nil {
		return errors.WithMessage(err, "hostname get")
	}
	sess, err := session.NewSession()
	if err != nil {
		return errors.WithMessage(err, "AWS session create")
	}
	meta, err := ec2metadata.New(sess).GetInstanceIdentityDocument()
	if err != nil {
		return errors.WithMessage(err, "ec2 instance metadata fetch")
	}
	svc := cloudwatch.New(sess, aws.NewConfig().WithRegion(meta.Region))
	mi, err := meminfo.New()
	if err != nil {
		return errors.WithMessage(err, "memory info fetch")
	}
	namespace := aws.String("Memory")
	dims := []*cloudwatch.Dimension{
		{Name: aws.String("InstanceID"), Value: &meta.InstanceID},
		{Name: aws.String("InstanceType"), Value: &meta.InstanceType},
		{Name: aws.String("Hostname"), Value: &hostname},
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for now := range ticker.C {
		if err := mi.Update(); err != nil {
			return errors.WithMessage(err, "memory info update")
		}
		input := cloudwatch.PutMetricDataInput{
			Namespace:  namespace,
			MetricData: metrics(mi, now, dims),
		}
		if err := putMetricData(svc, &input, 30*time.Second); err != nil {
			return errors.WithMessage(err, "CloudWatch metrics put")
		}
	}
	return nil
}

func putMetricData(svc *cloudwatch.CloudWatch, input *cloudwatch.PutMetricDataInput, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, err := svc.PutMetricDataWithContext(ctx, input)
	return err
}

func metrics(mi *meminfo.MemInfo, now time.Time, dims []*cloudwatch.Dimension) []*cloudwatch.MetricDatum {
	out := make([]*cloudwatch.MetricDatum, 0, 4)
	unit := aws.String(cloudwatch.StandardUnitBytes)
	for _, m := range []struct {
		name  string
		value int64
	}{
		{"Buffers", mi.Buffers()},
		{"Cached", mi.Cached()},
		{"Free", mi.Free()},
		{"FreeTotal", mi.FreeTotal()},
	} {
		out = append(out, &cloudwatch.MetricDatum{
			Dimensions: dims,
			MetricName: aws.String(m.name),
			Timestamp:  &now,
			Unit:       unit,
			Value:      aws.Float64(float64(m.value)),
		})
	}
	return out
}
