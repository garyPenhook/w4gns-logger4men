package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeRcloneScript is a minimal stand-in for the real rclone binary,
// implementing just the three subcommands runBackup/pruneRemoteBackups use
// (copyto, lsf --include, deletefile) against a plain local directory
// instead of an actual cloud remote. This lets tests exercise runBackup's
// real upload/retention logic end-to-end without any network access or
// rclone configuration — the "remote" is just $W4GNS_TEST_RCLONE_DIR, and
// $W4GNS_TEST_RCLONE_FAIL, if set to a filename (or "*" for any), makes
// copyto fail for that name so partial-upload-failure paths are testable.
const fakeRcloneScript = `#!/bin/sh
set -e
dir="$W4GNS_TEST_RCLONE_DIR"
mkdir -p "$dir"
cmd="$1"; shift
case "$cmd" in
  copyto)
    src="$1"; dst="$2"
    name="${dst##*/}"
    if [ "$W4GNS_TEST_RCLONE_FAIL" = "*" ]; then
      echo "simulated rclone failure for $name" >&2
      exit 1
    elif [ -n "$W4GNS_TEST_RCLONE_FAIL" ]; then
      case "$name" in
        *"$W4GNS_TEST_RCLONE_FAIL"*)
          echo "simulated rclone failure for $name" >&2
          exit 1
          ;;
      esac
    fi
    cp "$src" "$dir/$name"
    ;;
  lsf)
    shift # remote path argument, unused: the fake always operates on $dir
    pattern=""
    while [ $# -gt 0 ]; do
      case "$1" in
        --include) pattern="$2"; shift 2 ;;
        *) shift ;;
      esac
    done
    regex=$(printf '%s' "$pattern" | sed -e 's/[.]/\\./g' -e 's/[*]/.*/g')
    ( cd "$dir" && ls -1 2>/dev/null | grep -E "^${regex}\$" ) || true
    ;;
  deletefile)
    target="$1"
    name="${target##*/}"
    rm -f "$dir/$name"
    ;;
  *)
    echo "fake rclone: unsupported command $cmd" >&2
    exit 1
    ;;
esac
`

// newFakeRclone installs fakeRcloneScript as the only "rclone" on PATH and
// returns the local directory it uses as its fake remote, so tests can
// inspect uploaded files directly. Set t.Setenv("W4GNS_TEST_RCLONE_FAIL",
// name) before calling runBackup to simulate an upload failure for that
// filename.
func newFakeRclone(t *testing.T) (remoteDir string) {
	t.Helper()
	binDir := t.TempDir()
	scriptPath := filepath.Join(binDir, "rclone")
	if err := os.WriteFile(scriptPath, []byte(fakeRcloneScript), 0o755); err != nil {
		t.Fatalf("write fake rclone script: %v", err)
	}
	// Prepend so the fake rclone wins the lookup, while the script's own use
	// of mkdir/cp/ls/grep/rm still resolves against the real PATH.
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	remoteDir = t.TempDir()
	t.Setenv("W4GNS_TEST_RCLONE_DIR", remoteDir)
	return remoteDir
}

// TestBackupMuSerializesConcurrentBackups is a white-box test of the
// serialization primitive runBackupSerialized relies on: at most one backup
// (F8-triggered or the mandatory exit backup) may run its critical section at
// a time, so second-resolution backup filenames never collide and the exit
// backup never races an in-flight F8 backup.
func TestBackupMuSerializesConcurrentBackups(t *testing.T) {
	var active, maxActive int32
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			backupMu.Lock()
			defer backupMu.Unlock()
			n := atomic.AddInt32(&active, 1)
			for {
				observed := atomic.LoadInt32(&maxActive)
				if n <= observed || atomic.CompareAndSwapInt32(&maxActive, observed, n) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&active, -1)
		}()
	}
	wg.Wait()
	if maxActive > 1 {
		t.Fatalf("backupMu allowed %d concurrent critical sections, want at most 1", maxActive)
	}
}

// TestRunBackupSerializedReportsMissingRclone covers the ordinary failure
// path (no rclone in PATH, as in this test environment) without touching the
// network: it must return a descriptive error rather than panicking or
// hanging, and must always release backupMu.
func TestRunBackupSerializedReportsMissingRclone(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // guarantees rclone is not found
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := runBackupSerialized(context.Background(), st, profile.ID); err == nil {
		t.Fatal("expected an error when rclone is not in PATH")
	}

	// backupMu must have been released; a second call should not hang.
	done := make(chan struct{})
	go func() {
		runBackupSerialized(context.Background(), st, profile.ID)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runBackupSerialized did not release backupMu after returning")
	}
}

// TestRunBackupUploadsSnapshotAndReturnsMatchingNames covers the real
// success path end-to-end (VACUUM INTO snapshot, ADIF export, and both
// uploads) against a fake rclone, since the previous tests only covered the
// no-rclone-in-PATH failure path.
func TestRunBackupUploadsSnapshotAndReturnsMatchingNames(t *testing.T) {
	remoteDir := newFakeRclone(t)
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}
	q := validTestQSO()
	q.profileID = profile.ID
	if _, err := st.insertQSO(q); err != nil {
		t.Fatalf("insertQSO: %v", err)
	}

	result, err := runBackupSerialized(context.Background(), st, profile.ID)
	if err != nil {
		t.Fatalf("runBackupSerialized returned error: %v", err)
	}

	dbPath := filepath.Join(remoteDir, result.dbName)
	if info, err := os.Stat(dbPath); err != nil || info.Size() == 0 {
		t.Fatalf("uploaded database backup %s missing or empty: %v", dbPath, err)
	}
	adifPath := filepath.Join(remoteDir, result.adifName)
	adifBytes, err := os.ReadFile(adifPath)
	if err != nil {
		t.Fatalf("uploaded ADIF backup %s missing: %v", adifPath, err)
	}
	if !strings.Contains(string(adifBytes), "W1AW") {
		t.Errorf("uploaded ADIF backup = %q, want it to contain the logged QSO's call", adifBytes)
	}
}

// TestRunBackupReturnsErrorOnPartialUploadFailure covers the case the
// review flagged as lightly tested: if the database snapshot uploads but
// the ADIF upload fails, runBackup must report the failure (not silently
// succeed), and the database upload that did succeed is left in place
// rather than rolled back — documenting the actual, current behavior.
func TestRunBackupReturnsErrorOnPartialUploadFailure(t *testing.T) {
	remoteDir := newFakeRclone(t)
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("W4GNS_TEST_RCLONE_FAIL", "*") // fail every copyto
	if _, err := runBackupSerialized(context.Background(), st, profile.ID); err == nil {
		t.Fatal("runBackupSerialized returned no error despite every upload failing")
	}
	entries, err := os.ReadDir(remoteDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("fake remote has %d entries after a fully-failed backup, want 0", len(entries))
	}

	// Now let the database upload succeed but the ADIF upload fail (runBackup
	// uploads dbStaged before adifStaged): the fake matches by substring, so
	// ".adi" fails only the ADIF file regardless of its timestamp.
	t.Setenv("W4GNS_TEST_RCLONE_FAIL", ".adi")
	if _, err := runBackupSerialized(context.Background(), st, profile.ID); err == nil {
		t.Fatal("runBackupSerialized returned no error despite the ADIF upload failing")
	}
	entries, err = os.ReadDir(remoteDir)
	if err != nil {
		t.Fatal(err)
	}
	var sawDB, sawADIF bool
	for _, e := range entries {
		switch {
		case strings.HasSuffix(e.Name(), ".db"):
			sawDB = true
		case strings.HasSuffix(e.Name(), ".adi"):
			sawADIF = true
		}
	}
	if !sawDB {
		t.Error("the database upload, which should have succeeded, is missing from the fake remote")
	}
	if sawADIF {
		t.Error("the deliberately-failed ADIF upload exists in the fake remote")
	}
}

// TestBackupExportsFromSnapshotNotLiveDatabase guards against the DB and
// ADIF halves of one backup representing different states: VACUUM INTO
// takes a point-in-time snapshot, but the UI isn't blocked while a backup
// runs in the background, so exporting ADIF from the live database instead
// of that snapshot could pick up a QSO logged in between — a .adi file
// that doesn't match the .db snapshot uploaded alongside it. This exercises
// the exact mechanism runBackup now uses: open the staged snapshot file
// (not the live *store) and export from that.
func TestBackupExportsFromSnapshotNotLiveDatabase(t *testing.T) {
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}
	before := validTestQSO()
	before.call, before.profileID = "W1AW", profile.ID
	if _, err := st.insertQSO(before); err != nil {
		t.Fatal(err)
	}

	snapshotPath := filepath.Join(t.TempDir(), "snapshot.db")
	if _, err := st.db.Exec(`VACUUM INTO ?`, snapshotPath); err != nil {
		t.Fatal(err)
	}

	// Simulate a QSO logged in the window between VACUUM INTO and the ADIF
	// export — this must not appear in a backup taken from the snapshot.
	after := validTestQSO()
	after.call, after.profileID = "K1ABC", profile.ID
	if _, err := st.insertQSO(after); err != nil {
		t.Fatal(err)
	}

	snapshotDB, err := sql.Open("sqlite", snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshotDB.Close()
	snapshotStore := &store{db: snapshotDB}

	var adif strings.Builder
	if _, err := exportADIF(context.Background(), &adif, profile.ID, snapshotStore); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(adif.String(), "W1AW") {
		t.Error("snapshot export is missing the QSO logged before VACUUM INTO")
	}
	if strings.Contains(adif.String(), "K1ABC") {
		t.Error("snapshot export includes a QSO logged after VACUUM INTO — DB and ADIF backups would disagree")
	}
}

// TestPruneRemoteBackupsKeepsOnlyMostRecent covers the retention policy in
// isolation, pre-seeding more than backupKeepCount fake remote files instead
// of relying on real time passing between repeated runBackup calls (which
// would need second-resolution filename gaps to avoid collisions).
func TestPruneRemoteBackupsKeepsOnlyMostRecent(t *testing.T) {
	remoteDir := newFakeRclone(t)
	rclonePath, err := exec.LookPath("rclone")
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for i := 0; i < backupKeepCount+3; i++ {
		name := fmt.Sprintf("w4gns-2026083%d-120000.db", i)
		names = append(names, name)
		if err := os.WriteFile(filepath.Join(remoteDir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A different file type must be left untouched by a .db-pattern prune.
	if err := os.WriteFile(filepath.Join(remoteDir, "w4gns-20260830-120000.adi"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := pruneRemoteBackups(context.Background(), rclonePath, backupRemote+":"+backupRemoteDir, "w4gns-*.db"); err != nil {
		t.Fatalf("pruneRemoteBackups returned error: %v", err)
	}

	entries, err := os.ReadDir(remoteDir)
	if err != nil {
		t.Fatal(err)
	}
	var remainingDB []string
	var sawADIF bool
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".adi") {
			sawADIF = true
			continue
		}
		remainingDB = append(remainingDB, e.Name())
	}
	if !sawADIF {
		t.Error("pruning *.db deleted the unrelated .adi file")
	}
	if len(remainingDB) != backupKeepCount {
		t.Fatalf("remaining .db backups = %d, want %d", len(remainingDB), backupKeepCount)
	}
	sort.Strings(names)
	wantKept := names[len(names)-backupKeepCount:]
	sort.Strings(remainingDB)
	sort.Strings(wantKept)
	for i := range wantKept {
		if remainingDB[i] != wantKept[i] {
			t.Fatalf("remaining .db backups = %v, want the %d most recent: %v", remainingDB, backupKeepCount, wantKept)
		}
	}
}
