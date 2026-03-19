package internal

const titvoInfraSource = "https://github.com/KaribuLab/titvo-security-scan-infra-aws.git"
const titvoAuthSetupSource = "https://github.com/KaribuLab/titvo-auth-setup-aws.git"
const titvoTaskCliFilesSource = "https://github.com/KaribuLab/titvo-task-cli-files-aws.git"
const titvoTaskTriggerSource = "https://github.com/KaribuLab/titvo-task-trigger-aws.git"
const titvoTaskStatusSource = "https://github.com/KaribuLab/titvo-task-status-aws.git"
const titvoAgentAWS = "https://github.com/KaribuLab/titvo-agent-aws.git"
const titvoMCPGateway = "https://github.com/KaribuLab/titvo-mcp-gateway.git"
const titvoBitbucketCodeInsightsAWS = "https://github.com/KaribuLab/titvo-bitbucket-code-insights-aws.git"
const titvoGitCommitFilesAWS = "https://github.com/KaribuLab/titvo-git-commit-files-aws.git"
const titvoGithubIssueAWS = "https://github.com/KaribuLab/titvo-github-issue-aws.git"
const titvoIssueReportAWS = "https://github.com/KaribuLab/titvo-issue-report-aws.git"
const titvoInstallerECRPublisherSource = "https://github.com/KaribuLab/titvo-installer-ecr-publisher.git"

type batchJobSpec struct {
	Name    string
	EnvVars map[string]string
}

var downloadSourceFn = downloadSource
var deployInfraFn = deployInfra

func installerECRPublisherJobs(region string) []batchJobSpec {
	return []batchJobSpec{
		{
			Name: "installer-ecr-publisher-agent",
			EnvVars: map[string]string{
				"GIT_URL":    titvoAgentAWS,
				"IMAGE_REPO": "tvo-agent-ecr-prod",
				"REGION":     region,
			},
		},
		{
			Name: "installer-ecr-publisher-mcp-gateway",
			EnvVars: map[string]string{
				"GIT_URL":    titvoMCPGateway,
				"IMAGE_REPO": "tvo-mcp-gateway-ecr-prod",
				"REGION":     region,
			},
		},
	}
}

func DownloadInfraSource(dir string) error {
	return downloadSourceFn(dir, titvoInfraSource, "infra")
}

func DownloadAgentAWSSource(dir string) error {
	return downloadSourceFn(dir, titvoAgentAWS, "agent aws")
}

func DownloadMCPGatewaySource(dir string) error {
	return downloadSourceFn(dir, titvoMCPGateway, "MCP gateway")
}

func DownloadBitbucketCodeInsightsAWSSource(dir string) error {
	return downloadSourceFn(dir, titvoBitbucketCodeInsightsAWS, "bitbucket code insights aws")
}

func DownloadGitCommitFilesAWSSource(dir string) error {
	return downloadSourceFn(dir, titvoGitCommitFilesAWS, "git commit files aws")
}

func DownloadGithubIssueAWSSource(dir string) error {
	return downloadSourceFn(dir, titvoGithubIssueAWS, "github issue aws")
}

func DownloadIssueReportAWSSource(dir string) error {
	return downloadSourceFn(dir, titvoIssueReportAWS, "issue report aws")
}

func DownloadAuthSetupSource(dir string) error {
	return downloadSourceFn(dir, titvoAuthSetupSource, "auth setup")
}

func DownloadTaskCliFilesSource(dir string) error {
	return downloadSourceFn(dir, titvoTaskCliFilesSource, "task cli files")
}

func DownloadTaskTriggerSource(dir string) error {
	return downloadSourceFn(dir, titvoTaskTriggerSource, "task trigger")
}

func DownloadTaskStatusSource(dir string) error {
	return downloadSourceFn(dir, titvoTaskStatusSource, "task status")
}

func DownloadInstallerECRPublisherSource(dir string) error {
	return downloadSourceFn(dir, titvoInstallerECRPublisherSource, "installer ecr publisher")
}

type DeployConfig struct {
	AWSCredentials        AWSCredentials
	InstallToolConfig     InstallToolConfig
	VPCID                 string
	PrivateSubnetCIDR     string
	AvailabilityZone      string
	NatGatewayID          string
	AESSecret             string
	BitbucketClientKey    string
	BitbucketClientSecret string
	GithubAccessToken     string
	Debug                 bool
}

func DeployInfra(config DeployConfig) error {
	return deployInfraFn(config)
}
