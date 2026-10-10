// Package disk inspects mounted filesystems, block devices and inode usage.
package disk

import (
	"bufio"
	"os"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

// Mount describes one mounted filesystem and its usage.
type Mount struct {
	Device     string
	MountPoint string
	FSType     string

	TotalBytes int64
	UsedBytes  int64
	FreeBytes  int64 // available to unprivileged users
	TotalInode int64
	UsedInode  int64
	FreeInode  int64
}

// UsedPercent returns block usage as 0..100.
func (m Mount) UsedPercent() float64 {
	if m.TotalBytes <= 0 {
		return 0
	}
	return float64(m.UsedBytes) / float64(m.TotalBytes) * 100
}

// InodePercent returns inode usage as 0..100.
func (m Mount) InodePercent() float64 {
	if m.TotalInode <= 0 {
		return 0
	}
	return float64(m.UsedInode) / float64(m.TotalInode) * 100
}

// pseudoFS are filesystem types that do not represent real storage.
var pseudoFS = map[string]bool{
	"proc": true, "sysfs": true, "devtmpfs": true, "devpts": true,
	"cgroup": true, "cgroup2": true, "pstore": true, "bpf": true,
	"tracefs": true, "debugfs": true, "securityfs": true, "mqueue": true,
	"hugetlbfs": true, "fusectl": true, "configfs": true, "autofs": true,
	"binfmt_misc": true, "rpc_pipefs": true, "nsfs": true, "ramfs": true,
	"fuse.gvfsd-fuse": true, "fuse.portal": true, "efivarfs": true,
	// Container layer filesystems duplicate the backing device and only add noise.
	"overlay": true,
}

// Mounts returns real mounted filesystems with usage statistics, sorted by
// descending used bytes.
func Mounts(includePseudo bool) ([]Mount, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Mount
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		dev, mp, fstype := fields[0], unescape(fields[1]), fields[2]
		if !includePseudo && pseudoFS[fstype] {
			continue
		}
		if !includePseudo && fstype == "tmpfs" && !strings.HasPrefix(mp, "/tmp") && !strings.HasPrefix(mp, "/var") {
			// Keep /tmp and /var tmpfs (they hold cleanable data); skip the rest.
			continue
		}
		if seen[mp] {
			continue
		}
		seen[mp] = true

		var st unix.Statfs_t
		if err := unix.Statfs(mp, &st); err != nil {
			continue
		}
		bs := int64(st.Bsize)
		total := int64(st.Blocks) * bs
		if total == 0 {
			continue
		}
		free := int64(st.Bavail) * bs
		used := (int64(st.Blocks) - int64(st.Bfree)) * bs
		m := Mount{
			Device:     dev,
			MountPoint: mp,
			FSType:     fstype,
			TotalBytes: total,
			UsedBytes:  used,
			FreeBytes:  free,
			TotalInode: int64(st.Files),
			FreeInode:  int64(st.Ffree),
			UsedInode:  int64(st.Files) - int64(st.Ffree),
		}
		out = append(out, m)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UsedBytes > out[j].UsedBytes })
	return out, nil
}

// BlockDevice is a row from /proc/partitions.
type BlockDevice struct {
	Name   string
	Blocks int64 // 1 KiB blocks
}

// SizeBytes returns the device size in bytes.
func (b BlockDevice) SizeBytes() int64 { return b.Blocks * 1024 }

// Partitions lists block devices and partitions from /proc/partitions.
func Partitions() ([]BlockDevice, error) {
	f, err := os.Open("/proc/partitions")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []BlockDevice
	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		if first { // header line
			first = false
			continue
		}
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 {
			continue
		}
		var blocks int64
		for _, r := range fields[2] {
			if r < '0' || r > '9' {
				blocks = -1
				break
			}
			blocks = blocks*10 + int64(r-'0')
		}
		if blocks < 0 {
			continue
		}
		out = append(out, BlockDevice{Name: fields[3], Blocks: blocks})
	}
	return out, sc.Err()
}

// unescape decodes octal escapes (\040 etc.) used in /proc/mounts.
func unescape(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			v := (int(s[i+1]-'0') << 6) | (int(s[i+2]-'0') << 3) | int(s[i+3]-'0')
			b.WriteByte(byte(v))
			i += 3
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
