package store_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/authz"
	"github.com/Kyaxris-Labs/Noctaxris/internal/kernel/identity"
	"github.com/Kyaxris-Labs/Noctaxris/internal/store"
)

func TestBackupListDeleteVaultAndPlan(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	if err := store.EnsureBackupSchema(nil); err == nil {
		t.Fatal("want nil db error")
	}
	if err := st.EnsureBackupSchema(); err != nil {
		t.Fatal(err)
	}

	empty, err := st.ListBackupVaults(account, "")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty vaults=%v err=%v", empty, err)
	}
	emptyPlans, err := st.ListBackupPlans(account, "")
	if err != nil || len(emptyPlans) != 0 {
		t.Fatalf("empty plans=%v err=%v", emptyPlans, err)
	}

	vault, err := st.CreateBackupVault(account, "", "gap-vault", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(vault.BackupVaultARN, "backup-vault:gap-vault") {
		t.Fatalf("arn=%q", vault.BackupVaultARN)
	}
	listed, err := st.ListBackupVaults(account, "us-east-1")
	if err != nil || len(listed) != 1 || listed[0].BackupVaultName != "gap-vault" {
		t.Fatalf("list vaults=%+v err=%v", listed, err)
	}

	plan, err := st.CreateBackupPlan(account, "", "gap-plan", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetBackupPlan(account, "", plan.BackupPlanID)
	if err != nil || got.BackupPlanName != "gap-plan" {
		t.Fatalf("get plan=%+v err=%v", got, err)
	}
	plans, err := st.ListBackupPlans(account, "us-east-1")
	if err != nil || len(plans) != 1 {
		t.Fatalf("list plans=%+v err=%v", plans, err)
	}

	if _, err := st.CreateBackupPlan(account, "us-east-1", "  ", nil); !errors.Is(err, store.ErrBackupBadRequest) {
		t.Fatalf("empty plan name err=%v", err)
	}
	if _, err := st.GetBackupPlan(account, "us-east-1", "missing-plan"); !errors.Is(err, store.ErrBackupNotFound) {
		t.Fatalf("missing plan err=%v", err)
	}
	if err := st.DeleteBackupPlan(account, "us-east-1", "missing-plan"); !errors.Is(err, store.ErrBackupNotFound) {
		t.Fatalf("delete missing plan err=%v", err)
	}
	if err := st.DeleteBackupVault(account, "us-east-1", "missing-vault"); !errors.Is(err, store.ErrBackupNotFound) {
		t.Fatalf("delete missing vault err=%v", err)
	}

	_, rp, err := st.StartBackupJob(account, "us-east-1", "gap-vault", "arn:aws:s3:::gap-bucket", "arn:aws:iam::000000000001:role/Backup")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteBackupVault(account, "us-east-1", "gap-vault"); !errors.Is(err, store.ErrBackupConflict) {
		t.Fatalf("non-empty vault delete err=%v", err)
	}
	if err := st.DeleteRecoveryPoint(account, "us-east-1", "gap-vault", rp.RecoveryPointARN); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteBackupPlan(account, "", plan.BackupPlanID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetBackupPlan(account, "us-east-1", plan.BackupPlanID); !errors.Is(err, store.ErrBackupNotFound) {
		t.Fatalf("after delete plan err=%v", err)
	}
	if err := st.DeleteBackupVault(account, "", "gap-vault"); err != nil {
		t.Fatal(err)
	}
	after, err := st.ListBackupVaults(account, "us-east-1")
	if err != nil || len(after) != 0 {
		t.Fatalf("after delete vaults=%+v err=%v", after, err)
	}
}

func TestBeanstalkSolutionStacksAndNegatives(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	if err := store.EnsureBeanstalkSchema(nil); err == nil {
		t.Fatal("want nil db error")
	}
	if err := st.EnsureBeanstalkSchema(); err != nil {
		t.Fatal(err)
	}
	stacks := store.BeanstalkSolutionStacks()
	if len(stacks) < 2 || stacks[0] != store.DefaultBeanstalkSolutionStack {
		t.Fatalf("stacks=%v", stacks)
	}
	if arn := store.BeanstalkApplicationARN("", account, "app"); !strings.Contains(arn, "us-east-1") {
		t.Fatalf("app arn=%q", arn)
	}
	if arn := store.BeanstalkEnvironmentARN("", account, "env"); !strings.Contains(arn, "environment/env") {
		t.Fatalf("env arn=%q", arn)
	}

	if _, err := st.CreateBeanstalkApplication(account, "", "  ", ""); !errors.Is(err, store.ErrBeanstalkBadRequest) {
		t.Fatalf("empty app name err=%v", err)
	}
	app, err := st.CreateBeanstalkApplication(account, "", "gap-app", "desc")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBeanstalkApplication(account, "us-east-1", "gap-app", ""); !errors.Is(err, store.ErrBeanstalkExists) {
		t.Fatalf("dup app err=%v", err)
	}
	listed, err := st.DescribeBeanstalkApplications(account, "", []string{"gap-app", "missing"})
	if err != nil || len(listed) != 1 || listed[0].ApplicationName != app.ApplicationName {
		t.Fatalf("describe apps=%+v err=%v", listed, err)
	}

	if _, err := st.CreateBeanstalkApplicationVersion(account, "", "gap-app", "", "", "", ""); !errors.Is(err, store.ErrBeanstalkBadRequest) {
		t.Fatalf("empty version err=%v", err)
	}
	if _, err := st.CreateBeanstalkApplicationVersion(account, "", "missing-app", "v1", "", "", ""); !errors.Is(err, store.ErrBeanstalkNotFound) {
		t.Fatalf("missing app version err=%v", err)
	}
	if _, err := st.CreateBeanstalkApplicationVersion(account, "", "gap-app", "v1", "d", "b", "k"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBeanstalkApplicationVersion(account, "", "gap-app", "v1", "", "", ""); !errors.Is(err, store.ErrBeanstalkExists) {
		t.Fatalf("dup version err=%v", err)
	}

	if _, err := st.CreateBeanstalkEnvironment(account, "", "gap-app", "", "v1", "", "", ""); !errors.Is(err, store.ErrBeanstalkBadRequest) {
		t.Fatalf("empty env name err=%v", err)
	}
	env, err := st.CreateBeanstalkEnvironment(account, "", "gap-app", "gap-env", "v1", "", "lab", "gap")
	if err != nil {
		t.Fatal(err)
	}
	envs, err := st.DescribeBeanstalkEnvironments(account, "", "gap-app", []string{env.EnvironmentName}, false)
	if err != nil || len(envs) != 1 || envs[0].Status != "Ready" {
		t.Fatalf("envs=%+v err=%v", envs, err)
	}
	if err := st.DeleteBeanstalkApplication(account, "", "gap-app", false); !errors.Is(err, store.ErrBeanstalkBadRequest) {
		t.Fatalf("delete with active env err=%v", err)
	}
	if err := st.DeleteBeanstalkApplication(account, "", "missing-app", true); !errors.Is(err, store.ErrBeanstalkNotFound) {
		t.Fatalf("delete missing err=%v", err)
	}
	if err := st.DeleteBeanstalkApplication(account, "", "gap-app", true); err != nil {
		t.Fatal(err)
	}
}

func TestGuardDutyListEnsureAndNotFound(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	if err := store.EnsureGuardDutySchema(nil); err == nil {
		t.Fatal("want nil db error")
	}
	ids, err := st.ListGuardDutyDetectors(account)
	if err != nil || len(ids) != 0 {
		t.Fatalf("empty list=%v err=%v", ids, err)
	}
	id1, err := st.EnsureGuardDutyDetector(account)
	if err != nil || id1 == "" {
		t.Fatalf("ensure create id=%q err=%v", id1, err)
	}
	id2, err := st.EnsureGuardDutyDetector(account)
	if err != nil || id2 != id1 {
		t.Fatalf("ensure reuse got=%q want=%q err=%v", id2, id1, err)
	}
	listed, err := st.ListGuardDutyDetectors(account)
	if err != nil || len(listed) != 1 || listed[0] != id1 {
		t.Fatalf("list=%v err=%v", listed, err)
	}
	if _, err := st.GetGuardDutyDetector(account, "missing-detector"); !errors.Is(err, store.ErrGuardDutyNotFound) {
		t.Fatalf("missing detector err=%v", err)
	}
	if _, err := st.InjectGuardDutyFindings(account, id1, "", nil); !errors.Is(err, store.ErrGuardDutyBadRequest) {
		t.Fatalf("empty findings err=%v", err)
	}
	if _, err := st.InjectGuardDutyFindings(account, id1, "us-east-1", []store.GuardDutyFinding{{}}); !errors.Is(err, store.ErrGuardDutyBadRequest) {
		t.Fatalf("missing type err=%v", err)
	}
}

func TestCodePipelineDeleteAndState(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	if err := store.EnsureCodePipelineSchema(nil); err == nil {
		t.Fatal("want nil db error")
	}
	if err := st.EnsureCodePipelineSchema(); err != nil {
		t.Fatal(err)
	}
	if arn := store.CodePipelineARN("", account, "pipe"); !strings.Contains(arn, "us-east-1") {
		t.Fatalf("arn=%q", arn)
	}

	if _, err := st.CreateCodePipeline(account, "us-east-1", store.CodePipelineDeclaration{Name: ""}); !errors.Is(err, store.ErrCodePipelineBadReq) {
		t.Fatalf("empty name err=%v", err)
	}
	if _, err := st.CreateCodePipeline(account, "us-east-1", store.CodePipelineDeclaration{
		Name: "pipe-gap", Stages: []store.CodePipelineStageDecl{{Name: "Build", Actions: []store.CodePipelineActionDecl{{
			Name: "Build",
			ActionTypeID: store.CodePipelineActionTypeID{
				Category: "Build", Owner: "AWS", Provider: "CodeBuild", Version: "1",
			},
			Configuration: map[string]string{},
		}}}},
	}); !errors.Is(err, store.ErrCodePipelineBadReq) {
		t.Fatalf("missing project err=%v", err)
	}

	p, err := st.CreateCodePipeline(account, "us-east-1", store.CodePipelineDeclaration{
		Name: "pipe-gap",
		Stages: []store.CodePipelineStageDecl{{
			Name: "Build",
			Actions: []store.CodePipelineActionDecl{{
				Name: "Build",
				ActionTypeID: store.CodePipelineActionTypeID{
					Category: "Build", Owner: "AWS", Provider: "CodeBuild", Version: "1",
				},
				Configuration: map[string]string{"ProjectName": "lab-proj"},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	idle, err := st.GetCodePipelineState(account, p.Name)
	if err != nil || idle.Status != "Idle" {
		t.Fatalf("idle state=%+v err=%v", idle, err)
	}
	exec, err := st.StartCodePipelineExecution(account, p.Name, nil)
	if err != nil || exec.Status != "Succeeded" {
		t.Fatalf("exec=%+v err=%v", exec, err)
	}
	state, err := st.GetCodePipelineState(account, p.Name)
	if err != nil || state.ExecutionID != exec.ExecutionID || state.Status != "Succeeded" {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	if _, err := st.GetCodePipelineState(account, "missing-pipe"); !errors.Is(err, store.ErrCodePipelineNotFound) {
		t.Fatalf("missing state err=%v", err)
	}
	if err := st.DeleteCodePipeline(account, "missing-pipe"); !errors.Is(err, store.ErrCodePipelineNotFound) {
		t.Fatalf("delete missing err=%v", err)
	}
	if err := st.DeleteCodePipeline(account, p.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetCodePipeline(account, p.Name); !errors.Is(err, store.ErrCodePipelineNotFound) {
		t.Fatalf("after delete err=%v", err)
	}
}

func TestRoute53ListZonesAndQueryLogInject(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	if err := store.EnsureRoute53Schema(nil); err == nil {
		t.Fatal("want nil db error")
	}
	if err := st.EnsureRoute53Schema(); err != nil {
		t.Fatal(err)
	}
	empty, err := st.ListRoute53HostedZones(account)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty zones=%v err=%v", empty, err)
	}
	z1, err := st.CreateRoute53HostedZone(account, "b.example.com", "ref-b", false)
	if err != nil {
		t.Fatal(err)
	}
	z2, err := st.CreateRoute53HostedZone(account, "a.example.com", "ref-a", true)
	if err != nil {
		t.Fatal(err)
	}
	if z1.ID == "" || z2.ID == "" {
		t.Fatalf("zone ids empty z1=%+v z2=%+v", z1, z2)
	}
	zones, err := st.ListRoute53HostedZones(account)
	if err != nil || len(zones) != 2 {
		t.Fatalf("zones=%+v err=%v", zones, err)
	}
	if zones[0].Name != "a.example.com." || zones[1].Name != "b.example.com." {
		t.Fatalf("ordered zones=%+v", zones)
	}
	if !zones[0].PrivateZone || zones[1].PrivateZone {
		t.Fatalf("private flags zones=%+v", zones)
	}

	at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	stream := store.Route53QueryLabLogStreamName(at)
	if !strings.HasPrefix(stream, "route53-query/") {
		t.Fatalf("stream=%q", stream)
	}
	if _, err := st.InjectRoute53QueryLogs(account, "", "  ", "", nil, at); !errors.Is(err, store.ErrRoute53BadRequest) {
		t.Fatalf("empty log group err=%v", err)
	}
	n, err := st.InjectRoute53QueryLogs(account, "", "/aws/route53/gap", "", nil, at)
	if err != nil || n != 1 {
		t.Fatalf("default inject n=%d err=%v", n, err)
	}
	n, err = st.InjectRoute53QueryLogs(account, "us-east-1", "/aws/route53/gap", stream, []string{"", "  ", `{"query_name":"x."}`}, at)
	if err != nil || n != 1 {
		t.Fatalf("custom inject n=%d err=%v", n, err)
	}
	if _, err := st.InjectRoute53QueryLogs(account, "us-east-1", "/aws/route53/gap", stream, []string{"", " "}, at); !errors.Is(err, store.ErrRoute53BadRequest) {
		t.Fatalf("blank lines only err=%v", err)
	}
}

func TestLogsMetricFilterDeleteAndBoundaries(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	if err := store.EnsureLogsMetricFilterSchema(nil); err == nil {
		t.Fatal("want nil db error")
	}
	if err := st.EnsureLogsMetricFilterSchema(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLogGroup(account, "us-east-1", "/metrics/gap"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutMetricFilter(account, "", "f", "ERROR", "M", "NS", "1"); err == nil {
		t.Fatal("want required fields error")
	}
	if _, err := st.PutMetricFilter(account, "/missing", "f", "ERROR", "M", "NS", "1"); err == nil {
		t.Fatal("want missing log group error")
	}
	f, err := st.PutMetricFilter(account, "/metrics/gap", "errors", "ERROR", "ErrorCount", "Lab/Logs", "")
	if err != nil || f.MetricValue != "1" {
		t.Fatalf("put=%+v err=%v", f, err)
	}
	listed, err := st.DescribeMetricFilters(account, "/metrics/gap")
	if err != nil || len(listed) != 1 {
		t.Fatalf("describe=%+v err=%v", listed, err)
	}
	if _, err := st.GetLogsMetricDatapoints(account, "", "ErrorCount", 0, 0); err == nil {
		t.Fatal("want namespace required")
	}
	if err := st.DeleteMetricFilter(account, "/metrics/gap", "missing"); !errors.Is(err, store.ErrLogGroupNotFound) {
		t.Fatalf("delete missing err=%v", err)
	}
	if err := st.DeleteMetricFilter(account, "/metrics/gap", "errors"); err != nil {
		t.Fatal(err)
	}
	after, err := st.DescribeMetricFilters(account, "/metrics/gap")
	if err != nil || len(after) != 0 {
		t.Fatalf("after delete=%+v err=%v", after, err)
	}
}

func TestACMAndTranscribeSchemaNilAndNegatives(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"

	if err := store.EnsureACMSchema(nil); err == nil {
		t.Fatal("want acm nil db error")
	}
	if err := st.EnsureACMSchema(); err != nil {
		t.Fatal(err)
	}
	if arn := store.ACMCertificateARN("", account, "id"); !strings.Contains(arn, "us-east-1") {
		t.Fatalf("acm arn=%q", arn)
	}
	if _, err := st.RequestACMCertificate(account, "", "  "); !errors.Is(err, store.ErrACMBadRequest) {
		t.Fatalf("empty domain err=%v", err)
	}
	if err := st.DeleteACMCertificate(account, "arn:aws:acm:us-east-1:000000000001:certificate/missing"); !errors.Is(err, store.ErrACMNotFound) {
		t.Fatalf("delete missing err=%v", err)
	}

	if err := store.EnsureTranscribeSchema(nil); err == nil {
		t.Fatal("want transcribe nil db error")
	}
	if err := st.EnsureTranscribeSchema(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.StartTranscriptionJobStub(account, "", "", "s3://b/k", "en-US"); !errors.Is(err, store.ErrTranscribeBadRequest) {
		t.Fatalf("empty job name err=%v", err)
	}
	if _, err := st.StartTranscriptionJobStub(account, "", "job", "", "en-US"); !errors.Is(err, store.ErrTranscribeBadRequest) {
		t.Fatalf("empty media err=%v", err)
	}
	if _, err := st.GetTranscriptionJob(account, "missing"); !errors.Is(err, store.ErrTranscribeNotFound) {
		t.Fatalf("missing job err=%v", err)
	}
}

func TestMSKCountAndDescribeByName(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if err := st.EnsureMSKSchema(); err != nil {
		t.Fatal(err)
	}
	n, err := st.CountMSKClusters()
	if err != nil || n != 0 {
		t.Fatalf("count empty n=%d err=%v", n, err)
	}
	if _, err := st.DescribeMSKClusterByName(account, "missing"); !errors.Is(err, store.ErrMSKClusterNotFound) {
		t.Fatalf("missing by name err=%v", err)
	}
	c, err := st.CreateMSKCluster(account, "us-east-1", "GapCluster", "3.6.0", 2, false)
	if err != nil {
		t.Fatal(err)
	}
	n, err = st.CountMSKClusters()
	if err != nil || n != 1 {
		t.Fatalf("count after create n=%d err=%v", n, err)
	}
	got, err := st.DescribeMSKClusterByName(account, "GapCluster")
	if err != nil || got.ClusterARN != c.ClusterARN {
		t.Fatalf("by name=%+v err=%v", got, err)
	}
}

func TestPolicyListAttachedRefsNameFromARN(t *testing.T) {
	st := openTestStore(t)
	arn := "arn:aws:iam::000000000001:user/alice"
	policyARN := "arn:aws:iam::000000000001:policy/GapPolicy"
	if err := st.PutPolicy(policyARN, `{"Version":"2012-10-17","Statement":[]}`); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachPolicy(arn, policyARN); err != nil {
		t.Fatal(err)
	}
	// policy id with no slash forces policyNameFromARN fallback to the whole id
	if err := st.PutPolicy("NoSlashPolicy", `{"Version":"2012-10-17","Statement":[]}`); err != nil {
		t.Fatal(err)
	}
	if err := st.AttachPolicy(arn, "NoSlashPolicy"); err != nil {
		t.Fatal(err)
	}
	refs, err := st.ListAttachedPolicyRefs(arn)
	if err != nil || len(refs) != 2 {
		t.Fatalf("refs=%+v err=%v", refs, err)
	}
	byARN := map[string]string{}
	for _, r := range refs {
		byARN[r.PolicyARN] = r.PolicyName
	}
	if byARN[policyARN] != "GapPolicy" {
		t.Fatalf("policy name=%q want GapPolicy (from ARN)", byARN[policyARN])
	}
	if byARN["NoSlashPolicy"] != "NoSlashPolicy" {
		t.Fatalf("no-slash name=%q", byARN["NoSlashPolicy"])
	}
	empty, err := st.ListAttachedPolicyRefs("arn:aws:iam::000000000001:user/nobody")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty refs=%+v err=%v", empty, err)
	}
}

func TestGuardDutyFindingsBoundaries(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	det, err := st.CreateGuardDutyDetector(account)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := st.InjectGuardDutyFindings(account, det.DetectorID, "", []store.GuardDutyFinding{
		{Type: "Recon:EC2/PortProbeUnprotectedPort", Id: "f-1"},
	})
	if err != nil || len(ids) != 1 || ids[0] != "f-1" {
		t.Fatalf("inject=%v err=%v", ids, err)
	}
	got, err := st.GetGuardDutyFindings(account, det.DetectorID, []string{"f-1", "missing", " "})
	if err != nil || len(got) != 1 || got[0].Severity != 5 {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	if _, err := st.GetGuardDutyFindings(account, det.DetectorID, nil); !errors.Is(err, store.ErrGuardDutyBadRequest) {
		t.Fatalf("empty ids err=%v", err)
	}
	tooMany := make([]store.GuardDutyFinding, 51)
	for i := range tooMany {
		tooMany[i] = store.GuardDutyFinding{Type: "Recon:EC2/PortProbeUnprotectedPort"}
	}
	if _, err := st.InjectGuardDutyFindings(account, det.DetectorID, "us-east-1", tooMany); !errors.Is(err, store.ErrGuardDutyBadRequest) {
		t.Fatalf("cap findings err=%v", err)
	}
	listed, err := st.ListGuardDutyFindingIDs(account, det.DetectorID, 0)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list clamp=%v err=%v", listed, err)
	}
	if _, err := st.ListGuardDutyFindingIDs(account, "missing", 10); !errors.Is(err, store.ErrGuardDutyNotFound) {
		t.Fatalf("list missing detector err=%v", err)
	}
}

func TestBackupARNHelpersAndOtherResource(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if store.BackupVaultARN("", account, "v") == "" || store.BackupPlanARN("", account, "p") == "" {
		t.Fatal("arn helpers empty")
	}
	if store.BackupRecoveryPointARN("", account, "v", "id") == "" {
		t.Fatal("rp arn empty")
	}
	if _, err := st.CreateBackupVault(account, "us-east-1", "  ", ""); !errors.Is(err, store.ErrBackupBadRequest) {
		t.Fatalf("empty vault name err=%v", err)
	}
	if _, err := st.CreateBackupVault(account, "us-east-1", "other-vault", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBackupVault(account, "us-east-1", "other-vault", ""); !errors.Is(err, store.ErrBackupExists) {
		t.Fatalf("dup vault err=%v", err)
	}
	job, rp, err := st.StartBackupJob(account, "us-east-1", "other-vault", "arn:aws:ec2:us-east-1:1:instance/i-1", "")
	if err != nil || rp.ResourceType != "Other" || job.State != "COMPLETED" {
		t.Fatalf("job=%+v rp=%+v err=%v", job, rp, err)
	}
	if _, _, err := st.StartBackupJob(account, "us-east-1", "", "arn:aws:s3:::x", ""); !errors.Is(err, store.ErrBackupBadRequest) {
		t.Fatalf("empty vault start err=%v", err)
	}
	if _, _, err := st.StartBackupJob(account, "us-east-1", "missing", "arn:aws:s3:::x", ""); !errors.Is(err, store.ErrBackupNotFound) {
		t.Fatalf("missing vault start err=%v", err)
	}
	dj, err := st.DescribeBackupJob(account, "", job.BackupJobID)
	if err != nil || dj.BackupJobID != job.BackupJobID {
		t.Fatalf("describe job=%+v err=%v", dj, err)
	}
	if _, err := st.DescribeBackupJob(account, "us-east-1", "missing"); !errors.Is(err, store.ErrBackupNotFound) {
		t.Fatalf("missing job err=%v", err)
	}
	gotRP, err := st.DescribeRecoveryPoint(account, "", "other-vault", rp.RecoveryPointARN)
	if err != nil || gotRP.RecoveryPointARN != rp.RecoveryPointARN {
		t.Fatalf("describe rp=%+v err=%v", gotRP, err)
	}
	if _, err := st.DescribeRecoveryPoint(account, "us-east-1", "other-vault", "missing"); !errors.Is(err, store.ErrBackupNotFound) {
		t.Fatalf("missing rp err=%v", err)
	}
	listed, err := st.ListRecoveryPointsByBackupVault(account, "", "other-vault")
	if err != nil || len(listed) != 1 {
		t.Fatalf("list rp=%+v err=%v", listed, err)
	}
	if _, err := st.ListRecoveryPointsByBackupVault(account, "us-east-1", "missing"); !errors.Is(err, store.ErrBackupNotFound) {
		t.Fatalf("list rp missing vault err=%v", err)
	}
	jobs, err := st.ListBackupJobs(account, "", store.BackupJobListFilter{BackupVaultName: "other-vault"})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("list jobs vault filter=%+v err=%v", jobs, err)
	}
	plan, err := st.CreateBackupPlan(account, "us-east-1", "sel-plan", []any{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBackupSelection(account, "us-east-1", "missing-plan", "sel", "arn:aws:iam::1:role/r", []string{"arn:aws:s3:::x"}); !errors.Is(err, store.ErrBackupNotFound) {
		t.Fatalf("selection missing plan err=%v", err)
	}
	if _, err := st.CreateBackupSelection(account, "us-east-1", plan.BackupPlanID, "", "arn:aws:iam::1:role/r", nil); !errors.Is(err, store.ErrBackupBadRequest) {
		t.Fatalf("empty selection name err=%v", err)
	}
	sel, err := st.CreateBackupSelection(account, "", plan.BackupPlanID, "sel-1", "arn:aws:iam::1:role/r", []string{"arn:aws:s3:::x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetBackupSelection(account, "", plan.BackupPlanID, "missing"); !errors.Is(err, store.ErrBackupNotFound) {
		t.Fatalf("get selection missing err=%v", err)
	}
	gotSel, err := st.GetBackupSelection(account, "", plan.BackupPlanID, sel.SelectionID)
	if err != nil || gotSel.SelectionName != "sel-1" {
		t.Fatalf("get selection=%+v err=%v", gotSel, err)
	}
	if err := st.DeleteBackupSelection(account, "", plan.BackupPlanID, "missing"); !errors.Is(err, store.ErrBackupNotFound) {
		t.Fatalf("delete selection missing err=%v", err)
	}
}

func TestRoute53RecordNegativesAndDelete(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	zone, err := st.CreateRoute53HostedZone(account, "gap.example.com", "", false)
	if err != nil || zone.CallerRef == "" {
		t.Fatalf("zone=%+v err=%v", zone, err)
	}
	if _, err := st.CreateRoute53HostedZone(account, "  ", "ref", false); !errors.Is(err, store.ErrRoute53BadRequest) {
		t.Fatalf("empty name err=%v", err)
	}
	if _, err := st.GetRoute53HostedZone(account, "ZMISSING"); !errors.Is(err, store.ErrRoute53NotFound) {
		t.Fatalf("missing zone err=%v", err)
	}
	if err := st.ChangeRoute53ResourceRecordSets(account, zone.ID, []store.Route53Change{
		{Action: "CREATE", Name: "www.gap.example.com", Type: "TXT", Records: []string{"x"}},
	}); !errors.Is(err, store.ErrRoute53BadRequest) {
		t.Fatalf("bad type err=%v", err)
	}
	if err := st.ChangeRoute53ResourceRecordSets(account, zone.ID, []store.Route53Change{
		{Action: "NOPE", Name: "www.gap.example.com", Type: "A", Records: []string{"1.1.1.1"}},
	}); !errors.Is(err, store.ErrRoute53BadRequest) {
		t.Fatalf("bad action err=%v", err)
	}
	if err := st.ChangeRoute53ResourceRecordSets(account, zone.ID, []store.Route53Change{
		{Action: "CREATE", Name: "www.gap.example.com", Type: "A", Records: []string{"1.1.1.1"}},
		{Action: "DELETE", Name: "www.gap.example.com", Type: "A"},
	}); err != nil {
		t.Fatal(err)
	}
	sets, err := st.ListRoute53ResourceRecordSets(account, zone.ID)
	if err != nil || len(sets) != 0 {
		t.Fatalf("after delete sets=%+v err=%v", sets, err)
	}
	if err := st.DeleteRoute53HostedZone(account, "ZMISSING"); !errors.Is(err, store.ErrRoute53NotFound) {
		t.Fatalf("delete missing err=%v", err)
	}
}

func TestCodePipelineEmptyStagesAndDuplicate(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if _, err := st.CreateCodePipeline(account, "us-east-1", store.CodePipelineDeclaration{
		Name: "dup-pipe", Stages: nil,
	}); !errors.Is(err, store.ErrCodePipelineBadReq) {
		t.Fatalf("empty stages err=%v", err)
	}
	decl := store.CodePipelineDeclaration{
		Name: "dup-pipe",
		Stages: []store.CodePipelineStageDecl{{
			Name: "Build",
			Actions: []store.CodePipelineActionDecl{{
				Name: "Build",
				ActionTypeID: store.CodePipelineActionTypeID{
					Category: "Build", Owner: "AWS", Provider: "CodeBuild", Version: "1",
				},
				Configuration: map[string]string{"ProjectName": "lab"},
			}},
		}},
	}
	if _, err := st.CreateCodePipeline(account, "us-east-1", decl); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateCodePipeline(account, "us-east-1", decl); !errors.Is(err, store.ErrCodePipelineExists) {
		t.Fatalf("dup err=%v", err)
	}
	if _, err := st.GetCodePipeline(account, "missing"); !errors.Is(err, store.ErrCodePipelineNotFound) {
		t.Fatalf("get missing err=%v", err)
	}
}

func TestMSKSharedCountAndNameValidation(t *testing.T) {
	st := openTestStore(t)
	account := "000000000001"
	if err := st.EnsureMSKSchema(); err != nil {
		t.Fatal(err)
	}
	n, err := st.CountMSKSharedSlotHolders()
	if err != nil || n != 0 {
		t.Fatalf("shared empty n=%d err=%v", n, err)
	}
	if _, err := st.CreateMSKCluster(account, "us-east-1", "bad name", "3.6.0", 1, false); !errors.Is(err, store.ErrMSKBadRequest) {
		t.Fatalf("invalid cluster name err=%v", err)
	}
	if _, err := st.CreateMSKCluster(account, "us-east-1", "1bad", "3.6.0", 1, false); !errors.Is(err, store.ErrMSKBadRequest) {
		t.Fatalf("must start with letter err=%v", err)
	}
	if _, err := st.CreateMSKCluster(account, "us-east-1", "  ", "3.6.0", 1, false); !errors.Is(err, store.ErrMSKBadRequest) {
		t.Fatalf("empty name err=%v", err)
	}
	c, err := st.CreateMSKCluster(account, "us-east-1", "ok-msk", "", 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if c.KafkaVersion != "3.6.0" {
		t.Fatalf("default kafka version=%q", c.KafkaVersion)
	}
	n, err = st.CountMSKSharedSlotHolders()
	if err != nil || n != 1 {
		t.Fatalf("shared holders n=%d err=%v", n, err)
	}
	list, err := st.ListMSKClusters(account)
	if err != nil || len(list) != 1 || list[0].ClusterARN != c.ClusterARN {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if err := st.SetMSKContainerID(account, "ok-msk", "", "ACTIVE", "broker:9092"); !errors.Is(err, store.ErrMSKBadRequest) {
		t.Fatalf("ACTIVE requires container err=%v", err)
	}
	if err := st.SetMSKContainerID(account, "ok-msk", "ctr-1", "ACTIVE", "stub://x"); !errors.Is(err, store.ErrMSKBadRequest) {
		t.Fatalf("ACTIVE stub reject err=%v", err)
	}
}

func TestDynamoDBLSIHelpersAndSecondSlot(t *testing.T) {
	st := openKMSStore(t)
	account := "000000000001"
	lsis := []store.DynamoLSI{
		{IndexName: "ByStatus", RangeKeyName: "status", RangeKeyType: store.KeyTypeString},
		{IndexName: "ByCode", RangeKeyName: "code", RangeKeyType: store.KeyTypeString},
	}
	tbl, err := st.CreateTableWithIndexes(account, "us-east-1", "Orders2",
		"pk", store.KeyTypeString, "sk", store.KeyTypeString, "", "", nil, lsis)
	if err != nil {
		t.Fatal(err)
	}
	if !tbl.HasLSI() || !tbl.HasLSI2() {
		t.Fatalf("want two LSIs: %+v", tbl)
	}
	all := tbl.LSIs()
	if len(all) != 2 || all[1].IndexName != "ByCode" {
		t.Fatalf("LSIs=%+v", all)
	}
	lsi, slot, ok := tbl.LSIByName("ByCode")
	if !ok || slot != 2 || lsi.RangeKeyName != "code" {
		t.Fatalf("LSIByName=%+v slot=%d ok=%v", lsi, slot, ok)
	}
	if _, _, ok := tbl.LSIByName(""); ok {
		t.Fatal("empty name must miss")
	}
	if _, _, ok := tbl.LSIByName("Nope"); ok {
		t.Fatal("missing name must miss")
	}
	kind, _, lsi2, slot2, ok := tbl.IndexByName("ByStatus")
	if !ok || kind != "LSI" || slot2 != 1 || lsi2.IndexName != "ByStatus" {
		t.Fatalf("IndexByName LSI kind=%s slot=%d %+v", kind, slot2, lsi2)
	}
	if _, _, _, _, ok := tbl.IndexByName("missing"); ok {
		t.Fatal("IndexByName missing must fail")
	}

	if err := st.PutItemBytesIndexed(account, "Orders2",
		`{"S":"u1"}`, `{"S":"o1"}`, "", "", "", "", `{"S":"OPEN"}`, `{"S":"A"}`,
		[]byte(`{"pk":{"S":"u1"},"sk":{"S":"o1"},"status":{"S":"OPEN"},"code":{"S":"A"}}`), false, nil); err != nil {
		t.Fatal(err)
	}
	sk, err := st.GetItemLSISlotKeys(account, "Orders2", `{"S":"u1"}`, `{"S":"o1"}`, 1)
	if err != nil || sk == "" {
		t.Fatalf("slot1 sk=%q err=%v", sk, err)
	}
	sk2, err := st.GetItemLSISlotKeys(account, "Orders2", `{"S":"u1"}`, `{"S":"o1"}`, 2)
	if err != nil || sk2 == "" {
		t.Fatalf("slot2 sk=%q err=%v", sk2, err)
	}
	_, err = st.GetItemLSISlotKeys(account, "Orders2", `{"S":"u1"}`, `{"S":"missing"}`, 1)
	if !errors.Is(err, store.ErrNoSuchItem) {
		t.Fatalf("missing item err=%v", err)
	}
	page, err := st.QueryLSISlotItems(account, "Orders2", 2, `{"S":"u1"}`, 10, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("query slot2=%+v err=%v", page, err)
	}
}

func TestKinesisResourcePolicyCRUD(t *testing.T) {
	st := openKinesisStore(t)
	account := "000000000001"
	stream, err := st.CreateKinesisStream(account, "us-east-1", "pol-stream", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureKinesisResourcePolicySchema(); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureKinesisResourcePolicySchema(nil); err == nil {
		t.Fatal("nil db must fail")
	}

	_, err = st.GetKinesisResourcePolicy(account, "")
	if err == nil {
		t.Fatal("empty name must fail")
	}
	_, err = st.GetKinesisResourcePolicy(account, stream.StreamName)
	if !errors.Is(err, store.ErrNoSuchResourcePolicy) {
		t.Fatalf("empty policy err=%v", err)
	}
	if err := st.PutKinesisResourcePolicy(account, stream.StreamName, ""); err == nil {
		t.Fatal("empty policy body must fail")
	}
	if err := st.PutKinesisResourcePolicy(account, stream.StreamName, `{"Version":"2012-10-17","Statement":[]}`); err == nil {
		t.Fatal("policy without statements must fail validation")
	}
	pol := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":"kinesis:PutRecord","Resource":"` + stream.StreamARN + `"}]}`
	if err := st.PutKinesisResourcePolicy(account, stream.StreamARN, pol); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetKinesisResourcePolicy(account, stream.StreamARN)
	if err != nil || got != pol {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if err := st.DeleteKinesisResourcePolicy(account, stream.StreamName); err != nil {
		t.Fatal(err)
	}
	_, err = st.GetKinesisResourcePolicy(account, stream.StreamName)
	if !errors.Is(err, store.ErrNoSuchResourcePolicy) {
		t.Fatalf("after delete err=%v", err)
	}
	if err := st.DeleteKinesisResourcePolicy(account, "missing"); err == nil {
		t.Fatal("delete missing stream must fail")
	}
}

func TestSFNResourcePolicyCRUD(t *testing.T) {
	st := openSFNStore(t)
	account := "000000000001"
	def := `{"StartAt":"Hello","States":{"Hello":{"Type":"Pass","End":true}}}`
	sm, err := st.CreateSFNStateMachine(account, "us-east-1", "pol-sm", def, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureSFNResourcePolicySchema(); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureSFNResourcePolicySchema(nil); err == nil {
		t.Fatal("nil db must fail")
	}

	_, err = st.GetSFNResourcePolicy(account, sm.Name)
	if !errors.Is(err, store.ErrNoSuchResourcePolicy) {
		t.Fatalf("empty policy err=%v", err)
	}
	if err := st.PutSFNResourcePolicy(account, sm.Name, ""); err == nil {
		t.Fatal("empty policy must fail")
	}
	pol := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"events.amazonaws.com"},"Action":"states:StartExecution","Resource":"` + sm.StateMachineARN + `"}]}`
	if err := st.PutSFNResourcePolicy(account, sm.StateMachineARN, pol); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetSFNResourcePolicy(account, sm.Name)
	if err != nil || got != pol {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if err := st.DeleteSFNResourcePolicy(account, sm.Name); err != nil {
		t.Fatal(err)
	}
	_, err = st.GetSFNResourcePolicy(account, sm.Name)
	if !errors.Is(err, store.ErrNoSuchResourcePolicy) {
		t.Fatalf("after delete err=%v", err)
	}
	if err := st.DeleteSFNResourcePolicy(account, "missing-sm"); err == nil {
		t.Fatal("delete missing sm must fail")
	}
}

func TestCognitoTriggersConfigAPIAndSet(t *testing.T) {
	st := openCognitoTriggerStore(t)
	account := "000000000001"
	if err := st.EnsureCognitoTriggerSchema(); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureCognitoTriggerSchema(nil); err == nil {
		t.Fatal("nil db must fail")
	}

	empty := store.CognitoLambdaConfigFromAPI(nil)
	if store.CognitoLambdaConfigToAPI(empty) != nil {
		t.Fatal("empty config should omit API object")
	}
	cfg := store.CognitoLambdaConfigFromAPI(map[string]any{
		"PreSignUp": "arn:aws:lambda:us-east-1:1:function:pre",
		"PreTokenGenerationConfig": map[string]any{
			"LambdaArn": "arn:aws:lambda:us-east-1:1:function:ptg",
		},
		"CustomMessage": "arn:aws:lambda:us-east-1:1:function:msg",
	})
	if cfg.PreSignUp == "" || cfg.PreTokenGeneration == "" || cfg.CustomMessage == "" {
		t.Fatalf("cfg=%+v", cfg)
	}
	api := store.CognitoLambdaConfigToAPI(cfg)
	if api["PreSignUp"] == nil || api["PreTokenGeneration"] == nil {
		t.Fatalf("api=%v", api)
	}

	pool, err := st.CreateCognitoUserPool(account, "us-east-1", "trig-pool")
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.SetCognitoUserPoolTriggers(account, "", "arn:aws:iam::1:role/r", cfg)
	if !errors.Is(err, store.ErrCognitoBadRequest) {
		t.Fatalf("empty pool err=%v", err)
	}
	_, err = st.SetCognitoUserPoolTriggers(account, "missing", "arn:aws:iam::1:role/r", cfg)
	if err == nil {
		t.Fatal("missing pool must fail")
	}
	updated, err := st.SetCognitoUserPoolTriggers(account, pool.PoolID, "arn:aws:iam::1:role/r", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if updated.RoleArn == "" || !updated.LambdaConfig.HasTriggers() {
		t.Fatalf("updated=%+v", updated)
	}
}

func TestCloudTrailEventSelectorsStoreAndParse(t *testing.T) {
	st := openForensicsStore(t)
	account := "000000000001"
	bucket := "ct-sel-bucket"
	if _, err := st.CreateBucket(account, bucket); err != nil {
		t.Fatal(err)
	}
	trail, err := st.CreateCloudTrailTrail(account, store.CloudTrailTrail{
		Name:         "sel-trail",
		S3BucketName: bucket,
		HomeRegion:   store.DefaultCloudTrailRegion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutCloudTrailEventSelectors(account, trail.Name, store.CloudTrailEventSelectors{
		IncludeManagementEvents: false,
		S3DataEventsEnabled:     true,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetCloudTrailEventSelectors(account, trail.Name)
	if err != nil || got.IncludeManagementEvents || !got.S3DataEventsEnabled {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	_, err = st.GetCloudTrailEventSelectors(account, "missing")
	if err == nil {
		t.Fatal("missing trail must fail")
	}

	parsed := store.ParseCloudTrailEventSelectorsFromAPI([]any{
		map[string]any{
			"IncludeManagementEvents": "false",
			"DataResources": []any{
				map[string]any{"Type": "AWS::S3::Object"},
			},
		},
		"skip",
	})
	if parsed.IncludeManagementEvents || !parsed.S3DataEventsEnabled {
		t.Fatalf("parsed=%+v", parsed)
	}
	parsedBool := store.ParseCloudTrailEventSelectorsFromAPI([]any{
		map[string]any{"IncludeManagementEvents": true},
	})
	if !parsedBool.IncludeManagementEvents {
		t.Fatal("bool true management events")
	}
}

func TestEventBusPolicyResolveAndEvaluate(t *testing.T) {
	st := openEventsStore(t)
	account := "000000000001"

	owner, name, ok := store.ResolveEventBusRef(account, "")
	if !ok || owner != account || name != store.DefaultEventBusName {
		t.Fatalf("default ref owner=%s name=%s ok=%v", owner, name, ok)
	}
	owner, name, ok = store.ResolveEventBusRef(account, "custom")
	if !ok || name != "custom" {
		t.Fatalf("bare name=%s ok=%v", name, ok)
	}
	arn := "arn:aws:events:us-east-1:111122223333:event-bus/shared"
	owner, name, ok = store.ResolveEventBusRef(account, arn)
	if !ok || owner != "111122223333" || name != "shared" {
		t.Fatalf("arn ref owner=%s name=%s", owner, name)
	}
	_, _, _, ok = store.ParseEventBusARN("arn:aws:s3:::bucket")
	if ok {
		t.Fatal("non-events ARN must fail parse")
	}

	if _, err := st.CreateEventBus(account, "us-east-1", "pol-bus"); err != nil {
		t.Fatal(err)
	}
	if err := st.PutEventBusPolicy(account, "pol-bus", `{not-json`); err == nil {
		t.Fatal("invalid json must fail")
	}
	pol := `{"Version":"2012-10-17","Statement":[{"Sid":"AllowPut","Effect":"Allow","Principal":{"AWS":"*"},"Action":"events:PutEvents","Resource":"*"}]}`
	if err := st.PutEventBusPolicy(account, "pol-bus", pol); err != nil {
		t.Fatal(err)
	}
	bus, err := st.DescribeEventBus(account, "pol-bus")
	if err != nil {
		t.Fatal(err)
	}
	dec := store.EvaluateEventBusPutEvents(
		identity.AnonymousPrincipal(account),
		nil,
		bus,
		"us-east-1",
		nil,
	)
	if dec != authz.Allow {
		t.Fatalf("want allow, got %+v", dec)
	}
	if err := st.PutEventBusPermission(account, "pol-bus", "", "events.amazonaws.com", nil, nil); err == nil {
		t.Fatal("empty statement id must fail")
	}
	if err := st.RemoveEventBusPermission(account, "pol-bus", "AllowPut"); err != nil {
		t.Fatal(err)
	}
	if err := st.RemoveEventBusPermission(account, "pol-bus", "AllowPut"); err == nil {
		t.Fatal("second remove must fail")
	}
}
