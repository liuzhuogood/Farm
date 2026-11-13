package automation

import (
	"bytes"
	"context"
	"errors"
	"github.com/Eyevinn/mp4ff/mp4"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The fake ADB executes production device shell scripts against an isolated
// simulated /sdcard/Farm tree; no computer destination is accepted by act.
func artifactTestService(t *testing.T) (*Service, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix fake device")
	}
	dir := t.TempDir()
	var shot bytes.Buffer
	if err := png.Encode(&shot, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "screen.png"), shot.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	var movie bytes.Buffer
	moov := mp4.NewMoovBox()
	moov.AddChild(mp4.CreateMvhd())
	track := mp4.CreateEmptyTrak(1, 90000, "video", "und")
	track.Mdia.Minf.Stbl.Stsz.SampleNumber = 1
	track.Mdia.Minf.Stbl.Stsz.SampleUniformSize = 1
	track.Mdia.Minf.Stbl.Stts.SampleCount = []uint32{1}
	track.Mdia.Minf.Stbl.Stts.SampleTimeDelta = []uint32{90000}
	moov.AddChild(track)
	for _, box := range []mp4.Box{mp4.CreateFtyp(), moov, &mp4.MdatBox{Data: []byte{1}}} {
		if err := box.Encode(&movie); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "movie.mp4"), movie.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FARM_ARTIFACT_FIXTURE", dir)
	t.Setenv("FARM_ARTIFACT_MODE", "success")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	helper := `#!/usr/bin/env python3
import os,sys,shutil,time,subprocess
base=os.environ['FARM_ARTIFACT_FIXTURE']; kind=os.path.basename(sys.argv[0]); mode=os.environ['FARM_ARTIFACT_MODE']
if kind=='adb':
    args=sys.argv[1:]
    with open(os.path.join(base,'commands'),'a') as f: f.write(' '.join(args)+'\n')
    remote=args[4:] if args[2]=='shell' and len(args)>3 and args[3]=='-T' else args[3:]
    script=' '.join(remote).replace('/sdcard/Farm',os.path.join(base,'device'))
    if mode=='cleanup_fail' and script.startswith('cleanup_rc=0;') and 'record.pid' in script:
        print('cleanup failed',file=sys.stderr);sys.exit(7)
    if script.startswith('umask 077; cat > '):
        if mode=='upload_fail': print('upload failed',file=sys.stderr);sys.exit(8)
        if mode=='upload_cancel': time.sleep(10)
        if mode=='upload_short':
            script=script.replace('cat > ', 'head -c 1 > ', 1)
    if args[2]=='exec-out':
        # ADB exec-out merges remote stderr and loses its exit code. Binary
        # readers must use shell v2 or missing files look like successful data.
        result=subprocess.run(['/bin/sh','-c',script],stdout=subprocess.PIPE,stderr=subprocess.STDOUT)
        sys.stdout.buffer.write(result.stdout);sys.exit(0)
    os.execv('/bin/sh',['sh','-c',script])
if kind=='mv':
    args=[a for a in sys.argv[1:] if a not in ('-nT','--')]
    if mode=='collision':
        with open(args[1],'wb') as f: f.write(b'existing artifact')
    if not os.path.lexists(args[1]): os.rename(args[0],args[1])
    if mode=='cancel_publish':
        with open(os.path.join(base,'published'),'w') as f: f.write(args[1])
        time.sleep(10)
    sys.exit(0)
if kind=='screenrecord':
    if mode=='fail': print('device recorder failed',file=sys.stderr);sys.exit(4)
    if mode=='cancel': time.sleep(10)
source='screen.png' if kind=='screencap' else 'movie.mp4'
if kind=='screencap' and sys.argv[-1]=='-p':
    sys.stdout.buffer.write(b'invalid artifact' if mode=='invalid' else open(os.path.join(base,source),'rb').read())
    sys.exit(0)
with open(sys.argv[-1],'wb') as target:
    if mode=='invalid': target.write(b'invalid artifact')
    else:
        with open(os.path.join(base,source),'rb') as f: shutil.copyfileobj(f,target)
`
	for _, name := range []string{"adb", "screencap", "screenrecord", "mv"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(helper), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return NewService(nil, nil, nil, nil, filepath.Join(bin, "adb")), dir
}

func artifactDeviceDirectory(fixture, relative string) string {
	return filepath.Join(fixture, "device", "automation", filepath.FromSlash(relative))
}
func prepareArtifactDeviceDirectory(t *testing.T, svc *Service, relative string) {
	t.Helper()
	if _, err := svc.runActArtifact(context.Background(), "phone", "mkdir", relative, 0); err != nil {
		t.Fatal(err)
	}
}

func TestActArtifactsRemainOnDevice(t *testing.T) {
	svc, fixture := artifactTestService(t)
	directory := "中文 output/O'Reilly/nested"
	prepareArtifactDeviceDirectory(t, svc, directory)
	for _, action := range []string{"screenshot", "record"} {
		saved, err := svc.runActArtifact(context.Background(), "phone", action, directory, 1)
		if err != nil {
			t.Fatal(action, err)
		}
		if path.Dir(saved) != directory || path.IsAbs(saved) {
			t.Fatalf("not a relative device artifact: %q", saved)
		}
		data, err := svc.readDeviceArtifact(context.Background(), "phone", directory, path.Base(saved), actRecordingLimit)
		if err != nil || len(data) == 0 {
			t.Fatalf("device artifact missing %q %v", saved, err)
		}
	}
	files, err := os.ReadDir(artifactDeviceDirectory(fixture, directory))
	if err != nil || len(files) != 2 {
		t.Fatalf("device files=%v err=%v", files, err)
	}
	for _, f := range files {
		if strings.HasPrefix(f.Name(), ".") {
			t.Fatal("device staging leaked")
		}
	}
	commands, _ := os.ReadFile(filepath.Join(fixture, "commands"))
	if !strings.Contains(string(commands), "mv -nT") || !strings.Contains(string(commands), "--bit-rate 2000000 --time-limit 1") {
		t.Fatalf("missing device publish or bounded recording: %s", commands)
	}
}

func TestActArtifactsDeviceFailureDoesNotPublish(t *testing.T) {
	for _, action := range []string{"screenshot", "record"} {
		t.Run(action, func(t *testing.T) {
			svc, fixture := artifactTestService(t)
			prepareArtifactDeviceDirectory(t, svc, "results")
			t.Setenv("FARM_ARTIFACT_MODE", "invalid")
			if _, err := svc.runActArtifact(context.Background(), "phone", action, "results", 1); err == nil {
				t.Fatal("invalid artifact accepted")
			}
			files, _ := os.ReadDir(artifactDeviceDirectory(fixture, "results"))
			if len(files) != 0 {
				t.Fatalf("failed device artifact leaked: %v", files)
			}
		})
	}
}

func TestActRecordingDeviceCancellationCleansUp(t *testing.T) {
	svc, fixture := artifactTestService(t)
	prepareArtifactDeviceDirectory(t, svc, "results")
	t.Setenv("FARM_ARTIFACT_MODE", "cancel")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := svc.runActArtifact(ctx, "phone", "record", "results", 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	files, _ := os.ReadDir(artifactDeviceDirectory(fixture, "results"))
	if len(files) != 0 {
		t.Fatalf("cancelled device files=%v", files)
	}
	commands, _ := os.ReadFile(filepath.Join(fixture, "commands"))
	if !strings.Contains(string(commands), "kill -INT") {
		t.Fatal("recorder cleanup not attempted")
	}
}

func TestActRecordingCommandFailureCleansUpWithoutRetry(t *testing.T) {
	svc, fixture := artifactTestService(t)
	prepareArtifactDeviceDirectory(t, svc, "results")
	t.Setenv("FARM_ARTIFACT_MODE", "fail")
	if _, err := svc.runActArtifact(context.Background(), "phone", "record", "results", 1); err == nil || !strings.Contains(err.Error(), "device recorder failed") {
		t.Fatalf("err=%v", err)
	}
	commands, _ := os.ReadFile(filepath.Join(fixture, "commands"))
	if strings.Count(string(commands), "shell screenrecord ") != 1 {
		t.Fatalf("recording retried: %s", commands)
	}
	files, _ := os.ReadDir(artifactDeviceDirectory(fixture, "results"))
	if len(files) != 0 {
		t.Fatalf("failed device files=%v", files)
	}
}

func TestActRecordingRejectsZeroDurationAndTimescale(t *testing.T) {
	for _, invalid := range []string{"zero_duration", "zero_timescale"} {
		t.Run(invalid, func(t *testing.T) {
			svc, fixture := artifactTestService(t)
			prepareArtifactDeviceDirectory(t, svc, "results")
			moviePath := filepath.Join(fixture, "movie.mp4")
			data, _ := os.ReadFile(moviePath)
			movie, err := mp4.DecodeFile(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			track := movie.Moov.Traks[0]
			if invalid == "zero_duration" {
				track.Mdia.Minf.Stbl.Stts.SampleTimeDelta[0] = 0
			} else {
				track.Mdia.Mdhd.Timescale = 0
			}
			var output bytes.Buffer
			if err = movie.Encode(&output); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(moviePath, output.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = svc.runActArtifact(context.Background(), "phone", "record", "results", 1); err == nil || !strings.Contains(err.Error(), "positive duration") {
				t.Fatalf("invalid recording accepted: %v", err)
			}
			files, _ := os.ReadDir(artifactDeviceDirectory(fixture, "results"))
			if len(files) != 0 {
				t.Fatalf("invalid device files=%v", files)
			}
		})
	}
}

func TestActArtifactRelativeBoundsBeforeDeviceSideEffects(t *testing.T) {
	svc, fixture := artifactTestService(t)
	for _, directory := range []string{"", "/sdcard/out", "../out", "a/../b", "a\nb"} {
		if _, err := svc.runActArtifact(context.Background(), "phone", "mkdir", directory, 0); err == nil {
			t.Fatalf("accepted %q", directory)
		}
	}
	for _, seconds := range []int64{0, 31} {
		if _, err := svc.runActArtifact(context.Background(), "phone", "record", "results", seconds); err == nil {
			t.Fatal("invalid duration accepted")
		}
	}
	if _, err := os.Stat(filepath.Join(fixture, "commands")); !os.IsNotExist(err) {
		t.Fatal("invalid input reached device")
	}
	if _, err := svc.runActArtifact(context.Background(), "phone", "screenshot", "missing", 0); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing destination err=%v", err)
	}
}

func TestReadDeviceArtifactRejectsTraversalSymlinksAndOversize(t *testing.T) {
	svc, fixture := artifactTestService(t)
	prepareArtifactDeviceDirectory(t, svc, "results")
	base := artifactDeviceDirectory(fixture, "results")
	if err := os.WriteFile(filepath.Join(base, "large"), []byte("12345"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("large", filepath.Join(base, "link")); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"../outside", "/absolute", "a/../../b"} {
		if _, err := svc.readDeviceArtifact(context.Background(), "phone", "results", relative, 4); err == nil {
			t.Fatalf("accepted %q", relative)
		}
	}
	if _, err := svc.readDeviceArtifact(context.Background(), "phone", "results", "link", 10); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("symlink err=%v", err)
	}
	if _, err := svc.readDeviceArtifact(context.Background(), "phone", "results", "large", 4); err == nil {
		t.Fatal("oversize device artifact accepted")
	}
	if _, err := svc.readDeviceArtifact(context.Background(), "phone", "results", "missing", 4); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing err=%v", err)
	}
}

func TestActArtifactCancelledBeforeDeviceOperation(t *testing.T) {
	svc, fixture := artifactTestService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.runActArtifact(ctx, "phone", "mkdir", "results", 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture, "commands")); !os.IsNotExist(err) {
		t.Fatal("cancelled request touched device")
	}
}

func TestActArtifactCollisionPreservesExistingDeviceFile(t *testing.T) {
	svc, fixture := artifactTestService(t)
	prepareArtifactDeviceDirectory(t, svc, "results")
	t.Setenv("FARM_ARTIFACT_MODE", "collision")
	if _, err := svc.runActArtifact(context.Background(), "phone", "screenshot", "results", 0); err == nil {
		t.Fatal("collision reported success")
	}
	files, err := os.ReadDir(artifactDeviceDirectory(fixture, "results"))
	if err != nil || len(files) != 1 {
		t.Fatalf("collision removed existing file or leaked staging: %v %v", files, err)
	}
	data, err := os.ReadFile(filepath.Join(artifactDeviceDirectory(fixture, "results"), files[0].Name()))
	if err != nil || string(data) != "existing artifact" {
		t.Fatalf("existing artifact changed: %q %v", data, err)
	}
}

func TestActArtifactCancellationRollsBackOnlyPublishedDeviceFile(t *testing.T) {
	svc, fixture := artifactTestService(t)
	prepareArtifactDeviceDirectory(t, svc, "results")
	base := artifactDeviceDirectory(fixture, "results")
	if err := os.WriteFile(filepath.Join(base, "sibling.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FARM_ARTIFACT_MODE", "cancel_publish")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		for {
			if _, err := os.Stat(filepath.Join(fixture, "published")); err == nil {
				cancel()
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}()
	if _, err := svc.runActArtifact(ctx, "phone", "screenshot", "results", 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancelled publication: %v", err)
	}
	files, err := os.ReadDir(base)
	if err != nil || len(files) != 1 || files[0].Name() != "sibling.txt" {
		t.Fatalf("rollback deleted sibling or left own artifact: %v %v", files, err)
	}
}

func TestActArtifactRemoveDevicePathsAndPreserveSiblings(t *testing.T) {
	svc, fixture := artifactTestService(t)
	target := "results/O'Reilly $(literal)"
	prepareArtifactDeviceDirectory(t, svc, target)
	base := artifactDeviceDirectory(fixture, "results")
	for _, name := range []string{"sibling.txt", "O'Reilly $(literal)/child.txt"} {
		if err := os.WriteFile(filepath.Join(base, name), []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.runActArtifact(context.Background(), "phone", "rm", target, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, "sibling.txt")); err != nil {
		t.Fatal("rm deleted sibling", err)
	}
	if _, err := os.Stat(artifactDeviceDirectory(fixture, target)); !os.IsNotExist(err) {
		t.Fatal("rm did not remove requested directory")
	}
	if _, err := svc.runActArtifact(context.Background(), "phone", "rm", "results/sibling.txt", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.runActArtifact(context.Background(), "phone", "rm", "results/missing", 0); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing rm silently succeeded: %v", err)
	}
	for _, invalid := range []string{".", "/sdcard/Farm/automation", "a/../b"} {
		if _, err := svc.runActArtifact(context.Background(), "phone", "rm", invalid, 0); err == nil {
			t.Fatalf("rm accepted root/escape %q", invalid)
		}
	}
	if err := os.Symlink(fixture, filepath.Join(base, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.runActArtifact(context.Background(), "phone", "rm", "results/link", 0); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("rm accepted symlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture, "screen.png")); err != nil {
		t.Fatal("rm followed symlink", err)
	}
}
