package automation

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"farm/internal/dependencies"
)

// parseActADB accepts quoted arguments without invoking the host shell.
func parseActADB(input string) ([]string, error) {
	var args []string
	var word strings.Builder
	var quote rune
	escaped, started := false, false
	for _, c := range input {
		if c == 0 {
			return nil, fmt.Errorf("adb arguments contain NUL")
		}
		if escaped {
			word.WriteRune(c)
			escaped = false
			started = true
			continue
		}
		if c == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			} else {
				word.WriteRune(c)
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			started = true
			continue
		}
		if unicode.IsSpace(c) {
			if started {
				args = append(args, word.String())
				word.Reset()
				started = false
			}
			continue
		}
		word.WriteRune(c)
		started = true
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("adb arguments contain an unfinished quote or escape")
	}
	if started {
		args = append(args, word.String())
	}
	if len(args) == 0 || len(args) > 128 {
		return nil, fmt.Errorf("adb requires 1..128 arguments")
	}
	if args[0] == "adb" || strings.HasPrefix(args[0], "-") {
		return nil, fmt.Errorf("adb params must start with a subcommand, without adb or target options")
	}
	// Global target/server flags are parsed before the adb subcommand.
	// Arguments after shell/exec-out belong to the remote command and may
	// legitimately contain -s/-t or similar options.

	return args, nil
}

type actADBOutput struct {
	text      strings.Builder
	truncated bool
}

func (b *actADBOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := (1 << 20) - b.text.Len()
	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}
	b.text.Write(p)
	return n, nil
}

func (s *Service) runActADB(ctx context.Context, serial string, args []string) (result string, resultErr error) {
	started := time.Now()
	defer func() {
		kind := LogTypeAction
		if resultErr != nil {
			kind = LogTypeError
		}
		s.recordAutomationLogContext(ctx, AutomationLog{Type: kind, Serial: serial, Message: "自动化 ADB 命令完成", Fields: AutomationLogFields("subcommand", args[0], "elapsed_ms", time.Since(started).Milliseconds(), "failed", resultErr != nil)})
	}()
	commandArgs := actADBCommandArgs(args)
	cmd := dependencies.CommandContext(ctx, s.adbPath, append([]string{"-s", serial}, commandArgs...)...)
	cmd.WaitDelay = time.Second
	output := &actADBOutput{}
	// One stream prevents concurrent writes to the bounded buffer.
	cmd.Stdout, cmd.Stderr = output, output
	err := cmd.Run()
	result = output.text.String()
	if output.truncated {
		result += "\n[输出超过 1 MiB，已截断]\n"
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if err != nil {
		return result, fmt.Errorf("adb command failed: %w", err)
	}
	return result, nil
}

// Keep adb shell's own options separate from its quoted remote command.
func actADBCommandArgs(args []string) []string {
	if len(args) < 2 || (args[0] != "shell" && args[0] != "exec-out") {
		return args
	}
	start := 1
	if args[0] == "shell" {
	options:
		for start < len(args) {
			switch args[start] {
			case "-n", "-T", "-t", "-tt", "-x":
				start++
			case "-e":
				if start+1 >= len(args) {
					return args
				}
				start += 2
			default:
				break options
			}
		}
	}
	if start == len(args) {
		return args
	}
	remote := make([]string, 0, len(args)-start)
	for _, arg := range args[start:] {
		remote = append(remote, "'"+strings.ReplaceAll(arg, "'", "'\"'\"'")+"'")
	}
	result := append([]string(nil), args[:start]...)
	return append(result, strings.Join(remote, " "))
}
