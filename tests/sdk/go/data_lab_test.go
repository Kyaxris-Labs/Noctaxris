package sdk_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/appconfig"
	"github.com/aws/aws-sdk-go-v2/service/configservice"
	"github.com/aws/aws-sdk-go-v2/service/configservice/types"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	gluetypes "github.com/aws/aws-sdk-go-v2/service/glue/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func newGlue(t *testing.T, cfg aws.Config) *glue.Client {
	t.Helper()
	return glue.NewFromConfig(cfg, func(o *glue.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func newAppConfig(t *testing.T, cfg aws.Config) *appconfig.Client {
	t.Helper()
	return appconfig.NewFromConfig(cfg, func(o *appconfig.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func newConfigService(t *testing.T, cfg aws.Config) *configservice.Client {
	t.Helper()
	return configservice.NewFromConfig(cfg, func(o *configservice.Options) {
		o.BaseEndpoint = aws.String(endpoint())
	})
}

func TestGlueCrawlerCreatesTableFromCSV(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	s3c := newS3(t, cfg)
	gluec := newGlue(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	bucket := strings.ToLower(prefix + "-glue")
	dbName := strings.ReplaceAll(prefix, "-", "_") + "_db"
	if len(dbName) > 48 {
		dbName = dbName[:48]
	}
	crawler := prefix + "-crawler"
	if len(crawler) > 64 {
		crawler = crawler[:64]
	}
	csvKey := "data/people.csv"

	if _, err := s3c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s3c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(csvKey)})
		_, _ = s3c.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})
	if _, err := s3c.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(csvKey), Body: bytes.NewReader([]byte("id,name\n1,alice\n")),
	}); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	if _, err := gluec.CreateDatabase(ctx, &glue.CreateDatabaseInput{
		DatabaseInput: &gluetypes.DatabaseInput{Name: aws.String(dbName)},
	}); err != nil {
		t.Fatalf("CreateDatabase: %v", err)
	}
	if _, err := gluec.CreateCrawler(ctx, &glue.CreateCrawlerInput{
		Name:         aws.String(crawler),
		DatabaseName: aws.String(dbName),
		Targets: &gluetypes.CrawlerTargets{
			S3Targets: []gluetypes.S3Target{{Path: aws.String("s3://" + bucket + "/data/")}},
		},
	}); err != nil {
		t.Fatalf("CreateCrawler: %v", err)
	}
	if _, err := gluec.StartCrawler(ctx, &glue.StartCrawlerInput{Name: aws.String(crawler)}); err != nil {
		t.Fatalf("StartCrawler: %v", err)
	}
	got, err := gluec.GetCrawler(ctx, &glue.GetCrawlerInput{Name: aws.String(crawler)})
	if err != nil {
		t.Fatalf("GetCrawler: %v", err)
	}
	if got.Crawler == nil || got.Crawler.State != gluetypes.CrawlerStateReady {
		t.Fatalf("Crawler state=%v want READY", got.Crawler)
	}
	table, err := gluec.GetTable(ctx, &glue.GetTableInput{
		DatabaseName: aws.String(dbName), Name: aws.String("people"),
	})
	if err != nil {
		t.Fatalf("GetTable: %v", err)
	}
	names := map[string]bool{}
	if table.Table != nil && table.Table.StorageDescriptor != nil {
		for _, c := range table.Table.StorageDescriptor.Columns {
			if c.Name != nil {
				names[*c.Name] = true
			}
		}
	}
	if !names["id"] || !names["name"] {
		t.Fatalf("columns missing id/name: %+v", names)
	}
}

func TestAppConfigStartDeploymentGetConfiguration(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	ac := newAppConfig(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)

	app, err := ac.CreateApplication(ctx, &appconfig.CreateApplicationInput{Name: aws.String(prefix + "-app")})
	if err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}
	env, err := ac.CreateEnvironment(ctx, &appconfig.CreateEnvironmentInput{
		ApplicationId: app.Id, Name: aws.String("dev"),
	})
	if err != nil {
		t.Fatalf("CreateEnvironment: %v", err)
	}
	profile, err := ac.CreateConfigurationProfile(ctx, &appconfig.CreateConfigurationProfileInput{
		ApplicationId: app.Id, Name: aws.String("flags"), LocationUri: aws.String("hosted"),
	})
	if err != nil {
		t.Fatalf("CreateConfigurationProfile: %v", err)
	}
	content := []byte(`{"feature":true}`)
	hosted, err := ac.CreateHostedConfigurationVersion(ctx, &appconfig.CreateHostedConfigurationVersionInput{
		ApplicationId: app.Id, ConfigurationProfileId: profile.Id,
		ContentType: aws.String("application/json"), Content: content,
	})
	if err != nil {
		t.Fatalf("CreateHostedConfigurationVersion: %v", err)
	}
	dep, err := ac.StartDeployment(ctx, &appconfig.StartDeploymentInput{
		ApplicationId:          app.Id,
		EnvironmentId:          env.Id,
		ConfigurationProfileId: profile.Id,
		ConfigurationVersion:   aws.String(strconv.FormatInt(int64(hosted.VersionNumber), 10)),
		DeploymentStrategyId:   aws.String("AppConfig.AllAtOnce"),
	})
	if err != nil {
		t.Fatalf("StartDeployment: %v", err)
	}
	if dep.DeploymentNumber == 0 && dep.State == "" {
		t.Fatalf("unexpected StartDeployment response: %+v", dep)
	}
	got, err := ac.GetConfiguration(ctx, &appconfig.GetConfigurationInput{
		Application:   app.Id,
		Environment:   env.Id,
		Configuration: profile.Id,
		ClientId:      aws.String("lab"),
	})
	if err != nil {
		t.Fatalf("GetConfiguration: %v", err)
	}
	if string(got.Content) != `{"feature":true}` {
		t.Fatalf("GetConfiguration content=%q", got.Content)
	}
}

func TestConfigHistorySnapshotGetObject(t *testing.T) {
	requireReady(t)
	cfg := loadAWSConfig(t)
	s3c := newS3(t, cfg)
	cfgc := newConfigService(t, cfg)
	ctx := context.Background()
	prefix := uniquePrefix(t)
	bucket := strings.ToLower(prefix + "-config")
	recorder := prefix + "-rec"
	if len(recorder) > 64 {
		recorder = recorder[:64]
	}

	if _, err := s3c.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	t.Cleanup(func() {
		listed, _ := s3c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String("AWSLogs/")})
		if listed != nil {
			for _, obj := range listed.Contents {
				_, _ = s3c.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: obj.Key})
			}
		}
		_, _ = s3c.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	})

	if _, err := cfgc.PutConfigurationRecorder(ctx, &configservice.PutConfigurationRecorderInput{
		ConfigurationRecorder: &types.ConfigurationRecorder{Name: aws.String(recorder)},
	}); err != nil {
		t.Fatalf("PutConfigurationRecorder: %v", err)
	}
	if _, err := cfgc.PutDeliveryChannel(ctx, &configservice.PutDeliveryChannelInput{
		DeliveryChannel: &types.DeliveryChannel{Name: aws.String(recorder), S3BucketName: aws.String(bucket)},
	}); err != nil {
		t.Fatalf("PutDeliveryChannel: %v", err)
	}
	if _, err := cfgc.StartConfigurationRecorder(ctx, &configservice.StartConfigurationRecorderInput{
		ConfigurationRecorderName: aws.String(recorder),
	}); err != nil {
		t.Fatalf("StartConfigurationRecorder: %v", err)
	}

	listed, err := s3c.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String("AWSLogs/")})
	if err != nil {
		t.Fatalf("ListObjectsV2: %v", err)
	}
	var snapKey *string
	needle := "noctaxris-config-snapshot-" + recorder + "-"
	for _, obj := range listed.Contents {
		if obj.Key != nil && strings.Contains(*obj.Key, needle) {
			snapKey = obj.Key
			break
		}
	}
	if snapKey == nil {
		t.Fatalf("missing snapshot under AWSLogs/: %+v", listed.Contents)
	}
	obj, err := s3c.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: snapKey})
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	raw, err := io.ReadAll(obj.Body)
	_ = obj.Body.Close()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("empty snapshot")
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("snapshot JSON: %v body=%s", err, raw)
	}
}
