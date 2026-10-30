package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	agenteval "SWPUCAT/internal/agent/eval"
)

func TestRunValidateText(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"-mode=validate", "-role=member", "-format=text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	for _, fragment := range []string{
		"模式: validate",
		"用例: 18/54（角色过滤: member）",
		"工具覆盖（必需调用用例数）",
		"仅验证数据集与确定性评分器，不代表模型质量通过",
	} {
		if !strings.Contains(stdout.String(), fragment) {
			t.Errorf("stdout = %q, want fragment %q", stdout.String(), fragment)
		}
	}
}

func TestRunValidateJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"-mode", "validate", "-role", "captain", "-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, want 0; stderr=%q", code, stderr.String())
	}
	var report agenteval.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode JSON report: %v; output=%q", err, stdout.String())
	}
	if report.Mode != "validate" || report.Role != "captain" || report.SuiteCases != 54 || report.SelectedCases != 18 || !report.Success {
		t.Fatalf("JSON report = %#v", report)
	}
	if report.Coverage.Roles["captain"] != 18 || report.ValidationNotice == "" {
		t.Fatalf("JSON coverage/notice = %#v/%q", report.Coverage.Roles, report.ValidationNotice)
	}
}

func TestRunRejectsInvalidArgumentsWithStableExitCodes(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantCode     int
		wantError    string
		jsonErrorOut bool
	}{
		{name: "mode", args: []string{"-mode=preview"}, wantCode: 2, wantError: "-mode 必须是 validate 或 live"},
		{name: "role", args: []string{"-role=owner"}, wantCode: 1, wantError: "role must be one of"},
		{name: "format", args: []string{"-format=yaml"}, wantCode: 2, wantError: "-format 必须是 text 或 json"},
		{name: "repeat", args: []string{"-repeat=0"}, wantCode: 2, wantError: "-repeat 必须至少为 1"},
		{name: "positional", args: []string{"unexpected"}, wantCode: 2, wantError: "不接受位置参数"},
		{name: "role JSON", args: []string{"-role=owner", "-format=json"}, wantCode: 1, wantError: `"success":false`, jsonErrorOut: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := run(tt.args, &stdout, &stderr)
			if code != tt.wantCode {
				t.Fatalf("run(%v) code = %d, want %d; stderr=%q", tt.args, code, tt.wantCode, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), tt.wantError) {
				t.Fatalf("stderr = %q, want fragment %q", stderr.String(), tt.wantError)
			}
			if tt.jsonErrorOut {
				var payload struct {
					Success bool   `json:"success"`
					Error   string `json:"error"`
				}
				if err := json.Unmarshal(stderr.Bytes(), &payload); err != nil {
					t.Fatalf("decode JSON error: %v; stderr=%q", err, stderr.String())
				}
				if payload.Success || payload.Error == "" {
					t.Fatalf("JSON error payload = %#v", payload)
				}
			}
		})
	}
}

func TestValidateIgnoresBrokenLLMEnvironmentButLiveFailsBeforeNetwork(t *testing.T) {
	t.Setenv("CSA_LLM_TIMEOUT", "not-a-duration")
	t.Setenv("CSA_LLM_PROVIDER", "openai_compatible")
	t.Setenv("CSA_LLM_MODEL", "")
	t.Setenv("CSA_LLM_API_KEY", "")
	t.Setenv("CSA_LLM_BASE_URL", "")

	var validateOut bytes.Buffer
	var validateErr bytes.Buffer
	if code := run([]string{"-mode=validate"}, &validateOut, &validateErr); code != 0 {
		t.Fatalf("validate code = %d, want 0; stderr=%q", code, validateErr.String())
	}
	if !strings.Contains(validateOut.String(), "数据集验证通过") {
		t.Fatalf("validate stdout = %q, want success", validateOut.String())
	}

	var liveOut bytes.Buffer
	var liveErr bytes.Buffer
	if code := run([]string{"-mode=live", "-role=member", "-repeat=1"}, &liveOut, &liveErr); code == 0 {
		t.Fatalf("live code = 0, want non-zero with missing model config; stdout=%q", liveOut.String())
	}
	if liveOut.Len() != 0 {
		t.Fatalf("live stdout = %q, want empty", liveOut.String())
	}
	if !strings.Contains(liveErr.String(), "agent.llm_model is required") {
		t.Fatalf("live stderr = %q, want missing-model validation error", liveErr.String())
	}
}
