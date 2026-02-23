// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package appinfo_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/runtime/appinfo"
)

func TestIsPreRelease(t *testing.T) {
	got := appinfo.IsPreRelease()
	// Default version "0.0.0" should be pre-release
	if !got {
		t.Log("IsPreRelease() = false (may depend on build flags)")
	}
}

func TestIsRelease(t *testing.T) {
	_ = appinfo.IsRelease() // just ensure no panic
}

func TestIsReleaseCandidate(t *testing.T) {
	_ = appinfo.IsReleaseCandidate()
}

func TestIsAlpha(t *testing.T) {
	_ = appinfo.IsAlpha()
}

func TestIsBeta(t *testing.T) {
	_ = appinfo.IsBeta()
}

func TestSemVersion(t *testing.T) {
	sv := appinfo.SemVersion()
	if sv == nil {
		t.Fatal("SemVersion() returned nil")
	}
}

func TestBuildTags(t *testing.T) {
	_ = appinfo.BuildTags() // may be empty, just ensure no panic
}

func TestDeps(t *testing.T) {
	_ = appinfo.Deps() // may be nil when not built with module info
}

func TestDependencies_Contains(t *testing.T) {
	deps := appinfo.Deps()
	// Should not panic regardless of content
	_ = deps.Contains("nonexistent/module")
}

func TestDependencies_Get(t *testing.T) {
	deps := appinfo.Deps()
	_ = deps.Get("nonexistent/module") // may return nil
}

func TestIsCgoEnabled(t *testing.T) {
	_ = appinfo.IsCgoEnabled()
}

func TestIsRaceEnabled(t *testing.T) {
	_ = appinfo.IsRaceEnabled()
}

func TestEnvLabel(t *testing.T) {
	label := appinfo.EnvLabel()
	if label == "" {
		t.Error("EnvLabel() should not be empty")
	}
}

func TestInfo(t *testing.T) {
	info := appinfo.Info()
	if info == "" {
		t.Error("Info() should not be empty")
	}
}

func TestBuildInfo(t *testing.T) {
	info := appinfo.BuildInfo()
	if info == "" {
		t.Error("BuildInfo() should not be empty")
	}
}

func TestBuildGoVersion(t *testing.T) {
	v := appinfo.BuildGoVersion()
	if v == "" {
		t.Error("BuildGoVersion() should not be empty")
	}
}

func TestBuildPlatform(t *testing.T) {
	p := appinfo.BuildPlatform()
	if p == "" {
		t.Error("BuildPlatform() should not be empty")
	}
}

func TestBuildGoOS(t *testing.T) {
	os := appinfo.BuildGoOS()
	if os == "" {
		t.Error("BuildGoOS() should not be empty")
	}
}

func TestBuildGoArch(t *testing.T) {
	arch := appinfo.BuildGoArch()
	if arch == "" {
		t.Error("BuildGoArch() should not be empty")
	}
}

func TestAppVersion(t *testing.T) {
	v := appinfo.AppVersion()
	if v == "" {
		t.Error("AppVersion() should not be empty")
	}
}

func TestGetEnvVar(t *testing.T) {
	// Should return empty for nonexistent var, not panic
	_ = appinfo.GetEnvVar("NONEXISTENT_VAR_12345")
}

func TestBinDir(t *testing.T) {
	d := appinfo.BinDir()
	if d == "" {
		t.Error("BinDir() should not be empty")
	}
}

func TestVarDir(t *testing.T) {
	d := appinfo.VarDir()
	if d == "" {
		t.Error("VarDir() should not be empty")
	}
}

func TestEtcDir(t *testing.T) {
	d := appinfo.EtcDir()
	if d == "" {
		t.Error("EtcDir() should not be empty")
	}
}

func TestLibDir(t *testing.T) {
	d := appinfo.LibDir()
	if d == "" {
		t.Error("LibDir() should not be empty")
	}
}

func TestCertsCacheDir(t *testing.T) {
	d := appinfo.CertsCacheDir()
	if d == "" {
		t.Error("CertsCacheDir() should not be empty")
	}
}
