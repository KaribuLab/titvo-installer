package internal

import (
	"errors"
	"testing"
)

func withDownloadSourceStub(t *testing.T, stub func(dir, sourceURL, component string) error) {
	t.Helper()
	original := downloadSourceFn
	downloadSourceFn = stub
	t.Cleanup(func() {
		downloadSourceFn = original
	})
}

func withDeployInfraStub(t *testing.T, stub func(config DeployConfig) error) {
	t.Helper()
	original := deployInfraFn
	deployInfraFn = stub
	t.Cleanup(func() {
		deployInfraFn = original
	})
}

func TestDownloadFunctionsRouteToDownloadSource(t *testing.T) {
	tests := []struct {
		name           string
		call           func(string) error
		expectedURL    string
		expectedTarget string
	}{
		{name: "infra", call: DownloadInfraSource, expectedURL: titvoInfraSource, expectedTarget: "infra"},
		{name: "agent aws", call: DownloadAgentAWSSource, expectedURL: titvoAgentAWS, expectedTarget: "agent aws"},
		{name: "mcp gateway", call: DownloadMCPGatewaySource, expectedURL: titvoMCPGateway, expectedTarget: "MCP gateway"},
		{name: "bitbucket", call: DownloadBitbucketCodeInsightsAWSSource, expectedURL: titvoBitbucketCodeInsightsAWS, expectedTarget: "bitbucket code insights aws"},
		{name: "git commit files", call: DownloadGitCommitFilesAWSSource, expectedURL: titvoGitCommitFilesAWS, expectedTarget: "git commit files aws"},
		{name: "github issue", call: DownloadGithubIssueAWSSource, expectedURL: titvoGithubIssueAWS, expectedTarget: "github issue aws"},
		{name: "issue report", call: DownloadIssueReportAWSSource, expectedURL: titvoIssueReportAWS, expectedTarget: "issue report aws"},
		{name: "auth setup", call: DownloadAuthSetupSource, expectedURL: titvoAuthSetupSource, expectedTarget: "auth setup"},
		{name: "task cli files", call: DownloadTaskCliFilesSource, expectedURL: titvoTaskCliFilesSource, expectedTarget: "task cli files"},
		{name: "task trigger", call: DownloadTaskTriggerSource, expectedURL: titvoTaskTriggerSource, expectedTarget: "task trigger"},
		{name: "task status", call: DownloadTaskStatusSource, expectedURL: titvoTaskStatusSource, expectedTarget: "task status"},
		{name: "ecr publisher", call: DownloadInstallerECRPublisherSource, expectedURL: titvoInstallerECRPublisherSource, expectedTarget: "installer ecr publisher"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			withDownloadSourceStub(t, func(dir, sourceURL, component string) error {
				called = true
				if dir != "/tmp/titvo" {
					t.Fatalf("unexpected dir: %s", dir)
				}
				if sourceURL != tc.expectedURL {
					t.Fatalf("unexpected source URL: %s", sourceURL)
				}
				if component != tc.expectedTarget {
					t.Fatalf("unexpected component: %s", component)
				}
				return nil
			})

			if err := tc.call("/tmp/titvo"); err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
			if !called {
				t.Fatalf("expected downloadSourceFn to be called")
			}
		})
	}
}

func TestDownloadInfraSourcePropagatesError(t *testing.T) {
	expectedErr := errors.New("clone failed")
	withDownloadSourceStub(t, func(dir, sourceURL, component string) error {
		return expectedErr
	})

	err := DownloadInfraSource("/tmp/titvo")
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected wrapped error %v, got %v", expectedErr, err)
	}
}

func TestInstallerECRPublisherJobs(t *testing.T) {
	jobs := installerECRPublisherJobs("us-east-1")

	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}

	if jobs[0].Name != "installer-ecr-publisher" {
		t.Fatalf("unexpected first job name: %s", jobs[0].Name)
	}
	if jobs[0].EnvVars["GIT_URL"] != titvoAgentAWS {
		t.Fatalf("unexpected first job git url: %s", jobs[0].EnvVars["GIT_URL"])
	}
	if jobs[0].EnvVars["IMAGE_REPO"] != "tvo-agent-ecr-prod" {
		t.Fatalf("unexpected first job image repo: %s", jobs[0].EnvVars["IMAGE_REPO"])
	}
	if jobs[0].EnvVars["REGION"] != "us-east-1" {
		t.Fatalf("unexpected first job region: %s", jobs[0].EnvVars["REGION"])
	}

	if jobs[1].Name != "installer-ecr-publisher-mcp-gateway" {
		t.Fatalf("unexpected second job name: %s", jobs[1].Name)
	}
	if jobs[1].EnvVars["GIT_URL"] != titvoMCPGateway {
		t.Fatalf("unexpected second job git url: %s", jobs[1].EnvVars["GIT_URL"])
	}
	if jobs[1].EnvVars["IMAGE_REPO"] != "tvo-mcp-gateway-ecr-prod" {
		t.Fatalf("unexpected second job image repo: %s", jobs[1].EnvVars["IMAGE_REPO"])
	}
	if jobs[1].EnvVars["REGION"] != "us-east-1" {
		t.Fatalf("unexpected second job region: %s", jobs[1].EnvVars["REGION"])
	}
}

func TestInstallerECRPublisherJobsUsesProvidedRegion(t *testing.T) {
	region := "eu-west-1"
	jobs := installerECRPublisherJobs(region)

	for _, job := range jobs {
		if job.EnvVars["REGION"] != region {
			t.Fatalf("job %s has unexpected region: %s", job.Name, job.EnvVars["REGION"])
		}
	}
}

func TestDeployInfraDelegatesToRuntime(t *testing.T) {
	expected := errors.New("runtime failure")
	received := DeployConfig{}
	withDeployInfraStub(t, func(config DeployConfig) error {
		received = config
		return expected
	})

	config := DeployConfig{VPCID: "vpc-123"}
	err := DeployInfra(config)
	if !errors.Is(err, expected) {
		t.Fatalf("expected error %v, got %v", expected, err)
	}
	if received.VPCID != "vpc-123" {
		t.Fatalf("expected config to be delegated")
	}
}
