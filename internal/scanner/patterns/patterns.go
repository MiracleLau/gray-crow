package patterns

import "regexp"

type Pattern struct {
	ID       string
	Regex    string
	Severity string
	re       *regexp.Regexp
}

// 通用子集（gitleaks 风格，MIT）：覆盖主流云厂商与服务商的长随机凭据。
// 编译进二进制而非 embed 资源，避免运行期被篡改。
var All = []Pattern{
	{ID: "aws-access-key-id", Severity: "high", Regex: `\b(A3T[A-Z0-9]|AKIA|AGPA|AIDA|AROA|AIPA|ANPA|ANVA|ASIA)[A-Z0-9]{16}\b`},
	{ID: "aws-secret-access-key", Severity: "high", Regex: `(?i)aws.{0,20}(secret|private).{0,20}["'][0-9a-zA-Z/+]{40}["']`},
	{ID: "github-pat", Severity: "high", Regex: `\bghp_[0-9a-zA-Z]{36}\b`},
	{ID: "github-oauth", Severity: "high", Regex: `\bgho_[0-9a-zA-Z]{36}\b`},
	{ID: "github-app-token", Severity: "high", Regex: `\b(ghu|ghs)_[0-9a-zA-Z]{36}\b`},
	{ID: "github-refresh-token", Severity: "high", Regex: `\bghr_[0-9a-zA-Z]{76}\b`},
	{ID: "github-fine-grained-pat", Severity: "high", Regex: `\bgithub_pat_[0-9a-zA-Z_]{82}\b`},
	{ID: "google-api-key", Severity: "high", Regex: `\bAIza[0-9A-Za-z\-_]{35}\b`},
	{ID: "google-oauth-client-secret", Severity: "high", Regex: `\bGOCSPX-[0-9A-Za-z\-_]{28}\b`},
	{ID: "gcp-service-account", Severity: "high", Regex: `"type"\s*:\s*"service_account"`},
	{ID: "slack-token", Severity: "high", Regex: `\bxox[baprs]-[0-9a-zA-Z-]{10,72}\b`},
	{ID: "slack-webhook", Severity: "high", Regex: `https://hooks\.slack\.com/services/T[0-9A-Za-z_]{8,}/B[0-9A-Za-z_]{8,}/[0-9A-Za-z_]{20,}`},
	{ID: "stripe-key", Severity: "high", Regex: `\b(sk|rk)_(test|live)_[0-9a-zA-Z]{24,}\b`},
	{ID: "private-key", Severity: "high", Regex: `-----BEGIN (RSA|DSA|EC|OPENSSH|PGP|ENCRYPTED) PRIVATE KEY-----`},
	{ID: "db-connection-string", Severity: "high", Regex: `(?i)(mysql|mariadb|postgres|postgresql|mongodb(\+srv)?|redis|amqp)://[^:\s/]+:[^@\s/]+@[^\s"']+`},
	{ID: "anthropic-api-key", Severity: "high", Regex: `\bsk-ant-[A-Za-z0-9_\-]{20,}\b`},
	{ID: "openai-api-key", Severity: "high", Regex: `\bsk-(proj-)?[A-Za-z0-9_\-]{20,}\b`},
	{ID: "npm-token", Severity: "high", Regex: `\bnpm_[A-Za-z0-9]{36}\b`},
	{ID: "pypi-token", Severity: "high", Regex: `\bpypi-AgEIcHlwaS5vcmc[A-Za-z0-9_\-]{40,}\b`},
	{ID: "sendgrid-api-key", Severity: "high", Regex: `\bSG\.[A-Za-z0-9_\-]{22}\.[A-Za-z0-9_\-]{43}\b`},
	{ID: "discord-webhook", Severity: "high", Regex: `https://(discord|discordapp)\.com/api/webhooks/[0-9]+/[A-Za-z0-9_\-]+`},
	{ID: "telegram-bot-token", Severity: "high", Regex: `\b[0-9]{8,10}:AA[A-Za-z0-9_\-]{33}\b`},
	{ID: "digitalocean-pat", Severity: "high", Regex: `\bdop_v1_[a-f0-9]{64}\b`},
	{ID: "shopify-access-token", Severity: "high", Regex: `\bshp(at|ca|pa|ss)_[a-fA-F0-9]{32}\b`},
	{ID: "azure-storage-key", Severity: "high", Regex: `(?i)AccountKey=[A-Za-z0-9+/=]{88}`},
	{ID: "jwt", Severity: "medium", Regex: `\beyJ[A-Za-z0-9_\-]{10,}\.eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\b`},
	{ID: "mailgun-api-key", Severity: "medium", Regex: `\bkey-[0-9a-zA-Z]{32}\b`},
	{ID: "mailchimp-api-key", Severity: "medium", Regex: `\b[0-9a-f]{32}-us[0-9]{1,2}\b`},
	{ID: "twilio-api-key", Severity: "medium", Regex: `\bSK[0-9a-fA-F]{32}\b`},
	{ID: "discord-bot-token", Severity: "medium", Regex: `\b[MN][A-Za-z\d]{23}\.[\w-]{6}\.[\w-]{27}\b`},
	{ID: "heroku-api-key", Severity: "medium", Regex: `(?i)heroku.{0,20}[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`},
	{ID: "facebook-access-token", Severity: "medium", Regex: `\bEAACEdEose0cBA[0-9A-Za-z]+\b`},
	{ID: "generic-credential", Severity: "medium", Regex: `(?i)\b(password|passwd|pwd|secret|token|api_?key|access_?key|auth_?token)\b\s*[:=]\s*["']?[^\s"']{8,}`},
}

func init() {
	for i := range All {
		All[i].re = regexp.MustCompile(All[i].Regex)
	}
}

// Locate 返回规则在文本中的命中位置，用于截取上下文。
func (p *Pattern) Locate(text string) (int, int, bool) {
	loc := p.re.FindStringIndex(text)
	if loc == nil {
		return 0, 0, false
	}
	return loc[0], loc[1], true
}
