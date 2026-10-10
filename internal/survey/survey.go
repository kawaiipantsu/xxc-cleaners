// Package survey gathers read-only storage-usage breakdowns for the TUI's
// Homes / Databases / Mail / Web overview pages. Nothing here deletes anything.
package survey

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/kawaiipantsu/xxc-cleaners/internal/scan"
)

// Item is one measured location in a usage breakdown.
type Item struct {
	Label  string // display name
	Path   string // measured path (or glob)
	Size   int64  // bytes; -1 when unknown
	Detail string // short tag: data / logs / replication / queue / cache / running …
}

// ---- helpers ----------------------------------------------------------------

func statExists(p string) (os.FileInfo, bool) {
	fi, err := os.Stat(p)
	if err != nil {
		return nil, false
	}
	return fi, true
}

// dirItem measures a directory if it exists.
func dirItem(label, path, detail string) (Item, bool) {
	fi, ok := statExists(path)
	if !ok || !fi.IsDir() {
		return Item{}, false
	}
	sz, _ := scan.DirSize(path)
	return Item{Label: label, Path: path, Size: sz, Detail: detail}, true
}

// globItem sums files matching a glob pattern (e.g. binlogs).
func globItem(label, pattern, detail string) (Item, bool) {
	matches := scan.Expand(pattern)
	if len(matches) == 0 {
		return Item{}, false
	}
	var total int64
	for _, m := range matches {
		if fi, err := os.Stat(m); err == nil {
			if fi.IsDir() {
				if s, err := scan.DirSize(m); err == nil {
					total += s
				}
				continue
			}
			total += fi.Size()
		}
	}
	return Item{Label: label, Path: pattern, Size: total, Detail: detail}, true
}

func sortBySize(items []Item) []Item {
	sort.SliceStable(items, func(i, j int) bool { return items[i].Size > items[j].Size })
	return items
}

// homeDir pairs a username with its home directory.
type homeDir struct {
	User string
	Dir  string
}

// homes returns real user homes (uid 0 or >= 1000) from /etc/passwd.
func homes() []homeDir {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []homeDir
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		p := strings.Split(sc.Text(), ":")
		if len(p) < 6 {
			continue
		}
		uid, err := strconv.Atoi(p[2])
		if err != nil || (uid != 0 && uid < 1000) {
			continue
		}
		dir := p[5]
		if dir == "" || dir == "/" || dir == "/nonexistent" || seen[dir] {
			continue
		}
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			continue
		}
		seen[dir] = true
		out = append(out, homeDir{User: p[0], Dir: dir})
	}
	return out
}

// ---- pages ------------------------------------------------------------------

// Homes returns disk usage per user home directory (largest first).
func Homes() []Item {
	var items []Item
	for _, h := range homes() {
		sz, _ := scan.DirSize(h.Dir)
		detail := "home"
		// Note the cache slice, which is often the easy win.
		if cs, err := scan.DirSize(filepath.Join(h.Dir, ".cache")); err == nil && cs > 0 {
			detail = "home · cache " + humanBytesShort(cs)
		}
		items = append(items, Item{Label: h.User, Path: h.Dir, Size: sz, Detail: detail})
	}
	return sortBySize(items)
}

// Databases returns usage for databases, caches, queues and their logs/replication.
func Databases() []Item {
	specs := []struct{ label, path, detail string }{
		{"MySQL / MariaDB data", "/var/lib/mysql", "data"},
		{"MySQL logs", "/var/log/mysql", "logs"},
		{"PostgreSQL data", "/var/lib/postgresql", "data"},
		{"PostgreSQL logs", "/var/log/postgresql", "logs"},
		{"MongoDB data", "/var/lib/mongodb", "data"},
		{"MongoDB logs", "/var/log/mongodb", "logs"},
		{"Redis data", "/var/lib/redis", "data"},
		{"Redis logs", "/var/log/redis", "logs"},
		{"InfluxDB data", "/var/lib/influxdb", "data"},
		{"InfluxDB logs", "/var/log/influxdb", "logs"},
		{"Elasticsearch data", "/var/lib/elasticsearch", "data"},
		{"ClickHouse data", "/var/lib/clickhouse", "data"},
		{"RabbitMQ (queues)", "/var/lib/rabbitmq", "queue"},
		{"RabbitMQ logs", "/var/log/rabbitmq", "logs"},
		{"Kafka (queues)", "/var/lib/kafka", "queue"},
		{"Memcached", "/var/lib/memcached", "cache"},
	}
	var items []Item
	for _, s := range specs {
		if it, ok := dirItem(s.label, s.path, s.detail); ok {
			items = append(items, it)
		}
	}
	// Replication artifacts.
	globs := []struct{ label, pat, detail string }{
		{"MySQL binary logs", "/var/lib/mysql/mysql-bin.*", "replication"},
		{"MySQL relay logs", "/var/lib/mysql/relay-bin.*", "replication"},
		{"MariaDB binary logs", "/var/lib/mysql/mariadb-bin.*", "replication"},
		{"PostgreSQL WAL", "/var/lib/postgresql/*/main/pg_wal", "replication"},
	}
	for _, g := range globs {
		if it, ok := globItem(g.label, g.pat, g.detail); ok {
			items = append(items, it)
		}
	}
	return sortBySize(items)
}

// Mail returns usage for virtual mail, spools, queues, logs and caches.
func Mail() []Item {
	specs := []struct{ label, path, detail string }{
		{"Virtual mail (vmail)", "/var/vmail", "vmail"},
		{"System mailboxes", "/var/mail", "mail"},
		{"Mail spool", "/var/spool/mail", "spool"},
		{"Postfix queue", "/var/spool/postfix", "queue"},
		{"Exim queue", "/var/spool/exim4", "queue"},
		{"Dovecot data/index/cache", "/var/lib/dovecot", "cache"},
		{"Mailman", "/var/lib/mailman", "files"},
		{"Mail logs", "/var/log/mail", "logs"},
	}
	var items []Item
	for _, s := range specs {
		if it, ok := dirItem(s.label, s.path, s.detail); ok {
			items = append(items, it)
		}
	}
	if it, ok := globItem("Mail log files", "/var/log/mail.*", "logs"); ok {
		items = append(items, it)
	}
	return sortBySize(items)
}

// Web returns usage for web roots, logs and caches.
func Web() []Item {
	specs := []struct{ label, path, detail string }{
		{"Web root /var/www", "/var/www", "www"},
		{"Web root /srv/www", "/srv/www", "www"},
		{"Web root /srv/http", "/srv/http", "www"},
		{"Apache logs", "/var/log/apache2", "logs"},
		{"httpd logs", "/var/log/httpd", "logs"},
		{"Nginx logs", "/var/log/nginx", "logs"},
		{"Nginx cache", "/var/cache/nginx", "cache"},
		{"Apache cache", "/var/cache/apache2", "cache"},
		{"PHP sessions", "/var/lib/php/sessions", "cache"},
		{"PHP opcache/tmp", "/var/cache/php", "cache"},
		{"Varnish cache", "/var/lib/varnish", "cache"},
	}
	var items []Item
	for _, s := range specs {
		if it, ok := dirItem(s.label, s.path, s.detail); ok {
			items = append(items, it)
		}
	}
	return sortBySize(items)
}

// humanBytesShort is a tiny local formatter to avoid an import cycle risk; it is
// only used for inline detail notes.
func humanBytesShort(n int64) string {
	units := []string{"B", "K", "M", "G", "T"}
	f := float64(n)
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return strconv.FormatInt(n, 10) + "B"
	}
	return strconv.FormatFloat(f, 'f', 1, 64) + units[i]
}
