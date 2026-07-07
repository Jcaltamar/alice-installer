package asterisk

import (
	"strings"
	"testing"
)

func TestAMIContractRendersSharedCredentialsEverywhere(t *testing.T) {
	t.Parallel()

	contract := AMIContract{
		Enabled:   true,
		Host:      "127.0.0.1",
		Port:      5038,
		Username:  "guardian_ami",
		Password:  "same-secret",
		ConfigDir: "/opt/alice-config/asterisk",
	}

	renderers := map[string]string{
		"dot env":         RenderDotEnv(contract),
		"compose env":     RenderComposeEnv(contract),
		"integration env": RenderIntegrationEnv(contract),
	}

	for name, rendered := range renderers {
		rendered := rendered
		t.Run(name, func(t *testing.T) {
			for _, want := range []string{
				"ASTERISK_AMI_HOST=127.0.0.1",
				"ASTERISK_AMI_PORT=5038",
				"ASTERISK_AMI_USERNAME=guardian_ami",
				"ASTERISK_AMI_PASSWORD=same-secret",
				"ASTERISK_CONFIG_DIR=/opt/alice-config/asterisk",
			} {
				if !strings.Contains(rendered, want) {
					t.Fatalf("%s did not contain %q:\n%s", name, want, rendered)
				}
			}
		})
	}
}

func TestManagedSectionReplacementPreservesOperatorContent(t *testing.T) {
	t.Parallel()

	original := "[operator]\nkeep=yes\n; BEGIN alice-installer managed: ami\nold=true\n; END alice-installer managed: ami\n[tail]\nvalue=1\n"
	updated := ReplaceManagedSection(original, "ami", "[alice]\nsecret=new\n")

	for _, want := range []string{"[operator]\nkeep=yes", "[tail]\nvalue=1", "[alice]\nsecret=new"} {
		if !strings.Contains(updated, want) {
			t.Fatalf("updated config should contain %q, got:\n%s", want, updated)
		}
	}
	if strings.Contains(updated, "old=true") {
		t.Fatalf("managed section should be replaced, got:\n%s", updated)
	}
}
