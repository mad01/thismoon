package history

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// dataBlock renders a fast-import data command the way fast-export does:
// header, raw payload, then a separator newline that is not counted.
func dataBlock(payload string) string {
	return fmt.Sprintf("data %d\n%s\n", len(payload), payload)
}

func transform(t *testing.T, rw *Rewriter, in string) (string, Stats) {
	t.Helper()
	var out bytes.Buffer
	stats, err := rw.Transform(strings.NewReader(in), &out)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	return out.String(), stats
}

func TestTransformReplacesBlobContent(t *testing.T) {
	in := "blob\nmark :1\n" + dataBlock("password=SECRETTOKEN123\nok\n") +
		"commit refs/heads/main\nmark :2\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		dataBlock("add config\n") +
		"M 100644 :1 config.txt\n"

	rw := &Rewriter{Replacements: []string{"SECRETTOKEN123"}}
	out, stats := transform(t, rw, in)

	if strings.Contains(out, "SECRETTOKEN123") {
		t.Fatalf("secret still present in output:\n%s", out)
	}
	want := "password=" + RedactedPlaceholder + "\nok\n"
	if !strings.Contains(out, dataBlock(want)) {
		t.Fatalf("expected rewritten data block with recomputed length, got:\n%s", out)
	}
	if stats.Replacements["SECRETTOKEN123"] != 1 {
		t.Fatalf("stats = %+v, want 1 replacement", stats.Replacements)
	}
}

func TestTransformReplacesCommitMessage(t *testing.T) {
	in := "commit refs/heads/main\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		dataBlock("mention internal-repo-name here\n")

	rw := &Rewriter{Replacements: []string{"internal-repo-name"}}
	out, _ := transform(t, rw, in)

	if strings.Contains(out, "internal-repo-name") {
		t.Fatalf("blocked name still present in output:\n%s", out)
	}
	if !strings.Contains(out, "mention "+RedactedPlaceholder+" here") {
		t.Fatalf("expected rewritten message, got:\n%s", out)
	}
}

func TestTransformLeavesBinaryBlobsUntouched(t *testing.T) {
	payload := "\x00\x01binarySECRETTOKEN123data"
	in := "blob\nmark :1\n" + dataBlock(payload)

	rw := &Rewriter{Replacements: []string{"SECRETTOKEN123"}}
	out, stats := transform(t, rw, in)

	if !strings.Contains(out, payload) {
		t.Fatalf("binary blob was modified:\n%q", out)
	}
	if stats.BinaryHits != 1 {
		t.Fatalf("BinaryHits = %d, want 1", stats.BinaryHits)
	}
	if stats.Total() != 0 {
		t.Fatalf("no replacements expected in binary blobs, got %+v", stats.Replacements)
	}
}

func TestTransformRoundTripsWhenNothingMatches(t *testing.T) {
	in := "blob\nmark :1\n" + dataBlock("hello world\n") +
		"reset refs/heads/main\n" +
		"commit refs/heads/main\nmark :2\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		dataBlock("clean message\n") +
		"M 100644 :1 hello.txt\n" +
		"D old.txt\n"

	rw := &Rewriter{Replacements: []string{"absent"}}
	out, stats := transform(t, rw, in)

	if out != in {
		t.Fatalf("round trip not byte-identical:\nin:  %q\nout: %q", in, out)
	}
	if stats.Total() != 0 {
		t.Fatalf("expected zero stats, got %+v", stats)
	}
}

func TestTransformDropsSignatureAndStillRewritesMessage(t *testing.T) {
	in := "commit refs/heads/main\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		"gpgsig sha1 openpgp\n" +
		dataBlock("-----BEGIN PGP SIGNATURE-----\nxyz\n-----END PGP SIGNATURE-----\n") +
		dataBlock("mention SECRETTOKEN123 here\n")

	rw := &Rewriter{Replacements: []string{"SECRETTOKEN123"}}
	out, stats := transform(t, rw, in)

	if strings.Contains(out, "gpgsig") || strings.Contains(out, "PGP SIGNATURE") {
		t.Fatalf("signature not dropped:\n%s", out)
	}
	if !strings.Contains(out, "mention "+RedactedPlaceholder+" here") {
		t.Fatalf("message after dropped signature not rewritten:\n%s", out)
	}
	if stats.SignaturesDropped != 1 {
		t.Fatalf("SignaturesDropped = %d, want 1", stats.SignaturesDropped)
	}
}

func TestTransformHandlesInlineBlobData(t *testing.T) {
	in := "commit refs/heads/main\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		dataBlock("msg\n") +
		"M 100644 inline config.txt\n" +
		dataBlock("token=SECRETTOKEN123\n")

	rw := &Rewriter{Replacements: []string{"SECRETTOKEN123"}}
	out, _ := transform(t, rw, in)

	if strings.Contains(out, "SECRETTOKEN123") {
		t.Fatalf("inline blob data not rewritten:\n%s", out)
	}
	if !strings.Contains(out, "token="+RedactedPlaceholder+"\n") {
		t.Fatalf("expected rewritten inline data, got:\n%s", out)
	}
}

func TestTransformReplaceTableInBlobContent(t *testing.T) {
	in := "blob\nmark :1\n" + dataBlock("host=old.internal.net\nname=oldBrand\n") +
		"commit refs/heads/main\nmark :2\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		dataBlock("add config\n") +
		"M 100644 :1 config.txt\n"

	rw := &Rewriter{ReplaceTable: map[string]string{
		"old.internal.net": "new.example.com",
		"oldBrand":         "newBrand",
	}}
	out, stats := transform(t, rw, in)

	if strings.Contains(out, "old.internal.net") {
		t.Fatalf("old host still present:\n%s", out)
	}
	if strings.Contains(out, "oldBrand") {
		t.Fatalf("old name still present:\n%s", out)
	}
	want := "host=new.example.com\nname=newBrand\n"
	if !strings.Contains(out, dataBlock(want)) {
		t.Fatalf("expected mapped data block, got:\n%s", out)
	}
	if stats.Replacements["old.internal.net"] != 1 {
		t.Fatalf(
			"expected 1 replacement for old.internal.net, got %d",
			stats.Replacements["old.internal.net"],
		)
	}
	if stats.Replacements["oldBrand"] != 1 {
		t.Fatalf("expected 1 replacement for oldBrand, got %d", stats.Replacements["oldBrand"])
	}
}

func TestTransformReplaceTableLongestFirst(t *testing.T) {
	in := "blob\nmark :1\n" + dataBlock("connect to old.internal.net now\n")

	rw := &Rewriter{ReplaceTable: map[string]string{
		"old":              "new",
		"old.internal.net": "new.example.com",
	}}
	out, _ := transform(t, rw, in)

	if !strings.Contains(out, "new.example.com") {
		t.Fatalf("longest key should match first, got:\n%s", out)
	}
	if strings.Contains(out, "new.internal.net") {
		t.Fatalf("partial replacement happened:\n%s", out)
	}
}

func TestTransformReplaceTableBeforeBlanket(t *testing.T) {
	in := "blob\nmark :1\n" + dataBlock("host=old.internal.net token=SECRET123\n")

	rw := &Rewriter{
		Replacements: []string{"SECRET123"},
		ReplaceTable: map[string]string{"old.internal.net": "new.example.com"},
	}
	out, stats := transform(t, rw, in)

	want := "host=new.example.com token=" + RedactedPlaceholder + "\n"
	if !strings.Contains(out, dataBlock(want)) {
		t.Fatalf("expected both table and blanket replacement, got:\n%s", out)
	}
	if stats.Replacements["old.internal.net"] != 1 || stats.Replacements["SECRET123"] != 1 {
		t.Fatalf("unexpected stats: %+v", stats.Replacements)
	}
}

func TestTransformReplaceTableInCommitMessage(t *testing.T) {
	in := "commit refs/heads/main\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		dataBlock("deploy to old.internal.net\n")

	rw := &Rewriter{ReplaceTable: map[string]string{"old.internal.net": "new.example.com"}}
	out, _ := transform(t, rw, in)

	if strings.Contains(out, "old.internal.net") {
		t.Fatalf("old host still present in message:\n%s", out)
	}
	if !strings.Contains(out, "deploy to new.example.com") {
		t.Fatalf("expected mapped message, got:\n%s", out)
	}
}

func TestTransformProtectsGoSumReferencedBlob(t *testing.T) {
	goSumContent := "k8s.io/klog/v2 v2.140.0 h1:Tf+J3AH7xnUzZyVVXhTgKnFqye14aadWv7bzXdzc=\n"
	in := "blob\nmark :1\n" + dataBlock(goSumContent) +
		"commit refs/heads/main\nmark :2\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		dataBlock("add deps\n") +
		"M 100644 :1 go.sum\n"

	rw := &Rewriter{
		ReplaceTable:   map[string]string{"Kn": "replaced"},
		ProtectedPaths: DefaultProtectedPaths,
	}
	out, stats := transform(t, rw, in)

	if strings.Contains(out, "replaced") {
		t.Fatalf("go.sum blob was modified by replace table:\n%s", out)
	}
	if !strings.Contains(out, goSumContent) {
		t.Fatalf("go.sum content not preserved:\n%s", out)
	}
	if stats.ProtectedSkips != 1 {
		t.Fatalf("ProtectedSkips = %d, want 1", stats.ProtectedSkips)
	}
	if stats.Total() != 0 {
		t.Fatalf("expected no replacements in protected blob, got %+v", stats.Replacements)
	}
}

func TestTransformProtectsGoSumInlineBlob(t *testing.T) {
	goSumContent := "k8s.io/klog/v2 v2.140.0 h1:Tf+J3AH7xnUzZyVVXhTgKnFqye14aadWv7bzXdzc=\n"
	in := "commit refs/heads/main\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		dataBlock("add deps\n") +
		"M 100644 inline go.sum\n" +
		dataBlock(goSumContent)

	rw := &Rewriter{
		ReplaceTable:   map[string]string{"Kn": "replaced"},
		ProtectedPaths: DefaultProtectedPaths,
	}
	out, stats := transform(t, rw, in)

	if strings.Contains(out, "replaced") {
		t.Fatalf("inline go.sum blob was modified:\n%s", out)
	}
	if !strings.Contains(out, goSumContent) {
		t.Fatalf("go.sum content not preserved:\n%s", out)
	}
	if stats.ProtectedSkips != 1 {
		t.Fatalf("ProtectedSkips = %d, want 1", stats.ProtectedSkips)
	}
}

func TestTransformProtectsNestedGoSum(t *testing.T) {
	goSumContent := "example.com/pkg v1.0.0 h1:KnFqsomehash=\n"
	in := "blob\nmark :1\n" + dataBlock(goSumContent) +
		"commit refs/heads/main\nmark :2\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		dataBlock("add submodule deps\n") +
		"M 100644 :1 subdir/go.sum\n"

	rw := &Rewriter{
		ReplaceTable:   map[string]string{"Kn": "replaced"},
		ProtectedPaths: DefaultProtectedPaths,
	}
	out, stats := transform(t, rw, in)

	if strings.Contains(out, "replaced") {
		t.Fatalf("nested go.sum blob was modified:\n%s", out)
	}
	if stats.ProtectedSkips != 1 {
		t.Fatalf("ProtectedSkips = %d, want 1", stats.ProtectedSkips)
	}
}

func TestTransformProtectsPackageJSON(t *testing.T) {
	pkgJSON := `{"name":"my-app","dependencies":{"internal-lib":"1.0.0"}}` + "\n"
	in := "blob\nmark :1\n" + dataBlock(pkgJSON) +
		"commit refs/heads/main\nmark :2\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		dataBlock("add pkg\n") +
		"M 100644 :1 package.json\n"

	rw := &Rewriter{
		ReplaceTable:   map[string]string{"internal": "external"},
		ProtectedPaths: DefaultProtectedPaths,
	}
	out, stats := transform(t, rw, in)

	if strings.Contains(out, "external-lib") {
		t.Fatalf("package.json blob was modified:\n%s", out)
	}
	if stats.ProtectedSkips != 1 {
		t.Fatalf("ProtectedSkips = %d, want 1", stats.ProtectedSkips)
	}
}

func TestTransformStillReplacesUnprotectedBlobs(t *testing.T) {
	in := "blob\nmark :1\n" + dataBlock("host=old.internal.net\n") +
		"blob\nmark :2\n" + dataBlock("example.com/pkg v1.0.0 h1:KnFqhash=\n") +
		"commit refs/heads/main\nmark :3\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		dataBlock("add config\n") +
		"M 100644 :1 config.txt\n" +
		"M 100644 :2 go.sum\n"

	rw := &Rewriter{
		ReplaceTable:   map[string]string{"old.internal.net": "new.example.com"},
		ProtectedPaths: DefaultProtectedPaths,
	}
	out, stats := transform(t, rw, in)

	if strings.Contains(out, "old.internal.net") {
		t.Fatalf("unprotected config.txt blob should have been replaced:\n%s", out)
	}
	if !strings.Contains(out, "new.example.com") {
		t.Fatalf("expected replacement in config.txt, got:\n%s", out)
	}
	if stats.Replacements["old.internal.net"] != 1 {
		t.Fatalf(
			"expected 1 replacement in config.txt, got %d",
			stats.Replacements["old.internal.net"],
		)
	}
}

func TestTransformProtectionDoesNotAffectCommitMessages(t *testing.T) {
	goSumContent := "example.com/pkg v1.0.0 h1:hash=\n"
	in := "blob\nmark :1\n" + dataBlock(goSumContent) +
		"commit refs/heads/main\nmark :2\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		dataBlock("mention old.internal.net here\n") +
		"M 100644 :1 go.sum\n"

	rw := &Rewriter{
		ReplaceTable:   map[string]string{"old.internal.net": "new.example.com"},
		ProtectedPaths: DefaultProtectedPaths,
	}
	out, stats := transform(t, rw, in)

	if strings.Contains(out, "mention old.internal.net") {
		t.Fatalf("commit message should still be rewritten:\n%s", out)
	}
	if !strings.Contains(out, "mention new.example.com here") {
		t.Fatalf("expected rewritten commit message, got:\n%s", out)
	}
	if stats.Replacements["old.internal.net"] != 1 {
		t.Fatalf(
			"expected 1 replacement in commit message, got %d",
			stats.Replacements["old.internal.net"],
		)
	}
}

func TestTransformRedactsReferencedBlob(t *testing.T) {
	keyContent := "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaA==\n-----END OPENSSH PRIVATE KEY-----\n"
	in := "blob\nmark :1\n" + dataBlock(keyContent) +
		"commit refs/heads/main\nmark :2\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		dataBlock("add key\n") +
		"M 100644 :1 keys/id_rsa\n"

	rw := &Rewriter{RedactPaths: []string{"keys/id_rsa"}}
	out, stats := transform(t, rw, in)

	if strings.Contains(out, "PRIVATE KEY") {
		t.Fatalf("key content still present:\n%s", out)
	}
	if !strings.Contains(out, dataBlock(RedactedPlaceholder+"\n")) {
		t.Fatalf("expected redacted data block with recomputed length, got:\n%s", out)
	}
	if stats.BlobsRedacted != 1 {
		t.Fatalf("BlobsRedacted = %d, want 1", stats.BlobsRedacted)
	}
}

func TestTransformRedactsByBasename(t *testing.T) {
	in := "blob\nmark :1\n" + dataBlock("secret key material\n") +
		"commit refs/heads/main\nmark :2\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		dataBlock("add key\n") +
		"M 100644 :1 deep/nested/dir/id_rsa\n"

	rw := &Rewriter{RedactPaths: []string{"id_rsa"}}
	out, stats := transform(t, rw, in)

	if strings.Contains(out, "secret key material") {
		t.Fatalf("bare file name should match by basename:\n%s", out)
	}
	if stats.BlobsRedacted != 1 {
		t.Fatalf("BlobsRedacted = %d, want 1", stats.BlobsRedacted)
	}
}

func TestTransformRedactsInlineBlob(t *testing.T) {
	in := "commit refs/heads/main\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		dataBlock("add env\n") +
		"M 100644 inline .env\n" +
		dataBlock("TOKEN=SECRETTOKEN123\n")

	rw := &Rewriter{RedactPaths: []string{".env"}}
	out, stats := transform(t, rw, in)

	if strings.Contains(out, "SECRETTOKEN123") {
		t.Fatalf("inline blob not redacted:\n%s", out)
	}
	if stats.BlobsRedacted != 1 {
		t.Fatalf("BlobsRedacted = %d, want 1", stats.BlobsRedacted)
	}
}

func TestTransformRedactsBinaryBlob(t *testing.T) {
	payload := "\x00\x01binary key material"
	in := "blob\nmark :1\n" + dataBlock(payload) +
		"commit refs/heads/main\nmark :2\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		dataBlock("add key\n") +
		"M 100644 :1 secret.p12\n"

	rw := &Rewriter{RedactPaths: []string{"*.p12"}}
	out, stats := transform(t, rw, in)

	if strings.Contains(out, payload) {
		t.Fatalf("binary blob should be redacted, unlike string replacement:\n%s", out)
	}
	if stats.BlobsRedacted != 1 {
		t.Fatalf("BlobsRedacted = %d, want 1", stats.BlobsRedacted)
	}
	if stats.BinaryHits != 0 {
		t.Fatalf("BinaryHits = %d, want 0", stats.BinaryHits)
	}
}

func TestTransformRedactWinsOverProtected(t *testing.T) {
	in := "blob\nmark :1\n" + dataBlock("module secret\n") +
		"commit refs/heads/main\nmark :2\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		dataBlock("add\n") +
		"M 100644 :1 go.sum\n"

	rw := &Rewriter{
		ProtectedPaths: DefaultProtectedPaths,
		RedactPaths:    []string{"go.sum"},
	}
	out, stats := transform(t, rw, in)

	if strings.Contains(out, "module secret") {
		t.Fatalf("redact should win over protected-path skip:\n%s", out)
	}
	if stats.BlobsRedacted != 1 || stats.ProtectedSkips != 0 {
		t.Fatalf("stats = %+v, want 1 redaction and 0 protected skips", stats)
	}
}

func TestTransformRedactLeavesOtherFilesAlone(t *testing.T) {
	in := "blob\nmark :1\n" + dataBlock("keep me\n") +
		"blob\nmark :2\n" + dataBlock("redact me\n") +
		"commit refs/heads/main\nmark :3\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		dataBlock("add files\n") +
		"M 100644 :1 readme.txt\n" +
		"M 100644 :2 token.txt\n"

	rw := &Rewriter{RedactPaths: []string{"token.txt"}}
	out, stats := transform(t, rw, in)

	if !strings.Contains(out, dataBlock("keep me\n")) {
		t.Fatalf("unrelated blob was modified:\n%s", out)
	}
	if strings.Contains(out, "redact me") {
		t.Fatalf("matching blob not redacted:\n%s", out)
	}
	if stats.BlobsRedacted != 1 {
		t.Fatalf("BlobsRedacted = %d, want 1", stats.BlobsRedacted)
	}
}

func TestTransformInvalidRedactGlobFails(t *testing.T) {
	rw := &Rewriter{RedactPaths: []string{"[unclosed"}}
	var out bytes.Buffer
	if _, err := rw.Transform(strings.NewReader("blob\n"), &out); err == nil {
		t.Fatal("expected error for invalid redact glob")
	}
}

func TestTransformWithoutProtectedPathsStillReplaces(t *testing.T) {
	goSumContent := "k8s.io/klog/v2 v2.140.0 h1:KnFqhash=\n"
	in := "blob\nmark :1\n" + dataBlock(goSumContent) +
		"commit refs/heads/main\nmark :2\n" +
		"author Ada <ada@example.com> 1700000000 +0000\n" +
		"committer Ada <ada@example.com> 1700000000 +0000\n" +
		dataBlock("add\n") +
		"M 100644 :1 go.sum\n"

	rw := &Rewriter{
		ReplaceTable: map[string]string{"Kn": "replaced"},
	}
	out, stats := transform(t, rw, in)

	if !strings.Contains(out, "replaced") {
		t.Fatalf("without ProtectedPaths, go.sum should be replaced:\n%s", out)
	}
	if stats.Replacements["Kn"] != 1 {
		t.Fatalf("expected 1 replacement, got %d", stats.Replacements["Kn"])
	}
}
