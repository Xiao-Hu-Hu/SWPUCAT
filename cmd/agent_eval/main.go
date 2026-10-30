package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	agenteval "SWPUCAT/internal/agent/eval"
	"SWPUCAT/internal/infrastructure/config"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("agent_eval", flag.ContinueOnError)
	flags.SetOutput(stderr)
	mode := flags.String("mode", "validate", "运行模式：validate（离线）或 live（调用配置的 LLM）")
	suite := flags.String("suite", agenteval.DefaultSuite, "内置评测集名称")
	role := flags.String("role", "all", "角色过滤：all、member、captain 或 super_admin")
	repeat := flags.Int("repeat", 1, "live 模式下每个用例的执行次数，至少为 1")
	format := flags.String("format", "text", "输出格式：text 或 json")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "不接受位置参数")
		return 2
	}

	normalizedMode := strings.ToLower(strings.TrimSpace(*mode))
	normalizedFormat := strings.ToLower(strings.TrimSpace(*format))
	if normalizedMode != "validate" && normalizedMode != "live" {
		fmt.Fprintln(stderr, "-mode 必须是 validate 或 live")
		return 2
	}
	if normalizedFormat != "text" && normalizedFormat != "json" {
		fmt.Fprintln(stderr, "-format 必须是 text 或 json")
		return 2
	}
	if *repeat < 1 {
		fmt.Fprintln(stderr, "-repeat 必须至少为 1")
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var (
		report *agenteval.Report
		err    error
	)
	switch normalizedMode {
	case "validate":
		// This branch intentionally does not load application configuration.
		// It is safe in CI without credentials and cannot make network calls.
		report, err = agenteval.ValidateRole(*suite, *role)
	case "live":
		// Live mode is the sole opt-in path that reads CSA_LLM_* and contacts
		// the configured model provider. RunLive replaces all business-server
		// settings with its local deterministic fixture.
		cfg, loadErr := config.Load()
		if loadErr != nil {
			err = fmt.Errorf("加载配置: %w", loadErr)
			break
		}
		report, err = agenteval.RunLive(ctx, *suite, *role, *repeat, cfg.Agent)
	}
	if err != nil {
		writeError(stderr, normalizedFormat, err)
		return 1
	}
	if err := writeReport(stdout, normalizedFormat, report); err != nil {
		fmt.Fprintf(stderr, "输出评测报告: %v\n", err)
		return 1
	}
	if !report.Success {
		return 1
	}
	return 0
}

func writeReport(output io.Writer, format string, report *agenteval.Report) error {
	if format == "json" {
		encoder := json.NewEncoder(output)
		encoder.SetIndent("", "  ")
		encoder.SetEscapeHTML(false)
		return encoder.Encode(report)
	}
	if _, err := fmt.Fprintf(output,
		"模式: %s\n评测集: %s (%s)\n用例: %d/%d（角色过滤: %s）\n",
		report.Mode, report.Suite, report.SuiteID, report.SelectedCases, report.SuiteCases, report.Role,
	); err != nil {
		return err
	}
	for _, line := range []struct {
		label  string
		values map[string]int
	}{
		{label: "角色覆盖", values: report.Coverage.Roles},
		{label: "工具覆盖（必需调用用例数）", values: report.Coverage.Tools},
		{label: "结果覆盖", values: report.Coverage.Outcomes},
		{label: "标签覆盖", values: report.Coverage.Tags},
	} {
		if _, err := fmt.Fprintf(output, "%s: %s\n", line.label, formatCounts(line.values)); err != nil {
			return err
		}
	}
	if report.Mode == "validate" {
		_, err := fmt.Fprintf(output, "结论: 数据集验证通过。%s。\n", report.ValidationNotice)
		return err
	}
	if _, err := fmt.Fprintf(output,
		"执行: %d 次，通过 %d，失败 %d\n",
		len(report.Runs), report.PassedRuns, report.FailedRuns,
	); err != nil {
		return err
	}
	for _, item := range report.Runs {
		if item.Score.Passed {
			continue
		}
		if _, err := fmt.Fprintf(output, "失败 %s#%d: %s\n", item.CaseID, item.Iteration, strings.Join(item.Score.Failures, "；")); err != nil {
			return err
		}
	}
	if report.Success {
		_, err := fmt.Fprintln(output, "结论: live 评测通过")
		return err
	}
	_, err := fmt.Fprintln(output, "结论: live 评测失败")
	return err
}

func writeError(output io.Writer, format string, err error) {
	if format == "json" {
		_ = json.NewEncoder(output).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}
	fmt.Fprintf(output, "agent_eval 失败: %v\n", err)
}

func formatCounts(values map[string]int) string {
	keys := agenteval.SortedCoverageKeys(values)
	if len(keys) == 0 {
		return "无"
	}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, values[key]))
	}
	return strings.Join(parts, ", ")
}
