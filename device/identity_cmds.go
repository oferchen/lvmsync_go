package device

import (
	"os/exec"
	"time"
)

var (
	blkidPath string
	lvsPath   string
	lsblkPath string
)

const identityTimeout = 5 * time.Second

func init() {
	blkidPath, _ = exec.LookPath("blkid")
	lvsPath, _ = exec.LookPath("lvs")
	lsblkPath, _ = exec.LookPath("lsblk")
}
