package agenteval

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	agenttools "SWPUCAT/internal/agent/tools"
)

func TestEmbeddedSuiteIsStrictAndComplete(t *testing.T) {
	suite, err := LoadEmbedded(DefaultSuite)
	if err != nil {
		t.Fatalf("LoadEmbedded() error = %v", err)
	}
	if suite.Manifest.SuiteID != "swpucat-agent-role-eval-v1" {
		t.Fatalf("suite id = %q", suite.Manifest.SuiteID)
	}
	if len(suite.Cases) != 54 {
		t.Fatalf("case count = %d, want 54", len(suite.Cases))
	}
	counts := map[string]int{}
	for _, item := range suite.Cases {
		counts[item.Role]++
	}
	if !reflect.DeepEqual(counts, map[string]int{"member": 18, "captain": 18, "super_admin": 18}) {
		t.Fatalf("role counts = %#v", counts)
	}
}

func TestEvaluationToolContractMatchesProductionRegistry(t *testing.T) {
	registered, err := agenttools.NewRegistry(nil, true).Build(context.Background())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	actual := make([]string, 0, len(registered))
	for _, item := range registered {
		info, err := item.Info(context.Background())
		if err != nil {
			t.Fatalf("Info() error = %v", err)
		}
		actual = append(actual, info.Name)
	}
	expected := ToolNames()
	sort.Strings(actual)
	sort.Strings(expected)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("production tools = %v, eval contract = %v", actual, expected)
	}
}

func TestStrictDecoderRejectsUnknownFields(t *testing.T) {
	files := fstest.MapFS{
		"v1/manifest.json": &fstest.MapFile{Data: []byte(`{
          "schema_version": 1,
          "suite_id": "test",
          "description": "test",
          "expected_case_count": 0,
          "min_cases": 40,
          "max_cases": 60,
          "case_files": [],
          "fixture_file": "business_fixtures.json",
          "role_counts": {},
          "required_tools": [],
          "required_outcomes": [],
          "required_tags": [],
          "unexpected": true
        }`)},
		"v1/business_fixtures.json": &fstest.MapFile{Data: []byte(`{"version":1,"tools":{}}`)},
	}
	_, err := LoadFS(files, "v1")
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("LoadFS() error = %v, want unknown field", err)
	}
}

func TestValidationRejectsMemberTeamToolExpectation(t *testing.T) {
	suite, err := LoadEmbedded(DefaultSuite)
	if err != nil {
		t.Fatal(err)
	}
	clone := *suite
	clone.Cases = append([]Case(nil), suite.Cases...)
	index := -1
	for i := range clone.Cases {
		if clone.Cases[i].Role == "member" {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatal("member case missing")
	}
	clone.Cases[index].AllowedTools = append(append([]string(nil), clone.Cases[index].AllowedTools...), "query_team_members")
	clone.Cases[index].RequiredTools = append(append([]string(nil), clone.Cases[index].RequiredTools...), "query_team_members")
	clone.Cases[index].MaxToolCalls = 8
	err = clone.Validate()
	if err == nil || !strings.Contains(err.Error(), "member cannot") {
		t.Fatalf("Validate() error = %v, want member authorization failure", err)
	}
}

func TestValidationRejectsCrossUserMineFixture(t *testing.T) {
	suite, err := LoadEmbedded(DefaultSuite)
	if err != nil {
		t.Fatal(err)
	}
	clone := *suite
	clone.Cases = append([]Case(nil), suite.Cases...)
	for index := range clone.Cases {
		if clone.Cases[index].ID != "C08" {
			continue
		}
		fixtures := make(map[string]string, len(clone.Cases[index].Fixture))
		for toolName, variant := range clone.Cases[index].Fixture {
			fixtures[toolName] = variant
		}
		fixtures["get_report_materials"] = "mine"
		clone.Cases[index].Fixture = fixtures
		break
	}
	err = clone.Validate()
	if err == nil || !strings.Contains(err.Error(), "mine report 201 author_id=7, want evaluation user 9") {
		t.Fatalf("Validate() error = %v, want cross-user mine fixture failure", err)
	}
}

func TestValidationRequiresKnowledgeRequestRule(t *testing.T) {
	suite, err := LoadEmbedded(DefaultSuite)
	if err != nil {
		t.Fatal(err)
	}
	clone := *suite
	clone.Fixtures = suite.Fixtures
	clone.Fixtures.RequestRules = nil
	err = clone.Validate()
	if err == nil || !strings.Contains(err.Error(), "has no request rule") {
		t.Fatalf("Validate() error = %v, want missing request-rule failure", err)
	}
}
