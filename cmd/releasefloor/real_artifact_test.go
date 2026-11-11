package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/feiiiiii5/releasefloor/floor"
)

func TestRealArtifactInspection(t *testing.T) {
	dir := t.TempDir()

	// 1. Compile a real Go binary on the host
	goBin := filepath.Join(dir, "my_go_app")
	cmd := exec.Command("go", "build", "-trimpath", "-o", goBin, "./main.go")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v: %s", err, string(out))
	}

	var out, errOut bytes.Buffer
	rc := run([]string{"scan", goBin, "--format", "json"}, &out, &errOut)
	if rc != 0 {
		t.Fatalf("scan real Go binary failed (%d): %s", rc, errOut.String())
	}
	var r floor.Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if r.Status != "pass" || len(r.Inventories) != 1 || len(r.Inventories[0].Binaries) == 0 {
		t.Fatalf("expected clean pass on real Go binary, got: %+v", r)
	}

	// 2. Package real binary into a Python Wheel (.whl)
	whlPath := filepath.Join(dir, "mypackage-1.0.0-py3-none-any.whl")
	whlFile, err := os.Create(whlPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(whlFile)
	binData, err := os.ReadFile(goBin)
	if err != nil {
		t.Fatal(err)
	}
	w, err := zw.Create("mypackage/app.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(binData); err != nil {
		t.Fatal(err)
	}
	doc, err := zw.Create("mypackage-1.0.0.dist-info/METADATA")
	if err != nil {
		t.Fatal(err)
	}
	doc.Write([]byte("Metadata-Version: 2.1\nName: mypackage\nVersion: 1.0.0\n"))
	zw.Close()
	whlFile.Close()

	out.Reset()
	errOut.Reset()
	if rc := run([]string{"scan", whlPath, "--format", "json"}, &out, &errOut); rc != 0 {
		t.Fatalf("scan real python wheel failed (%d): %s", rc, errOut.String())
	}
	var whlReport floor.Report
	if err := json.Unmarshal(out.Bytes(), &whlReport); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if whlReport.Status != "pass" || len(whlReport.Inventories[0].Binaries) != 1 || whlReport.Inventories[0].Ignored != 1 {
		t.Fatalf("unexpected wheel report: %+v", whlReport)
	}

	// 3. Package real binary into a Java JAR (.jar) with JNI native layout
	jarPath := filepath.Join(dir, "my-app.jar")
	jarFile, err := os.Create(jarPath)
	if err != nil {
		t.Fatal(err)
	}
	jw := zip.NewWriter(jarFile)
	jwNative, err := jw.Create("BOOT-INF/lib/native-app.bin")
	if err != nil {
		t.Fatal(err)
	}
	jwNative.Write(binData)
	manifest, err := jw.Create("META-INF/MANIFEST.MF")
	if err != nil {
		t.Fatal(err)
	}
	manifest.Write([]byte("Manifest-Version: 1.0\n"))
	jw.Close()
	jarFile.Close()

	out.Reset()
	errOut.Reset()
	if rc := run([]string{"scan", jarPath, "--format", "json"}, &out, &errOut); rc != 0 {
		t.Fatalf("scan real Java JAR failed (%d): %s", rc, errOut.String())
	}
	var jarReport floor.Report
	if err := json.Unmarshal(out.Bytes(), &jarReport); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if jarReport.Status != "pass" || len(jarReport.Inventories[0].Binaries) != 1 {
		t.Fatalf("unexpected jar report: %+v", jarReport)
	}

	// 4. If clang or gcc is available, compile a real C program and scan it
	cc, err := exec.LookPath("clang")
	if err != nil {
		cc, err = exec.LookPath("gcc")
	}
	if err == nil {
		cSrc := filepath.Join(dir, "test.c")
		os.WriteFile(cSrc, []byte("#include <stdio.h>\nint main(){printf(\"hi\");return 0;}\n"), 0600)
		cBin := filepath.Join(dir, "c_app")
		if compileOut, compileErr := exec.Command(cc, "-O2", "-o", cBin, cSrc).CombinedOutput(); compileErr == nil {
			var cOut, cErr bytes.Buffer
			if rc := run([]string{"scan", cBin, "--format", "json"}, &cOut, &cErr); rc != 0 {
				t.Fatalf("scan real C binary failed (%d): %s", rc, cErr.String())
			}
			var cReport floor.Report
			if err := json.Unmarshal(cOut.Bytes(), &cReport); err != nil {
				t.Fatalf("invalid json: %v", err)
			}
			if cReport.Status != "pass" || len(cReport.Inventories[0].Binaries) != 1 {
				t.Fatalf("unexpected C binary report: %+v", cReport)
			}
		} else {
			t.Logf("C compiler skipped: %v (%s)", compileErr, string(compileOut))
		}
	}
}
