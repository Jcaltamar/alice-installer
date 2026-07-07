package asterisk

import (
	"fmt"
	"sort"
	"strings"
)

const (
	managedBeginFormat = "; BEGIN alice-installer managed: %s"
	managedEndFormat   = "; END alice-installer managed: %s"
)

func NormalizeContract(contract AMIContract, configRoot string) AMIContract {
	if configRoot == "" {
		configRoot = DefaultConfigRoot
	}
	if contract.Host == "" {
		contract.Host = DefaultAMIHost
	}
	if contract.Port == 0 {
		contract.Port = DefaultAMIPort
	}
	if contract.ConfigDir == "" {
		contract.ConfigDir = configRoot
	}
	contract.Enabled = true
	return contract
}

func RenderDotEnv(contract AMIContract) string {
	return renderAMIEnv(contract)
}

func RenderComposeEnv(contract AMIContract) string {
	return renderAMIEnv(contract)
}

func RenderIntegrationEnv(contract AMIContract) string {
	return renderAMIEnv(contract)
}

func renderAMIEnv(contract AMIContract) string {
	contract = NormalizeContract(contract, contract.ConfigDir)
	values := map[string]string{
		"ASTERISK_ENABLED":         fmt.Sprintf("%t", contract.Enabled),
		"ASTERISK_AMI_HOST":        contract.Host,
		"ASTERISK_AMI_PORT":        fmt.Sprintf("%d", contract.Port),
		"ASTERISK_AMI_USERNAME":    contract.Username,
		"ASTERISK_AMI_PASSWORD":    contract.Password,
		"ASTERISK_CONFIG_DIR":      contract.ConfigDir,
		"ASTERISK_INTEGRATION_ENV": contract.ConfigDir + "/integration.env",
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(values[key])
		builder.WriteByte('\n')
	}
	return builder.String()
}

func RenderManagerAMISection(contract AMIContract) string {
	contract = NormalizeContract(contract, contract.ConfigDir)
	return fmt.Sprintf(`[general]
enabled=yes
webenabled=no
bindaddr=%s
port=%d

[%s]
; username=%s
secret=%s
read=system,call,log,verbose,command,agent,user,config,dtmf,reporting,cdr,dialplan,originate
write=system,call,command,agent,user,config,originate
`, contract.Host, contract.Port, contract.Username, contract.Username, contract.Password)
}

func RenderExtensionsSection() string {
	return `[alice-guardian]
; Reserved context for Alice Guardian managed SIP resources.
`
}

func ReplaceManagedSection(original, name, body string) string {
	begin := fmt.Sprintf(managedBeginFormat, name)
	end := fmt.Sprintf(managedEndFormat, name)
	section := begin + "\n" + strings.TrimRight(body, "\n") + "\n" + end + "\n"

	start := strings.Index(original, begin)
	if start == -1 {
		prefix := original
		if prefix != "" && !strings.HasSuffix(prefix, "\n") {
			prefix += "\n"
		}
		return prefix + section
	}

	endStart := strings.Index(original[start:], end)
	if endStart == -1 {
		prefix := strings.TrimRight(original[:start], "\n")
		if prefix != "" {
			prefix += "\n"
		}
		return prefix + section
	}
	endIndex := start + endStart + len(end)
	if endIndex < len(original) && original[endIndex] == '\n' {
		endIndex++
	}
	return original[:start] + section + original[endIndex:]
}

func RemoveManagedSection(original, name string) string {
	begin := fmt.Sprintf(managedBeginFormat, name)
	end := fmt.Sprintf(managedEndFormat, name)
	start := strings.Index(original, begin)
	if start == -1 {
		return original
	}
	endStart := strings.Index(original[start:], end)
	if endStart == -1 {
		return original
	}
	endIndex := start + endStart + len(end)
	if endIndex < len(original) && original[endIndex] == '\n' {
		endIndex++
	}
	return original[:start] + original[endIndex:]
}
