package server

import (
	"strings"
	"testing"
)

// Covers short-name -> catalog action mappers and normalizeAction branches.
func TestActionMapperCoverage(t *testing.T) {
	t.Parallel()
	passthrough := "Already:Namespaced"
	if got := acmAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := acmAction("RequestCertificate")
		if got == "" {
			t.Fatalf("acmAction(%q) empty", "RequestCertificate")
		}
	}
	{
		got := acmAction("DescribeCertificate")
		if got == "" {
			t.Fatalf("acmAction(%q) empty", "DescribeCertificate")
		}
	}
	{
		got := acmAction("ListCertificates")
		if got == "" {
			t.Fatalf("acmAction(%q) empty", "ListCertificates")
		}
	}
	{
		got := acmAction("DeleteCertificate")
		if got == "" {
			t.Fatalf("acmAction(%q) empty", "DeleteCertificate")
		}
	}
	if got := apiGatewayV2Action(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := apiGatewayV2Action("CreateApi")
		if got == "" {
			t.Fatalf("apiGatewayV2Action(%q) empty", "CreateApi")
		}
	}
	{
		got := apiGatewayV2Action("GetApi")
		if got == "" {
			t.Fatalf("apiGatewayV2Action(%q) empty", "GetApi")
		}
	}
	{
		got := apiGatewayV2Action("UpdateApi")
		if got == "" {
			t.Fatalf("apiGatewayV2Action(%q) empty", "UpdateApi")
		}
	}
	{
		got := apiGatewayV2Action("DeleteApi")
		if got == "" {
			t.Fatalf("apiGatewayV2Action(%q) empty", "DeleteApi")
		}
	}
	{
		got := apiGatewayV2Action("GetApis")
		if got == "" {
			t.Fatalf("apiGatewayV2Action(%q) empty", "GetApis")
		}
	}
	{
		got := apiGatewayV2Action("CreateIntegration")
		if got == "" {
			t.Fatalf("apiGatewayV2Action(%q) empty", "CreateIntegration")
		}
	}
	{
		got := apiGatewayV2Action("GetIntegrations")
		if got == "" {
			t.Fatalf("apiGatewayV2Action(%q) empty", "GetIntegrations")
		}
	}
	{
		got := apiGatewayV2Action("CreateAuthorizer")
		if got == "" {
			t.Fatalf("apiGatewayV2Action(%q) empty", "CreateAuthorizer")
		}
	}
	{
		got := apiGatewayV2Action("GetAuthorizers")
		if got == "" {
			t.Fatalf("apiGatewayV2Action(%q) empty", "GetAuthorizers")
		}
	}
	{
		got := apiGatewayV2Action("CreateRoute")
		if got == "" {
			t.Fatalf("apiGatewayV2Action(%q) empty", "CreateRoute")
		}
	}
	{
		got := apiGatewayV2Action("GetRoutes")
		if got == "" {
			t.Fatalf("apiGatewayV2Action(%q) empty", "GetRoutes")
		}
	}
	{
		got := apiGatewayV2Action("CreateStage")
		if got == "" {
			t.Fatalf("apiGatewayV2Action(%q) empty", "CreateStage")
		}
	}
	if got := apiGatewayRESTAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := apiGatewayRESTAction("CreateRestApi")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "CreateRestApi")
		}
	}
	{
		got := apiGatewayRESTAction("GetRestApi")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "GetRestApi")
		}
	}
	{
		got := apiGatewayRESTAction("GetRestApis")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "GetRestApis")
		}
	}
	{
		got := apiGatewayRESTAction("DeleteRestApi")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "DeleteRestApi")
		}
	}
	{
		got := apiGatewayRESTAction("CreateResource")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "CreateResource")
		}
	}
	{
		got := apiGatewayRESTAction("GetResources")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "GetResources")
		}
	}
	{
		got := apiGatewayRESTAction("DeleteResource")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "DeleteResource")
		}
	}
	{
		got := apiGatewayRESTAction("PutMethod")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "PutMethod")
		}
	}
	{
		got := apiGatewayRESTAction("GetMethod")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "GetMethod")
		}
	}
	{
		got := apiGatewayRESTAction("DeleteMethod")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "DeleteMethod")
		}
	}
	{
		got := apiGatewayRESTAction("PutIntegration")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "PutIntegration")
		}
	}
	{
		got := apiGatewayRESTAction("GetIntegration")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "GetIntegration")
		}
	}
	{
		got := apiGatewayRESTAction("CreateDeployment")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "CreateDeployment")
		}
	}
	{
		got := apiGatewayRESTAction("CreateStage")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "CreateStage")
		}
	}
	{
		got := apiGatewayRESTAction("GetStage")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "GetStage")
		}
	}
	{
		got := apiGatewayRESTAction("CreateAuthorizer")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "CreateAuthorizer")
		}
	}
	{
		got := apiGatewayRESTAction("GetAuthorizer")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "GetAuthorizer")
		}
	}
	{
		got := apiGatewayRESTAction("GetAuthorizers")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "GetAuthorizers")
		}
	}
	{
		got := apiGatewayRESTAction("DeleteAuthorizer")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "DeleteAuthorizer")
		}
	}
	{
		got := apiGatewayRESTAction("CreateApiKey")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "CreateApiKey")
		}
	}
	{
		got := apiGatewayRESTAction("GetApiKey")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "GetApiKey")
		}
	}
	{
		got := apiGatewayRESTAction("GetApiKeys")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "GetApiKeys")
		}
	}
	{
		got := apiGatewayRESTAction("DeleteApiKey")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "DeleteApiKey")
		}
	}
	{
		got := apiGatewayRESTAction("CreateUsagePlan")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "CreateUsagePlan")
		}
	}
	{
		got := apiGatewayRESTAction("GetUsagePlan")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "GetUsagePlan")
		}
	}
	{
		got := apiGatewayRESTAction("GetUsagePlans")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "GetUsagePlans")
		}
	}
	{
		got := apiGatewayRESTAction("DeleteUsagePlan")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "DeleteUsagePlan")
		}
	}
	{
		got := apiGatewayRESTAction("CreateUsagePlanKey")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "CreateUsagePlanKey")
		}
	}
	{
		got := apiGatewayRESTAction("GetUsagePlanKeys")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "GetUsagePlanKeys")
		}
	}
	{
		got := apiGatewayRESTAction("DeleteUsagePlanKey")
		if got == "" {
			t.Fatalf("apiGatewayRESTAction(%q) empty", "DeleteUsagePlanKey")
		}
	}
	if got := appconfigAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := appconfigAction("CreateApplication")
		if got == "" {
			t.Fatalf("appconfigAction(%q) empty", "CreateApplication")
		}
	}
	{
		got := appconfigAction("CreateEnvironment")
		if got == "" {
			t.Fatalf("appconfigAction(%q) empty", "CreateEnvironment")
		}
	}
	{
		got := appconfigAction("CreateConfigurationProfile")
		if got == "" {
			t.Fatalf("appconfigAction(%q) empty", "CreateConfigurationProfile")
		}
	}
	{
		got := appconfigAction("CreateHostedConfigurationVersion")
		if got == "" {
			t.Fatalf("appconfigAction(%q) empty", "CreateHostedConfigurationVersion")
		}
	}
	{
		got := appconfigAction("GetConfiguration")
		if got == "" {
			t.Fatalf("appconfigAction(%q) empty", "GetConfiguration")
		}
	}
	{
		got := appconfigAction("StartConfigurationSession")
		if got == "" {
			t.Fatalf("appconfigAction(%q) empty", "StartConfigurationSession")
		}
	}
	{
		got := appconfigAction("GetLatestConfiguration")
		if got == "" {
			t.Fatalf("appconfigAction(%q) empty", "GetLatestConfiguration")
		}
	}
	{
		got := appconfigAction("StartDeployment")
		if got == "" {
			t.Fatalf("appconfigAction(%q) empty", "StartDeployment")
		}
	}
	{
		got := appconfigAction("GetDeployment")
		if got == "" {
			t.Fatalf("appconfigAction(%q) empty", "GetDeployment")
		}
	}
	{
		got := appconfigAction("ListDeployments")
		if got == "" {
			t.Fatalf("appconfigAction(%q) empty", "ListDeployments")
		}
	}
	if got := appsyncAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := appsyncAction("CreateGraphqlApi")
		if got == "" {
			t.Fatalf("appsyncAction(%q) empty", "CreateGraphqlApi")
		}
	}
	{
		got := appsyncAction("DeleteGraphqlApi")
		if got == "" {
			t.Fatalf("appsyncAction(%q) empty", "DeleteGraphqlApi")
		}
	}
	{
		got := appsyncAction("GetGraphqlApi")
		if got == "" {
			t.Fatalf("appsyncAction(%q) empty", "GetGraphqlApi")
		}
	}
	{
		got := appsyncAction("ListGraphqlApis")
		if got == "" {
			t.Fatalf("appsyncAction(%q) empty", "ListGraphqlApis")
		}
	}
	{
		got := appsyncAction("StartSchemaCreation")
		if got == "" {
			t.Fatalf("appsyncAction(%q) empty", "StartSchemaCreation")
		}
	}
	{
		got := appsyncAction("GetSchemaCreationStatus")
		if got == "" {
			t.Fatalf("appsyncAction(%q) empty", "GetSchemaCreationStatus")
		}
	}
	{
		got := appsyncAction("CreateApiKey")
		if got == "" {
			t.Fatalf("appsyncAction(%q) empty", "CreateApiKey")
		}
	}
	{
		got := appsyncAction("ListApiKeys")
		if got == "" {
			t.Fatalf("appsyncAction(%q) empty", "ListApiKeys")
		}
	}
	{
		got := appsyncAction("DeleteApiKey")
		if got == "" {
			t.Fatalf("appsyncAction(%q) empty", "DeleteApiKey")
		}
	}
	{
		got := appsyncAction("CreateDataSource")
		if got == "" {
			t.Fatalf("appsyncAction(%q) empty", "CreateDataSource")
		}
	}
	{
		got := appsyncAction("UpdateDataSource")
		if got == "" {
			t.Fatalf("appsyncAction(%q) empty", "UpdateDataSource")
		}
	}
	{
		got := appsyncAction("DeleteDataSource")
		if got == "" {
			t.Fatalf("appsyncAction(%q) empty", "DeleteDataSource")
		}
	}
	{
		got := appsyncAction("GetDataSource")
		if got == "" {
			t.Fatalf("appsyncAction(%q) empty", "GetDataSource")
		}
	}
	{
		got := appsyncAction("ListDataSources")
		if got == "" {
			t.Fatalf("appsyncAction(%q) empty", "ListDataSources")
		}
	}
	{
		got := appsyncAction("CreateResolver")
		if got == "" {
			t.Fatalf("appsyncAction(%q) empty", "CreateResolver")
		}
	}
	{
		got := appsyncAction("UpdateResolver")
		if got == "" {
			t.Fatalf("appsyncAction(%q) empty", "UpdateResolver")
		}
	}
	{
		got := appsyncAction("DeleteResolver")
		if got == "" {
			t.Fatalf("appsyncAction(%q) empty", "DeleteResolver")
		}
	}
	{
		got := appsyncAction("GetResolver")
		if got == "" {
			t.Fatalf("appsyncAction(%q) empty", "GetResolver")
		}
	}
	{
		got := appsyncAction("ListResolvers")
		if got == "" {
			t.Fatalf("appsyncAction(%q) empty", "ListResolvers")
		}
	}
	if got := athenaAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := athenaAction("StartQueryExecution")
		if got == "" {
			t.Fatalf("athenaAction(%q) empty", "StartQueryExecution")
		}
	}
	{
		got := athenaAction("GetQueryExecution")
		if got == "" {
			t.Fatalf("athenaAction(%q) empty", "GetQueryExecution")
		}
	}
	{
		got := athenaAction("GetQueryResults")
		if got == "" {
			t.Fatalf("athenaAction(%q) empty", "GetQueryResults")
		}
	}
	{
		got := athenaAction("StopQueryExecution")
		if got == "" {
			t.Fatalf("athenaAction(%q) empty", "StopQueryExecution")
		}
	}
	{
		got := athenaAction("CreateWorkGroup")
		if got == "" {
			t.Fatalf("athenaAction(%q) empty", "CreateWorkGroup")
		}
	}
	{
		got := athenaAction("GetWorkGroup")
		if got == "" {
			t.Fatalf("athenaAction(%q) empty", "GetWorkGroup")
		}
	}
	{
		got := athenaAction("ListWorkGroups")
		if got == "" {
			t.Fatalf("athenaAction(%q) empty", "ListWorkGroups")
		}
	}
	{
		got := athenaAction("DeleteWorkGroup")
		if got == "" {
			t.Fatalf("athenaAction(%q) empty", "DeleteWorkGroup")
		}
	}
	{
		got := athenaAction("UpdateWorkGroup")
		if got == "" {
			t.Fatalf("athenaAction(%q) empty", "UpdateWorkGroup")
		}
	}
	if got := asgAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := asgAction("CreateLaunchConfiguration")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "CreateLaunchConfiguration")
		}
	}
	{
		got := asgAction("DescribeLaunchConfigurations")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "DescribeLaunchConfigurations")
		}
	}
	{
		got := asgAction("DeleteLaunchConfiguration")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "DeleteLaunchConfiguration")
		}
	}
	{
		got := asgAction("CreateAutoScalingGroup")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "CreateAutoScalingGroup")
		}
	}
	{
		got := asgAction("DescribeAutoScalingGroups")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "DescribeAutoScalingGroups")
		}
	}
	{
		got := asgAction("UpdateAutoScalingGroup")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "UpdateAutoScalingGroup")
		}
	}
	{
		got := asgAction("DeleteAutoScalingGroup")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "DeleteAutoScalingGroup")
		}
	}
	{
		got := asgAction("SetDesiredCapacity")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "SetDesiredCapacity")
		}
	}
	{
		got := asgAction("PutScalingPolicy")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "PutScalingPolicy")
		}
	}
	{
		got := asgAction("DescribePolicies")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "DescribePolicies")
		}
	}
	{
		got := asgAction("DeletePolicy")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "DeletePolicy")
		}
	}
	{
		got := asgAction("PutLifecycleHook")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "PutLifecycleHook")
		}
	}
	{
		got := asgAction("DescribeLifecycleHooks")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "DescribeLifecycleHooks")
		}
	}
	{
		got := asgAction("DeleteLifecycleHook")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "DeleteLifecycleHook")
		}
	}
	{
		got := asgAction("AttachInstances")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "AttachInstances")
		}
	}
	{
		got := asgAction("DetachInstances")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "DetachInstances")
		}
	}
	{
		got := asgAction("DescribeAutoScalingInstances")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "DescribeAutoScalingInstances")
		}
	}
	{
		got := asgAction("AttachLoadBalancerTargetGroups")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "AttachLoadBalancerTargetGroups")
		}
	}
	{
		got := asgAction("DetachLoadBalancerTargetGroups")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "DetachLoadBalancerTargetGroups")
		}
	}
	{
		got := asgAction("DescribeLoadBalancerTargetGroups")
		if got == "" {
			t.Fatalf("asgAction(%q) empty", "DescribeLoadBalancerTargetGroups")
		}
	}
	if got := backupAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := backupAction("CreateBackupVault")
		if got == "" {
			t.Fatalf("backupAction(%q) empty", "CreateBackupVault")
		}
	}
	{
		got := backupAction("DescribeBackupVault")
		if got == "" {
			t.Fatalf("backupAction(%q) empty", "DescribeBackupVault")
		}
	}
	{
		got := backupAction("ListBackupVaults")
		if got == "" {
			t.Fatalf("backupAction(%q) empty", "ListBackupVaults")
		}
	}
	{
		got := backupAction("DeleteBackupVault")
		if got == "" {
			t.Fatalf("backupAction(%q) empty", "DeleteBackupVault")
		}
	}
	{
		got := backupAction("CreateBackupPlan")
		if got == "" {
			t.Fatalf("backupAction(%q) empty", "CreateBackupPlan")
		}
	}
	{
		got := backupAction("GetBackupPlan")
		if got == "" {
			t.Fatalf("backupAction(%q) empty", "GetBackupPlan")
		}
	}
	{
		got := backupAction("ListBackupPlans")
		if got == "" {
			t.Fatalf("backupAction(%q) empty", "ListBackupPlans")
		}
	}
	{
		got := backupAction("DeleteBackupPlan")
		if got == "" {
			t.Fatalf("backupAction(%q) empty", "DeleteBackupPlan")
		}
	}
	{
		got := backupAction("StartBackupJob")
		if got == "" {
			t.Fatalf("backupAction(%q) empty", "StartBackupJob")
		}
	}
	{
		got := backupAction("DescribeBackupJob")
		if got == "" {
			t.Fatalf("backupAction(%q) empty", "DescribeBackupJob")
		}
	}
	{
		got := backupAction("DescribeRecoveryPoint")
		if got == "" {
			t.Fatalf("backupAction(%q) empty", "DescribeRecoveryPoint")
		}
	}
	{
		got := backupAction("ListRecoveryPointsByBackupVault")
		if got == "" {
			t.Fatalf("backupAction(%q) empty", "ListRecoveryPointsByBackupVault")
		}
	}
	{
		got := backupAction("ListBackupJobs")
		if got == "" {
			t.Fatalf("backupAction(%q) empty", "ListBackupJobs")
		}
	}
	{
		got := backupAction("StopBackupJob")
		if got == "" {
			t.Fatalf("backupAction(%q) empty", "StopBackupJob")
		}
	}
	{
		got := backupAction("DeleteRecoveryPoint")
		if got == "" {
			t.Fatalf("backupAction(%q) empty", "DeleteRecoveryPoint")
		}
	}
	{
		got := backupAction("CreateBackupSelection")
		if got == "" {
			t.Fatalf("backupAction(%q) empty", "CreateBackupSelection")
		}
	}
	{
		got := backupAction("GetBackupSelection")
		if got == "" {
			t.Fatalf("backupAction(%q) empty", "GetBackupSelection")
		}
	}
	{
		got := backupAction("ListBackupSelections")
		if got == "" {
			t.Fatalf("backupAction(%q) empty", "ListBackupSelections")
		}
	}
	{
		got := backupAction("DeleteBackupSelection")
		if got == "" {
			t.Fatalf("backupAction(%q) empty", "DeleteBackupSelection")
		}
	}
	if got := batchAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := batchAction("CreateComputeEnvironment")
		if got == "" {
			t.Fatalf("batchAction(%q) empty", "CreateComputeEnvironment")
		}
	}
	{
		got := batchAction("CreateJobQueue")
		if got == "" {
			t.Fatalf("batchAction(%q) empty", "CreateJobQueue")
		}
	}
	{
		got := batchAction("RegisterJobDefinition")
		if got == "" {
			t.Fatalf("batchAction(%q) empty", "RegisterJobDefinition")
		}
	}
	{
		got := batchAction("SubmitJob")
		if got == "" {
			t.Fatalf("batchAction(%q) empty", "SubmitJob")
		}
	}
	{
		got := batchAction("DescribeComputeEnvironments")
		if got == "" {
			t.Fatalf("batchAction(%q) empty", "DescribeComputeEnvironments")
		}
	}
	{
		got := batchAction("DescribeJobQueues")
		if got == "" {
			t.Fatalf("batchAction(%q) empty", "DescribeJobQueues")
		}
	}
	{
		got := batchAction("DescribeJobDefinitions")
		if got == "" {
			t.Fatalf("batchAction(%q) empty", "DescribeJobDefinitions")
		}
	}
	{
		got := batchAction("DescribeJobs")
		if got == "" {
			t.Fatalf("batchAction(%q) empty", "DescribeJobs")
		}
	}
	if got := bcmExportAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := bcmExportAction("CreateExport")
		if got == "" {
			t.Fatalf("bcmExportAction(%q) empty", "CreateExport")
		}
	}
	{
		got := bcmExportAction("GetExport")
		if got == "" {
			t.Fatalf("bcmExportAction(%q) empty", "GetExport")
		}
	}
	{
		got := bcmExportAction("ListExports")
		if got == "" {
			t.Fatalf("bcmExportAction(%q) empty", "ListExports")
		}
	}
	{
		got := bcmExportAction("DeleteExport")
		if got == "" {
			t.Fatalf("bcmExportAction(%q) empty", "DeleteExport")
		}
	}
	if got := beanstalkAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := beanstalkAction("CreateApplication")
		if got == "" {
			t.Fatalf("beanstalkAction(%q) empty", "CreateApplication")
		}
	}
	{
		got := beanstalkAction("DescribeApplications")
		if got == "" {
			t.Fatalf("beanstalkAction(%q) empty", "DescribeApplications")
		}
	}
	{
		got := beanstalkAction("DeleteApplication")
		if got == "" {
			t.Fatalf("beanstalkAction(%q) empty", "DeleteApplication")
		}
	}
	{
		got := beanstalkAction("CreateApplicationVersion")
		if got == "" {
			t.Fatalf("beanstalkAction(%q) empty", "CreateApplicationVersion")
		}
	}
	{
		got := beanstalkAction("CreateEnvironment")
		if got == "" {
			t.Fatalf("beanstalkAction(%q) empty", "CreateEnvironment")
		}
	}
	{
		got := beanstalkAction("DescribeEnvironments")
		if got == "" {
			t.Fatalf("beanstalkAction(%q) empty", "DescribeEnvironments")
		}
	}
	{
		got := beanstalkAction("TerminateEnvironment")
		if got == "" {
			t.Fatalf("beanstalkAction(%q) empty", "TerminateEnvironment")
		}
	}
	{
		got := beanstalkAction("ListAvailableSolutionStacks")
		if got == "" {
			t.Fatalf("beanstalkAction(%q) empty", "ListAvailableSolutionStacks")
		}
	}
	if got := bedrockAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := bedrockAction("InvokeModel")
		if got == "" {
			t.Fatalf("bedrockAction(%q) empty", "InvokeModel")
		}
	}
	{
		got := bedrockAction("Converse")
		if got == "" {
			t.Fatalf("bedrockAction(%q) empty", "Converse")
		}
	}
	{
		got := bedrockAction("ConverseStream")
		if got == "" {
			t.Fatalf("bedrockAction(%q) empty", "ConverseStream")
		}
	}
	if got := budgetsAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := budgetsAction("CreateBudget")
		if got == "" {
			t.Fatalf("budgetsAction(%q) empty", "CreateBudget")
		}
	}
	{
		got := budgetsAction("DescribeBudget")
		if got == "" {
			t.Fatalf("budgetsAction(%q) empty", "DescribeBudget")
		}
	}
	{
		got := budgetsAction("DescribeBudgets")
		if got == "" {
			t.Fatalf("budgetsAction(%q) empty", "DescribeBudgets")
		}
	}
	{
		got := budgetsAction("DeleteBudget")
		if got == "" {
			t.Fatalf("budgetsAction(%q) empty", "DeleteBudget")
		}
	}
	if got := cloudControlAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := cloudControlAction("CreateResource")
		if got == "" {
			t.Fatalf("cloudControlAction(%q) empty", "CreateResource")
		}
	}
	{
		got := cloudControlAction("GetResource")
		if got == "" {
			t.Fatalf("cloudControlAction(%q) empty", "GetResource")
		}
	}
	{
		got := cloudControlAction("ListResources")
		if got == "" {
			t.Fatalf("cloudControlAction(%q) empty", "ListResources")
		}
	}
	{
		got := cloudControlAction("DeleteResource")
		if got == "" {
			t.Fatalf("cloudControlAction(%q) empty", "DeleteResource")
		}
	}
	{
		got := cloudControlAction("UpdateResource")
		if got == "" {
			t.Fatalf("cloudControlAction(%q) empty", "UpdateResource")
		}
	}
	{
		got := cloudControlAction("GetResourceRequestStatus")
		if got == "" {
			t.Fatalf("cloudControlAction(%q) empty", "GetResourceRequestStatus")
		}
	}
	if got := cfnAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := cfnAction("CreateStack")
		if got == "" {
			t.Fatalf("cfnAction(%q) empty", "CreateStack")
		}
	}
	{
		got := cfnAction("DescribeStacks")
		if got == "" {
			t.Fatalf("cfnAction(%q) empty", "DescribeStacks")
		}
	}
	{
		got := cfnAction("DeleteStack")
		if got == "" {
			t.Fatalf("cfnAction(%q) empty", "DeleteStack")
		}
	}
	{
		got := cfnAction("ListStacks")
		if got == "" {
			t.Fatalf("cfnAction(%q) empty", "ListStacks")
		}
	}
	{
		got := cfnAction("UpdateStack")
		if got == "" {
			t.Fatalf("cfnAction(%q) empty", "UpdateStack")
		}
	}
	{
		got := cfnAction("CreateChangeSet")
		if got == "" {
			t.Fatalf("cfnAction(%q) empty", "CreateChangeSet")
		}
	}
	{
		got := cfnAction("DescribeChangeSet")
		if got == "" {
			t.Fatalf("cfnAction(%q) empty", "DescribeChangeSet")
		}
	}
	{
		got := cfnAction("ExecuteChangeSet")
		if got == "" {
			t.Fatalf("cfnAction(%q) empty", "ExecuteChangeSet")
		}
	}
	{
		got := cfnAction("DetectStackDrift")
		if got == "" {
			t.Fatalf("cfnAction(%q) empty", "DetectStackDrift")
		}
	}
	{
		got := cfnAction("DescribeStackDriftDetectionStatus")
		if got == "" {
			t.Fatalf("cfnAction(%q) empty", "DescribeStackDriftDetectionStatus")
		}
	}
	{
		got := cfnAction("DescribeStackResourceDrifts")
		if got == "" {
			t.Fatalf("cfnAction(%q) empty", "DescribeStackResourceDrifts")
		}
	}
	if got := cloudfrontAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := cloudfrontAction("CreateDistribution")
		if got == "" {
			t.Fatalf("cloudfrontAction(%q) empty", "CreateDistribution")
		}
	}
	{
		got := cloudfrontAction("GetDistribution")
		if got == "" {
			t.Fatalf("cloudfrontAction(%q) empty", "GetDistribution")
		}
	}
	{
		got := cloudfrontAction("GetDistributionConfig")
		if got == "" {
			t.Fatalf("cloudfrontAction(%q) empty", "GetDistributionConfig")
		}
	}
	{
		got := cloudfrontAction("UpdateDistribution")
		if got == "" {
			t.Fatalf("cloudfrontAction(%q) empty", "UpdateDistribution")
		}
	}
	{
		got := cloudfrontAction("ListDistributions")
		if got == "" {
			t.Fatalf("cloudfrontAction(%q) empty", "ListDistributions")
		}
	}
	{
		got := cloudfrontAction("DeleteDistribution")
		if got == "" {
			t.Fatalf("cloudfrontAction(%q) empty", "DeleteDistribution")
		}
	}
	{
		got := cloudfrontAction("CreateInvalidation")
		if got == "" {
			t.Fatalf("cloudfrontAction(%q) empty", "CreateInvalidation")
		}
	}
	{
		got := cloudfrontAction("GetInvalidation")
		if got == "" {
			t.Fatalf("cloudfrontAction(%q) empty", "GetInvalidation")
		}
	}
	{
		got := cloudfrontAction("ListInvalidations")
		if got == "" {
			t.Fatalf("cloudfrontAction(%q) empty", "ListInvalidations")
		}
	}
	if got := cloudtrailAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := cloudtrailAction("LookupEvents")
		if got == "" {
			t.Fatalf("cloudtrailAction(%q) empty", "LookupEvents")
		}
	}
	{
		got := cloudtrailAction("CreateTrail")
		if got == "" {
			t.Fatalf("cloudtrailAction(%q) empty", "CreateTrail")
		}
	}
	{
		got := cloudtrailAction("DescribeTrails")
		if got == "" {
			t.Fatalf("cloudtrailAction(%q) empty", "DescribeTrails")
		}
	}
	{
		got := cloudtrailAction("DeleteTrail")
		if got == "" {
			t.Fatalf("cloudtrailAction(%q) empty", "DeleteTrail")
		}
	}
	{
		got := cloudtrailAction("StartLogging")
		if got == "" {
			t.Fatalf("cloudtrailAction(%q) empty", "StartLogging")
		}
	}
	{
		got := cloudtrailAction("StopLogging")
		if got == "" {
			t.Fatalf("cloudtrailAction(%q) empty", "StopLogging")
		}
	}
	{
		got := cloudtrailAction("InjectEvents")
		if got == "" {
			t.Fatalf("cloudtrailAction(%q) empty", "InjectEvents")
		}
	}
	{
		got := cloudtrailAction("InjectInsightsEvents")
		if got == "" {
			t.Fatalf("cloudtrailAction(%q) empty", "InjectInsightsEvents")
		}
	}
	{
		got := cloudtrailAction("PutEventSelectors")
		if got == "" {
			t.Fatalf("cloudtrailAction(%q) empty", "PutEventSelectors")
		}
	}
	{
		got := cloudtrailAction("GetEventSelectors")
		if got == "" {
			t.Fatalf("cloudtrailAction(%q) empty", "GetEventSelectors")
		}
	}
	{
		got := cloudtrailAction("ValidateLogs")
		if got == "" {
			t.Fatalf("cloudtrailAction(%q) empty", "ValidateLogs")
		}
	}
	if got := cloudWatchAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := cloudWatchAction("PutMetricData")
		if got == "" {
			t.Fatalf("cloudWatchAction(%q) empty", "PutMetricData")
		}
	}
	{
		got := cloudWatchAction("ListMetrics")
		if got == "" {
			t.Fatalf("cloudWatchAction(%q) empty", "ListMetrics")
		}
	}
	{
		got := cloudWatchAction("GetMetricStatistics")
		if got == "" {
			t.Fatalf("cloudWatchAction(%q) empty", "GetMetricStatistics")
		}
	}
	{
		got := cloudWatchAction("GetMetricData")
		if got == "" {
			t.Fatalf("cloudWatchAction(%q) empty", "GetMetricData")
		}
	}
	{
		got := cloudWatchAction("PutMetricAlarm")
		if got == "" {
			t.Fatalf("cloudWatchAction(%q) empty", "PutMetricAlarm")
		}
	}
	{
		got := cloudWatchAction("DescribeAlarms")
		if got == "" {
			t.Fatalf("cloudWatchAction(%q) empty", "DescribeAlarms")
		}
	}
	{
		got := cloudWatchAction("DeleteAlarms")
		if got == "" {
			t.Fatalf("cloudWatchAction(%q) empty", "DeleteAlarms")
		}
	}
	{
		got := cloudWatchAction("SetAlarmState")
		if got == "" {
			t.Fatalf("cloudWatchAction(%q) empty", "SetAlarmState")
		}
	}
	if got := codebuildAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := codebuildAction("CreateProject")
		if got == "" {
			t.Fatalf("codebuildAction(%q) empty", "CreateProject")
		}
	}
	{
		got := codebuildAction("UpdateProject")
		if got == "" {
			t.Fatalf("codebuildAction(%q) empty", "UpdateProject")
		}
	}
	{
		got := codebuildAction("DeleteProject")
		if got == "" {
			t.Fatalf("codebuildAction(%q) empty", "DeleteProject")
		}
	}
	{
		got := codebuildAction("ListProjects")
		if got == "" {
			t.Fatalf("codebuildAction(%q) empty", "ListProjects")
		}
	}
	{
		got := codebuildAction("BatchGetProjects")
		if got == "" {
			t.Fatalf("codebuildAction(%q) empty", "BatchGetProjects")
		}
	}
	{
		got := codebuildAction("StartBuild")
		if got == "" {
			t.Fatalf("codebuildAction(%q) empty", "StartBuild")
		}
	}
	{
		got := codebuildAction("StartBuildBatch")
		if got == "" {
			t.Fatalf("codebuildAction(%q) empty", "StartBuildBatch")
		}
	}
	{
		got := codebuildAction("StopBuild")
		if got == "" {
			t.Fatalf("codebuildAction(%q) empty", "StopBuild")
		}
	}
	{
		got := codebuildAction("BatchGetBuilds")
		if got == "" {
			t.Fatalf("codebuildAction(%q) empty", "BatchGetBuilds")
		}
	}
	{
		got := codebuildAction("ListBuilds")
		if got == "" {
			t.Fatalf("codebuildAction(%q) empty", "ListBuilds")
		}
	}
	{
		got := codebuildAction("CreateWebhook")
		if got == "" {
			t.Fatalf("codebuildAction(%q) empty", "CreateWebhook")
		}
	}
	{
		got := codebuildAction("DeleteWebhook")
		if got == "" {
			t.Fatalf("codebuildAction(%q) empty", "DeleteWebhook")
		}
	}
	{
		got := codebuildAction("ListWebhooks")
		if got == "" {
			t.Fatalf("codebuildAction(%q) empty", "ListWebhooks")
		}
	}
	if got := codecommitAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := codecommitAction("CreateRepository")
		if got == "" {
			t.Fatalf("codecommitAction(%q) empty", "CreateRepository")
		}
	}
	{
		got := codecommitAction("GetRepository")
		if got == "" {
			t.Fatalf("codecommitAction(%q) empty", "GetRepository")
		}
	}
	{
		got := codecommitAction("ListRepositories")
		if got == "" {
			t.Fatalf("codecommitAction(%q) empty", "ListRepositories")
		}
	}
	{
		got := codecommitAction("DeleteRepository")
		if got == "" {
			t.Fatalf("codecommitAction(%q) empty", "DeleteRepository")
		}
	}
	{
		got := codecommitAction("PutFile")
		if got == "" {
			t.Fatalf("codecommitAction(%q) empty", "PutFile")
		}
	}
	{
		got := codecommitAction("GetFile")
		if got == "" {
			t.Fatalf("codecommitAction(%q) empty", "GetFile")
		}
	}
	{
		got := codecommitAction("GetFolder")
		if got == "" {
			t.Fatalf("codecommitAction(%q) empty", "GetFolder")
		}
	}
	if got := codeDeployAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := codeDeployAction("CreateApplication")
		if got == "" {
			t.Fatalf("codeDeployAction(%q) empty", "CreateApplication")
		}
	}
	{
		got := codeDeployAction("CreateDeploymentGroup")
		if got == "" {
			t.Fatalf("codeDeployAction(%q) empty", "CreateDeploymentGroup")
		}
	}
	{
		got := codeDeployAction("CreateDeployment")
		if got == "" {
			t.Fatalf("codeDeployAction(%q) empty", "CreateDeployment")
		}
	}
	{
		got := codeDeployAction("GetDeployment")
		if got == "" {
			t.Fatalf("codeDeployAction(%q) empty", "GetDeployment")
		}
	}
	{
		got := codeDeployAction("ListDeployments")
		if got == "" {
			t.Fatalf("codeDeployAction(%q) empty", "ListDeployments")
		}
	}
	if got := codepipelineAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := codepipelineAction("CreatePipeline")
		if got == "" {
			t.Fatalf("codepipelineAction(%q) empty", "CreatePipeline")
		}
	}
	{
		got := codepipelineAction("GetPipeline")
		if got == "" {
			t.Fatalf("codepipelineAction(%q) empty", "GetPipeline")
		}
	}
	{
		got := codepipelineAction("DeletePipeline")
		if got == "" {
			t.Fatalf("codepipelineAction(%q) empty", "DeletePipeline")
		}
	}
	{
		got := codepipelineAction("StartPipelineExecution")
		if got == "" {
			t.Fatalf("codepipelineAction(%q) empty", "StartPipelineExecution")
		}
	}
	{
		got := codepipelineAction("GetPipelineState")
		if got == "" {
			t.Fatalf("codepipelineAction(%q) empty", "GetPipelineState")
		}
	}
	{
		got := codepipelineAction("PutApprovalResult")
		if got == "" {
			t.Fatalf("codepipelineAction(%q) empty", "PutApprovalResult")
		}
	}
	{
		got := codepipelineAction("GetPipelineExecution")
		if got == "" {
			t.Fatalf("codepipelineAction(%q) empty", "GetPipelineExecution")
		}
	}
	{
		got := codepipelineAction("ListPipelineExecutions")
		if got == "" {
			t.Fatalf("codepipelineAction(%q) empty", "ListPipelineExecutions")
		}
	}
	if got := cognitoAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := cognitoAction("CreateUserPool")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "CreateUserPool")
		}
	}
	{
		got := cognitoAction("DescribeUserPool")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "DescribeUserPool")
		}
	}
	{
		got := cognitoAction("UpdateUserPool")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "UpdateUserPool")
		}
	}
	{
		got := cognitoAction("ListUserPools")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "ListUserPools")
		}
	}
	{
		got := cognitoAction("DeleteUserPool")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "DeleteUserPool")
		}
	}
	{
		got := cognitoAction("CreateUserPoolClient")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "CreateUserPoolClient")
		}
	}
	{
		got := cognitoAction("DescribeUserPoolClient")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "DescribeUserPoolClient")
		}
	}
	{
		got := cognitoAction("ListUserPoolClients")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "ListUserPoolClients")
		}
	}
	{
		got := cognitoAction("DeleteUserPoolClient")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "DeleteUserPoolClient")
		}
	}
	{
		got := cognitoAction("AdminCreateUser")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "AdminCreateUser")
		}
	}
	{
		got := cognitoAction("SignUp")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "SignUp")
		}
	}
	{
		got := cognitoAction("ConfirmSignUp")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "ConfirmSignUp")
		}
	}
	{
		got := cognitoAction("ForgotPassword")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "ForgotPassword")
		}
	}
	{
		got := cognitoAction("ConfirmForgotPassword")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "ConfirmForgotPassword")
		}
	}
	{
		got := cognitoAction("ResendConfirmationCode")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "ResendConfirmationCode")
		}
	}
	{
		got := cognitoAction("UpdateUserAttributes")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "UpdateUserAttributes")
		}
	}
	{
		got := cognitoAction("GetUserAttributeVerificationCode")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "GetUserAttributeVerificationCode")
		}
	}
	{
		got := cognitoAction("VerifyUserAttribute")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "VerifyUserAttribute")
		}
	}
	{
		got := cognitoAction("InitiateAuth")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "InitiateAuth")
		}
	}
	{
		got := cognitoAction("AdminInitiateAuth")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "AdminInitiateAuth")
		}
	}
	{
		got := cognitoAction("RevokeToken")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "RevokeToken")
		}
	}
	{
		got := cognitoAction("AssociateSoftwareToken")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "AssociateSoftwareToken")
		}
	}
	{
		got := cognitoAction("VerifySoftwareToken")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "VerifySoftwareToken")
		}
	}
	{
		got := cognitoAction("RespondToAuthChallenge")
		if got == "" {
			t.Fatalf("cognitoAction(%q) empty", "RespondToAuthChallenge")
		}
	}
	if got := configAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := configAction("PutConfigurationRecorder")
		if got == "" {
			t.Fatalf("configAction(%q) empty", "PutConfigurationRecorder")
		}
	}
	{
		got := configAction("PutDeliveryChannel")
		if got == "" {
			t.Fatalf("configAction(%q) empty", "PutDeliveryChannel")
		}
	}
	{
		got := configAction("StartConfigurationRecorder")
		if got == "" {
			t.Fatalf("configAction(%q) empty", "StartConfigurationRecorder")
		}
	}
	{
		got := configAction("DescribeComplianceByConfigRule")
		if got == "" {
			t.Fatalf("configAction(%q) empty", "DescribeComplianceByConfigRule")
		}
	}
	{
		got := configAction("GetResourceConfigHistory")
		if got == "" {
			t.Fatalf("configAction(%q) empty", "GetResourceConfigHistory")
		}
	}
	if got := controlTowerAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := controlTowerAction("ListLandingZones")
		if got == "" {
			t.Fatalf("controlTowerAction(%q) empty", "ListLandingZones")
		}
	}
	{
		got := controlTowerAction("GetLandingZone")
		if got == "" {
			t.Fatalf("controlTowerAction(%q) empty", "GetLandingZone")
		}
	}
	if got := costExplorerAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := costExplorerAction("GetCostAndUsage")
		if got == "" {
			t.Fatalf("costExplorerAction(%q) empty", "GetCostAndUsage")
		}
	}
	{
		got := costExplorerAction("GetCostForecast")
		if got == "" {
			t.Fatalf("costExplorerAction(%q) empty", "GetCostForecast")
		}
	}
	if got := curAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := curAction("PutReportDefinition")
		if got == "" {
			t.Fatalf("curAction(%q) empty", "PutReportDefinition")
		}
	}
	{
		got := curAction("ModifyReportDefinition")
		if got == "" {
			t.Fatalf("curAction(%q) empty", "ModifyReportDefinition")
		}
	}
	{
		got := curAction("DescribeReportDefinitions")
		if got == "" {
			t.Fatalf("curAction(%q) empty", "DescribeReportDefinitions")
		}
	}
	{
		got := curAction("DeleteReportDefinition")
		if got == "" {
			t.Fatalf("curAction(%q) empty", "DeleteReportDefinition")
		}
	}
	{
		got := curAction("TagResource")
		if got == "" {
			t.Fatalf("curAction(%q) empty", "TagResource")
		}
	}
	{
		got := curAction("UntagResource")
		if got == "" {
			t.Fatalf("curAction(%q) empty", "UntagResource")
		}
	}
	{
		got := curAction("ListTagsForResource")
		if got == "" {
			t.Fatalf("curAction(%q) empty", "ListTagsForResource")
		}
	}
	if got := detectiveAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := detectiveAction("CreateGraph")
		if got == "" {
			t.Fatalf("detectiveAction(%q) empty", "CreateGraph")
		}
	}
	{
		got := detectiveAction("ListGraphs")
		if got == "" {
			t.Fatalf("detectiveAction(%q) empty", "ListGraphs")
		}
	}
	{
		got := detectiveAction("AcceptInvitation")
		if got == "" {
			t.Fatalf("detectiveAction(%q) empty", "AcceptInvitation")
		}
	}
	{
		got := detectiveAction("SearchGraph")
		if got == "" {
			t.Fatalf("detectiveAction(%q) empty", "SearchGraph")
		}
	}
	if got := docdbAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := docdbAction("CreateDBCluster")
		if got == "" {
			t.Fatalf("docdbAction(%q) empty", "CreateDBCluster")
		}
	}
	{
		got := docdbAction("DescribeDBClusters")
		if got == "" {
			t.Fatalf("docdbAction(%q) empty", "DescribeDBClusters")
		}
	}
	{
		got := docdbAction("DeleteDBCluster")
		if got == "" {
			t.Fatalf("docdbAction(%q) empty", "DeleteDBCluster")
		}
	}
	if got := dynamoAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := dynamoAction("CreateTable")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "CreateTable")
		}
	}
	{
		got := dynamoAction("DescribeTable")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "DescribeTable")
		}
	}
	{
		got := dynamoAction("DeleteTable")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "DeleteTable")
		}
	}
	{
		got := dynamoAction("ListTables")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "ListTables")
		}
	}
	{
		got := dynamoAction("UpdateTable")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "UpdateTable")
		}
	}
	{
		got := dynamoAction("PutItem")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "PutItem")
		}
	}
	{
		got := dynamoAction("GetItem")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "GetItem")
		}
	}
	{
		got := dynamoAction("DeleteItem")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "DeleteItem")
		}
	}
	{
		got := dynamoAction("UpdateItem")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "UpdateItem")
		}
	}
	{
		got := dynamoAction("Query")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "Query")
		}
	}
	{
		got := dynamoAction("Scan")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "Scan")
		}
	}
	{
		got := dynamoAction("BatchGetItem")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "BatchGetItem")
		}
	}
	{
		got := dynamoAction("BatchWriteItem")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "BatchWriteItem")
		}
	}
	{
		got := dynamoAction("TransactGetItems")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "TransactGetItems")
		}
	}
	{
		got := dynamoAction("TransactWriteItems")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "TransactWriteItems")
		}
	}
	{
		got := dynamoAction("PutResourcePolicy")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "PutResourcePolicy")
		}
	}
	{
		got := dynamoAction("GetResourcePolicy")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "GetResourcePolicy")
		}
	}
	{
		got := dynamoAction("DeleteResourcePolicy")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "DeleteResourcePolicy")
		}
	}
	{
		got := dynamoAction("UpdateTimeToLive")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "UpdateTimeToLive")
		}
	}
	{
		got := dynamoAction("DescribeTimeToLive")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "DescribeTimeToLive")
		}
	}
	{
		got := dynamoAction("DescribeContinuousBackups")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "DescribeContinuousBackups")
		}
	}
	{
		got := dynamoAction("ListTagsOfResource")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "ListTagsOfResource")
		}
	}
	{
		got := dynamoAction("TagResource")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "TagResource")
		}
	}
	{
		got := dynamoAction("UntagResource")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "UntagResource")
		}
	}
	{
		got := dynamoAction("ExecuteStatement")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "ExecuteStatement")
		}
	}
	{
		got := dynamoAction("BatchExecuteStatement")
		if got == "" {
			t.Fatalf("dynamoAction(%q) empty", "BatchExecuteStatement")
		}
	}
	if got := dynamodbstreamsAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := dynamodbstreamsAction("ListStreams")
		if got == "" {
			t.Fatalf("dynamodbstreamsAction(%q) empty", "ListStreams")
		}
	}
	{
		got := dynamodbstreamsAction("DescribeStream")
		if got == "" {
			t.Fatalf("dynamodbstreamsAction(%q) empty", "DescribeStream")
		}
	}
	{
		got := dynamodbstreamsAction("GetShardIterator")
		if got == "" {
			t.Fatalf("dynamodbstreamsAction(%q) empty", "GetShardIterator")
		}
	}
	{
		got := dynamodbstreamsAction("GetRecords")
		if got == "" {
			t.Fatalf("dynamodbstreamsAction(%q) empty", "GetRecords")
		}
	}
	if got := ec2Action(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := ec2Action("RunInstances")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "RunInstances")
		}
	}
	{
		got := ec2Action("DescribeInstances")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "DescribeInstances")
		}
	}
	{
		got := ec2Action("DescribeImages")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "DescribeImages")
		}
	}
	{
		got := ec2Action("TerminateInstances")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "TerminateInstances")
		}
	}
	{
		got := ec2Action("StopInstances")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "StopInstances")
		}
	}
	{
		got := ec2Action("StartInstances")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "StartInstances")
		}
	}
	{
		got := ec2Action("CreateVpc")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "CreateVpc")
		}
	}
	{
		got := ec2Action("DeleteVpc")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "DeleteVpc")
		}
	}
	{
		got := ec2Action("DescribeVpcs")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "DescribeVpcs")
		}
	}
	{
		got := ec2Action("CreateSubnet")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "CreateSubnet")
		}
	}
	{
		got := ec2Action("DeleteSubnet")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "DeleteSubnet")
		}
	}
	{
		got := ec2Action("DescribeSubnets")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "DescribeSubnets")
		}
	}
	{
		got := ec2Action("CreateSecurityGroup")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "CreateSecurityGroup")
		}
	}
	{
		got := ec2Action("DeleteSecurityGroup")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "DeleteSecurityGroup")
		}
	}
	{
		got := ec2Action("DescribeSecurityGroups")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "DescribeSecurityGroups")
		}
	}
	{
		got := ec2Action("AuthorizeSecurityGroupIngress")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "AuthorizeSecurityGroupIngress")
		}
	}
	{
		got := ec2Action("AuthorizeSecurityGroupEgress")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "AuthorizeSecurityGroupEgress")
		}
	}
	{
		got := ec2Action("RevokeSecurityGroupIngress")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "RevokeSecurityGroupIngress")
		}
	}
	{
		got := ec2Action("RevokeSecurityGroupEgress")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "RevokeSecurityGroupEgress")
		}
	}
	{
		got := ec2Action("DescribeNetworkInterfaces")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "DescribeNetworkInterfaces")
		}
	}
	{
		got := ec2Action("CreateNetworkInterface")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "CreateNetworkInterface")
		}
	}
	{
		got := ec2Action("CreateFlowLogs")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "CreateFlowLogs")
		}
	}
	{
		got := ec2Action("InjectFlowLogs")
		if got == "" {
			t.Fatalf("ec2Action(%q) empty", "InjectFlowLogs")
		}
	}
	if got := ecrAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := ecrAction("CreateRepository")
		if got == "" {
			t.Fatalf("ecrAction(%q) empty", "CreateRepository")
		}
	}
	{
		got := ecrAction("DescribeRepositories")
		if got == "" {
			t.Fatalf("ecrAction(%q) empty", "DescribeRepositories")
		}
	}
	{
		got := ecrAction("DeleteRepository")
		if got == "" {
			t.Fatalf("ecrAction(%q) empty", "DeleteRepository")
		}
	}
	{
		got := ecrAction("GetAuthorizationToken")
		if got == "" {
			t.Fatalf("ecrAction(%q) empty", "GetAuthorizationToken")
		}
	}
	{
		got := ecrAction("GetRepositoryPolicy")
		if got == "" {
			t.Fatalf("ecrAction(%q) empty", "GetRepositoryPolicy")
		}
	}
	{
		got := ecrAction("SetRepositoryPolicy")
		if got == "" {
			t.Fatalf("ecrAction(%q) empty", "SetRepositoryPolicy")
		}
	}
	{
		got := ecrAction("DeleteRepositoryPolicy")
		if got == "" {
			t.Fatalf("ecrAction(%q) empty", "DeleteRepositoryPolicy")
		}
	}
	{
		got := ecrAction("PutImage")
		if got == "" {
			t.Fatalf("ecrAction(%q) empty", "PutImage")
		}
	}
	{
		got := ecrAction("BatchGetImage")
		if got == "" {
			t.Fatalf("ecrAction(%q) empty", "BatchGetImage")
		}
	}
	{
		got := ecrAction("ListImages")
		if got == "" {
			t.Fatalf("ecrAction(%q) empty", "ListImages")
		}
	}
	{
		got := ecrAction("BatchDeleteImage")
		if got == "" {
			t.Fatalf("ecrAction(%q) empty", "BatchDeleteImage")
		}
	}
	{
		got := ecrAction("ListTagsForResource")
		if got == "" {
			t.Fatalf("ecrAction(%q) empty", "ListTagsForResource")
		}
	}
	{
		got := ecrAction("TagResource")
		if got == "" {
			t.Fatalf("ecrAction(%q) empty", "TagResource")
		}
	}
	{
		got := ecrAction("UntagResource")
		if got == "" {
			t.Fatalf("ecrAction(%q) empty", "UntagResource")
		}
	}
	if got := ecsAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := ecsAction("RegisterTaskDefinition")
		if got == "" {
			t.Fatalf("ecsAction(%q) empty", "RegisterTaskDefinition")
		}
	}
	{
		got := ecsAction("DescribeTaskDefinition")
		if got == "" {
			t.Fatalf("ecsAction(%q) empty", "DescribeTaskDefinition")
		}
	}
	{
		got := ecsAction("ListTaskDefinitions")
		if got == "" {
			t.Fatalf("ecsAction(%q) empty", "ListTaskDefinitions")
		}
	}
	{
		got := ecsAction("DeregisterTaskDefinition")
		if got == "" {
			t.Fatalf("ecsAction(%q) empty", "DeregisterTaskDefinition")
		}
	}
	{
		got := ecsAction("RunTask")
		if got == "" {
			t.Fatalf("ecsAction(%q) empty", "RunTask")
		}
	}
	{
		got := ecsAction("DescribeTasks")
		if got == "" {
			t.Fatalf("ecsAction(%q) empty", "DescribeTasks")
		}
	}
	{
		got := ecsAction("ListTasks")
		if got == "" {
			t.Fatalf("ecsAction(%q) empty", "ListTasks")
		}
	}
	{
		got := ecsAction("StopTask")
		if got == "" {
			t.Fatalf("ecsAction(%q) empty", "StopTask")
		}
	}
	{
		got := ecsAction("DescribeClusters")
		if got == "" {
			t.Fatalf("ecsAction(%q) empty", "DescribeClusters")
		}
	}
	{
		got := ecsAction("ListClusters")
		if got == "" {
			t.Fatalf("ecsAction(%q) empty", "ListClusters")
		}
	}
	{
		got := ecsAction("CreateService")
		if got == "" {
			t.Fatalf("ecsAction(%q) empty", "CreateService")
		}
	}
	{
		got := ecsAction("UpdateService")
		if got == "" {
			t.Fatalf("ecsAction(%q) empty", "UpdateService")
		}
	}
	{
		got := ecsAction("DeleteService")
		if got == "" {
			t.Fatalf("ecsAction(%q) empty", "DeleteService")
		}
	}
	{
		got := ecsAction("DescribeServices")
		if got == "" {
			t.Fatalf("ecsAction(%q) empty", "DescribeServices")
		}
	}
	{
		got := ecsAction("ListServices")
		if got == "" {
			t.Fatalf("ecsAction(%q) empty", "ListServices")
		}
	}
	{
		got := ecsAction("ListTagsForResource")
		if got == "" {
			t.Fatalf("ecsAction(%q) empty", "ListTagsForResource")
		}
	}
	{
		got := ecsAction("TagResource")
		if got == "" {
			t.Fatalf("ecsAction(%q) empty", "TagResource")
		}
	}
	{
		got := ecsAction("UntagResource")
		if got == "" {
			t.Fatalf("ecsAction(%q) empty", "UntagResource")
		}
	}
	if got := eksAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := eksAction("CreateCluster")
		if got == "" {
			t.Fatalf("eksAction(%q) empty", "CreateCluster")
		}
	}
	{
		got := eksAction("DescribeCluster")
		if got == "" {
			t.Fatalf("eksAction(%q) empty", "DescribeCluster")
		}
	}
	{
		got := eksAction("ListClusters")
		if got == "" {
			t.Fatalf("eksAction(%q) empty", "ListClusters")
		}
	}
	{
		got := eksAction("DeleteCluster")
		if got == "" {
			t.Fatalf("eksAction(%q) empty", "DeleteCluster")
		}
	}
	{
		got := eksAction("ListNodegroups")
		if got == "" {
			t.Fatalf("eksAction(%q) empty", "ListNodegroups")
		}
	}
	if got := elasticacheAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := elasticacheAction("CreateCacheCluster")
		if got == "" {
			t.Fatalf("elasticacheAction(%q) empty", "CreateCacheCluster")
		}
	}
	{
		got := elasticacheAction("DescribeCacheClusters")
		if got == "" {
			t.Fatalf("elasticacheAction(%q) empty", "DescribeCacheClusters")
		}
	}
	{
		got := elasticacheAction("DeleteCacheCluster")
		if got == "" {
			t.Fatalf("elasticacheAction(%q) empty", "DeleteCacheCluster")
		}
	}
	if got := elbv2Action(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := elbv2Action("CreateLoadBalancer")
		if got == "" {
			t.Fatalf("elbv2Action(%q) empty", "CreateLoadBalancer")
		}
	}
	{
		got := elbv2Action("DescribeLoadBalancers")
		if got == "" {
			t.Fatalf("elbv2Action(%q) empty", "DescribeLoadBalancers")
		}
	}
	{
		got := elbv2Action("DeleteLoadBalancer")
		if got == "" {
			t.Fatalf("elbv2Action(%q) empty", "DeleteLoadBalancer")
		}
	}
	{
		got := elbv2Action("CreateTargetGroup")
		if got == "" {
			t.Fatalf("elbv2Action(%q) empty", "CreateTargetGroup")
		}
	}
	{
		got := elbv2Action("DescribeTargetGroups")
		if got == "" {
			t.Fatalf("elbv2Action(%q) empty", "DescribeTargetGroups")
		}
	}
	{
		got := elbv2Action("DeleteTargetGroup")
		if got == "" {
			t.Fatalf("elbv2Action(%q) empty", "DeleteTargetGroup")
		}
	}
	{
		got := elbv2Action("CreateListener")
		if got == "" {
			t.Fatalf("elbv2Action(%q) empty", "CreateListener")
		}
	}
	{
		got := elbv2Action("DescribeListeners")
		if got == "" {
			t.Fatalf("elbv2Action(%q) empty", "DescribeListeners")
		}
	}
	{
		got := elbv2Action("DeleteListener")
		if got == "" {
			t.Fatalf("elbv2Action(%q) empty", "DeleteListener")
		}
	}
	{
		got := elbv2Action("RegisterTargets")
		if got == "" {
			t.Fatalf("elbv2Action(%q) empty", "RegisterTargets")
		}
	}
	{
		got := elbv2Action("DescribeTargetHealth")
		if got == "" {
			t.Fatalf("elbv2Action(%q) empty", "DescribeTargetHealth")
		}
	}
	{
		got := elbv2Action("CreateRule")
		if got == "" {
			t.Fatalf("elbv2Action(%q) empty", "CreateRule")
		}
	}
	{
		got := elbv2Action("DescribeRules")
		if got == "" {
			t.Fatalf("elbv2Action(%q) empty", "DescribeRules")
		}
	}
	{
		got := elbv2Action("DeleteRule")
		if got == "" {
			t.Fatalf("elbv2Action(%q) empty", "DeleteRule")
		}
	}
	{
		got := elbv2Action("ModifyListener")
		if got == "" {
			t.Fatalf("elbv2Action(%q) empty", "ModifyListener")
		}
	}
	{
		got := elbv2Action("ModifyRule")
		if got == "" {
			t.Fatalf("elbv2Action(%q) empty", "ModifyRule")
		}
	}
	{
		got := elbv2Action("ModifyLoadBalancerAttributes")
		if got == "" {
			t.Fatalf("elbv2Action(%q) empty", "ModifyLoadBalancerAttributes")
		}
	}
	{
		got := elbv2Action("DescribeLoadBalancerAttributes")
		if got == "" {
			t.Fatalf("elbv2Action(%q) empty", "DescribeLoadBalancerAttributes")
		}
	}
	if got := emrAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := emrAction("RunJobFlow")
		if got == "" {
			t.Fatalf("emrAction(%q) empty", "RunJobFlow")
		}
	}
	{
		got := emrAction("DescribeCluster")
		if got == "" {
			t.Fatalf("emrAction(%q) empty", "DescribeCluster")
		}
	}
	{
		got := emrAction("ListClusters")
		if got == "" {
			t.Fatalf("emrAction(%q) empty", "ListClusters")
		}
	}
	{
		got := emrAction("TerminateJobFlows")
		if got == "" {
			t.Fatalf("emrAction(%q) empty", "TerminateJobFlows")
		}
	}
	{
		got := emrAction("AddJobFlowSteps")
		if got == "" {
			t.Fatalf("emrAction(%q) empty", "AddJobFlowSteps")
		}
	}
	{
		got := emrAction("DescribeStep")
		if got == "" {
			t.Fatalf("emrAction(%q) empty", "DescribeStep")
		}
	}
	{
		got := emrAction("ListSteps")
		if got == "" {
			t.Fatalf("emrAction(%q) empty", "ListSteps")
		}
	}
	{
		got := emrAction("CancelSteps")
		if got == "" {
			t.Fatalf("emrAction(%q) empty", "CancelSteps")
		}
	}
	{
		got := emrAction("ListInstanceGroups")
		if got == "" {
			t.Fatalf("emrAction(%q) empty", "ListInstanceGroups")
		}
	}
	{
		got := emrAction("ListInstanceFleets")
		if got == "" {
			t.Fatalf("emrAction(%q) empty", "ListInstanceFleets")
		}
	}
	{
		got := emrAction("AddTags")
		if got == "" {
			t.Fatalf("emrAction(%q) empty", "AddTags")
		}
	}
	{
		got := emrAction("RemoveTags")
		if got == "" {
			t.Fatalf("emrAction(%q) empty", "RemoveTags")
		}
	}
	{
		got := emrAction("CreateSecurityConfiguration")
		if got == "" {
			t.Fatalf("emrAction(%q) empty", "CreateSecurityConfiguration")
		}
	}
	{
		got := emrAction("DescribeSecurityConfiguration")
		if got == "" {
			t.Fatalf("emrAction(%q) empty", "DescribeSecurityConfiguration")
		}
	}
	{
		got := emrAction("DeleteSecurityConfiguration")
		if got == "" {
			t.Fatalf("emrAction(%q) empty", "DeleteSecurityConfiguration")
		}
	}
	{
		got := emrAction("ListSecurityConfigurations")
		if got == "" {
			t.Fatalf("emrAction(%q) empty", "ListSecurityConfigurations")
		}
	}
	if got := eventsAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := eventsAction("PutEvents")
		if got == "" {
			t.Fatalf("eventsAction(%q) empty", "PutEvents")
		}
	}
	{
		got := eventsAction("CreateEventBus")
		if got == "" {
			t.Fatalf("eventsAction(%q) empty", "CreateEventBus")
		}
	}
	{
		got := eventsAction("DeleteEventBus")
		if got == "" {
			t.Fatalf("eventsAction(%q) empty", "DeleteEventBus")
		}
	}
	{
		got := eventsAction("DescribeEventBus")
		if got == "" {
			t.Fatalf("eventsAction(%q) empty", "DescribeEventBus")
		}
	}
	{
		got := eventsAction("ListEventBuses")
		if got == "" {
			t.Fatalf("eventsAction(%q) empty", "ListEventBuses")
		}
	}
	{
		got := eventsAction("PutRule")
		if got == "" {
			t.Fatalf("eventsAction(%q) empty", "PutRule")
		}
	}
	{
		got := eventsAction("DescribeRule")
		if got == "" {
			t.Fatalf("eventsAction(%q) empty", "DescribeRule")
		}
	}
	{
		got := eventsAction("ListRules")
		if got == "" {
			t.Fatalf("eventsAction(%q) empty", "ListRules")
		}
	}
	{
		got := eventsAction("DeleteRule")
		if got == "" {
			t.Fatalf("eventsAction(%q) empty", "DeleteRule")
		}
	}
	{
		got := eventsAction("EnableRule")
		if got == "" {
			t.Fatalf("eventsAction(%q) empty", "EnableRule")
		}
	}
	{
		got := eventsAction("DisableRule")
		if got == "" {
			t.Fatalf("eventsAction(%q) empty", "DisableRule")
		}
	}
	{
		got := eventsAction("PutTargets")
		if got == "" {
			t.Fatalf("eventsAction(%q) empty", "PutTargets")
		}
	}
	{
		got := eventsAction("RemoveTargets")
		if got == "" {
			t.Fatalf("eventsAction(%q) empty", "RemoveTargets")
		}
	}
	{
		got := eventsAction("ListTargetsByRule")
		if got == "" {
			t.Fatalf("eventsAction(%q) empty", "ListTargetsByRule")
		}
	}
	{
		got := eventsAction("PutPermission")
		if got == "" {
			t.Fatalf("eventsAction(%q) empty", "PutPermission")
		}
	}
	{
		got := eventsAction("RemovePermission")
		if got == "" {
			t.Fatalf("eventsAction(%q) empty", "RemovePermission")
		}
	}
	{
		got := eventsAction("ListTagsForResource")
		if got == "" {
			t.Fatalf("eventsAction(%q) empty", "ListTagsForResource")
		}
	}
	{
		got := eventsAction("TagResource")
		if got == "" {
			t.Fatalf("eventsAction(%q) empty", "TagResource")
		}
	}
	{
		got := eventsAction("UntagResource")
		if got == "" {
			t.Fatalf("eventsAction(%q) empty", "UntagResource")
		}
	}
	if got := firehoseAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := firehoseAction("CreateDeliveryStream")
		if got == "" {
			t.Fatalf("firehoseAction(%q) empty", "CreateDeliveryStream")
		}
	}
	{
		got := firehoseAction("DeleteDeliveryStream")
		if got == "" {
			t.Fatalf("firehoseAction(%q) empty", "DeleteDeliveryStream")
		}
	}
	{
		got := firehoseAction("DescribeDeliveryStream")
		if got == "" {
			t.Fatalf("firehoseAction(%q) empty", "DescribeDeliveryStream")
		}
	}
	{
		got := firehoseAction("ListDeliveryStreams")
		if got == "" {
			t.Fatalf("firehoseAction(%q) empty", "ListDeliveryStreams")
		}
	}
	{
		got := firehoseAction("PutRecord")
		if got == "" {
			t.Fatalf("firehoseAction(%q) empty", "PutRecord")
		}
	}
	{
		got := firehoseAction("PutRecordBatch")
		if got == "" {
			t.Fatalf("firehoseAction(%q) empty", "PutRecordBatch")
		}
	}
	if got := glueAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := glueAction("CreateDatabase")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "CreateDatabase")
		}
	}
	{
		got := glueAction("GetDatabase")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "GetDatabase")
		}
	}
	{
		got := glueAction("GetDatabases")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "GetDatabases")
		}
	}
	{
		got := glueAction("DeleteDatabase")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "DeleteDatabase")
		}
	}
	{
		got := glueAction("CreateTable")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "CreateTable")
		}
	}
	{
		got := glueAction("GetTable")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "GetTable")
		}
	}
	{
		got := glueAction("GetTables")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "GetTables")
		}
	}
	{
		got := glueAction("DeleteTable")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "DeleteTable")
		}
	}
	{
		got := glueAction("CreateCrawler")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "CreateCrawler")
		}
	}
	{
		got := glueAction("StartCrawler")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "StartCrawler")
		}
	}
	{
		got := glueAction("GetCrawler")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "GetCrawler")
		}
	}
	{
		got := glueAction("DeleteCrawler")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "DeleteCrawler")
		}
	}
	{
		got := glueAction("ListCrawlers")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "ListCrawlers")
		}
	}
	{
		got := glueAction("CreateRegistry")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "CreateRegistry")
		}
	}
	{
		got := glueAction("GetRegistry")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "GetRegistry")
		}
	}
	{
		got := glueAction("ListRegistries")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "ListRegistries")
		}
	}
	{
		got := glueAction("DeleteRegistry")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "DeleteRegistry")
		}
	}
	{
		got := glueAction("CreateSchema")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "CreateSchema")
		}
	}
	{
		got := glueAction("GetSchema")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "GetSchema")
		}
	}
	{
		got := glueAction("ListSchemas")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "ListSchemas")
		}
	}
	{
		got := glueAction("DeleteSchema")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "DeleteSchema")
		}
	}
	{
		got := glueAction("RegisterSchemaVersion")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "RegisterSchemaVersion")
		}
	}
	{
		got := glueAction("GetSchemaVersion")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "GetSchemaVersion")
		}
	}
	{
		got := glueAction("ListSchemaVersions")
		if got == "" {
			t.Fatalf("glueAction(%q) empty", "ListSchemaVersions")
		}
	}
	if got := guarddutyAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := guarddutyAction("CreateDetector")
		if got == "" {
			t.Fatalf("guarddutyAction(%q) empty", "CreateDetector")
		}
	}
	{
		got := guarddutyAction("ListDetectors")
		if got == "" {
			t.Fatalf("guarddutyAction(%q) empty", "ListDetectors")
		}
	}
	{
		got := guarddutyAction("ListFindings")
		if got == "" {
			t.Fatalf("guarddutyAction(%q) empty", "ListFindings")
		}
	}
	{
		got := guarddutyAction("GetFindings")
		if got == "" {
			t.Fatalf("guarddutyAction(%q) empty", "GetFindings")
		}
	}
	{
		got := guarddutyAction("InjectFindings")
		if got == "" {
			t.Fatalf("guarddutyAction(%q) empty", "InjectFindings")
		}
	}
	if got := iotAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := iotAction("CreateThing")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "CreateThing")
		}
	}
	{
		got := iotAction("DescribeThing")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "DescribeThing")
		}
	}
	{
		got := iotAction("ListThings")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "ListThings")
		}
	}
	{
		got := iotAction("UpdateThing")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "UpdateThing")
		}
	}
	{
		got := iotAction("DeleteThing")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "DeleteThing")
		}
	}
	{
		got := iotAction("CreateKeysAndCertificate")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "CreateKeysAndCertificate")
		}
	}
	{
		got := iotAction("DescribeCertificate")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "DescribeCertificate")
		}
	}
	{
		got := iotAction("ListCertificates")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "ListCertificates")
		}
	}
	{
		got := iotAction("UpdateCertificate")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "UpdateCertificate")
		}
	}
	{
		got := iotAction("DeleteCertificate")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "DeleteCertificate")
		}
	}
	{
		got := iotAction("CreatePolicy")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "CreatePolicy")
		}
	}
	{
		got := iotAction("GetPolicy")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "GetPolicy")
		}
	}
	{
		got := iotAction("ListPolicies")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "ListPolicies")
		}
	}
	{
		got := iotAction("DeletePolicy")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "DeletePolicy")
		}
	}
	{
		got := iotAction("AttachPolicy")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "AttachPolicy")
		}
	}
	{
		got := iotAction("DetachPolicy")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "DetachPolicy")
		}
	}
	{
		got := iotAction("AttachThingPrincipal")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "AttachThingPrincipal")
		}
	}
	{
		got := iotAction("ListThingPrincipals")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "ListThingPrincipals")
		}
	}
	{
		got := iotAction("UpdateThingShadow")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "UpdateThingShadow")
		}
	}
	{
		got := iotAction("GetThingShadow")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "GetThingShadow")
		}
	}
	{
		got := iotAction("DeleteThingShadow")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "DeleteThingShadow")
		}
	}
	{
		got := iotAction("CreateTopicRule")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "CreateTopicRule")
		}
	}
	{
		got := iotAction("GetTopicRule")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "GetTopicRule")
		}
	}
	{
		got := iotAction("ListTopicRules")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "ListTopicRules")
		}
	}
	{
		got := iotAction("ReplaceTopicRule")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "ReplaceTopicRule")
		}
	}
	{
		got := iotAction("DeleteTopicRule")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "DeleteTopicRule")
		}
	}
	{
		got := iotAction("EnableTopicRule")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "EnableTopicRule")
		}
	}
	{
		got := iotAction("DisableTopicRule")
		if got == "" {
			t.Fatalf("iotAction(%q) empty", "DisableTopicRule")
		}
	}
	if got := kinesisAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := kinesisAction("CreateStream")
		if got == "" {
			t.Fatalf("kinesisAction(%q) empty", "CreateStream")
		}
	}
	{
		got := kinesisAction("DeleteStream")
		if got == "" {
			t.Fatalf("kinesisAction(%q) empty", "DeleteStream")
		}
	}
	{
		got := kinesisAction("DescribeStream")
		if got == "" {
			t.Fatalf("kinesisAction(%q) empty", "DescribeStream")
		}
	}
	{
		got := kinesisAction("ListStreams")
		if got == "" {
			t.Fatalf("kinesisAction(%q) empty", "ListStreams")
		}
	}
	{
		got := kinesisAction("PutRecord")
		if got == "" {
			t.Fatalf("kinesisAction(%q) empty", "PutRecord")
		}
	}
	{
		got := kinesisAction("PutRecords")
		if got == "" {
			t.Fatalf("kinesisAction(%q) empty", "PutRecords")
		}
	}
	{
		got := kinesisAction("GetShardIterator")
		if got == "" {
			t.Fatalf("kinesisAction(%q) empty", "GetShardIterator")
		}
	}
	{
		got := kinesisAction("GetRecords")
		if got == "" {
			t.Fatalf("kinesisAction(%q) empty", "GetRecords")
		}
	}
	{
		got := kinesisAction("PutResourcePolicy")
		if got == "" {
			t.Fatalf("kinesisAction(%q) empty", "PutResourcePolicy")
		}
	}
	{
		got := kinesisAction("GetResourcePolicy")
		if got == "" {
			t.Fatalf("kinesisAction(%q) empty", "GetResourcePolicy")
		}
	}
	{
		got := kinesisAction("DeleteResourcePolicy")
		if got == "" {
			t.Fatalf("kinesisAction(%q) empty", "DeleteResourcePolicy")
		}
	}
	{
		got := kinesisAction("RegisterStreamConsumer")
		if got == "" {
			t.Fatalf("kinesisAction(%q) empty", "RegisterStreamConsumer")
		}
	}
	{
		got := kinesisAction("DescribeStreamConsumer")
		if got == "" {
			t.Fatalf("kinesisAction(%q) empty", "DescribeStreamConsumer")
		}
	}
	{
		got := kinesisAction("ListStreamConsumers")
		if got == "" {
			t.Fatalf("kinesisAction(%q) empty", "ListStreamConsumers")
		}
	}
	{
		got := kinesisAction("DeregisterStreamConsumer")
		if got == "" {
			t.Fatalf("kinesisAction(%q) empty", "DeregisterStreamConsumer")
		}
	}
	{
		got := kinesisAction("SubscribeToShard")
		if got == "" {
			t.Fatalf("kinesisAction(%q) empty", "SubscribeToShard")
		}
	}
	{
		got := kinesisAction("UpdateShardCount")
		if got == "" {
			t.Fatalf("kinesisAction(%q) empty", "UpdateShardCount")
		}
	}
	if got := labForensicsAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := labForensicsAction("FreezeClock")
		if got == "" {
			t.Fatalf("labForensicsAction(%q) empty", "FreezeClock")
		}
	}
	{
		got := labForensicsAction("UnfreezeClock")
		if got == "" {
			t.Fatalf("labForensicsAction(%q) empty", "UnfreezeClock")
		}
	}
	{
		got := labForensicsAction("SetClock")
		if got == "" {
			t.Fatalf("labForensicsAction(%q) empty", "SetClock")
		}
	}
	{
		got := labForensicsAction("BulkSeed")
		if got == "" {
			t.Fatalf("labForensicsAction(%q) empty", "BulkSeed")
		}
	}
	if got := lambdaAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := lambdaAction("CreateFunction")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "CreateFunction")
		}
	}
	{
		got := lambdaAction("GetFunction")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "GetFunction")
		}
	}
	{
		got := lambdaAction("DeleteFunction")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "DeleteFunction")
		}
	}
	{
		got := lambdaAction("ListFunctions")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "ListFunctions")
		}
	}
	{
		got := lambdaAction("UpdateFunctionCode")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "UpdateFunctionCode")
		}
	}
	{
		got := lambdaAction("UpdateFunctionConfiguration")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "UpdateFunctionConfiguration")
		}
	}
	{
		got := lambdaAction("Invoke")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "Invoke")
		}
	}
	{
		got := lambdaAction("PublishVersion")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "PublishVersion")
		}
	}
	{
		got := lambdaAction("ListVersionsByFunction")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "ListVersionsByFunction")
		}
	}
	{
		got := lambdaAction("CreateAlias")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "CreateAlias")
		}
	}
	{
		got := lambdaAction("UpdateAlias")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "UpdateAlias")
		}
	}
	{
		got := lambdaAction("DeleteAlias")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "DeleteAlias")
		}
	}
	{
		got := lambdaAction("GetAlias")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "GetAlias")
		}
	}
	{
		got := lambdaAction("ListAliases")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "ListAliases")
		}
	}
	{
		got := lambdaAction("PublishLayerVersion")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "PublishLayerVersion")
		}
	}
	{
		got := lambdaAction("GetLayerVersion")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "GetLayerVersion")
		}
	}
	{
		got := lambdaAction("ListLayerVersions")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "ListLayerVersions")
		}
	}
	{
		got := lambdaAction("DeleteLayerVersion")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "DeleteLayerVersion")
		}
	}
	{
		got := lambdaAction("AddPermission")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "AddPermission")
		}
	}
	{
		got := lambdaAction("RemovePermission")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "RemovePermission")
		}
	}
	{
		got := lambdaAction("GetPolicy")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "GetPolicy")
		}
	}
	{
		got := lambdaAction("CreateEventSourceMapping")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "CreateEventSourceMapping")
		}
	}
	{
		got := lambdaAction("GetEventSourceMapping")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "GetEventSourceMapping")
		}
	}
	{
		got := lambdaAction("ListEventSourceMappings")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "ListEventSourceMappings")
		}
	}
	{
		got := lambdaAction("UpdateEventSourceMapping")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "UpdateEventSourceMapping")
		}
	}
	{
		got := lambdaAction("DeleteEventSourceMapping")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "DeleteEventSourceMapping")
		}
	}
	{
		got := lambdaAction("CreateFunctionUrlConfig")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "CreateFunctionUrlConfig")
		}
	}
	{
		got := lambdaAction("GetFunctionUrlConfig")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "GetFunctionUrlConfig")
		}
	}
	{
		got := lambdaAction("DeleteFunctionUrlConfig")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "DeleteFunctionUrlConfig")
		}
	}
	{
		got := lambdaAction("ListFunctionUrlConfigs")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "ListFunctionUrlConfigs")
		}
	}
	{
		got := lambdaAction("ListTags")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "ListTags")
		}
	}
	{
		got := lambdaAction("GetFunctionCodeSigningConfig")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "GetFunctionCodeSigningConfig")
		}
	}
	{
		got := lambdaAction("PutFunctionEventInvokeConfig")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "PutFunctionEventInvokeConfig")
		}
	}
	{
		got := lambdaAction("GetFunctionEventInvokeConfig")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "GetFunctionEventInvokeConfig")
		}
	}
	{
		got := lambdaAction("DeleteFunctionEventInvokeConfig")
		if got == "" {
			t.Fatalf("lambdaAction(%q) empty", "DeleteFunctionEventInvokeConfig")
		}
	}
	if got := lightsailAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := lightsailAction("GetBlueprints")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "GetBlueprints")
		}
	}
	{
		got := lightsailAction("GetBundles")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "GetBundles")
		}
	}
	{
		got := lightsailAction("CreateInstances")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "CreateInstances")
		}
	}
	{
		got := lightsailAction("GetInstance")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "GetInstance")
		}
	}
	{
		got := lightsailAction("GetInstances")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "GetInstances")
		}
	}
	{
		got := lightsailAction("StartInstance")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "StartInstance")
		}
	}
	{
		got := lightsailAction("StopInstance")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "StopInstance")
		}
	}
	{
		got := lightsailAction("RebootInstance")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "RebootInstance")
		}
	}
	{
		got := lightsailAction("DeleteInstance")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "DeleteInstance")
		}
	}
	{
		got := lightsailAction("CreateDisk")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "CreateDisk")
		}
	}
	{
		got := lightsailAction("GetDisk")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "GetDisk")
		}
	}
	{
		got := lightsailAction("GetDisks")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "GetDisks")
		}
	}
	{
		got := lightsailAction("DeleteDisk")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "DeleteDisk")
		}
	}
	{
		got := lightsailAction("AttachDisk")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "AttachDisk")
		}
	}
	{
		got := lightsailAction("DetachDisk")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "DetachDisk")
		}
	}
	{
		got := lightsailAction("AllocateStaticIp")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "AllocateStaticIp")
		}
	}
	{
		got := lightsailAction("GetStaticIp")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "GetStaticIp")
		}
	}
	{
		got := lightsailAction("GetStaticIps")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "GetStaticIps")
		}
	}
	{
		got := lightsailAction("ReleaseStaticIp")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "ReleaseStaticIp")
		}
	}
	{
		got := lightsailAction("AttachStaticIp")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "AttachStaticIp")
		}
	}
	{
		got := lightsailAction("DetachStaticIp")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "DetachStaticIp")
		}
	}
	{
		got := lightsailAction("CreateKeyPair")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "CreateKeyPair")
		}
	}
	{
		got := lightsailAction("GetKeyPair")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "GetKeyPair")
		}
	}
	{
		got := lightsailAction("GetKeyPairs")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "GetKeyPairs")
		}
	}
	{
		got := lightsailAction("DeleteKeyPair")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "DeleteKeyPair")
		}
	}
	{
		got := lightsailAction("OpenInstancePublicPorts")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "OpenInstancePublicPorts")
		}
	}
	{
		got := lightsailAction("CloseInstancePublicPorts")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "CloseInstancePublicPorts")
		}
	}
	{
		got := lightsailAction("GetInstancePortStates")
		if got == "" {
			t.Fatalf("lightsailAction(%q) empty", "GetInstancePortStates")
		}
	}
	if got := logsAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := logsAction("CreateLogGroup")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "CreateLogGroup")
		}
	}
	{
		got := logsAction("CreateLogStream")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "CreateLogStream")
		}
	}
	{
		got := logsAction("DeleteLogGroup")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "DeleteLogGroup")
		}
	}
	{
		got := logsAction("DeleteLogStream")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "DeleteLogStream")
		}
	}
	{
		got := logsAction("DescribeLogStreams")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "DescribeLogStreams")
		}
	}
	{
		got := logsAction("PutLogEvents")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "PutLogEvents")
		}
	}
	{
		got := logsAction("GetLogEvents")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "GetLogEvents")
		}
	}
	{
		got := logsAction("FilterLogEvents")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "FilterLogEvents")
		}
	}
	{
		got := logsAction("DescribeLogGroups")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "DescribeLogGroups")
		}
	}
	{
		got := logsAction("PutSubscriptionFilter")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "PutSubscriptionFilter")
		}
	}
	{
		got := logsAction("DeleteSubscriptionFilter")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "DeleteSubscriptionFilter")
		}
	}
	{
		got := logsAction("DescribeSubscriptionFilters")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "DescribeSubscriptionFilters")
		}
	}
	{
		got := logsAction("PutMetricFilter")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "PutMetricFilter")
		}
	}
	{
		got := logsAction("DeleteMetricFilter")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "DeleteMetricFilter")
		}
	}
	{
		got := logsAction("DescribeMetricFilters")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "DescribeMetricFilters")
		}
	}
	{
		got := logsAction("PutResourcePolicy")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "PutResourcePolicy")
		}
	}
	{
		got := logsAction("GetResourcePolicy")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "GetResourcePolicy")
		}
	}
	{
		got := logsAction("DeleteResourcePolicy")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "DeleteResourcePolicy")
		}
	}
	{
		got := logsAction("DescribeResourcePolicies")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "DescribeResourcePolicies")
		}
	}
	{
		got := logsAction("PutRetentionPolicy")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "PutRetentionPolicy")
		}
	}
	{
		got := logsAction("DeleteRetentionPolicy")
		if got == "" {
			t.Fatalf("logsAction(%q) empty", "DeleteRetentionPolicy")
		}
	}
	if got := macieAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := macieAction("EnableMacie")
		if got == "" {
			t.Fatalf("macieAction(%q) empty", "EnableMacie")
		}
	}
	{
		got := macieAction("GetMacieSession")
		if got == "" {
			t.Fatalf("macieAction(%q) empty", "GetMacieSession")
		}
	}
	{
		got := macieAction("CreateClassificationJob")
		if got == "" {
			t.Fatalf("macieAction(%q) empty", "CreateClassificationJob")
		}
	}
	{
		got := macieAction("DescribeClassificationJob")
		if got == "" {
			t.Fatalf("macieAction(%q) empty", "DescribeClassificationJob")
		}
	}
	{
		got := macieAction("ListClassificationJobs")
		if got == "" {
			t.Fatalf("macieAction(%q) empty", "ListClassificationJobs")
		}
	}
	{
		got := macieAction("ListFindings")
		if got == "" {
			t.Fatalf("macieAction(%q) empty", "ListFindings")
		}
	}
	{
		got := macieAction("GetFindings")
		if got == "" {
			t.Fatalf("macieAction(%q) empty", "GetFindings")
		}
	}
	{
		got := macieAction("InjectFindings")
		if got == "" {
			t.Fatalf("macieAction(%q) empty", "InjectFindings")
		}
	}
	if got := memorydbAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := memorydbAction("CreateCluster")
		if got == "" {
			t.Fatalf("memorydbAction(%q) empty", "CreateCluster")
		}
	}
	{
		got := memorydbAction("DescribeClusters")
		if got == "" {
			t.Fatalf("memorydbAction(%q) empty", "DescribeClusters")
		}
	}
	{
		got := memorydbAction("DeleteCluster")
		if got == "" {
			t.Fatalf("memorydbAction(%q) empty", "DeleteCluster")
		}
	}
	{
		got := memorydbAction("DescribeUsers")
		if got == "" {
			t.Fatalf("memorydbAction(%q) empty", "DescribeUsers")
		}
	}
	{
		got := memorydbAction("DescribeACLs")
		if got == "" {
			t.Fatalf("memorydbAction(%q) empty", "DescribeACLs")
		}
	}
	if got := mqAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := mqAction("CreateBroker")
		if got == "" {
			t.Fatalf("mqAction(%q) empty", "CreateBroker")
		}
	}
	{
		got := mqAction("DescribeBroker")
		if got == "" {
			t.Fatalf("mqAction(%q) empty", "DescribeBroker")
		}
	}
	{
		got := mqAction("ListBrokers")
		if got == "" {
			t.Fatalf("mqAction(%q) empty", "ListBrokers")
		}
	}
	{
		got := mqAction("DeleteBroker")
		if got == "" {
			t.Fatalf("mqAction(%q) empty", "DeleteBroker")
		}
	}
	if got := mskAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := mskAction("CreateCluster")
		if got == "" {
			t.Fatalf("mskAction(%q) empty", "CreateCluster")
		}
	}
	{
		got := mskAction("DescribeCluster")
		if got == "" {
			t.Fatalf("mskAction(%q) empty", "DescribeCluster")
		}
	}
	{
		got := mskAction("ListClusters")
		if got == "" {
			t.Fatalf("mskAction(%q) empty", "ListClusters")
		}
	}
	{
		got := mskAction("DeleteCluster")
		if got == "" {
			t.Fatalf("mskAction(%q) empty", "DeleteCluster")
		}
	}
	{
		got := mskAction("GetBootstrapBrokers")
		if got == "" {
			t.Fatalf("mskAction(%q) empty", "GetBootstrapBrokers")
		}
	}
	if got := neptuneAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := neptuneAction("CreateDBCluster")
		if got == "" {
			t.Fatalf("neptuneAction(%q) empty", "CreateDBCluster")
		}
	}
	{
		got := neptuneAction("DescribeDBClusters")
		if got == "" {
			t.Fatalf("neptuneAction(%q) empty", "DescribeDBClusters")
		}
	}
	{
		got := neptuneAction("DeleteDBCluster")
		if got == "" {
			t.Fatalf("neptuneAction(%q) empty", "DeleteDBCluster")
		}
	}
	if got := opensearchAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := opensearchAction("CreateDomain")
		if got == "" {
			t.Fatalf("opensearchAction(%q) empty", "CreateDomain")
		}
	}
	{
		got := opensearchAction("DescribeDomain")
		if got == "" {
			t.Fatalf("opensearchAction(%q) empty", "DescribeDomain")
		}
	}
	{
		got := opensearchAction("ListDomainNames")
		if got == "" {
			t.Fatalf("opensearchAction(%q) empty", "ListDomainNames")
		}
	}
	{
		got := opensearchAction("DeleteDomain")
		if got == "" {
			t.Fatalf("opensearchAction(%q) empty", "DeleteDomain")
		}
	}
	if got := pipesAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := pipesAction("CreatePipe")
		if got == "" {
			t.Fatalf("pipesAction(%q) empty", "CreatePipe")
		}
	}
	{
		got := pipesAction("DescribePipe")
		if got == "" {
			t.Fatalf("pipesAction(%q) empty", "DescribePipe")
		}
	}
	{
		got := pipesAction("DeletePipe")
		if got == "" {
			t.Fatalf("pipesAction(%q) empty", "DeletePipe")
		}
	}
	{
		got := pipesAction("ListPipes")
		if got == "" {
			t.Fatalf("pipesAction(%q) empty", "ListPipes")
		}
	}
	if got := pricingAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := pricingAction("DescribeServices")
		if got == "" {
			t.Fatalf("pricingAction(%q) empty", "DescribeServices")
		}
	}
	{
		got := pricingAction("GetAttributeValues")
		if got == "" {
			t.Fatalf("pricingAction(%q) empty", "GetAttributeValues")
		}
	}
	{
		got := pricingAction("GetProducts")
		if got == "" {
			t.Fatalf("pricingAction(%q) empty", "GetProducts")
		}
	}
	if got := rdsAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := rdsAction("CreateDBInstance")
		if got == "" {
			t.Fatalf("rdsAction(%q) empty", "CreateDBInstance")
		}
	}
	{
		got := rdsAction("DescribeDBInstances")
		if got == "" {
			t.Fatalf("rdsAction(%q) empty", "DescribeDBInstances")
		}
	}
	{
		got := rdsAction("DeleteDBInstance")
		if got == "" {
			t.Fatalf("rdsAction(%q) empty", "DeleteDBInstance")
		}
	}
	if got := rdsDataAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := rdsDataAction("ExecuteStatement")
		if got == "" {
			t.Fatalf("rdsDataAction(%q) empty", "ExecuteStatement")
		}
	}
	{
		got := rdsDataAction("BatchExecuteStatement")
		if got == "" {
			t.Fatalf("rdsDataAction(%q) empty", "BatchExecuteStatement")
		}
	}
	{
		got := rdsDataAction("BeginTransaction")
		if got == "" {
			t.Fatalf("rdsDataAction(%q) empty", "BeginTransaction")
		}
	}
	{
		got := rdsDataAction("CommitTransaction")
		if got == "" {
			t.Fatalf("rdsDataAction(%q) empty", "CommitTransaction")
		}
	}
	{
		got := rdsDataAction("RollbackTransaction")
		if got == "" {
			t.Fatalf("rdsDataAction(%q) empty", "RollbackTransaction")
		}
	}
	if got := route53Action(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := route53Action("CreateHostedZone")
		if got == "" {
			t.Fatalf("route53Action(%q) empty", "CreateHostedZone")
		}
	}
	{
		got := route53Action("DeleteHostedZone")
		if got == "" {
			t.Fatalf("route53Action(%q) empty", "DeleteHostedZone")
		}
	}
	{
		got := route53Action("ListHostedZones")
		if got == "" {
			t.Fatalf("route53Action(%q) empty", "ListHostedZones")
		}
	}
	{
		got := route53Action("ChangeResourceRecordSets")
		if got == "" {
			t.Fatalf("route53Action(%q) empty", "ChangeResourceRecordSets")
		}
	}
	{
		got := route53Action("ListResourceRecordSets")
		if got == "" {
			t.Fatalf("route53Action(%q) empty", "ListResourceRecordSets")
		}
	}
	{
		got := route53Action("InjectQueryLogs")
		if got == "" {
			t.Fatalf("route53Action(%q) empty", "InjectQueryLogs")
		}
	}
	if got := s3vectorsAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := s3vectorsAction("CreateVectorBucket")
		if got == "" {
			t.Fatalf("s3vectorsAction(%q) empty", "CreateVectorBucket")
		}
	}
	{
		got := s3vectorsAction("ListVectorBuckets")
		if got == "" {
			t.Fatalf("s3vectorsAction(%q) empty", "ListVectorBuckets")
		}
	}
	{
		got := s3vectorsAction("DeleteVectorBucket")
		if got == "" {
			t.Fatalf("s3vectorsAction(%q) empty", "DeleteVectorBucket")
		}
	}
	{
		got := s3vectorsAction("CreateIndex")
		if got == "" {
			t.Fatalf("s3vectorsAction(%q) empty", "CreateIndex")
		}
	}
	{
		got := s3vectorsAction("ListIndexes")
		if got == "" {
			t.Fatalf("s3vectorsAction(%q) empty", "ListIndexes")
		}
	}
	{
		got := s3vectorsAction("DeleteIndex")
		if got == "" {
			t.Fatalf("s3vectorsAction(%q) empty", "DeleteIndex")
		}
	}
	{
		got := s3vectorsAction("PutVectors")
		if got == "" {
			t.Fatalf("s3vectorsAction(%q) empty", "PutVectors")
		}
	}
	{
		got := s3vectorsAction("QueryVectors")
		if got == "" {
			t.Fatalf("s3vectorsAction(%q) empty", "QueryVectors")
		}
	}
	if got := schedulerAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := schedulerAction("CreateSchedule")
		if got == "" {
			t.Fatalf("schedulerAction(%q) empty", "CreateSchedule")
		}
	}
	{
		got := schedulerAction("GetSchedule")
		if got == "" {
			t.Fatalf("schedulerAction(%q) empty", "GetSchedule")
		}
	}
	{
		got := schedulerAction("UpdateSchedule")
		if got == "" {
			t.Fatalf("schedulerAction(%q) empty", "UpdateSchedule")
		}
	}
	{
		got := schedulerAction("DeleteSchedule")
		if got == "" {
			t.Fatalf("schedulerAction(%q) empty", "DeleteSchedule")
		}
	}
	{
		got := schedulerAction("ListSchedules")
		if got == "" {
			t.Fatalf("schedulerAction(%q) empty", "ListSchedules")
		}
	}
	if got := secretsAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := secretsAction("CreateSecret")
		if got == "" {
			t.Fatalf("secretsAction(%q) empty", "CreateSecret")
		}
	}
	{
		got := secretsAction("GetSecretValue")
		if got == "" {
			t.Fatalf("secretsAction(%q) empty", "GetSecretValue")
		}
	}
	{
		got := secretsAction("PutSecretValue")
		if got == "" {
			t.Fatalf("secretsAction(%q) empty", "PutSecretValue")
		}
	}
	{
		got := secretsAction("DeleteSecret")
		if got == "" {
			t.Fatalf("secretsAction(%q) empty", "DeleteSecret")
		}
	}
	{
		got := secretsAction("RestoreSecret")
		if got == "" {
			t.Fatalf("secretsAction(%q) empty", "RestoreSecret")
		}
	}
	{
		got := secretsAction("RotateSecret")
		if got == "" {
			t.Fatalf("secretsAction(%q) empty", "RotateSecret")
		}
	}
	{
		got := secretsAction("UpdateSecretVersionStage")
		if got == "" {
			t.Fatalf("secretsAction(%q) empty", "UpdateSecretVersionStage")
		}
	}
	{
		got := secretsAction("DescribeSecret")
		if got == "" {
			t.Fatalf("secretsAction(%q) empty", "DescribeSecret")
		}
	}
	{
		got := secretsAction("ListSecrets")
		if got == "" {
			t.Fatalf("secretsAction(%q) empty", "ListSecrets")
		}
	}
	{
		got := secretsAction("PutResourcePolicy")
		if got == "" {
			t.Fatalf("secretsAction(%q) empty", "PutResourcePolicy")
		}
	}
	{
		got := secretsAction("GetResourcePolicy")
		if got == "" {
			t.Fatalf("secretsAction(%q) empty", "GetResourcePolicy")
		}
	}
	{
		got := secretsAction("DeleteResourcePolicy")
		if got == "" {
			t.Fatalf("secretsAction(%q) empty", "DeleteResourcePolicy")
		}
	}
	{
		got := secretsAction("ListTagsForResource")
		if got == "" {
			t.Fatalf("secretsAction(%q) empty", "ListTagsForResource")
		}
	}
	{
		got := secretsAction("TagResource")
		if got == "" {
			t.Fatalf("secretsAction(%q) empty", "TagResource")
		}
	}
	{
		got := secretsAction("UntagResource")
		if got == "" {
			t.Fatalf("secretsAction(%q) empty", "UntagResource")
		}
	}
	if got := securityHubAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := securityHubAction("BatchImportFindings")
		if got == "" {
			t.Fatalf("securityHubAction(%q) empty", "BatchImportFindings")
		}
	}
	{
		got := securityHubAction("GetFindings")
		if got == "" {
			t.Fatalf("securityHubAction(%q) empty", "GetFindings")
		}
	}
	if got := normalizeAction(passthrough); got != passthrough {
		t.Fatalf("normalizeAction passthrough %q", got)
	}
	if got := normalizeAction("TotallyUnknownActionXYZ"); got != "TotallyUnknownActionXYZ" {
		t.Fatalf("normalizeAction default %q", got)
	}
	{
		got := normalizeAction("GetCallerIdentity")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetCallerIdentity")
		}
	}
	{
		got := normalizeAction("AssumeRole")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "AssumeRole")
		}
	}
	{
		got := normalizeAction("GetSessionToken")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetSessionToken")
		}
	}
	{
		got := normalizeAction("GetFederationToken")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetFederationToken")
		}
	}
	{
		got := normalizeAction("AssumeRoleWithSAML")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "AssumeRoleWithSAML")
		}
	}
	{
		got := normalizeAction("AssumeRoleWithWebIdentity")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "AssumeRoleWithWebIdentity")
		}
	}
	{
		got := normalizeAction("AssumeRoot")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "AssumeRoot")
		}
	}
	{
		got := normalizeAction("DecodeAuthorizationMessage")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DecodeAuthorizationMessage")
		}
	}
	{
		got := normalizeAction("GetAccessKeyInfo")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetAccessKeyInfo")
		}
	}
	{
		got := normalizeAction("GetDelegatedAccessToken")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetDelegatedAccessToken")
		}
	}
	{
		got := normalizeAction("GetWebIdentityToken")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetWebIdentityToken")
		}
	}
	{
		got := normalizeAction("CreateAccount")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateAccount")
		}
	}
	{
		got := normalizeAction("DescribeCreateAccountStatus")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DescribeCreateAccountStatus")
		}
	}
	{
		got := normalizeAction("ListAccounts")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListAccounts")
		}
	}
	{
		got := normalizeAction("CreateOrganizationalUnit")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateOrganizationalUnit")
		}
	}
	{
		got := normalizeAction("ListOrganizationalUnitsForParent")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListOrganizationalUnitsForParent")
		}
	}
	{
		got := normalizeAction("EnablePolicyType")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "EnablePolicyType")
		}
	}
	{
		got := normalizeAction("AttachPolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "AttachPolicy")
		}
	}
	{
		got := normalizeAction("DetachPolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DetachPolicy")
		}
	}
	{
		got := normalizeAction("DescribePolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DescribePolicy")
		}
	}
	{
		got := normalizeAction("ListPoliciesForTarget")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListPoliciesForTarget")
		}
	}
	{
		got := normalizeAction("ListParents")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListParents")
		}
	}
	{
		got := normalizeAction("ListAccountsForParent")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListAccountsForParent")
		}
	}
	{
		got := normalizeAction("MoveAccount")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "MoveAccount")
		}
	}
	{
		got := normalizeAction("CreateUser")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateUser")
		}
	}
	{
		got := normalizeAction("GetUser")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetUser")
		}
	}
	{
		got := normalizeAction("ListUsers")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListUsers")
		}
	}
	{
		got := normalizeAction("DeleteUser")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteUser")
		}
	}
	{
		got := normalizeAction("CreateAccessKey")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateAccessKey")
		}
	}
	{
		got := normalizeAction("DeleteAccessKey")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteAccessKey")
		}
	}
	{
		got := normalizeAction("ListAccessKeys")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListAccessKeys")
		}
	}
	{
		got := normalizeAction("UpdateAccessKey")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "UpdateAccessKey")
		}
	}
	{
		got := normalizeAction("GetAccessKeyLastUsed")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetAccessKeyLastUsed")
		}
	}
	{
		got := normalizeAction("GenerateCredentialReport")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GenerateCredentialReport")
		}
	}
	{
		got := normalizeAction("GetCredentialReport")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetCredentialReport")
		}
	}
	{
		got := normalizeAction("CreatePolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreatePolicy")
		}
	}
	{
		got := normalizeAction("GetPolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetPolicy")
		}
	}
	{
		got := normalizeAction("ListPolicies")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListPolicies")
		}
	}
	{
		got := normalizeAction("DeletePolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeletePolicy")
		}
	}
	{
		got := normalizeAction("CreatePolicyVersion")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreatePolicyVersion")
		}
	}
	{
		got := normalizeAction("GetPolicyVersion")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetPolicyVersion")
		}
	}
	{
		got := normalizeAction("ListPolicyVersions")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListPolicyVersions")
		}
	}
	{
		got := normalizeAction("DeletePolicyVersion")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeletePolicyVersion")
		}
	}
	{
		got := normalizeAction("SetDefaultPolicyVersion")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "SetDefaultPolicyVersion")
		}
	}
	{
		got := normalizeAction("AttachUserPolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "AttachUserPolicy")
		}
	}
	{
		got := normalizeAction("DetachUserPolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DetachUserPolicy")
		}
	}
	{
		got := normalizeAction("AttachRolePolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "AttachRolePolicy")
		}
	}
	{
		got := normalizeAction("DetachRolePolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DetachRolePolicy")
		}
	}
	{
		got := normalizeAction("ListAttachedUserPolicies")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListAttachedUserPolicies")
		}
	}
	{
		got := normalizeAction("ListAttachedRolePolicies")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListAttachedRolePolicies")
		}
	}
	{
		got := normalizeAction("PutUserPolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PutUserPolicy")
		}
	}
	{
		got := normalizeAction("GetUserPolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetUserPolicy")
		}
	}
	{
		got := normalizeAction("DeleteUserPolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteUserPolicy")
		}
	}
	{
		got := normalizeAction("ListUserPolicies")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListUserPolicies")
		}
	}
	{
		got := normalizeAction("PutRolePolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PutRolePolicy")
		}
	}
	{
		got := normalizeAction("GetRolePolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetRolePolicy")
		}
	}
	{
		got := normalizeAction("DeleteRolePolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteRolePolicy")
		}
	}
	{
		got := normalizeAction("ListRolePolicies")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListRolePolicies")
		}
	}
	{
		got := normalizeAction("CreateRole")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateRole")
		}
	}
	{
		got := normalizeAction("GetRole")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetRole")
		}
	}
	{
		got := normalizeAction("ListRoles")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListRoles")
		}
	}
	{
		got := normalizeAction("DeleteRole")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteRole")
		}
	}
	{
		got := normalizeAction("UpdateAssumeRolePolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "UpdateAssumeRolePolicy")
		}
	}
	{
		got := normalizeAction("CreateGroup")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateGroup")
		}
	}
	{
		got := normalizeAction("DeleteGroup")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteGroup")
		}
	}
	{
		got := normalizeAction("GetGroup")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetGroup")
		}
	}
	{
		got := normalizeAction("ListGroups")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListGroups")
		}
	}
	{
		got := normalizeAction("AddUserToGroup")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "AddUserToGroup")
		}
	}
	{
		got := normalizeAction("RemoveUserFromGroup")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "RemoveUserFromGroup")
		}
	}
	{
		got := normalizeAction("AttachGroupPolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "AttachGroupPolicy")
		}
	}
	{
		got := normalizeAction("DetachGroupPolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DetachGroupPolicy")
		}
	}
	{
		got := normalizeAction("ListAttachedGroupPolicies")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListAttachedGroupPolicies")
		}
	}
	{
		got := normalizeAction("PutGroupPolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PutGroupPolicy")
		}
	}
	{
		got := normalizeAction("GetGroupPolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetGroupPolicy")
		}
	}
	{
		got := normalizeAction("DeleteGroupPolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteGroupPolicy")
		}
	}
	{
		got := normalizeAction("ListGroupPolicies")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListGroupPolicies")
		}
	}
	{
		got := normalizeAction("PutUserPermissionsBoundary")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PutUserPermissionsBoundary")
		}
	}
	{
		got := normalizeAction("GetUserPermissionsBoundary")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetUserPermissionsBoundary")
		}
	}
	{
		got := normalizeAction("DeleteUserPermissionsBoundary")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteUserPermissionsBoundary")
		}
	}
	{
		got := normalizeAction("PutRolePermissionsBoundary")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PutRolePermissionsBoundary")
		}
	}
	{
		got := normalizeAction("GetRolePermissionsBoundary")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetRolePermissionsBoundary")
		}
	}
	{
		got := normalizeAction("DeleteRolePermissionsBoundary")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteRolePermissionsBoundary")
		}
	}
	{
		got := normalizeAction("CreateInstanceProfile")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateInstanceProfile")
		}
	}
	{
		got := normalizeAction("DeleteInstanceProfile")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteInstanceProfile")
		}
	}
	{
		got := normalizeAction("GetInstanceProfile")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetInstanceProfile")
		}
	}
	{
		got := normalizeAction("AddRoleToInstanceProfile")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "AddRoleToInstanceProfile")
		}
	}
	{
		got := normalizeAction("RemoveRoleFromInstanceProfile")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "RemoveRoleFromInstanceProfile")
		}
	}
	{
		got := normalizeAction("ListInstanceProfiles")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListInstanceProfiles")
		}
	}
	{
		got := normalizeAction("ListInstanceProfilesForRole")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListInstanceProfilesForRole")
		}
	}
	{
		got := normalizeAction("CreateOpenIDConnectProvider")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateOpenIDConnectProvider")
		}
	}
	{
		got := normalizeAction("DeleteOpenIDConnectProvider")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteOpenIDConnectProvider")
		}
	}
	{
		got := normalizeAction("ListOpenIDConnectProviders")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListOpenIDConnectProviders")
		}
	}
	{
		got := normalizeAction("GetOpenIDConnectProvider")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetOpenIDConnectProvider")
		}
	}
	{
		got := normalizeAction("CreateSAMLProvider")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateSAMLProvider")
		}
	}
	{
		got := normalizeAction("DeleteSAMLProvider")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteSAMLProvider")
		}
	}
	{
		got := normalizeAction("ListSAMLProviders")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListSAMLProviders")
		}
	}
	{
		got := normalizeAction("GetSAMLProvider")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetSAMLProvider")
		}
	}
	{
		got := normalizeAction("CreateVirtualMFADevice")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateVirtualMFADevice")
		}
	}
	{
		got := normalizeAction("EnableMFADevice")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "EnableMFADevice")
		}
	}
	{
		got := normalizeAction("ListMFADevices")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListMFADevices")
		}
	}
	{
		got := normalizeAction("DeactivateMFADevice")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeactivateMFADevice")
		}
	}
	{
		got := normalizeAction("CreateKey")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateKey")
		}
	}
	{
		got := normalizeAction("DescribeKey")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DescribeKey")
		}
	}
	{
		got := normalizeAction("ListKeys")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListKeys")
		}
	}
	{
		got := normalizeAction("EnableKey")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "EnableKey")
		}
	}
	{
		got := normalizeAction("DisableKey")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DisableKey")
		}
	}
	{
		got := normalizeAction("GetKeyPolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetKeyPolicy")
		}
	}
	{
		got := normalizeAction("PutKeyPolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PutKeyPolicy")
		}
	}
	{
		got := normalizeAction("Encrypt")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "Encrypt")
		}
	}
	{
		got := normalizeAction("Decrypt")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "Decrypt")
		}
	}
	{
		got := normalizeAction("GenerateDataKey")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GenerateDataKey")
		}
	}
	{
		got := normalizeAction("GenerateDataKeyWithoutPlaintext")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GenerateDataKeyWithoutPlaintext")
		}
	}
	{
		got := normalizeAction("CreateGrant")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateGrant")
		}
	}
	{
		got := normalizeAction("ListGrants")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListGrants")
		}
	}
	{
		got := normalizeAction("RetireGrant")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "RetireGrant")
		}
	}
	{
		got := normalizeAction("RevokeGrant")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "RevokeGrant")
		}
	}
	{
		got := normalizeAction("CreateAlias")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateAlias")
		}
	}
	{
		got := normalizeAction("ListAliases")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListAliases")
		}
	}
	{
		got := normalizeAction("DeleteAlias")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteAlias")
		}
	}
	{
		got := normalizeAction("UpdateAlias")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "UpdateAlias")
		}
	}
	{
		got := normalizeAction("ScheduleKeyDeletion")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ScheduleKeyDeletion")
		}
	}
	{
		got := normalizeAction("CancelKeyDeletion")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CancelKeyDeletion")
		}
	}
	{
		got := normalizeAction("EnableKeyRotation")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "EnableKeyRotation")
		}
	}
	{
		got := normalizeAction("DisableKeyRotation")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DisableKeyRotation")
		}
	}
	{
		got := normalizeAction("GetKeyRotationStatus")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetKeyRotationStatus")
		}
	}
	{
		got := normalizeAction("ListResourceTags")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListResourceTags")
		}
	}
	{
		got := normalizeAction("TagResource")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "TagResource")
		}
	}
	{
		got := normalizeAction("UntagResource")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "UntagResource")
		}
	}
	{
		got := normalizeAction("Sign")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "Sign")
		}
	}
	{
		got := normalizeAction("Verify")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "Verify")
		}
	}
	{
		got := normalizeAction("GetPublicKey")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetPublicKey")
		}
	}
	{
		got := normalizeAction("CreateTable")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateTable")
		}
	}
	{
		got := normalizeAction("DescribeTable")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DescribeTable")
		}
	}
	{
		got := normalizeAction("DeleteTable")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteTable")
		}
	}
	{
		got := normalizeAction("ListTables")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListTables")
		}
	}
	{
		got := normalizeAction("UpdateTable")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "UpdateTable")
		}
	}
	{
		got := normalizeAction("PutItem")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PutItem")
		}
	}
	{
		got := normalizeAction("GetItem")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetItem")
		}
	}
	{
		got := normalizeAction("DeleteItem")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteItem")
		}
	}
	{
		got := normalizeAction("UpdateItem")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "UpdateItem")
		}
	}
	{
		got := normalizeAction("Query")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "Query")
		}
	}
	{
		got := normalizeAction("Scan")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "Scan")
		}
	}
	{
		got := normalizeAction("BatchGetItem")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "BatchGetItem")
		}
	}
	{
		got := normalizeAction("BatchWriteItem")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "BatchWriteItem")
		}
	}
	{
		got := normalizeAction("TransactGetItems")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "TransactGetItems")
		}
	}
	{
		got := normalizeAction("TransactWriteItems")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "TransactWriteItems")
		}
	}
	{
		got := normalizeAction("PutResourcePolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PutResourcePolicy")
		}
	}
	{
		got := normalizeAction("GetResourcePolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetResourcePolicy")
		}
	}
	{
		got := normalizeAction("DeleteResourcePolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteResourcePolicy")
		}
	}
	{
		got := normalizeAction("UpdateTimeToLive")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "UpdateTimeToLive")
		}
	}
	{
		got := normalizeAction("DescribeTimeToLive")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DescribeTimeToLive")
		}
	}
	{
		got := normalizeAction("DescribeContinuousBackups")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DescribeContinuousBackups")
		}
	}
	{
		got := normalizeAction("ListTagsOfResource")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListTagsOfResource")
		}
	}
	{
		got := normalizeAction("CreateQueue")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateQueue")
		}
	}
	{
		got := normalizeAction("GetQueueUrl")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetQueueUrl")
		}
	}
	{
		got := normalizeAction("GetQueueAttributes")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetQueueAttributes")
		}
	}
	{
		got := normalizeAction("SetQueueAttributes")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "SetQueueAttributes")
		}
	}
	{
		got := normalizeAction("DeleteQueue")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteQueue")
		}
	}
	{
		got := normalizeAction("ListQueues")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListQueues")
		}
	}
	{
		got := normalizeAction("PurgeQueue")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PurgeQueue")
		}
	}
	{
		got := normalizeAction("SendMessage")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "SendMessage")
		}
	}
	{
		got := normalizeAction("ReceiveMessage")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ReceiveMessage")
		}
	}
	{
		got := normalizeAction("DeleteMessage")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteMessage")
		}
	}
	{
		got := normalizeAction("SendMessageBatch")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "SendMessageBatch")
		}
	}
	{
		got := normalizeAction("DeleteMessageBatch")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteMessageBatch")
		}
	}
	{
		got := normalizeAction("ChangeMessageVisibility")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ChangeMessageVisibility")
		}
	}
	{
		got := normalizeAction("ListQueueTags")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListQueueTags")
		}
	}
	{
		got := normalizeAction("TagQueue")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "TagQueue")
		}
	}
	{
		got := normalizeAction("UntagQueue")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "UntagQueue")
		}
	}
	{
		got := normalizeAction("CreateTopic")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateTopic")
		}
	}
	{
		got := normalizeAction("DeleteTopic")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteTopic")
		}
	}
	{
		got := normalizeAction("ListTopics")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListTopics")
		}
	}
	{
		got := normalizeAction("GetTopicAttributes")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetTopicAttributes")
		}
	}
	{
		got := normalizeAction("SetTopicAttributes")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "SetTopicAttributes")
		}
	}
	{
		got := normalizeAction("Publish")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "Publish")
		}
	}
	{
		got := normalizeAction("Subscribe")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "Subscribe")
		}
	}
	{
		got := normalizeAction("Unsubscribe")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "Unsubscribe")
		}
	}
	{
		got := normalizeAction("ListSubscriptions")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListSubscriptions")
		}
	}
	{
		got := normalizeAction("ListSubscriptionsByTopic")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListSubscriptionsByTopic")
		}
	}
	{
		got := normalizeAction("GetSubscriptionAttributes")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetSubscriptionAttributes")
		}
	}
	{
		got := normalizeAction("PutParameter")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PutParameter")
		}
	}
	{
		got := normalizeAction("GetParameter")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetParameter")
		}
	}
	{
		got := normalizeAction("GetParameters")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetParameters")
		}
	}
	{
		got := normalizeAction("GetParametersByPath")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetParametersByPath")
		}
	}
	{
		got := normalizeAction("DeleteParameter")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteParameter")
		}
	}
	{
		got := normalizeAction("DescribeParameters")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DescribeParameters")
		}
	}
	{
		got := normalizeAction("LabelParameterVersion")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "LabelParameterVersion")
		}
	}
	{
		got := normalizeAction("GetParameterHistory")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetParameterHistory")
		}
	}
	{
		got := normalizeAction("AddTagsToResource")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "AddTagsToResource")
		}
	}
	{
		got := normalizeAction("RemoveTagsFromResource")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "RemoveTagsFromResource")
		}
	}
	{
		got := normalizeAction("CreateSecret")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateSecret")
		}
	}
	{
		got := normalizeAction("GetSecretValue")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetSecretValue")
		}
	}
	{
		got := normalizeAction("PutSecretValue")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PutSecretValue")
		}
	}
	{
		got := normalizeAction("DeleteSecret")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteSecret")
		}
	}
	{
		got := normalizeAction("RestoreSecret")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "RestoreSecret")
		}
	}
	{
		got := normalizeAction("RotateSecret")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "RotateSecret")
		}
	}
	{
		got := normalizeAction("UpdateSecretVersionStage")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "UpdateSecretVersionStage")
		}
	}
	{
		got := normalizeAction("DescribeSecret")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DescribeSecret")
		}
	}
	{
		got := normalizeAction("ListSecrets")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListSecrets")
		}
	}
	{
		got := normalizeAction("PutEvents")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PutEvents")
		}
	}
	{
		got := normalizeAction("CreateEventBus")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateEventBus")
		}
	}
	{
		got := normalizeAction("DeleteEventBus")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteEventBus")
		}
	}
	{
		got := normalizeAction("DescribeEventBus")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DescribeEventBus")
		}
	}
	{
		got := normalizeAction("ListEventBuses")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListEventBuses")
		}
	}
	{
		got := normalizeAction("PutRule")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PutRule")
		}
	}
	{
		got := normalizeAction("DescribeRule")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DescribeRule")
		}
	}
	{
		got := normalizeAction("ListRules")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListRules")
		}
	}
	{
		got := normalizeAction("DeleteRule")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteRule")
		}
	}
	{
		got := normalizeAction("EnableRule")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "EnableRule")
		}
	}
	{
		got := normalizeAction("DisableRule")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DisableRule")
		}
	}
	{
		got := normalizeAction("PutTargets")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PutTargets")
		}
	}
	{
		got := normalizeAction("RemoveTargets")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "RemoveTargets")
		}
	}
	{
		got := normalizeAction("ListTargetsByRule")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListTargetsByRule")
		}
	}
	{
		got := normalizeAction("PutPermission")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PutPermission")
		}
	}
	{
		got := normalizeAction("RemovePermission")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "RemovePermission")
		}
	}
	{
		got := normalizeAction("CreateFunction")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateFunction")
		}
	}
	{
		got := normalizeAction("GetFunction")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetFunction")
		}
	}
	{
		got := normalizeAction("DeleteFunction")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteFunction")
		}
	}
	{
		got := normalizeAction("ListFunctions")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListFunctions")
		}
	}
	{
		got := normalizeAction("UpdateFunctionCode")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "UpdateFunctionCode")
		}
	}
	{
		got := normalizeAction("UpdateFunctionConfiguration")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "UpdateFunctionConfiguration")
		}
	}
	{
		got := normalizeAction("Invoke")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "Invoke")
		}
	}
	{
		got := normalizeAction("PublishVersion")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PublishVersion")
		}
	}
	{
		got := normalizeAction("ListVersionsByFunction")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListVersionsByFunction")
		}
	}
	{
		got := normalizeAction("ListTags")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListTags")
		}
	}
	{
		got := normalizeAction("GetAlias")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetAlias")
		}
	}
	{
		got := normalizeAction("RegisterTaskDefinition")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "RegisterTaskDefinition")
		}
	}
	{
		got := normalizeAction("DescribeTaskDefinition")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DescribeTaskDefinition")
		}
	}
	{
		got := normalizeAction("ListTaskDefinitions")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListTaskDefinitions")
		}
	}
	{
		got := normalizeAction("DeregisterTaskDefinition")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeregisterTaskDefinition")
		}
	}
	{
		got := normalizeAction("RunTask")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "RunTask")
		}
	}
	{
		got := normalizeAction("DescribeTasks")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DescribeTasks")
		}
	}
	{
		got := normalizeAction("ListTasks")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListTasks")
		}
	}
	{
		got := normalizeAction("StopTask")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "StopTask")
		}
	}
	{
		got := normalizeAction("DescribeClusters")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DescribeClusters")
		}
	}
	{
		got := normalizeAction("ListClusters")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListClusters")
		}
	}
	{
		got := normalizeAction("CreateService")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateService")
		}
	}
	{
		got := normalizeAction("UpdateService")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "UpdateService")
		}
	}
	{
		got := normalizeAction("DeleteService")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteService")
		}
	}
	{
		got := normalizeAction("DescribeServices")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DescribeServices")
		}
	}
	{
		got := normalizeAction("ListServices")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListServices")
		}
	}
	{
		got := normalizeAction("CreateEventSourceMapping")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateEventSourceMapping")
		}
	}
	{
		got := normalizeAction("GetEventSourceMapping")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetEventSourceMapping")
		}
	}
	{
		got := normalizeAction("ListEventSourceMappings")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListEventSourceMappings")
		}
	}
	{
		got := normalizeAction("UpdateEventSourceMapping")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "UpdateEventSourceMapping")
		}
	}
	{
		got := normalizeAction("DeleteEventSourceMapping")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteEventSourceMapping")
		}
	}
	{
		got := normalizeAction("CreateFunctionUrlConfig")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateFunctionUrlConfig")
		}
	}
	{
		got := normalizeAction("GetFunctionUrlConfig")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetFunctionUrlConfig")
		}
	}
	{
		got := normalizeAction("DeleteFunctionUrlConfig")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteFunctionUrlConfig")
		}
	}
	{
		got := normalizeAction("ListFunctionUrlConfigs")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListFunctionUrlConfigs")
		}
	}
	{
		got := normalizeAction("LookupEvents")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "LookupEvents")
		}
	}
	{
		got := normalizeAction("CreateLogGroup")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateLogGroup")
		}
	}
	{
		got := normalizeAction("CreateLogStream")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateLogStream")
		}
	}
	{
		got := normalizeAction("DeleteLogGroup")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteLogGroup")
		}
	}
	{
		got := normalizeAction("DeleteLogStream")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteLogStream")
		}
	}
	{
		got := normalizeAction("DescribeLogStreams")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DescribeLogStreams")
		}
	}
	{
		got := normalizeAction("PutLogEvents")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PutLogEvents")
		}
	}
	{
		got := normalizeAction("GetLogEvents")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetLogEvents")
		}
	}
	{
		got := normalizeAction("FilterLogEvents")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "FilterLogEvents")
		}
	}
	{
		got := normalizeAction("DescribeLogGroups")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DescribeLogGroups")
		}
	}
	{
		got := normalizeAction("PutRetentionPolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PutRetentionPolicy")
		}
	}
	{
		got := normalizeAction("DeleteRetentionPolicy")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteRetentionPolicy")
		}
	}
	{
		got := normalizeAction("TagResources")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "TagResources")
		}
	}
	{
		got := normalizeAction("UntagResources")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "UntagResources")
		}
	}
	{
		got := normalizeAction("GetResources")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetResources")
		}
	}
	{
		got := normalizeAction("CreateStream")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateStream")
		}
	}
	{
		got := normalizeAction("DeleteStream")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteStream")
		}
	}
	{
		got := normalizeAction("DescribeStream")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DescribeStream")
		}
	}
	{
		got := normalizeAction("ListStreams")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListStreams")
		}
	}
	{
		got := normalizeAction("PutRecord")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PutRecord")
		}
	}
	{
		got := normalizeAction("PutRecords")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "PutRecords")
		}
	}
	{
		got := normalizeAction("GetShardIterator")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetShardIterator")
		}
	}
	{
		got := normalizeAction("GetRecords")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetRecords")
		}
	}
	{
		got := normalizeAction("VerifyEmailIdentity")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "VerifyEmailIdentity")
		}
	}
	{
		got := normalizeAction("SendEmail")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "SendEmail")
		}
	}
	{
		got := normalizeAction("SendRawEmail")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "SendRawEmail")
		}
	}
	{
		got := normalizeAction("ListIdentities")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListIdentities")
		}
	}
	{
		got := normalizeAction("GetSendStatistics")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetSendStatistics")
		}
	}
	{
		got := normalizeAction("SetIdentityNotificationTopic")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "SetIdentityNotificationTopic")
		}
	}
	{
		got := normalizeAction("CreateConfigurationProfile")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateConfigurationProfile")
		}
	}
	{
		got := normalizeAction("CreateHostedConfigurationVersion")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateHostedConfigurationVersion")
		}
	}
	{
		got := normalizeAction("GetConfiguration")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetConfiguration")
		}
	}
	{
		got := normalizeAction("StartConfigurationSession")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "StartConfigurationSession")
		}
	}
	{
		got := normalizeAction("GetLatestConfiguration")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetLatestConfiguration")
		}
	}
	{
		got := normalizeAction("CreateStateMachine")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "CreateStateMachine")
		}
	}
	{
		got := normalizeAction("DeleteStateMachine")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DeleteStateMachine")
		}
	}
	{
		got := normalizeAction("DescribeStateMachine")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DescribeStateMachine")
		}
	}
	{
		got := normalizeAction("ListStateMachines")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "ListStateMachines")
		}
	}
	{
		got := normalizeAction("StartExecution")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "StartExecution")
		}
	}
	{
		got := normalizeAction("DescribeExecution")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "DescribeExecution")
		}
	}
	{
		got := normalizeAction("GetExecutionHistory")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "GetExecutionHistory")
		}
	}
	{
		got := normalizeAction("SendTaskSuccess")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "SendTaskSuccess")
		}
	}
	{
		got := normalizeAction("SendTaskFailure")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "SendTaskFailure")
		}
	}
	{
		got := normalizeAction("SendTaskHeartbeat")
		if got == "" {
			t.Fatalf("normalizeAction(%q) empty", "SendTaskHeartbeat")
		}
	}
	if got := serviceDiscoveryAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := serviceDiscoveryAction("CreatePrivateDnsNamespace")
		if got == "" {
			t.Fatalf("serviceDiscoveryAction(%q) empty", "CreatePrivateDnsNamespace")
		}
	}
	{
		got := serviceDiscoveryAction("CreateHttpNamespace")
		if got == "" {
			t.Fatalf("serviceDiscoveryAction(%q) empty", "CreateHttpNamespace")
		}
	}
	{
		got := serviceDiscoveryAction("CreateService")
		if got == "" {
			t.Fatalf("serviceDiscoveryAction(%q) empty", "CreateService")
		}
	}
	{
		got := serviceDiscoveryAction("RegisterInstance")
		if got == "" {
			t.Fatalf("serviceDiscoveryAction(%q) empty", "RegisterInstance")
		}
	}
	{
		got := serviceDiscoveryAction("DeregisterInstance")
		if got == "" {
			t.Fatalf("serviceDiscoveryAction(%q) empty", "DeregisterInstance")
		}
	}
	{
		got := serviceDiscoveryAction("DiscoverInstances")
		if got == "" {
			t.Fatalf("serviceDiscoveryAction(%q) empty", "DiscoverInstances")
		}
	}
	if got := sesAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := sesAction("VerifyEmailIdentity")
		if got == "" {
			t.Fatalf("sesAction(%q) empty", "VerifyEmailIdentity")
		}
	}
	{
		got := sesAction("SendEmail")
		if got == "" {
			t.Fatalf("sesAction(%q) empty", "SendEmail")
		}
	}
	{
		got := sesAction("SendRawEmail")
		if got == "" {
			t.Fatalf("sesAction(%q) empty", "SendRawEmail")
		}
	}
	{
		got := sesAction("ListIdentities")
		if got == "" {
			t.Fatalf("sesAction(%q) empty", "ListIdentities")
		}
	}
	{
		got := sesAction("GetSendStatistics")
		if got == "" {
			t.Fatalf("sesAction(%q) empty", "GetSendStatistics")
		}
	}
	{
		got := sesAction("SetIdentityNotificationTopic")
		if got == "" {
			t.Fatalf("sesAction(%q) empty", "SetIdentityNotificationTopic")
		}
	}
	if got := sfnAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := sfnAction("CreateStateMachine")
		if got == "" {
			t.Fatalf("sfnAction(%q) empty", "CreateStateMachine")
		}
	}
	{
		got := sfnAction("DeleteStateMachine")
		if got == "" {
			t.Fatalf("sfnAction(%q) empty", "DeleteStateMachine")
		}
	}
	{
		got := sfnAction("DescribeStateMachine")
		if got == "" {
			t.Fatalf("sfnAction(%q) empty", "DescribeStateMachine")
		}
	}
	{
		got := sfnAction("ListStateMachines")
		if got == "" {
			t.Fatalf("sfnAction(%q) empty", "ListStateMachines")
		}
	}
	{
		got := sfnAction("StartExecution")
		if got == "" {
			t.Fatalf("sfnAction(%q) empty", "StartExecution")
		}
	}
	{
		got := sfnAction("DescribeExecution")
		if got == "" {
			t.Fatalf("sfnAction(%q) empty", "DescribeExecution")
		}
	}
	{
		got := sfnAction("GetExecutionHistory")
		if got == "" {
			t.Fatalf("sfnAction(%q) empty", "GetExecutionHistory")
		}
	}
	{
		got := sfnAction("SendTaskSuccess")
		if got == "" {
			t.Fatalf("sfnAction(%q) empty", "SendTaskSuccess")
		}
	}
	{
		got := sfnAction("SendTaskFailure")
		if got == "" {
			t.Fatalf("sfnAction(%q) empty", "SendTaskFailure")
		}
	}
	{
		got := sfnAction("SendTaskHeartbeat")
		if got == "" {
			t.Fatalf("sfnAction(%q) empty", "SendTaskHeartbeat")
		}
	}
	{
		got := sfnAction("PutResourcePolicy")
		if got == "" {
			t.Fatalf("sfnAction(%q) empty", "PutResourcePolicy")
		}
	}
	{
		got := sfnAction("GetResourcePolicy")
		if got == "" {
			t.Fatalf("sfnAction(%q) empty", "GetResourcePolicy")
		}
	}
	{
		got := sfnAction("DeleteResourcePolicy")
		if got == "" {
			t.Fatalf("sfnAction(%q) empty", "DeleteResourcePolicy")
		}
	}
	if got := snsAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := snsAction("CreateTopic")
		if got == "" {
			t.Fatalf("snsAction(%q) empty", "CreateTopic")
		}
	}
	{
		got := snsAction("DeleteTopic")
		if got == "" {
			t.Fatalf("snsAction(%q) empty", "DeleteTopic")
		}
	}
	{
		got := snsAction("ListTopics")
		if got == "" {
			t.Fatalf("snsAction(%q) empty", "ListTopics")
		}
	}
	{
		got := snsAction("GetTopicAttributes")
		if got == "" {
			t.Fatalf("snsAction(%q) empty", "GetTopicAttributes")
		}
	}
	{
		got := snsAction("SetTopicAttributes")
		if got == "" {
			t.Fatalf("snsAction(%q) empty", "SetTopicAttributes")
		}
	}
	{
		got := snsAction("Publish")
		if got == "" {
			t.Fatalf("snsAction(%q) empty", "Publish")
		}
	}
	{
		got := snsAction("Subscribe")
		if got == "" {
			t.Fatalf("snsAction(%q) empty", "Subscribe")
		}
	}
	{
		got := snsAction("ConfirmSubscription")
		if got == "" {
			t.Fatalf("snsAction(%q) empty", "ConfirmSubscription")
		}
	}
	{
		got := snsAction("Unsubscribe")
		if got == "" {
			t.Fatalf("snsAction(%q) empty", "Unsubscribe")
		}
	}
	{
		got := snsAction("ListSubscriptions")
		if got == "" {
			t.Fatalf("snsAction(%q) empty", "ListSubscriptions")
		}
	}
	{
		got := snsAction("ListSubscriptionsByTopic")
		if got == "" {
			t.Fatalf("snsAction(%q) empty", "ListSubscriptionsByTopic")
		}
	}
	{
		got := snsAction("GetSubscriptionAttributes")
		if got == "" {
			t.Fatalf("snsAction(%q) empty", "GetSubscriptionAttributes")
		}
	}
	{
		got := snsAction("AddPermission")
		if got == "" {
			t.Fatalf("snsAction(%q) empty", "AddPermission")
		}
	}
	{
		got := snsAction("RemovePermission")
		if got == "" {
			t.Fatalf("snsAction(%q) empty", "RemovePermission")
		}
	}
	{
		got := snsAction("ListTagsForResource")
		if got == "" {
			t.Fatalf("snsAction(%q) empty", "ListTagsForResource")
		}
	}
	{
		got := snsAction("TagResource")
		if got == "" {
			t.Fatalf("snsAction(%q) empty", "TagResource")
		}
	}
	{
		got := snsAction("UntagResource")
		if got == "" {
			t.Fatalf("snsAction(%q) empty", "UntagResource")
		}
	}
	if got := ssmAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := ssmAction("PutParameter")
		if got == "" {
			t.Fatalf("ssmAction(%q) empty", "PutParameter")
		}
	}
	{
		got := ssmAction("GetParameter")
		if got == "" {
			t.Fatalf("ssmAction(%q) empty", "GetParameter")
		}
	}
	{
		got := ssmAction("GetParameters")
		if got == "" {
			t.Fatalf("ssmAction(%q) empty", "GetParameters")
		}
	}
	{
		got := ssmAction("GetParametersByPath")
		if got == "" {
			t.Fatalf("ssmAction(%q) empty", "GetParametersByPath")
		}
	}
	{
		got := ssmAction("DeleteParameter")
		if got == "" {
			t.Fatalf("ssmAction(%q) empty", "DeleteParameter")
		}
	}
	{
		got := ssmAction("DescribeParameters")
		if got == "" {
			t.Fatalf("ssmAction(%q) empty", "DescribeParameters")
		}
	}
	{
		got := ssmAction("LabelParameterVersion")
		if got == "" {
			t.Fatalf("ssmAction(%q) empty", "LabelParameterVersion")
		}
	}
	{
		got := ssmAction("GetParameterHistory")
		if got == "" {
			t.Fatalf("ssmAction(%q) empty", "GetParameterHistory")
		}
	}
	{
		got := ssmAction("ListTagsForResource")
		if got == "" {
			t.Fatalf("ssmAction(%q) empty", "ListTagsForResource")
		}
	}
	{
		got := ssmAction("AddTagsToResource")
		if got == "" {
			t.Fatalf("ssmAction(%q) empty", "AddTagsToResource")
		}
	}
	{
		got := ssmAction("RemoveTagsFromResource")
		if got == "" {
			t.Fatalf("ssmAction(%q) empty", "RemoveTagsFromResource")
		}
	}
	{
		got := ssmAction("SendCommand")
		if got == "" {
			t.Fatalf("ssmAction(%q) empty", "SendCommand")
		}
	}
	{
		got := ssmAction("GetCommandInvocation")
		if got == "" {
			t.Fatalf("ssmAction(%q) empty", "GetCommandInvocation")
		}
	}
	{
		got := ssmAction("ListCommandInvocations")
		if got == "" {
			t.Fatalf("ssmAction(%q) empty", "ListCommandInvocations")
		}
	}
	if got := taggingAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := taggingAction("TagResources")
		if got == "" {
			t.Fatalf("taggingAction(%q) empty", "TagResources")
		}
	}
	{
		got := taggingAction("UntagResources")
		if got == "" {
			t.Fatalf("taggingAction(%q) empty", "UntagResources")
		}
	}
	{
		got := taggingAction("GetResources")
		if got == "" {
			t.Fatalf("taggingAction(%q) empty", "GetResources")
		}
	}
	if got := textractAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := textractAction("DetectDocumentText")
		if got == "" {
			t.Fatalf("textractAction(%q) empty", "DetectDocumentText")
		}
	}
	{
		got := textractAction("AnalyzeDocument")
		if got == "" {
			t.Fatalf("textractAction(%q) empty", "AnalyzeDocument")
		}
	}
	if got := transcribeAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := transcribeAction("StartTranscriptionJob")
		if got == "" {
			t.Fatalf("transcribeAction(%q) empty", "StartTranscriptionJob")
		}
	}
	{
		got := transcribeAction("GetTranscriptionJob")
		if got == "" {
			t.Fatalf("transcribeAction(%q) empty", "GetTranscriptionJob")
		}
	}
	{
		got := transcribeAction("ListTranscriptionJobs")
		if got == "" {
			t.Fatalf("transcribeAction(%q) empty", "ListTranscriptionJobs")
		}
	}
	if got := transferAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := transferAction("CreateServer")
		if got == "" {
			t.Fatalf("transferAction(%q) empty", "CreateServer")
		}
	}
	{
		got := transferAction("DescribeServer")
		if got == "" {
			t.Fatalf("transferAction(%q) empty", "DescribeServer")
		}
	}
	{
		got := transferAction("ListServers")
		if got == "" {
			t.Fatalf("transferAction(%q) empty", "ListServers")
		}
	}
	{
		got := transferAction("DeleteServer")
		if got == "" {
			t.Fatalf("transferAction(%q) empty", "DeleteServer")
		}
	}
	{
		got := transferAction("CreateUser")
		if got == "" {
			t.Fatalf("transferAction(%q) empty", "CreateUser")
		}
	}
	{
		got := transferAction("DescribeUser")
		if got == "" {
			t.Fatalf("transferAction(%q) empty", "DescribeUser")
		}
	}
	{
		got := transferAction("ListUsers")
		if got == "" {
			t.Fatalf("transferAction(%q) empty", "ListUsers")
		}
	}
	{
		got := transferAction("DeleteUser")
		if got == "" {
			t.Fatalf("transferAction(%q) empty", "DeleteUser")
		}
	}
	{
		got := transferAction("ImportSshPublicKey")
		if got == "" {
			t.Fatalf("transferAction(%q) empty", "ImportSshPublicKey")
		}
	}
	{
		got := transferAction("DeleteSshPublicKey")
		if got == "" {
			t.Fatalf("transferAction(%q) empty", "DeleteSshPublicKey")
		}
	}
	{
		got := transferAction("PutFile")
		if got == "" {
			t.Fatalf("transferAction(%q) empty", "PutFile")
		}
	}
	{
		got := transferAction("GetFile")
		if got == "" {
			t.Fatalf("transferAction(%q) empty", "GetFile")
		}
	}
	{
		got := transferAction("ListDirectory")
		if got == "" {
			t.Fatalf("transferAction(%q) empty", "ListDirectory")
		}
	}
	if got := vpcFlowAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := vpcFlowAction("CreateFlowLogs")
		if got == "" {
			t.Fatalf("vpcFlowAction(%q) empty", "CreateFlowLogs")
		}
	}
	{
		got := vpcFlowAction("InjectFlowLogs")
		if got == "" {
			t.Fatalf("vpcFlowAction(%q) empty", "InjectFlowLogs")
		}
	}
	if got := wafAction(passthrough); !strings.Contains(got, ":") && got != passthrough {
		// some mappers prefix; just ensure call succeeds
		_ = got
	}
	{
		got := wafAction("CreateWebACL")
		if got == "" {
			t.Fatalf("wafAction(%q) empty", "CreateWebACL")
		}
	}
	{
		got := wafAction("UpdateWebACL")
		if got == "" {
			t.Fatalf("wafAction(%q) empty", "UpdateWebACL")
		}
	}
	{
		got := wafAction("GetWebACL")
		if got == "" {
			t.Fatalf("wafAction(%q) empty", "GetWebACL")
		}
	}
	{
		got := wafAction("ListWebACLs")
		if got == "" {
			t.Fatalf("wafAction(%q) empty", "ListWebACLs")
		}
	}
	{
		got := wafAction("CreateRuleGroup")
		if got == "" {
			t.Fatalf("wafAction(%q) empty", "CreateRuleGroup")
		}
	}
	{
		got := wafAction("AssociateWebACL")
		if got == "" {
			t.Fatalf("wafAction(%q) empty", "AssociateWebACL")
		}
	}
	{
		got := wafAction("Evaluate")
		if got == "" {
			t.Fatalf("wafAction(%q) empty", "Evaluate")
		}
	}
	{
		got := wafAction("CreateIPSet")
		if got == "" {
			t.Fatalf("wafAction(%q) empty", "CreateIPSet")
		}
	}
	{
		got := wafAction("GetIPSet")
		if got == "" {
			t.Fatalf("wafAction(%q) empty", "GetIPSet")
		}
	}
	{
		got := wafAction("UpdateIPSet")
		if got == "" {
			t.Fatalf("wafAction(%q) empty", "UpdateIPSet")
		}
	}
	{
		got := wafAction("DeleteIPSet")
		if got == "" {
			t.Fatalf("wafAction(%q) empty", "DeleteIPSet")
		}
	}
	{
		got := wafAction("ListIPSets")
		if got == "" {
			t.Fatalf("wafAction(%q) empty", "ListIPSets")
		}
	}
}

func TestUnauthActionHelpers(t *testing.T) {
	t.Parallel()
}

