package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestFindTerminalInvocationUsesPreferredTerminal(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "kitty" {
			return "/usr/bin/kitty", nil
		}
		return "", errors.New("not found")
	}
	invocation, err := findTerminalInvocation("/opt/w4gns-logger", "kitty", lookPath)
	if err != nil {
		t.Fatalf("findTerminalInvocation returned error: %v", err)
	}
	if invocation.program != "/usr/bin/kitty" {
		t.Errorf("program = %q", invocation.program)
	}
	wantArgs := []string{"-e", "/opt/w4gns-logger", terminalChildArg}
	if !reflect.DeepEqual(invocation.args, wantArgs) {
		t.Errorf("args = %#v, want %#v", invocation.args, wantArgs)
	}
}

func TestFindTerminalInvocationFallsBackToSupportedTerminal(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "xterm" {
			return "/usr/bin/xterm", nil
		}
		return "", errors.New("not found")
	}
	invocation, err := findTerminalInvocation("/opt/w4gns-logger", "unknown", lookPath)
	if err != nil {
		t.Fatalf("findTerminalInvocation returned error: %v", err)
	}
	if invocation.program != "/usr/bin/xterm" {
		t.Errorf("program = %q", invocation.program)
	}
}

func TestFindTerminalInvocationReportsMissingTerminal(t *testing.T) {
	_, err := findTerminalInvocation("/opt/w4gns-logger", "", func(string) (string, error) {
		return "", errors.New("not found")
	})
	if err == nil {
		t.Fatal("findTerminalInvocation succeeded, want error")
	}
}

func TestHasArg(t *testing.T) {
	if !hasArg([]string{"--foo", terminalChildArg}, terminalChildArg) {
		t.Fatal("hasArg did not find child marker")
	}
	if hasArg([]string{"--foo"}, terminalChildArg) {
		t.Fatal("hasArg found missing marker")
	}
}
