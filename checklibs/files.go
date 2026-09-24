// ********************************************************************
// Watchdog - the utility for monitoring system integrity.
// And for web monitoring of websites availability.
// ********************************************************************
// Author: Sergey Kalinin
// License: GPL v3
// ********************************************************************
// The file system status check module (directories and files).
// It implements the verification of changes to the specified files
// and directories (modification, appearance of new ones).
//
// Event-based filtering is supported: for each root in the config,
// you can specify watch = [create, modify, delete], and only these
// categories of changes will be included in the report.
// ********************************************************************

package checklibs

import (
	"fmt"
	"io/fs"
    	"io"
	"log"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

    "github.com/zeebo/blake3"

	"watchdog/mainlibs"
)

type PathSpec struct {
    Path    string
    Watch   []string // "create", "modify", "delete"
    Exclude []string
    Hash    string
}

type FileMeta struct {
    Path     string
    FileType string // "file" / "dir" / "symlink" / "other"
    Size     int64
    Mode     fs.FileMode
    Owner    string
    Group    string
    ModTime  time.Time
    Watched []string
    Hash    string //BLAKE3 hash
	// CreateTime time.Time
}

type FileList map[string]FileMeta

type FieldChange struct {
	Field string
	Old   string
	New   string
}

type FileDiff struct {
	Path    string
	Changes []FieldChange
}

type DiffResult struct {
	OnlyInBase   []string
	OnlyInActual []string
	Changed      []FileDiff
	Unchanged    []string
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func PrintDiff(d DiffResult, showUnchanged bool) string {
	var sb strings.Builder

	sort.Strings(d.OnlyInBase)
	sort.Strings(d.OnlyInActual)

	hostname, err := mainlibs.GetFQDN()
	if err != nil {
		hostname = "unknown"
		log.Println("Hostname error:", err)
	}

	fmt.Fprintf(&sb, "File changes alarm on host %s:\n", hostname)

	if len(d.OnlyInBase) > 0 {
		fmt.Fprintf(&sb, "\nDeleted (%d):\n", len(d.OnlyInBase))
		for _, p := range d.OnlyInBase {
			fmt.Fprintf(&sb, "   - %s\n", p)
		}
	}

	if len(d.OnlyInActual) > 0 {
		fmt.Fprintf(&sb, "\nAdded (%d):\n", len(d.OnlyInActual))
		for _, p := range d.OnlyInActual {
			fmt.Fprintf(&sb, "   + %s\n", p)
		}
	}

	if len(d.Changed) > 0 {
		fmt.Fprintf(&sb, "\nChanged (%d):\n", len(d.Changed))
		sort.Slice(d.Changed, func(i, j int) bool {
			return d.Changed[i].Path < d.Changed[j].Path
		})
		for _, fd := range d.Changed {
			fmt.Fprintf(&sb, "   ~ %s\n", fd.Path)
			for _, ch := range fd.Changes {
				fmt.Fprintf(&sb, "       %-6s: %q --> %q\n", ch.Field, ch.Old, ch.New)
			}
		}
	}

	if showUnchanged && len(d.Unchanged) > 0 {
		fmt.Fprintf(&sb, "\n✓ Unchanged (%d):\n", len(d.Unchanged))
		sort.Strings(d.Unchanged)
		for _, p := range d.Unchanged {
			fmt.Fprintf(&sb, "   = %s\n", p)
		}
	}

	fmt.Fprintf(&sb, "\nSummary: deleted %d, added %d, changed %d, all %d\n",
		len(d.OnlyInBase), len(d.OnlyInActual),
		len(d.Changed), len(d.Unchanged))

	if len(d.OnlyInBase) > 0 || len(d.OnlyInActual) > 0 || len(d.Changed) > 0 {
		return sb.String()
	}
	return ""
}

func CompareFilesList(base, actual FileList) DiffResult {
	var res DiffResult

	for path, b := range base {
		a, ok := actual[path]
		if !ok {
			res.OnlyInBase = append(res.OnlyInBase, path)
			continue
		}

		var changes []FieldChange

		if b.FileType != a.FileType {
			changes = append(changes, FieldChange{"FileType", b.FileType, a.FileType})
		}
		if b.Size != a.Size {
			changes = append(changes, FieldChange{"size",
				fmt.Sprint(b.Size), fmt.Sprint(a.Size)})
		}
        if b.Hash != a.Hash {
            changes = append(changes, FieldChange{"hash",
            fmt.Sprint(b.Hash), fmt.Sprint(a.Hash)})
        }
		if b.Mode.Perm() != a.Mode.Perm() {
			changes = append(changes, FieldChange{"mode",
				b.Mode.Perm().String(), a.Mode.Perm().String()})
		}
		if b.Owner != a.Owner {
			changes = append(changes, FieldChange{"owner", b.Owner, a.Owner})
		}
		if b.Group != a.Group {
			changes = append(changes, FieldChange{"group", b.Group, a.Group})
		}
		if !b.ModTime.Equal(a.ModTime) {
			changes = append(changes, FieldChange{"mtime",
				b.ModTime.Format("2006-01-02 15:04:05"),
				a.ModTime.Format("2006-01-02 15:04:05")})
		}

		if len(changes) > 0 {
			res.Changed = append(res.Changed, FileDiff{Path: path, Changes: changes})
		} else {
			res.Unchanged = append(res.Unchanged, path)
		}
	}

	// Поиск новых файлов: есть в actual, нет в base.
	for path := range actual {
		if _, ok := base[path]; !ok {
			res.OnlyInActual = append(res.OnlyInActual, path)
		}
	}

	return res
}

// filterDiffByWatch убирает из diff категории, которые не отслеживаются
// для соответствующих путей.
func filterDiffByWatch(d DiffResult, prev, curr FileList) DiffResult {
	var res DiffResult

	// Удалённые: показываем, если для этого пути включён "delete".
	for _, p := range d.OnlyInBase {
		if meta, ok := prev[p]; ok {
			if len(meta.Watched) == 0 || contains(meta.Watched, "delete") {
				res.OnlyInBase = append(res.OnlyInBase, p)
			}
		}
	}

	// Добавленные: показываем, если включён "create".
	for _, p := range d.OnlyInActual {
		if meta, ok := curr[p]; ok {
			if len(meta.Watched) == 0 || contains(meta.Watched, "create") {
				res.OnlyInActual = append(res.OnlyInActual, p)
			}
		}
	}

	// Изменённые: показываем, если включён "modify".
	for _, fd := range d.Changed {
		if meta, ok := curr[fd.Path]; ok {
			if len(meta.Watched) == 0 || contains(meta.Watched, "modify") {
				res.Changed = append(res.Changed, fd)
			}
		}
	}

	res.Unchanged = d.Unchanged
	return res
}

func GetFilesList(dir string, watch []string, exclude []string, hashCalc string) (FileList, []error) {
	result := make(FileList)
    var (
        errs []error
        fileHash string
    ) 

	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p, err))
			return nil
		}

		if isExcluded(p, exclude) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p, err))
			return nil
		}

		fileType := "other"
		switch {
		case info.IsDir():
			fileType = "dir"
		case info.Mode()&os.ModeSymlink != 0:
			fileType = "symlink"
		case info.Mode().IsRegular():
			fileType = "file"
		}

        if hashCalc != "" && info.Mode().IsRegular() {
            fileHash, err = HashFile(p)
            // log.Printf("File %s has hash %s", p, fileHash)
            if err != nil {
                log.Println("Hash calculation error: ", err)
            }
        } else {
            fileHash = ""
        }

		meta := FileMeta{
            Path:     p,
            FileType: fileType,
            Size:     info.Size(),
            Mode:     info.Mode(),
            ModTime:  info.ModTime(),
            Watched:  watch,
            Hash:     fileHash,
		}

		// Unix only
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			meta.Owner = mainlibs.ResolveUser(stat.Uid)
			meta.Group = mainlibs.ResolveGroup(stat.Gid)
		}

		result[p] = meta
		return nil
	})
	if err != nil {
		errs = append(errs, err)
	}

	return result, errs
}


func isExcluded(p string, exclude []string) bool {
	for _, ex := range exclude {
		if p == ex {
			return true
		}
		if strings.HasPrefix(p, ex+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func HashFile(path string) (string, error) {
    f, err := os.Open(path)
    if err != nil {
        return "", err
    }
    defer f.Close()

    hasher := blake3.New()

    if _, err := io.Copy(hasher, f); err != nil {
        return "", err
    }

    return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}


func FilesCheck(paths []PathSpec, previousFilesList FileList) (FileList, string) {
	currentFilesList := make(FileList)

	for _, ps := range paths {
		log.Println("Running CheckFiles for", ps.Path)

		filesList, errs := GetFilesList(ps.Path, ps.Watch, ps.Exclude, ps.Hash)

		for path, meta := range filesList {
			currentFilesList[path] = meta
		}

		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "warn:", e)
		}
	}

	if previousFilesList == nil {
		return maps.Clone(currentFilesList), ""
	}

	diff := CompareFilesList(previousFilesList, currentFilesList)

	diff = filterDiffByWatch(diff, previousFilesList, currentFilesList)

	out := PrintDiff(diff, false)

	return maps.Clone(currentFilesList), out
}
