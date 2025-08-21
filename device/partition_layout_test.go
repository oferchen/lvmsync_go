package device

import "testing"

const sampleSfdisk1 = `# partition table of /dev/sda
unit: sectors

/dev/sda1 : start=        2048, size=     1024, type=83
/dev/sda2 : start=        4096, size=     2048, type=8e
`

const sampleSfdisk2 = `# partition table of /dev/sda
unit: sectors

/dev/sda1 : start=        2048, size=     1024, type=83
/dev/sda2 : start=        4096, size=     1024, type=83
`

func TestParseSfdiskDump(t *testing.T) {
	parts, err := parseSfdiskDump(sampleSfdisk1)
	if err != nil {
		t.Fatalf("parseSfdiskDump: %v", err)
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 partitions, got %d", len(parts))
	}
	if parts[0].Start != 2048 || parts[0].End != 3071 || parts[0].Type != "83" {
		t.Fatalf("unexpected first partition: %+v", parts[0])
	}
	if parts[1].Start != 4096 || parts[1].End != 6143 || parts[1].Type != "8e" {
		t.Fatalf("unexpected second partition: %+v", parts[1])
	}
}

func TestDiffPartitionLayouts(t *testing.T) {
	a, err := parseSfdiskDump(sampleSfdisk1)
	if err != nil {
		t.Fatalf("parseSfdiskDump a: %v", err)
	}
	b, err := parseSfdiskDump(sampleSfdisk2)
	if err != nil {
		t.Fatalf("parseSfdiskDump b: %v", err)
	}
	diffs := diffPartitionLayouts(a, b)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	d := diffs[0]
	if d.Index != 1 || d.A.Type != "8e" || d.B.Type != "83" {
		t.Fatalf("unexpected diff: %+v", d)
	}
}
