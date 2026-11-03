package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wubinstu/mihomo-cli/internal/app"
)

// sandbox 把 app 的路径全部指到临时目录 (内部包, 可以直接改包级变量)
func sandbox(t *testing.T) *app.Settings {
	t.Helper()
	dir := t.TempDir()
	app.BaseDir = dir
	app.BinDir = filepath.Join(dir, "bin")
	app.ProfileDir = filepath.Join(dir, "profiles")
	app.RuntimeDir = filepath.Join(dir, "runtime")
	app.VersionsDir = filepath.Join(app.BinDir, "versions")
	app.LogDir = filepath.Join(dir, "logs")
	app.CoreBin = filepath.Join(app.BinDir, "mihomo")
	app.CoreBinOld = filepath.Join(app.BinDir, "mihomo.old")
	app.SettingsFile = filepath.Join(dir, "config.toml")
	app.RuntimeConfig = filepath.Join(app.RuntimeDir, "config.yaml")
	app.LogFile = filepath.Join(app.LogDir, "mihomo.log")
	if err := os.MkdirAll(app.BinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	s := app.DefaultSettings()
	return s
}

// fakeCore 造一个"内核二进制" (VersionShort 只认输出里的版本号, 内容无所谓)
func fakeCore(t *testing.T, ver string) {
	t.Helper()
	if err := os.MkdirAll(app.BinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "#!/bin/sh\necho 'Mihomo Meta " + ver + " linux amd64 with go1.26.8'\n"
	if err := os.WriteFile(app.CoreBin, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// upgrade 模拟一次换版本: 老版本入栈(带停用时刻), 新二进制就位
func upgrade(t *testing.T, s *app.Settings, ver, flavor string) {
	t.Helper()
	if err := pushHistory(s, ver, flavor, "amd64"); err != nil {
		t.Fatal(err)
	}
	fakeCore(t, ver)
}

func TestPushHistoryRecordsStopTime(t *testing.T) {
	s := sandbox(t)
	fakeCore(t, "v1.19.30")
	upgrade(t, s, "v1.19.31", "")

	h := History()
	if len(h) != 1 {
		t.Fatalf("want 1 history entry, got %d", len(h))
	}
	if h[0].Version != "v1.19.30" {
		t.Errorf("history[0].Version = %q, want v1.19.30", h[0].Version)
	}
	if h[0].StoppedAt.IsZero() {
		t.Error("StoppedAt must be recorded")
	}
	if _, err := os.Stat(filepath.Join(app.VersionsDir, "v1.19.30", "mihomo")); err != nil {
		t.Errorf("old binary must be archived for offline rollback: %v", err)
	}
}

// rollback 必须是往返的: 回滚之后被换下来的版本也要在栈里, 否则再也回不去,
// 而且它的二进制还留在 bin/versions/ 下没人管 (孤儿)。
func TestRollbackIsAToggle(t *testing.T) {
	s := sandbox(t)
	fakeCore(t, "v1.19.31")
	upgrade(t, s, "v1.19.32", "") // 31 -> 32, 栈: [31]

	if v, err := Rollback(); err != nil || v != "v1.19.31" {
		t.Fatalf("Rollback() = %q, %v; want v1.19.31, nil", v, err)
	}
	if got := VersionShort(); got != "v1.19.31" {
		t.Errorf("current version = %q, want v1.19.31", got)
	}
	h := History()
	if len(h) != 1 || h[0].Version != "v1.19.32" {
		t.Fatalf("after rollback history = %+v, want [v1.19.32]", h)
	}
	if _, err := os.Stat(filepath.Join(app.VersionsDir, "v1.19.32", "mihomo")); err != nil {
		t.Errorf("the version we just left must stay archived: %v", err)
	}

	// 再 rollback 一次就回到 32
	if v, err := Rollback(); err != nil || v != "v1.19.32" {
		t.Fatalf("second Rollback() = %q, %v; want v1.19.32, nil", v, err)
	}
	h = History()
	if len(h) != 1 || h[0].Version != "v1.19.31" {
		t.Fatalf("after second rollback history = %+v, want [v1.19.31]", h)
	}
}

// 栈顶的存档丢了要能跳过它看下一个 (rollback 递归弹)
func TestRollbackSkipsMissingArchive(t *testing.T) {
	s := sandbox(t)
	fakeCore(t, "v1.19.29")
	upgrade(t, s, "v1.19.30", "")
	upgrade(t, s, "v1.19.31", "")
	if err := os.RemoveAll(filepath.Join(app.VersionsDir, "v1.19.30")); err != nil {
		t.Fatal(err)
	}
	if v, err := Rollback(); err != nil || v != "v1.19.29" {
		t.Fatalf("Rollback() = %q, %v; want v1.19.29, nil", v, err)
	}
	for _, h := range History() {
		if h.Version == "v1.19.30" {
			t.Error("entry with a missing archive must be dropped from the stack")
		}
	}
}

// MaxHistory 之外的旧版本要连存档一起删 (否则磁盘只涨不减)
func TestHistoryTrimsArchive(t *testing.T) {
	s := sandbox(t)
	for _, v := range []string{"v1.19.25", "v1.19.26", "v1.19.27", "v1.19.28", "v1.19.29"} {
		fakeCore(t, v)
		if err := pushHistory(s, "next", "", "amd64"); err != nil {
			t.Fatal(err)
		}
		fakeCore(t, "next")
		if err := pushHistory(s, v, "", "amd64"); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(History()); got != MaxHistory {
		t.Errorf("len(History()) = %d, want %d", got, MaxHistory)
	}
	if _, err := os.Stat(filepath.Join(app.VersionsDir, "v1.19.25")); err == nil {
		t.Error("archive of a trimmed version must be deleted")
	}
}
