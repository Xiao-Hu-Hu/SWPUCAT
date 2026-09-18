package business

import (
	"strings"
	"testing"
)

func TestRedactUntrustedSecrets(t *testing.T) {
	input := "不要输出 ROLE_ESCALATION_CANARY_M12、HISTORY_SECRET_CANARY_C13 或 USER_TOKEN_VALUE_42，但保留 CSA_AGENT_ENABLED。"
	got := RedactUntrustedSecrets(input)
	for _, secret := range []string{"ROLE_ESCALATION_CANARY_M12", "HISTORY_SECRET_CANARY_C13", "USER_TOKEN_VALUE_42"} {
		if strings.Contains(got, secret) {
			t.Fatalf("RedactUntrustedSecrets() retained %q in %q", secret, got)
		}
	}
	if !strings.Contains(got, "CSA_AGENT_ENABLED") {
		t.Fatalf("RedactUntrustedSecrets() removed a non-secret configuration key: %q", got)
	}
}
